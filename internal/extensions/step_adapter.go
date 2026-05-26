package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/ext"
)

// StepAdapter bridges fork-style PrepareStepHook message mutation with the
// extension host's StepHook lifecycle. It converts coordinator-level message
// mutators into StepHook.OnPrepareStep callbacks dispatched by agent.go's
// PrepareStep callback.
//
// The fork's PrepareStepHook mutates (ctx, opts, prepared) including tools.
// The extension host's OnPrepareStep only sees messages, so this adapter
// constrains the bridge to the message mutation subset.
type StepAdapter struct {
	mu       sync.RWMutex
	host     ext.HostContext
	mutators []MessageMutator
	active   bool
}

// MessageMutator mutates prepared messages before each LLM step. It mirrors
// the fork's PrepareStepHook message capability constrained to what the
// extension host's StepHook.OnPrepareStep supports. Implementations must be
// safe for concurrent use.
type MessageMutator func(ctx context.Context, sessionID string, messages []fantasy.Message) ([]fantasy.Message, error)

func (e *StepAdapter) Name() string { return "step-adapter" }

func (e *StepAdapter) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.active = true
	return nil
}

func (e *StepAdapter) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mutators = nil
	e.active = false
	return nil
}

// AddMutator registers a message mutator called during PrepareStep via
// the extension host's StepHook dispatch.
func (e *StepAdapter) AddMutator(m MessageMutator) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mutators = append(e.mutators, m)
}

// StepHooks wraps each registered mutator as an OnPrepareStep callback.
// OnStepFinish and StopCondition are nil — this adapter is purely for
// message mutation.
func (e *StepAdapter) StepHooks() []ext.StepHook {
	e.mu.RLock()
	mutators := make([]MessageMutator, len(e.mutators))
	copy(mutators, e.mutators)
	e.mu.RUnlock()

	if !e.active || len(mutators) == 0 {
		return nil
	}

	hooks := make([]ext.StepHook, len(mutators))
	for i, m := range mutators {
		m := m
		hooks[i] = ext.StepHook{
			Name: "step-adapter-mutator",
			OnPrepareStep: func(ctx context.Context, sessionID string, messages []fantasy.Message) ([]fantasy.Message, error) {
				return m(ctx, sessionID, messages)
			},
		}
	}
	return hooks
}

var (
	_ ext.Extension        = (*StepAdapter)(nil)
	_ ext.StepHookProvider = (*StepAdapter)(nil)
)
