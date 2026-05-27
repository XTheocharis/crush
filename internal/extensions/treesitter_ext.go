//go:build treesitter

package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
)

// TreesitterExtension wraps the tree-sitter validation pipeline as a
// ToolProvider. Only compiled when the "treesitter" build tag is set.
type TreesitterExtension struct {
	mu     sync.RWMutex
	host   ext.HostContext
	tool   fantasy.AgentTool
	active bool
}

func (e *TreesitterExtension) Name() string { return "treesitter-validation" }

func (e *TreesitterExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	cfg := host.Config()
	if cfg == nil || cfg.Options == nil || cfg.Options.Validation == nil {
		e.active = false
		return nil
	}

	e.active = true
	return nil
}

func (e *TreesitterExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tool = nil
	e.active = false
	return nil
}

func (e *TreesitterExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	if e.tool == nil {
		pipeline := tools.NewValidationPipeline(nil)
		_ = pipeline
	}
	return nil, nil
}

func (e *TreesitterExtension) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return nil
}

var (
	_ ext.Extension    = (*TreesitterExtension)(nil)
	_ ext.ToolProvider = (*TreesitterExtension)(nil)
)
