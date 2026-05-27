package extensions

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
)

// maxAutoFixIterations caps the auto-fix loop to prevent infinite cycles.
const maxAutoFixIterations = 3

// AutofixExtension wraps the auto-fix loop as a RunHookProvider.
// It runs post-turn validation after each agent run to catch and auto-fix
// lint errors. Only active on top-level agents (sub-agents have nil
// ExtensionHost, so hooks never fire for them).
type AutofixExtension struct {
	mu     sync.RWMutex
	host   ext.HostContext
	active bool
}

func (e *AutofixExtension) Name() string { return "autofix" }

func (e *AutofixExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.active = true
	return nil
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
			OnRunStart: func(_ context.Context, _ string, _ string) error { return nil },
			OnRunEnd:   e.onRunEnd,
		},
	}
}

// onRunEnd runs the auto-fix loop after each agent turn:
//  1. Collect Go files in the working directory.
//  2. Run the auto-fix loop: lint → format → re-lint (max 3 iterations).
//  3. If still failing after max retries, rollback and log remaining errors.
func (e *AutofixExtension) onRunEnd(
	ctx context.Context,
	_ string,
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

	workingDir := e.host.WorkingDir()
	filePaths := collectGoFilePaths(ctx, workingDir)
	if len(filePaths) == 0 {
		return nil
	}

	linter := &agent.GoLinter{WorkingDir: workingDir}
	rollback := tools.NewRollbackManager()

	snapshot, err := rollback.Capture(filePaths)
	if err != nil {
		slog.Debug("Autofix extension: snapshot capture failed", "error", err)
		return nil
	}

	for attempt := 1; attempt <= maxAutoFixIterations; attempt++ {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		lintErrors, lintErr := linter.RunLint(ctx, filePaths)
		if lintErr != nil {
			slog.Debug("Autofix extension: lint failed",
				"attempt", attempt, "error", lintErr)
			return nil
		}

		if len(lintErrors) == 0 {
			slog.Info("Autofix extension: clean after attempt",
				"attempt", attempt)
			return nil
		}

		slog.Info("Autofix extension: lint errors detected",
			"attempt", attempt, "errors", len(lintErrors))

		appliedFixes := runFormatters(ctx, workingDir)
		if len(appliedFixes) > 0 {
			slog.Info("Autofix extension: applied formatters",
				"attempt", attempt, "fixes", appliedFixes)
		}
	}

	finalErrors, lintErr := linter.RunLint(ctx, filePaths)
	if lintErr != nil {
		slog.Debug("Autofix extension: final lint failed", "error", lintErr)
		return nil
	}

	if len(finalErrors) > 0 {
		slog.Warn("Autofix extension: rolling back, errors remain after max retries",
			"max_retries", maxAutoFixIterations,
			"remaining_errors", len(finalErrors))
		if restoreErr := rollback.Restore(snapshot); restoreErr != nil {
			slog.Error("Autofix extension: rollback failed", "error", restoreErr)
		}
	}

	return nil
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

var (
	_ ext.Extension       = (*AutofixExtension)(nil)
	_ ext.RunHookProvider = (*AutofixExtension)(nil)
)
