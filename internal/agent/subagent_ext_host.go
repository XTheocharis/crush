package agent

import (
	"context"
	"log/slog"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
)

// subAgentLimitsExt implements a lightweight extension that enforces resource
// limits (tokens, steps, duration) on sub-agents. It lives in the agent
// package to avoid an import cycle with the extensions package.
type subAgentLimitsExt struct {
	mu     sync.RWMutex
	host   ext.HostContext
	usage  *ResourceUsage
	active bool
	limits SubagentLimits
}

func (e *subAgentLimitsExt) Name() string { return "sub-agent-resource-limits" }

func (e *subAgentLimitsExt) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.usage = NewResourceUsage()
	e.active = true
	e.limits = DefaultLimitsProfile().Get("task")
	return nil
}

func (e *subAgentLimitsExt) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.usage = nil
	e.active = false
	return nil
}

func (e *subAgentLimitsExt) StepHooks() []ext.StepHook {
	if !e.active {
		return nil
	}
	return []ext.StepHook{
		{
			Name: "sub-agent-resource-limits-check",
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
					slog.Warn("Hard token limit exceeded, stopping sub-agent",
						"tokens_used", snapshot.TokensUsed,
						"hard_limit", e.limits.MaxTokens.Hard,
					)
					return true
				}
				if e.limits.MaxSteps.Exceeded(int(snapshot.StepsTaken)) {
					slog.Warn("Hard step limit exceeded, stopping sub-agent",
						"steps_taken", snapshot.StepsTaken,
						"hard_limit", e.limits.MaxSteps.Hard,
					)
					return true
				}
				if e.limits.DurationExceeded(snapshot.Elapsed) {
					slog.Warn("Hard duration limit exceeded, stopping sub-agent",
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

func (e *subAgentLimitsExt) RunHooks() []ext.RunHook {
	if !e.active {
		return nil
	}
	return []ext.RunHook{
		{
			Name: "sub-agent-resource-limits-per-run-reset",
			OnRunStart: func(_ context.Context, _ string, _ string) error {
				e.mu.Lock()
				defer e.mu.Unlock()
				if !e.active {
					return nil
				}
				e.usage = NewResourceUsage()
				return nil
			},
		},
	}
}

var (
	_ ext.Extension        = (*subAgentLimitsExt)(nil)
	_ ext.StepHookProvider = (*subAgentLimitsExt)(nil)
	_ ext.RunHookProvider  = (*subAgentLimitsExt)(nil)
)

// newSubAgentHost creates a lightweight ExtensionHost for sub-agents with only
// resource-limits enforcement.
func newSubAgentHost(cfg *config.ConfigStore) (*ext.ExtensionHost, error) {
	host := ext.NewLightweightHost(ext.HostDeps{
		Config:     cfg,
		WorkingDir: cfg.WorkingDir(),
	}, []ext.Extension{&subAgentLimitsExt{}})
	if err := host.Bootstrap(context.Background()); err != nil {
		return nil, err
	}
	return host, nil
}
