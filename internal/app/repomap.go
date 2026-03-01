package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/filetracker"
	"github.com/charmbracelet/crush/internal/repomap"
)

// RepoMapController handles explicit repo-map refresh/reset control paths.
type RepoMapController struct {
	svc         *repomap.Service
	cfg         *config.Config
	filetracker filetracker.Service
}

func newRepoMapController(svc *repomap.Service, cfg *config.Config, ft filetracker.Service) *RepoMapController {
	if svc == nil || cfg == nil {
		return nil
	}
	return &RepoMapController{svc: svc, cfg: cfg, filetracker: ft}
}

func (c *RepoMapController) Refresh(ctx context.Context, sessionID string, sync bool) (string, error) {
	if c == nil || c.svc == nil || !c.svc.Available() {
		return "", errors.New("repo map refresh is not available in this session")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", errors.New("session ID is required for map refresh")
	}
	opts := c.buildGenerateOpts(ctx, sessionID, true)
	if sync {
		if _, _, err := c.svc.Refresh(ctx, sessionID, opts); err != nil {
			return "", err
		}
		return "Repository map refreshed.", nil
	}
	c.svc.RefreshAsync(sessionID, opts)
	return "Repository map refresh scheduled.", nil
}

func (c *RepoMapController) Reset(ctx context.Context, sessionID string) error {
	if c == nil || c.svc == nil || !c.svc.Available() {
		return errors.New("repo map reset is not available in this session")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session ID is required for map reset")
	}
	if err := c.svc.Reset(ctx, sessionID); err != nil {
		return err
	}
	_, _, err := c.svc.Refresh(ctx, sessionID, c.buildGenerateOpts(ctx, sessionID, true))
	return err
}

func (c *RepoMapController) buildGenerateOpts(ctx context.Context, sessionID string, forceRefresh bool) repomap.GenerateOpts {
	opts := repomap.GenerateOpts{
		SessionID:    sessionID,
		ForceRefresh: forceRefresh,
	}
	if c == nil || c.cfg == nil {
		return opts
	}
	if model := c.cfg.GetModelByType(config.SelectedModelTypeLarge); model != nil {
		ctxWindow := int(model.ContextWindow)
		tok := config.DefaultRepoMapMaxTokens(ctxWindow)
		if repoCfg := c.cfg.Options.RepoMap; repoCfg != nil && repoCfg.MaxTokens > 0 {
			tok = repoCfg.MaxTokens
		}
		opts.TokenBudget = tok
		opts.MaxContextWindow = ctxWindow
		opts.Model = model.ID
	}
	if c.cfg.Options != nil && c.cfg.Options.LCM != nil {
		opts.ParityMode = strings.EqualFold(strings.TrimSpace(c.cfg.Options.LCM.ExplorerOutputProfile), "parity")
	}
	if opts.ParityMode {
		opts.PromptCachingEnabled = true
		opts.EnhancementTiers = "none"
		opts.DeterministicMode = true
		opts.TokenCounterMode = "tokenizer_backed"
	}
	if c.filetracker != nil {
		readPaths, err := c.filetracker.ListReadFiles(ctx, sessionID)
		if err == nil {
			chatFiles := make([]string, 0, len(readPaths))
			for _, p := range readPaths {
				rel, relErr := filepath.Rel(c.cfg.WorkingDir(), p)
				if relErr != nil {
					continue
				}
				chatFiles = append(chatFiles, filepath.ToSlash(rel))
			}
			opts.ChatFiles = chatFiles
		}
	}
	return opts
}

func IsRepoMapResetCommand(commandID string) bool {
	return strings.TrimSpace(commandID) == "project:map-reset"
}

func (app *App) RunRepoMapControl(ctx context.Context, commandID, sessionID string) (bool, string, error) {
	switch strings.TrimSpace(commandID) {
	case "project:map-refresh":
		if app == nil || app.repoMapCtl == nil {
			return true, "", errors.New("repo map refresh is not available in this session")
		}
		msg, err := app.repoMapCtl.Refresh(ctx, sessionID, true)
		return true, msg, err
	case "project:map-reset":
		if app == nil || app.repoMapCtl == nil {
			return true, "", errors.New("repo map reset is not available in this session")
		}
		if err := app.repoMapCtl.Reset(ctx, sessionID); err != nil {
			return true, "", err
		}
		return true, "Repository map reset and rebuilt.", nil
	default:
		return false, "", nil
	}
}

func (app *App) initRepoMap(ctx context.Context, conn *sql.DB) []agent.CoordinatorOption {
	if app == nil || app.config == nil || app.config.Options == nil || app.config.Options.RepoMap == nil || app.config.Options.RepoMap.Disabled {
		return nil
	}

	// Validate ExcludeGlobs patterns at init time so users get early
	// feedback about malformed patterns.
	for _, pattern := range app.config.Options.RepoMap.ExcludeGlobs {
		if _, err := doublestar.Match(pattern, ""); err != nil {
			slog.Warn("Malformed ExcludeGlobs pattern in repo map config",
				"pattern", pattern, "error", err)
		}
	}

	q := db.New(conn)
	svc := repomap.NewService(app.config, q, conn, app.config.WorkingDir(), ctx)
	app.repoMapSvc = svc
	app.repoMapCtl = newRepoMapController(svc, app.config, app.FileTracker)
	go svc.PreIndex()

	opts := []agent.CoordinatorOption{agent.WithRepoMap(svc)}

	provider, err := repomap.NewDefaultTokenCounterProvider()
	if err == nil && provider != nil {
		opts = append(opts, agent.WithTokenCounterProvider(provider))
	}

	return opts
}

func (app *App) mapRefreshSync(ctx context.Context, sessionID string) error {
	if app == nil || app.repoMapCtl == nil {
		return errors.New("repo map refresh is not available in this session")
	}
	_, err := app.repoMapCtl.Refresh(ctx, sessionID, true)
	return err
}

func (app *App) mapRefreshAsync(ctx context.Context, sessionID string) error {
	if app == nil || app.repoMapCtl == nil {
		return errors.New("repo map refresh is not available in this session")
	}
	_, err := app.repoMapCtl.Refresh(ctx, sessionID, false)
	return err
}

func (app *App) mapReset(ctx context.Context, sessionID string) error {
	if app == nil || app.repoMapCtl == nil {
		return errors.New("repo map reset is not available in this session")
	}
	if err := app.repoMapCtl.Reset(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to reset repository map: %w", err)
	}
	return nil
}
