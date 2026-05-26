package ext

import (
	"context"

	"charm.land/fantasy"
)

// RunHook wraps the agent.Stream() call. Extensions can intercept the
// start and end of a complete agent run.
type RunHook struct {
	Name       string
	OnRunStart func(ctx context.Context, sessionID string, prompt string) error
	OnRunEnd   func(ctx context.Context, sessionID string, result *fantasy.AgentResult, err error) error
}

// StepHook wraps PrepareStep/OnStepFinish in the streaming loop.
// Extensions can modify messages before each step, observe step
// results, and signal early termination.
type StepHook struct {
	Name          string
	OnPrepareStep func(ctx context.Context, sessionID string, messages []fantasy.Message) ([]fantasy.Message, error)
	OnStepFinish  func(ctx context.Context, sessionID string, step fantasy.StepResult) error
	StopCondition func(ctx context.Context, steps []fantasy.StepResult) bool
}

// PromptHook wraps prompt preparation. Extensions can modify messages
// and the system prompt before they are sent to the model.
type PromptHook struct {
	Name                 string
	OnPreparePrompt      func(ctx context.Context, sessionID string, messages []fantasy.Message) ([]fantasy.Message, error)
	SystemPromptModifier func(ctx context.Context, sessionID string, systemPrompt string) (string, error)
}
