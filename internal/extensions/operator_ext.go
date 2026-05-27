package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/ext"
)

// OperatorExtension wraps the operator pattern decomposer as a ToolProvider.
type OperatorExtension struct {
	mu     sync.RWMutex
	host   ext.HostContext
	active bool
}

func (e *OperatorExtension) Name() string { return "operator" }

func (e *OperatorExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.active = true
	return nil
}

func (e *OperatorExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active = false
	return nil
}

func (e *OperatorExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	return nil, nil
}

func (e *OperatorExtension) ToolNames() []string {
	return nil
}

// NewOperator creates an operator for task decomposition with defaults.
func (e *OperatorExtension) NewOperator(strategy agent.DecomposeStrategy) *agent.Operator {
	return agent.NewOperator(agent.OperatorConfig{Strategy: strategy}, nil, nil)
}

var _ ext.Extension = (*OperatorExtension)(nil)
