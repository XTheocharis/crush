//go:build treesitter

package extensions

import (
	"context"
	"log/slog"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/repomap"
)

// buildRepomapTools creates the repo-map tools with real refresh functions
// backed by repomap.Service when tree-sitter is available.
func (e *RepomapExtension) buildRepomapTools(ctx context.Context, host ext.HostContext) []fantasy.AgentTool {
	rawDB := host.DB()
	if rawDB == nil {
		slog.Warn("RepomapExtension: no DB available, using nil refresh functions")
		return baseRepomapTools(nil, nil)
	}

	cfg := host.Config()
	if cfg == nil || cfg.Options == nil || cfg.Options.RepoMap == nil || cfg.Options.RepoMap.Disabled {
		slog.Debug("RepomapExtension: repo map disabled in config")
		return baseRepomapTools(nil, nil)
	}

	q := db.New(rawDB)
	svc := repomap.NewService(cfg, q, rawDB, host.WorkingDir(), ctx)

	go svc.PreIndex()

	repomap.InitTiktokenLoader(repomap.TiktokenCacheDir())

	refreshSync := func(ctx context.Context, sessionID string) error {
		opts := repomap.GenerateOpts{
			SessionID:    sessionID,
			ForceRefresh: true,
		}
		if _, _, err := svc.Refresh(ctx, sessionID, opts); err != nil {
			return err
		}
		return nil
	}

	refreshAsync := func(_ context.Context, sessionID string) error {
		opts := repomap.GenerateOpts{
			SessionID:    sessionID,
			ForceRefresh: true,
		}
		svc.RefreshAsync(sessionID, opts)
		return nil
	}

	e.asyncRefresh = refreshAsync

	return baseRepomapTools(refreshSync, refreshAsync)
}

// triggerRefresh fires an asynchronous repo-map refresh using the service
// created during Init.
func (e *RepomapExtension) triggerRefresh(ctx context.Context, sessionID string) error {
	if sessionID == "" || e.asyncRefresh == nil {
		return nil
	}
	return e.asyncRefresh(ctx, sessionID)
}

func baseRepomapTools(syncFn, asyncFn tools.MapRefreshFn) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		tools.NewAgenticMapTool(),
		tools.NewLlmMapTool(),
		tools.NewMapRefreshTool(syncFn, asyncFn),
	}
}
