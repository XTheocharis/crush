//go:build !treesitter

package extensions

import (
	"context"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
)

func (e *RepomapExtension) buildRepomapTools(_ context.Context, _ ext.HostContext) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		tools.NewAgenticMapTool(),
		tools.NewLlmMapTool(),
		tools.NewMapRefreshTool(nil, nil),
	}
}

// triggerRefresh is a no-op without tree-sitter.
func (e *RepomapExtension) triggerRefresh(_ context.Context, _ string) error {
	return nil
}
