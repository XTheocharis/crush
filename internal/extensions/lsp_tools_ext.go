package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/lsp"
)

// LSPToolsExtension contributes all LSP-backed tools as a ToolProvider.
// It uses host.LSP() to obtain the LSP manager during Init.
type LSPToolsExtension struct {
	mu     sync.RWMutex
	host   ext.HostContext
	tools  []fantasy.AgentTool
	names  []string
	active bool
}

func (e *LSPToolsExtension) Name() string { return "lsp-tools" }

func (e *LSPToolsExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	lspManager := host.LSP()
	if lspManager == nil {
		e.active = false
		return nil
	}

	e.tools = buildLSPTools(lspManager)
	e.names = make([]string, len(e.tools))
	for i, t := range e.tools {
		e.names[i] = t.Info().Name
	}
	e.active = true
	return nil
}

func (e *LSPToolsExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tools = nil
	e.names = nil
	e.active = false
	return nil
}

func (e *LSPToolsExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	return append([]fantasy.AgentTool{}, e.tools...), nil
}

func (e *LSPToolsExtension) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return append([]string{}, e.names...)
}

func buildLSPTools(mgr *lsp.Manager) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		tools.NewDefinitionTool(mgr),
		tools.NewRenameTool(mgr),
		tools.NewCodeActionTool(mgr),
		tools.NewSafeDeleteTool(mgr),
		tools.NewReplaceSymbolTool(),
		tools.NewInsertBeforeTool(),
		tools.NewInsertAfterTool(),
		tools.NewFormattingTool(mgr),
		tools.NewHoverTool(mgr),
		tools.NewCompletionTool(mgr),
		tools.NewSignatureHelpTool(mgr),
		tools.NewLSPRestartTool(mgr),
		tools.NewSymbolsTool(mgr),
		tools.NewDocumentSymbolsTool(mgr),
		tools.NewWorkspaceSymbolsTool(mgr),
	}
}

var (
	_ ext.Extension    = (*LSPToolsExtension)(nil)
	_ ext.ToolProvider = (*LSPToolsExtension)(nil)
)
