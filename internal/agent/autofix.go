package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/crush/internal/agent/tools"
)

// Linter runs a linter over the given file paths and returns error messages.
type Linter interface {
	RunLint(ctx context.Context, filePaths []string) ([]string, error)
}

// Tester runs tests and returns failure output lines.
type Tester interface {
	RunTests(ctx context.Context) ([]string, error)
}

// AutoFixLoopResult is the outcome of a full auto-fix cycle.
type AutoFixLoopResult struct {
	Success         bool
	Attempts        []LoopFixAttempt
	FinalLintErrors []string
	FinalTestErrors []string
}

// LoopFixAttempt records one iteration of the auto-fix loop.
type LoopFixAttempt struct {
	AttemptNum   int
	LintErrors   []string
	TestErrors   []string
	FixesApplied []string
	ReflectNotes []string
	RolledBack   bool
}

// AutoFixLoop orchestrates the auto-lint → fix → test → reflect cycle.
// It composes a Linter, Tester, AutoFixer, and RollbackManager to
// iteratively fix diagnostic issues until the linter and tests pass or
// MaxRetries is exhausted.
type AutoFixLoop struct {
	Linter        Linter
	Tester        Tester
	Fixer         *tools.AutoFixer
	Rollback      *tools.RollbackManager
	MaxRetries    int
	Enabled       bool
	AutoCommitCfg AutoCommitConfig
	WorkingDir    string
}

// AutoFixLoopOptions holds optional configuration for NewAutoFixLoop.
// Zero values fall back to sensible defaults.
type AutoFixLoopOptions struct {
	// MaxRetries caps the number of fix iterations. Defaults to 3.
	MaxRetries int
	// AutoCommit controls automatic git commits after successful fixes.
	// Defaults to disabled.
	AutoCommit AutoCommitConfig
}

// NewAutoFixLoop creates an AutoFixLoop with sensible defaults.
// When opts fields are zero, defaults are applied:
//   - MaxRetries: 3
//   - AutoCommit: disabled
func NewAutoFixLoop(
	linter Linter,
	tester Tester,
	fixer *tools.AutoFixer,
	rollback *tools.RollbackManager,
	opts ...AutoFixLoopOptions,
) *AutoFixLoop {
	var o AutoFixLoopOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	maxRetries := o.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &AutoFixLoop{
		Linter:        linter,
		Tester:        tester,
		Fixer:         fixer,
		Rollback:      rollback,
		MaxRetries:    maxRetries,
		Enabled:       true,
		AutoCommitCfg: o.AutoCommit,
	}
}

// Run executes the auto-fix loop over the given file paths.
//
// Flow:
//  1. Capture snapshot via RollbackManager.
//  2. Run linter → collect errors.
//  3. If no errors → run tests to verify → done.
//  4. Run AutoFixer on errors.
//  5. Re-run linter to verify.
//  6. Run tests to verify.
//  7. If still failing, reflect: parse error output into actionable
//     descriptions.
//  8. Repeat from step 4 (max MaxRetries times).
//  9. If retries exhausted, rollback via RollbackManager.
func (loop *AutoFixLoop) Run(ctx context.Context, filePaths []string) (*AutoFixLoopResult, error) {
	result := &AutoFixLoopResult{
		Attempts:        []LoopFixAttempt{},
		FinalLintErrors: []string{},
		FinalTestErrors: []string{},
	}

	if !loop.Enabled {
		return result, nil
	}

	snapshot, err := loop.Rollback.Capture(filePaths)
	if err != nil {
		return nil, fmt.Errorf("auto-fix loop: capture snapshot: %w", err)
	}

	for attempt := 1; attempt <= loop.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			// Restore on cancellation.
			if restoreErr := loop.Rollback.Restore(snapshot); restoreErr != nil {
				slog.Error("Auto-fix loop: rollback on cancel failed", "error", restoreErr)
			}
			return result, ctx.Err()
		default:
		}

		attemptRecord := LoopFixAttempt{
			AttemptNum: attempt,
		}

		lintErrors, err := loop.Linter.RunLint(ctx, filePaths)
		if err != nil {
			return nil, fmt.Errorf("auto-fix loop: run linter (attempt %d): %w", attempt, err)
		}
		attemptRecord.LintErrors = lintErrors

		// No lint errors — verify tests pass.
		if len(lintErrors) == 0 {
			testErrors, testErr := loop.Tester.RunTests(ctx)
			if testErr != nil {
				return nil, fmt.Errorf("auto-fix loop: run tests (attempt %d): %w", attempt, testErr)
			}
			attemptRecord.TestErrors = testErrors

			if len(testErrors) == 0 {
				result.Success = true
				result.Attempts = append(result.Attempts, attemptRecord)
				loop.autoCommit(ctx, attempt)
				return result, nil
			}

			// Tests failed despite clean lint — reflect and continue.
			attemptRecord.ReflectNotes = reflectOnErrors(testErrors)
			result.Attempts = append(result.Attempts, attemptRecord)
			continue
		}

		var allAppliedFixes []string
		for _, fp := range filePaths {
			fixResult, fixErr := loop.Fixer.Run(ctx, fp)
			if fixErr != nil {
				slog.Error("Auto-fix loop: fixer failed",
					"file", fp,
					"attempt", attempt,
					"error", fixErr,
				)
				continue
			}
			for _, fa := range fixResult.Attempts {
				allAppliedFixes = append(allAppliedFixes, fa.AppliedFixes...)
			}
		}
		attemptRecord.FixesApplied = allAppliedFixes

		postFixLintErrors, err := loop.Linter.RunLint(ctx, filePaths)
		if err != nil {
			return nil, fmt.Errorf("auto-fix loop: re-lint (attempt %d): %w", attempt, err)
		}

		testErrors, testErr := loop.Tester.RunTests(ctx)
		if testErr != nil {
			return nil, fmt.Errorf("auto-fix loop: run tests (attempt %d): %w", attempt, testErr)
		}
		attemptRecord.TestErrors = testErrors

		if len(postFixLintErrors) == 0 && len(testErrors) == 0 {
			result.Success = true
			result.Attempts = append(result.Attempts, attemptRecord)
			loop.autoCommit(ctx, attempt)
			return result, nil
		}

		allErrors := append([]string{}, postFixLintErrors...)
		allErrors = append(allErrors, testErrors...)
		attemptRecord.ReflectNotes = reflectOnErrors(allErrors)

		result.Attempts = append(result.Attempts, attemptRecord)
	}

	slog.Warn("Auto-fix loop: max retries exhausted, rolling back",
		"max_retries", loop.MaxRetries,
	)
	if restoreErr := loop.Rollback.Restore(snapshot); restoreErr != nil {
		slog.Error("Auto-fix loop: rollback failed", "error", restoreErr)
		return nil, fmt.Errorf("auto-fix loop: rollback failed: %w", restoreErr)
	}

	if len(result.Attempts) > 0 {
		result.Attempts[len(result.Attempts)-1].RolledBack = true
	}

	finalLint, lintErr := loop.Linter.RunLint(ctx, filePaths)
	if lintErr != nil {
		slog.Error("Auto-fix loop: final lint check failed", "error", lintErr)
	}
	result.FinalLintErrors = finalLint

	finalTests, testErr := loop.Tester.RunTests(ctx)
	if testErr != nil {
		slog.Error("Auto-fix loop: final test check failed", "error", testErr)
	}
	result.FinalTestErrors = finalTests

	return result, nil
}

// autoCommit calls AutoCommit when enabled after a successful fix cycle.
func (loop *AutoFixLoop) autoCommit(ctx context.Context, attempt int) {
	if !loop.AutoCommitCfg.Enabled {
		return
	}
	msg := fmt.Sprintf("auto-fix pass %d", attempt)
	if _, err := AutoCommit(ctx, loop.WorkingDir, msg, loop.AutoCommitCfg); err != nil {
		slog.Warn("AutoCommit failed", "error", err)
	}
}

// lintErrorPattern matches common linter output formats like:
//
//	file.go:42: some error message
//	file.go:42:10: some error message
var lintErrorPattern = regexp.MustCompile(`^(\S+):(\d+)(?::\d+)?:?\s*(.+)$`)

// testErrorPattern matches Go test failure lines like:
//
//	--- FAIL: TestName (0.00s)
//	    file_test.go:10: error message
var testErrorPattern = regexp.MustCompile(`^(\S+):(\d+):?\s*(.+)$`)

// reflectOnErrors parses error output lines into actionable descriptions
// that can guide subsequent fix attempts. Each returned string has the
// format "file:line: message".
func reflectOnErrors(errorLines []string) []string {
	var notes []string
	seen := make(map[string]struct{})

	for _, line := range errorLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if m := lintErrorPattern.FindStringSubmatch(line); len(m) >= 4 {
			note := fmt.Sprintf("%s:%s: %s", m[1], m[2], m[3])
			if _, ok := seen[note]; !ok {
				notes = append(notes, note)
				seen[note] = struct{}{}
			}
			continue
		}

		if m := testErrorPattern.FindStringSubmatch(line); len(m) >= 4 {
			note := fmt.Sprintf("%s:%s: %s", m[1], m[2], m[3])
			if _, ok := seen[note]; !ok {
				notes = append(notes, note)
				seen[note] = struct{}{}
			}
			continue
		}

		if _, ok := seen[line]; !ok {
			notes = append(notes, line)
			seen[line] = struct{}{}
		}
	}

	return notes
}

// AutoCommitConfig controls auto-commit behavior.
type AutoCommitConfig struct {
	// Enabled controls whether auto-commit is active.
	Enabled bool
	// MessagePrefix is the prefix for commit messages.
	MessagePrefix string
}

// DefaultAutoCommitConfig returns an AutoCommitConfig with default values.
func DefaultAutoCommitConfig() AutoCommitConfig {
	return AutoCommitConfig{
		Enabled:       true,
		MessagePrefix: "auto: ",
	}
}

// AutoCommit stages all changes and creates a commit. Returns the commit hash
// or an error. If git is not available, returns an empty string without error.
func AutoCommit(ctx context.Context, workingDir string, message string, cfg AutoCommitConfig) (string, error) {
	if !cfg.Enabled {
		return "", nil
	}

	prefix := cfg.MessagePrefix
	if prefix == "" {
		prefix = "auto: "
	}
	fullMessage := prefix + message

	stageCmd := exec.CommandContext(ctx, "git", "add", "-A")
	stageCmd.Dir = workingDir
	if err := stageCmd.Run(); err != nil {
		return "", nil
	}

	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", fullMessage)
	commitCmd.Dir = workingDir
	var stderr bytes.Buffer
	commitCmd.Stderr = &stderr
	if err := commitCmd.Run(); err != nil {
		return "", nil
	}

	revCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	revCmd.Dir = workingDir
	out, err := revCmd.Output()
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// TestFailure represents a parsed Go test failure.
type TestFailure struct {
	TestName string
	File     string
	Line     int
	Message  string
}

// goTestFailPattern matches Go test failure lines like:
//
//	--- FAIL: TestName (file.go:line)
//	--- FAIL: TestName (0.00s)
//	--- FAIL: TestName
var goTestFailPattern = regexp.MustCompile(`^--- FAIL: (\S+)(?:\s+\(([^)]+)\))?$`)

// ParseGoTestOutput parses Go test output and extracts failure information.
// Supports the standard format: "--- FAIL: TestName (file.go:line)" and
// collects subsequent message lines until the next "---" marker or
// "FAIL"/"PASS"/"ok" summary line.
func ParseGoTestOutput(output string) []TestFailure {
	var failures []TestFailure
	lines := strings.Split(output, "\n")

	for i := range lines {
		line := lines[i]
		m := goTestFailPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		tf := TestFailure{
			TestName: m[1],
		}
		if m[2] != "" {
			if idx := strings.LastIndex(m[2], ":"); idx >= 0 {
				tf.File = m[2][:idx]
				if n, err := strconv.Atoi(m[2][idx+1:]); err == nil {
					tf.Line = n
				}
			}
		}

		var msgLines []string
		for j := i + 1; j < len(lines); j++ {
			next := lines[j]
			if strings.HasPrefix(next, "---") ||
				strings.HasPrefix(next, "=== RUN") ||
				strings.HasPrefix(next, "FAIL") ||
				strings.HasPrefix(next, "PASS") ||
				strings.HasPrefix(next, "ok ") {
				break
			}
			trimmed := strings.TrimSpace(next)
			if trimmed != "" {
				msgLines = append(msgLines, trimmed)
			}
		}
		tf.Message = strings.Join(msgLines, "\n")

		failures = append(failures, tf)
	}

	return failures
}

// GoTestJSONResult holds the parsed result of a single test from go test -json
// output.
type GoTestJSONResult struct {
	Package string
	Test    string
	Action  string // "pass", "fail", or "skip".
	Output  string
	Elapsed float64 // Duration in seconds.
}

// goTestJSONEvent represents a single JSON line from go test -json output.
type goTestJSONEvent struct {
	Time    string  `json:"Time"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

// ParseGoTestJSON parses go test -json output (one JSON object per line) and
// returns structured test results. Each "output" event is accumulated under
// the correct test. Only tests with a terminal action (pass/fail/skip) are
// returned. Malformed lines are skipped with a warning.
func ParseGoTestJSON(input string) []GoTestJSONResult {
	type pending struct {
		outputs []string
		action  string
		elapsed float64
	}

	pendingMap := make(map[string]*pending)
	var order []string

	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var evt goTestJSONEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			slog.Warn("Skipping malformed go test JSON line", "line", string(line), "error", err)
			continue
		}

		if evt.Test == "" {
			continue
		}

		key := evt.Package + "\x00" + evt.Test
		p, ok := pendingMap[key]
		if !ok {
			p = &pending{}
			pendingMap[key] = p
			order = append(order, key)
		}

		switch evt.Action {
		case "output":
			p.outputs = append(p.outputs, strings.TrimRight(evt.Output, "\n"))
		case "pass", "fail", "skip":
			p.action = evt.Action
			p.elapsed = evt.Elapsed
		}
	}

	results := make([]GoTestJSONResult, 0, len(order))
	for _, key := range order {
		p := pendingMap[key]
		idx := strings.IndexByte(key, 0)

		if p.action == "" {
			continue
		}

		results = append(results, GoTestJSONResult{
			Package: key[:idx],
			Test:    key[idx+1:],
			Action:  p.action,
			Output:  strings.Join(p.outputs, "\n"),
			Elapsed: p.elapsed,
		})
	}

	return results
}

// ReflectionStrategy generates a fix prompt from a test failure and the
// source content at the failure location.
func ReflectionStrategy(failure TestFailure, sourceContent string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "The test %s failed", failure.TestName)
	if failure.File != "" {
		fmt.Fprintf(&b, " at %s:%d", failure.File, failure.Line)
	}
	b.WriteString(":\n")

	if failure.Message != "" {
		fmt.Fprintf(&b, "%s\n\n", failure.Message)
	}

	if sourceContent != "" {
		b.WriteString("Source code at the failure location:\n")
		b.WriteString(sourceContent)
		b.WriteString("\n\n")
	}

	b.WriteString("Suggest a fix that addresses the test failure.")
	return b.String()
}
