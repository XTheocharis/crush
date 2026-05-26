package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
)

// OrchestrationExtension wraps multi-agent orchestration tools as a
// ToolProvider.
type OrchestrationExtension struct {
	mu       sync.RWMutex
	host     ext.HostContext
	registry tools.AgentRegistry
	mailbox  tools.Mailbox
	tools    []fantasy.AgentTool
	names    []string
	active   bool
}

func (e *OrchestrationExtension) Name() string { return "orchestration" }

func (e *OrchestrationExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	e.tools = buildOrchestrationTools(e.registry, e.mailbox)
	e.names = make([]string, len(e.tools))
	for i, t := range e.tools {
		e.names[i] = t.Info().Name
	}
	e.active = true
	return nil
}

func (e *OrchestrationExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tools = nil
	e.names = nil
	e.registry = nil
	e.mailbox = nil
	e.active = false
	return nil
}

func (e *OrchestrationExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	return append([]fantasy.AgentTool{}, e.tools...), nil
}

func (e *OrchestrationExtension) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return append([]string{}, e.names...)
}

// SetRegistry sets the agent registry for orchestration tools. Must be called
// before or during Init for tools to function.
func (e *OrchestrationExtension) SetRegistry(registry tools.AgentRegistry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registry = registry
}

// SetMailbox sets the mailbox for inter-agent messaging.
func (e *OrchestrationExtension) SetMailbox(mailbox tools.Mailbox) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mailbox = mailbox
}

func buildOrchestrationTools(registry tools.AgentRegistry, mailbox tools.Mailbox) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		tools.NewSendMessageTool(registry, mailbox),
		tools.NewTeamCreateTool(registry, mailbox),
		tools.NewTeamDeleteTool(registry),
		tools.NewTaskStopTool(registry),
	}
}

var (
	_ ext.Extension    = (*OrchestrationExtension)(nil)
	_ ext.ToolProvider = (*OrchestrationExtension)(nil)
)
