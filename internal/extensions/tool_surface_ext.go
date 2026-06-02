package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/ext"
)

// ToolSurfaceExtension wraps the dynamic tool surface as a RunHookProvider.
// It updates tool visibility based on runtime context (LSP presence, MCP
// availability, etc.) at the start and end of each agent run.
//
// TECH DEBT: The write path (UpdateCapabilities via OnRunStart) is active and
// computes tool visibility metadata on every agent run. However, the read path
// on the underlying ToolSurface — GetVisibleTools(), GetHiddenTools(),
// IsVisible(), and PhaseFilteredTools() — is NOT consumed by any production
// code. These methods compute phase-based tool visibility (e.g., hiding edit
// tools during Planning phase) that no caller currently acts upon. The
// infrastructure was designed for future phase-based tool routing where the
// coordinator would filter its tool surface based on conversation phase and
// complexity tiers. Until that routing is wired in, the computed visibility
// data is discarded after each UpdateCapabilities call.
//
// TODO(sisyphus): Wire ToolSurface read path into coordinator.buildTools() or
// the prompt assembly pipeline so that phase-based tool filtering takes effect.
// See internal/agent/tool_surface.go:PhaseFilteredTools and the
// AgentPhase/phaseHiddenTools definitions.
type ToolSurfaceExtension struct {
	mu      sync.RWMutex
	host    ext.HostContext
	surface *agent.ToolSurface
	active  bool
}

func (e *ToolSurfaceExtension) Name() string { return "tool-surface" }

func (e *ToolSurfaceExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.surface = agent.NewToolSurface()
	e.active = true
	return nil
}

func (e *ToolSurfaceExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.surface = nil
	e.active = false
	return nil
}

func (e *ToolSurfaceExtension) RunHooks() []ext.RunHook {
	if !e.active {
		return nil
	}
	return []ext.RunHook{
		{
			Name: "tool-surface-update",
			OnRunStart: func(_ context.Context, _ string, _ string) error {
				e.mu.Lock()
				defer e.mu.Unlock()
				if e.surface == nil {
					return nil
				}
				cfg := e.host.Config()
				betaTools := cfg != nil && cfg.Options != nil && cfg.Options.BetaTools
				lspManager := e.host.LSP()
				ctx := agent.SurfaceContext{
					HasLSP:     lspManager != nil && lspManager.Clients().Len() > 0,
					HasLCM:     TheLCMExtension.Manager() != nil,
					HasRepoMap: TheRepomapExtension.isActive(),
					HasMCP:     hasMCPTools(),
					BetaTools:  betaTools,
				}
				e.surface.UpdateCapabilities(ctx)
				return nil
			},
			OnRunEnd: func(_ context.Context, _ string, _ *fantasy.AgentResult, _ error) error {
				return nil
			},
		},
	}
}

func hasMCPTools() bool {
	for range mcp.Tools() {
		return true
	}
	return false
}

var (
	_ ext.Extension       = (*ToolSurfaceExtension)(nil)
	_ ext.RunHookProvider = (*ToolSurfaceExtension)(nil)
)
