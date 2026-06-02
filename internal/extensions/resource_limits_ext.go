package extensions

import (
	"context"
	"log/slog"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/ext"
)

// ResourceLimitsExtension wraps the resource usage tracker as a StepHookProvider.
// It monitors per-step token consumption, step count, and duration, signalling
// when soft or hard limits are exceeded.
type ResourceLimitsExtension struct {
	mu     sync.RWMutex
	host   ext.HostContext
	usage  *agent.ResourceUsage
	active bool
	limits agent.SubagentLimits
}

func (e *ResourceLimitsExtension) Name() string { return "resource-limits" }

func (e *ResourceLimitsExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.usage = agent.NewResourceUsage()
	e.active = true
	e.limits = agent.DefaultLimitsProfile().Get("task")
	return nil
}

func (e *ResourceLimitsExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.usage = nil
	e.active = false
	return nil
}

func (e *ResourceLimitsExtension) StepHooks() []ext.StepHook {
	if !e.active {
		return nil
	}
	return []ext.StepHook{
		{
			Name: "resource-limits-check",
			OnStepFinish: func(_ context.Context, _ string, step fantasy.StepResult) error {
				e.mu.Lock()
				defer e.mu.Unlock()
				if !e.active || e.usage == nil {
					return nil
				}

				e.usage.AddStep()

				if text := step.Content.Text(); text != "" {
					e.usage.AddTokens(text)
				}

				// Soft-limit warnings only (no stop).
				e.usage.WarnTokensOnce(e.limits.MaxTokens)
				e.usage.WarnStepsOnce(e.limits.MaxSteps)
				e.usage.WarnDurationOnce(e.limits)

				return nil
			},
			StopCondition: func(_ context.Context, _ []fantasy.StepResult) bool {
				e.mu.RLock()
				defer e.mu.RUnlock()
				if !e.active || e.usage == nil {
					return false
				}

				snapshot := e.usage.Snapshot()

				if e.limits.MaxTokens.Exceeded(int(snapshot.TokensUsed)) {
					slog.Warn("Hard token limit exceeded, stopping agent",
						"tokens_used", snapshot.TokensUsed,
						"hard_limit", e.limits.MaxTokens.Hard,
					)
					return true
				}
				if e.limits.MaxSteps.Exceeded(int(snapshot.StepsTaken)) {
					slog.Warn("Hard step limit exceeded, stopping agent",
						"steps_taken", snapshot.StepsTaken,
						"hard_limit", e.limits.MaxSteps.Hard,
					)
					return true
				}
				if e.limits.DurationExceeded(snapshot.Elapsed) {
					slog.Warn("Hard duration limit exceeded, stopping agent",
						"elapsed", snapshot.Elapsed,
						"hard_limit", e.limits.MaxDuration,
					)
					return true
				}

				return false
			},
		},
	}
}

var (
	_ ext.Extension        = (*ResourceLimitsExtension)(nil)
	_ ext.StepHookProvider = (*ResourceLimitsExtension)(nil)
)
