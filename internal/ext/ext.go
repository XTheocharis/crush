package ext

import (
	"context"

	"charm.land/fantasy"
)

// Extension is the base interface all extensions must implement.
type Extension interface {
	Name() string
	Init(ctx context.Context, host HostContext) error
	Shutdown(ctx context.Context) error
}

// ToolProvider is a capability interface for contributing tools.
type ToolProvider interface {
	Extension
	Tools(ctx context.Context) ([]fantasy.AgentTool, error)
	ToolNames() []string
}

// RunHookProvider is a capability for run-lifecycle hooks.
type RunHookProvider interface {
	Extension
	RunHooks() []RunHook
}

// StepHookProvider is a capability for step-lifecycle hooks.
type StepHookProvider interface {
	Extension
	StepHooks() []StepHook
}

// PromptHookProvider is a capability for prompt manipulation hooks.
type PromptHookProvider interface {
	Extension
	PromptHook() *PromptHook
}
