package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
)

// EditExtension wraps fuzzy edit, anchor edit, and batch edit tools as a
// ToolProvider. It provides the enhanced editing capabilities from the fork.
type EditExtension struct {
	mu     sync.RWMutex
	host   ext.HostContext
	tools  []fantasy.AgentTool
	names  []string
	active bool
}

func (e *EditExtension) Name() string { return "edit-advanced" }

func (e *EditExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	store := tools.NewMapContentStore(nil)

	batchTool := tools.NewBatchEditTool(store)

	e.tools = []fantasy.AgentTool{batchTool}
	e.names = []string{tools.BatchEditToolName}
	e.active = true
	return nil
}

func (e *EditExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tools = nil
	e.names = nil
	e.active = false
	return nil
}

func (e *EditExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	return append([]fantasy.AgentTool{}, e.tools...), nil
}

func (e *EditExtension) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return append([]string{}, e.names...)
}

var (
	_ ext.Extension    = (*EditExtension)(nil)
	_ ext.ToolProvider = (*EditExtension)(nil)
)
