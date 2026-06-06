package extensions

import (
	"context"
	"fmt"
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
	mu               sync.RWMutex
	host             ext.HostContext
	detector         *agent.ProductiveLoopDetector
	active           bool
	steps            []fantasy.StepResult
	maxWindow        int
	pendingWarning   string
	pendingLevel     agent.EscalationLevel
	lastDoomDetected bool
}

func (e *DoomExtension) Name() string { return "doom-loop" }

func (e *DoomExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	doomDetector := agent.NewDoomLoopDetector(agent.DefaultDoomLoopThresholds, 0)
	doomDetector.SetInterventionMode(host.Config().Options.DoomLoopIntervention)
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
	e.lastDoomDetected = false
	return nil
}

func (e *DoomExtension) StepHooks() []ext.StepHook {
	if !e.active {
		return nil
	}
	return []ext.StepHook{
		{
			Name: "doom-loop-detect",
			OnPrepareStep: func(_ context.Context, _ string, messages []fantasy.Message) ([]fantasy.Message, error) {
				e.mu.Lock()
				warning := e.pendingWarning
				level := e.pendingLevel
				e.pendingWarning = ""
				e.pendingLevel = agent.EscalationNone
				e.mu.Unlock()

				if warning == "" {
					return messages, nil
				}

				levelStr := "soft"
				if level == agent.EscalationMedium {
					levelStr = "medium"
				}

				warningText := fmt.Sprintf(`<doom-loop-warning level="%s">%s</doom-loop-warning>`, levelStr, warning)
				warningMsg := fantasy.Message{
					Role: fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{
						&fantasy.TextPart{Text: warningText},
					},
				}
				return append([]fantasy.Message{warningMsg}, messages...), nil
			},
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
				e.lastDoomDetected = result.Level == agent.EscalationHard
				if result.Level == agent.EscalationSoft || result.Level == agent.EscalationMedium {
					e.pendingLevel = result.Level
					e.pendingWarning = result.Message
				}
				return nil
			},
			StopCondition: func(_ context.Context, _ []fantasy.StepResult) bool {
				e.mu.RLock()
				defer e.mu.RUnlock()
				if !e.active || e.detector == nil {
					return false
				}
				return e.lastDoomDetected
			},
		},
	}
}

var (
	_ ext.Extension        = (*DoomExtension)(nil)
	_ ext.StepHookProvider = (*DoomExtension)(nil)
)
