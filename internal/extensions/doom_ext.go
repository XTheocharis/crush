package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/ext"
)

// DoomExtension wraps the productive loop detector as a StepHookProvider.
// It analyses recent step results to detect repeated tool-call patterns and
// escalates through soft/medium/hard levels to break loops. Loops that produce
// diverse outputs (productive loops) are not escalated.
type DoomExtension struct {
	mu        sync.RWMutex
	host      ext.HostContext
	detector  *agent.ProductiveLoopDetector
	active    bool
	steps     []fantasy.StepResult
	maxWindow int
}

func (e *DoomExtension) Name() string { return "doom-loop" }

func (e *DoomExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	doomDetector := agent.NewDoomLoopDetector(agent.DefaultDoomLoopThresholds, 0)
	e.detector = agent.NewProductiveLoopDetector(doomDetector)
	e.maxWindow = 10
	e.active = true
	return nil
}

func (e *DoomExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.detector = nil
	e.steps = nil
	e.active = false
	return nil
}

func (e *DoomExtension) StepHooks() []ext.StepHook {
	if !e.active {
		return nil
	}
	return []ext.StepHook{
		{
			Name: "doom-loop-detect",
			OnStepFinish: func(_ context.Context, _ string, step fantasy.StepResult) error {
				e.mu.Lock()
				defer e.mu.Unlock()
				if !e.active || e.detector == nil {
					return nil
				}
				e.steps = append(e.steps, step)
				if len(e.steps) > e.maxWindow {
					e.steps = e.steps[len(e.steps)-e.maxWindow:]
				}
				result := e.detector.Detect(e.steps)
				_ = result
				return nil
			},
			StopCondition: func(_ context.Context, steps []fantasy.StepResult) bool {
				e.mu.RLock()
				defer e.mu.RUnlock()
				if !e.active || e.detector == nil {
					return false
				}
				result := e.detector.Detect(steps)
				return result.Level == agent.EscalationHard
			},
		},
	}
}

var (
	_ ext.Extension        = (*DoomExtension)(nil)
	_ ext.StepHookProvider = (*DoomExtension)(nil)
)
