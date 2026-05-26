package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/ext"
)

// XrushExtension wraps forked agent sessions as a ToolProvider.
// It manages the agent registry and mailbox for parent-child agent orchestration.
type XrushExtension struct {
	mu       sync.RWMutex
	host     ext.HostContext
	registry *agent.AgentRegistry
	mailbox  *agent.Mailbox
	active   bool
}

func (e *XrushExtension) Name() string { return "xrush-sessions" }

func (e *XrushExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.registry = agent.NewAgentRegistry()
	e.mailbox = agent.NewMailbox(agent.DefaultMailboxCapacity)
	e.active = true
	return nil
}

func (e *XrushExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registry = nil
	e.mailbox = nil
	e.active = false
	return nil
}

func (e *XrushExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	return nil, nil
}

func (e *XrushExtension) ToolNames() []string {
	return nil
}

func (e *XrushExtension) Registry() *agent.AgentRegistry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.registry
}

func (e *XrushExtension) Mailbox() *agent.Mailbox {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mailbox
}

var (
	_ ext.Extension    = (*XrushExtension)(nil)
	_ ext.ToolProvider = (*XrushExtension)(nil)
)
