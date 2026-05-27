package hooks

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/shell"
)

// RunPostToolUse executes all matching PostToolUse hooks. Unlike PreToolUse,
// PostToolUse hooks are non-blocking: exit codes 2 and 49 do NOT halt the
// pipeline or deny the result (the tool already executed). The primary use
// case is output rewriting/sanitization via UpdatedOutput.
func (r *Runner) RunPostToolUse(ctx context.Context, eventName, sessionID, toolName, toolInputJSON, toolOutput string, durationMs int64) (AggregateResult, error) {
	matching := r.matchingHooks(toolName)
	if len(matching) == 0 {
		return AggregateResult{Decision: DecisionNone}, nil
	}

	seen := make(map[string]bool, len(matching))
	var deduped []config.HookConfig
	for _, h := range matching {
		if seen[h.Command] {
			continue
		}
		seen[h.Command] = true
		deduped = append(deduped, h)
	}

	envVars := BuildPostEnv(eventName, toolName, sessionID, r.cwd, r.projectDir, toolInputJSON, toolOutput, durationMs)
	payload := BuildPostPayload(eventName, sessionID, r.cwd, toolName, toolInputJSON, toolOutput, durationMs)

	results := make([]HookResult, len(deduped))
	var wg sync.WaitGroup
	wg.Add(len(deduped))
	for i, h := range deduped {
		go func(idx int, hook config.HookConfig) {
			defer wg.Done()
			results[idx] = r.runOnePost(ctx, hook, envVars, payload)
		}(i, h)
	}
	wg.Wait()

	agg := aggregate(results, toolInputJSON)
	agg.Hooks = make([]HookInfo, len(deduped))
	for i, h := range deduped {
		agg.Hooks[i] = HookInfo{
			Name:         h.Command,
			Matcher:      h.Matcher,
			Decision:     results[i].Decision.String(),
			Halt:         results[i].Halt,
			Reason:       results[i].Reason,
			InputRewrite: results[i].UpdatedInput != "",
		}
	}
	slog.Info(
		"PostToolUse hook completed",
		"event", eventName,
		"tool", toolName,
		"hooks", len(deduped),
		"decision", agg.Decision.String(),
	)
	return agg, nil
}

// runOnePost executes a single PostToolUse hook. Exit codes 2 and 49 are
// non-blocking (unlike PreToolUse): the tool already ran so they become
// warnings rather than deny/halt decisions.
func (r *Runner) runOnePost(parentCtx context.Context, hook config.HookConfig, envVars []string, payload []byte) HookResult {
	timeout := hook.TimeoutDuration()
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runShell(ctx, shell.RunOptions{
			Command: hook.Command,
			Cwd:     r.cwd,
			Env:     envVars,
			Stdin:   bytes.NewReader(payload),
			Stdout:  &stdout,
			Stderr:  &stderr,
		})
	}()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		select {
		case err = <-done:
		case <-time.After(abandonGrace):
			slog.Warn(
				"PostToolUse hook did not yield after cancel; abandoning goroutine",
				"command", hook.Command,
				"timeout", timeout,
			)
			return HookResult{Decision: DecisionNone}
		}
	}

	if shell.IsInterrupt(err) {
		if parentCtx.Err() != nil {
			slog.Debug("PostToolUse hook cancelled by parent context", "command", hook.Command)
		} else {
			slog.Warn("PostToolUse hook timed out", "command", hook.Command, "timeout", timeout)
		}
		return HookResult{Decision: DecisionNone}
	}

	if err != nil {
		exitCode := shell.ExitCode(err)
		// xrush: all non-zero exit codes are non-blocking for PostToolUse.
		// The tool already executed; we just log and move on.
		slog.Warn(
			"PostToolUse hook failed (non-blocking)",
			"command", hook.Command,
			"exit_code", exitCode,
			"stderr", strings.TrimSpace(stderr.String()),
			"error", err,
		)
		return HookResult{Decision: DecisionNone}
	}

	modified := parsePostStdout(stdout.String())
	slog.Debug(
		"PostToolUse hook executed",
		"command", hook.Command,
		"modified_output", modified != "",
	)
	return HookResult{UpdatedOutput: modified}
}
