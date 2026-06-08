package extensions

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/filetracker"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

// defaultMaxAutoFixIterations is the fallback when
// Validation.MaxAutoFixRetries is not set.
const defaultMaxAutoFixIterations = 3

// convergenceThreshold is the number of consecutive iterations with the same
// error fingerprint that triggers a non-convergence stop.
const convergenceThreshold = 2

// maxAutoFixerIterations caps the number of AutoFixer.Run() iterations to
// prevent autofix-diagnostic feedback loops.
const maxAutoFixerIterations = 5

// AutofixExtension wraps the auto-fix loop as a RunHookProvider.
// It runs post-turn validation after each agent run to catch and auto-fix
// lint errors. Only active on top-level agents (sub-agents have nil
// ExtensionHost, so hooks never fire for them).
type AutofixExtension struct {
	mu     sync.RWMutex
	host   ext.HostContext
	active bool

	filetracker  filetracker.Service
	lspManager   *lsp.Manager
	runStartTime time.Time

	loopEnabled   bool
	maxIterations int
	autoCommit    bool

	autofixEnabled bool
}

func (e *AutofixExtension) Name() string { return "autofix" }

func (e *AutofixExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.active = true
	e.filetracker = host.FileTracker()
	e.lspManager = host.LSP()
	cfg := host.Config()
	e.loopEnabled = autofixLoopEnabled(cfg)
	e.maxIterations = autofixMaxRetries(cfg)
	e.autoCommit = autofixAutoCommit(cfg)
	e.autofixEnabled = autofixIsEnabled(cfg)
	return nil
}

// autofixLoopEnabled reads the AutoFixLoopEnabled flag from config. Returns
// false when the config or Validation sub-config is nil.
func autofixLoopEnabled(cfg *config.Config) bool {
	if cfg == nil || cfg.Options == nil || cfg.Options.Validation == nil {
		return false
	}
	return cfg.Options.Validation.AutoFixLoopEnabled
}

// autofixMaxRetries reads MaxAutoFixRetries from config, falling back to
// defaultMaxAutoFixIterations when zero or unset.
func autofixMaxRetries(cfg *config.Config) int {
	if cfg == nil || cfg.Options == nil || cfg.Options.Validation == nil {
		return defaultMaxAutoFixIterations
	}
	if cfg.Options.Validation.MaxAutoFixRetries > 0 {
		return cfg.Options.Validation.MaxAutoFixRetries
	}
	return defaultMaxAutoFixIterations
}

// autofixAutoCommit reads the AutoFix flag from config for auto-commit
// behavior. Returns false when the config or Validation sub-config is nil.
func autofixAutoCommit(cfg *config.Config) bool {
	if cfg == nil || cfg.Options == nil || cfg.Options.Validation == nil {
		return false
	}
	return cfg.Options.Validation.AutoFix
}

func autofixIsEnabled(cfg *config.Config) bool {
	if cfg == nil || cfg.Options == nil || cfg.Options.Validation == nil {
		return true
	}
	v := cfg.Options.Validation
	if v.AutoFixLoopEnabled && !v.AutoFix {
		return false
	}
	return true
}

func (e *AutofixExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active = false
	return nil
}

func (e *AutofixExtension) RunHooks() []ext.RunHook {
	if !e.active {
		return nil
	}
	return []ext.RunHook{
		{
			Name:       "autofix-pre-commit",
			OnRunStart: func(_ context.Context, _ string, _ string) error { e.runStartTime = time.Now(); return nil },
			OnRunEnd:   e.onRunEnd,
		},
	}
}

// onRunEnd runs the auto-fix loop after each agent turn:
//  1. Collect Go files in the working directory.
//  2. When loopEnabled=false: lint → format → re-lint (max 3 iterations).
//  3. When loopEnabled=true: full lint → fix → test → reflect cycle with
//     convergence detection and rollback on exhaustion.
//  4. When autofixEnabled: create AutoFixer with fix strategies and run
//     alongside formatters, with cycle detection (max 5 iterations).
func (e *AutofixExtension) onRunEnd(
	ctx context.Context,
	sessionID string,
	_ *fantasy.AgentResult,
	runErr error,
) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}

	if runErr != nil {
		return nil
	}

	if ext.StoppedByCondition(ctx) {
		slog.Debug("Autofix extension: skipping, agent was stopped by condition")
		return nil
	}

	if e.filetracker != nil && !e.filetracker.HasWritesSince(ctx, sessionID, e.runStartTime) {
		slog.Debug("Autofix extension: skipping, no files written this turn")
		return nil
	}

	workingDir := e.host.WorkingDir()
	filePaths := collectGoFilePaths(ctx, workingDir)
	if len(filePaths) == 0 {
		return nil
	}

	if e.loopEnabled {
		timeout := agent.DefaultLintTimeout
		if cfg := e.host.Config(); cfg != nil && cfg.Options != nil && cfg.Options.AutofixTimeout > 0 {
			timeout = cfg.Options.AutofixTimeout
		}
		linter := &agent.GoLinter{WorkingDir: workingDir, Timeout: timeout}
		rollback := tools.NewRollbackManager()
		e.fullAutoFixCycle(ctx, workingDir, filePaths, linter, rollback)
		return nil
	}

	if e.autofixEnabled {
		e.runAutoFixer(ctx, workingDir, filePaths)
		return nil
	}

	return nil
}

// runAutoFixer creates an AutoFixer with fix strategies and runs it over
// the given file paths. It wires the DiagnosticProvider to LSP diagnostics
// when available, falling back to CLI linters (go vet, staticcheck).
func (e *AutofixExtension) runAutoFixer(
	ctx context.Context,
	workingDir string,
	filePaths []string,
) {
	strategies := []tools.FixStrategy{
		&tools.MissingImportFixer{},
		&tools.UnusedVarFixer{},
		&tools.FormattingFixer{},
	}

	diagProvider := e.buildDiagnosticProvider(ctx, workingDir, filePaths)
	contentProvider := func(filePath string) (string, error) {
		data, err := os.ReadFile(filePath)
		return string(data), err
	}
	contentSetter := func(filePath, content string) error {
		return os.WriteFile(filePath, []byte(content), 0o644)
	}

	fixer := tools.NewAutoFixer(strategies, diagProvider, contentProvider, contentSetter)
	fixer.MaxAttempts = maxAutoFixerIterations

	rollback := tools.NewRollbackManager()
	snapshot, err := rollback.Capture(filePaths)
	if err != nil {
		slog.Debug("Autofix extension: snapshot capture failed", "error", err)
		return
	}

	appliedFormatters := runFormatters(ctx, workingDir)
	if len(appliedFormatters) > 0 {
		slog.Info("Autofix extension: applied formatters", "fixes", appliedFormatters)
	}

	var totalFixes int
	for _, fp := range filePaths {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result, fixErr := fixer.Run(ctx, fp)
		if fixErr != nil {
			slog.Debug("Autofix extension: fixer failed", "file", fp, "error", fixErr)
			continue
		}
		totalFixes += result.FixesApplied
		if len(result.RemainingErrors) > 0 {
			slog.Warn("Autofix extension: remaining errors",
				"file", fp,
				"remaining", len(result.RemainingErrors),
			)
		}
	}

	if totalFixes > 0 {
		slog.Info("Autofix extension: auto-fixer applied fixes", "total", totalFixes)
	}

	timeout := agent.DefaultLintTimeout
	if cfg := e.host.Config(); cfg != nil && cfg.Options != nil && cfg.Options.AutofixTimeout > 0 {
		timeout = cfg.Options.AutofixTimeout
	}
	linter := &agent.GoLinter{WorkingDir: workingDir, Timeout: timeout}
	finalErrors, lintErr := linter.RunLint(ctx, filePaths)
	if lintErr != nil {
		slog.Debug("Autofix extension: final lint failed", "error", lintErr)
		return
	}

	if len(finalErrors) > 0 {
		slog.Warn("Autofix extension: rolling back, errors remain after autofix",
			"remaining_errors", len(finalErrors))
		if restoreErr := rollback.Restore(snapshot); restoreErr != nil {
			slog.Error("Autofix extension: rollback failed", "error", restoreErr)
		}
	}
}

func (e *AutofixExtension) buildDiagnosticProvider(
	ctx context.Context,
	workingDir string,
	filePaths []string,
) tools.DiagnosticProvider {
	if e.lspManager != nil {
		return func(filePath string) []tools.DiagnosticInfo {
			return e.collectLSPDiagnostics(filePath)
		}
	}

	timeout := agent.DefaultLintTimeout
	if cfg := e.host.Config(); cfg != nil && cfg.Options != nil && cfg.Options.AutofixTimeout > 0 {
		timeout = cfg.Options.AutofixTimeout
	}
	linter := &agent.GoLinter{WorkingDir: workingDir, Timeout: timeout}

	return func(filePath string) []tools.DiagnosticInfo {
		lintErrors, err := linter.RunLint(ctx, []string{filePath})
		if err != nil {
			return nil
		}
		return lintErrorsToDiagnostics(lintErrors)
	}
}

func (e *AutofixExtension) collectLSPDiagnostics(filePath string) []tools.DiagnosticInfo {
	if e.lspManager == nil {
		return nil
	}
	var infos []tools.DiagnosticInfo
	for _, client := range e.lspManager.Clients().Seq2() {
		for uri, diags := range client.GetDiagnostics() {
			path, err := uri.Path()
			if err != nil || path != filePath {
				continue
			}
			for _, d := range diags {
				infos = append(infos, tools.DiagnosticInfo{
					FilePath:  path,
					Line:      d.Range.Start.Line,
					Character: d.Range.Start.Character,
					Severity:  convertProtocolSeverity(d.Severity),
					Message:   d.Message,
				})
			}
		}
	}
	return infos
}

func convertProtocolSeverity(sev protocol.DiagnosticSeverity) tools.DiagnosticSeverity {
	switch sev {
	case protocol.SeverityError:
		return tools.SeverityError
	case protocol.SeverityWarning:
		return tools.SeverityWarning
	case protocol.SeverityInformation:
		return tools.SeverityInfo
	case protocol.SeverityHint:
		return tools.SeverityHint
	default:
		return tools.SeverityInfo
	}
}

func lintErrorsToDiagnostics(errors []string) []tools.DiagnosticInfo {
	var diags []tools.DiagnosticInfo
	for _, e := range errors {
		m := lintErrorRe.FindStringSubmatch(e)
		if len(m) < 4 {
			continue
		}
		var line uint32
		if n, err := fmt.Sscanf(m[2], "%d", new(int)); err == nil && n == 1 {
			line = uint32(mustParseInt(m[2]))
		}
		diags = append(diags, tools.DiagnosticInfo{
			FilePath: m[1],
			Line:     line,
			Severity: tools.SeverityError,
			Message:  m[3],
		})
	}
	return diags
}

func mustParseInt(s string) int {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func runFormatters(ctx context.Context, workingDir string) []string {
	var applied []string
	if runFormatter(ctx, workingDir, "gofumpt", "-w", ".") {
		applied = append(applied, "gofumpt")
	}
	if runFormatter(ctx, workingDir, "goimports", "-w", ".") {
		applied = append(applied, "goimports")
	}
	return applied
}

func runFormatter(ctx context.Context, workingDir, name string, args ...string) bool {
	path, err := exec.LookPath(name)
	if err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, path, args...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	if err := cmd.Run(); err != nil {
		slog.Debug("Autofix extension: formatter failed",
			"formatter", name, "error", err)
		return false
	}
	return true
}

func collectGoFilePaths(ctx context.Context, workingDir string) []string {
	cmd := exec.CommandContext(ctx, "go", "list", "-f",
		"{{range .GoFiles}}{{$.Dir}}/{{.}}\n{{end}}",
		"./...",
	)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	return collectNonEmptyLines(string(output))
}

func collectNonEmptyLines(s string) []string {
	var lines []string
	for l := range strings.SplitSeq(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// lintErrorRe matches common linter output formats like:
//
//	file.go:42: some error message
//	file.go:42:10: some error message
var lintErrorRe = regexp.MustCompile(`^(\S+):(\d+)(?::\d+)?:?\s*(.+)$`)

// testErrorRe matches Go test failure lines like:
//
//	--- FAIL: TestName (0.00s)
//	    file_test.go:10: error message
var testErrorRe = regexp.MustCompile(`^(\S+):(\d+):?\s*(.+)$`)

// reflectOnErrors parses error output lines into actionable descriptions
// that can guide subsequent fix attempts. Each returned string has the
// format "file:line: message". Deduplicates identical notes.
func reflectOnErrors(errorLines []string) []string {
	var notes []string
	seen := make(map[string]struct{})

	for _, line := range errorLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if m := lintErrorRe.FindStringSubmatch(line); len(m) >= 4 {
			note := fmt.Sprintf("%s:%s: %s", m[1], m[2], m[3])
			if _, ok := seen[note]; !ok {
				notes = append(notes, note)
				seen[note] = struct{}{}
			}
			continue
		}

		if m := testErrorRe.FindStringSubmatch(line); len(m) >= 4 {
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

// fingerprintErrors produces a stable hash of sorted, deduplicated error
// lines. The same set of errors always yields the same fingerprint regardless
// of order.
func fingerprintErrors(errors []string) string {
	if len(errors) == 0 {
		return ""
	}
	deduped := make(map[string]struct{}, len(errors))
	for _, e := range errors {
		deduped[strings.TrimSpace(e)] = struct{}{}
	}
	sorted := make([]string, 0, len(deduped))
	for e := range deduped {
		sorted = append(sorted, e)
	}
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return fmt.Sprintf("%x", h[:])
}

// fullAutoFixCycle runs the complete lint → fix → test → reflect cycle.
// It mirrors the AutoFixLoop from internal/agent/autofix.go but is
// self-contained within the extension.
//
// Flow per iteration:
//  1. Run linter → collect errors.
//  2. If no lint errors → run tests to verify → done if clean.
//  3. Apply formatters as the fix step.
//  4. Re-run linter to verify.
//  5. Run tests to verify.
//  6. If both clean → done.
//  7. If still failing → reflect on remaining errors, continue.
//  8. If retries exhausted → rollback via snapshot.
func (e *AutofixExtension) fullAutoFixCycle(
	ctx context.Context,
	workingDir string,
	filePaths []string,
	linter agent.Linter,
	rollback *tools.RollbackManager,
) {
	snapshot, err := rollback.Capture(filePaths)
	if err != nil {
		slog.Debug("Autofix extension: snapshot capture failed", "error", err)
		return
	}

	tester := &agent.GoTester{WorkingDir: workingDir}
	var lastFingerprint string
	var convergenceCount int

	for attempt := 1; attempt <= e.maxIterations; attempt++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		lintErrors, lintErr := linter.RunLint(ctx, filePaths)
		if lintErr != nil {
			slog.Debug("Autofix extension: lint failed",
				"attempt", attempt, "error", lintErr)
			return
		}

		// No lint errors — verify tests pass.
		if len(lintErrors) == 0 {
			testErrors, testErr := tester.RunTests(ctx)
			if testErr != nil {
				slog.Debug("Autofix extension: test run failed",
					"attempt", attempt, "error", testErr)
				return
			}

			if len(testErrors) == 0 {
				slog.Info("Autofix extension: clean after attempt (lint + tests)",
					"attempt", attempt)
				return
			}

			// Tests failed despite clean lint — reflect and continue.
			notes := reflectOnErrors(testErrors)
			slog.Warn("Autofix extension: tests failed with clean lint",
				"attempt", attempt,
				"test_errors", len(testErrors),
				"reflections", notes,
			)

			fp := fingerprintErrors(testErrors)
			if fp == lastFingerprint {
				convergenceCount++
			} else {
				convergenceCount = 1
			}
			lastFingerprint = fp
			if convergenceCount >= convergenceThreshold {
				slog.Warn("Autofix extension: not converging, same errors repeating",
					"consecutive_repeats", convergenceCount)
				break
			}
			continue
		}

		slog.Info("Autofix extension: lint errors detected",
			"attempt", attempt, "errors", len(lintErrors))

		// Apply formatters as the fix step.
		appliedFixes := runFormatters(ctx, workingDir)
		if len(appliedFixes) > 0 {
			slog.Info("Autofix extension: applied formatters",
				"attempt", attempt, "fixes", appliedFixes)
		}

		// Re-lint after fix.
		postFixLintErrors, err := linter.RunLint(ctx, filePaths)
		if err != nil {
			slog.Debug("Autofix extension: re-lint failed",
				"attempt", attempt, "error", err)
			return
		}

		// Run tests to verify.
		testErrors, testErr := tester.RunTests(ctx)
		if testErr != nil {
			slog.Debug("Autofix extension: test run failed",
				"attempt", attempt, "error", testErr)
			return
		}

		if len(postFixLintErrors) == 0 && len(testErrors) == 0 {
			slog.Info("Autofix extension: clean after fix and tests",
				"attempt", attempt)
			return
		}

		// Reflect on all remaining errors.
		allErrors := append([]string{}, postFixLintErrors...)
		allErrors = append(allErrors, testErrors...)
		notes := reflectOnErrors(allErrors)
		slog.Warn("Autofix extension: errors remain after fix",
			"attempt", attempt,
			"lint_errors", len(postFixLintErrors),
			"test_errors", len(testErrors),
			"reflections", notes,
		)

		fp := fingerprintErrors(allErrors)
		if fp == lastFingerprint {
			convergenceCount++
		} else {
			convergenceCount = 1
		}
		lastFingerprint = fp
		if convergenceCount >= convergenceThreshold {
			slog.Warn("Autofix extension: not converging, same errors repeating",
				"consecutive_repeats", convergenceCount)
			break
		}
	}

	// Exhausted retries — rollback.
	finalErrors, lintErr := linter.RunLint(ctx, filePaths)
	if lintErr != nil {
		slog.Debug("Autofix extension: final lint check failed", "error", lintErr)
	}

	if len(finalErrors) > 0 {
		slog.Warn("Autofix extension: rolling back, errors remain after max retries",
			"max_retries", e.maxIterations,
			"remaining_errors", len(finalErrors))
		if restoreErr := rollback.Restore(snapshot); restoreErr != nil {
			slog.Error("Autofix extension: rollback failed", "error", restoreErr)
		}
	}
}

var (
	_ ext.Extension       = (*AutofixExtension)(nil)
	_ ext.RunHookProvider = (*AutofixExtension)(nil)
)
