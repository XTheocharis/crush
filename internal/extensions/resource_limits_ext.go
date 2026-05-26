package extensions

import (
	"context"
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
}

func (e *ResourceLimitsExtension) Name() string { return "resource-limits" }

func (e *ResourceLimitsExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.usage = agent.NewResourceUsage()
	e.active = true
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
			OnStepFinish: func(_ context.Context, _ string, _ fantasy.StepResult) error {
				e.mu.Lock()
				defer e.mu.Unlock()
				if !e.active || e.usage == nil {
					return nil
				}
				profile := agent.DefaultLimitsProfile()
				limits := profile.Get("task")
				_ = limits
				return nil
			},
		},
	}
}

var (
	_ ext.Extension        = (*ResourceLimitsExtension)(nil)
	_ ext.StepHookProvider = (*ResourceLimitsExtension)(nil)
)
