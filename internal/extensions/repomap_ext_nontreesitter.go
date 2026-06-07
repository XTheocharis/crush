//go:build !treesitter

package extensions

import (
	"context"
	"log/slog"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
)

func (e *RepomapExtension) buildRepomapTools(_ context.Context, _ ext.HostContext) []fantasy.AgentTool {
	slog.Warn("RepomapExtension: built WITHOUT treesitter tag — repo-map refresh is disabled, " +
		"all repo-map tables (file_cache, tags, imports, session_rankings) will remain empty. " +
		"Rebuild with CGO_ENABLED=1 and -tags=treesitter to enable repo-map.")
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
