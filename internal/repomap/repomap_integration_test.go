//go:build treesitter
// +build treesitter

package repomap

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/testutil"
	"github.com/stretchr/testify/require"
)

func crushRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}

func TestIntegration_Generate_AgainstCrushRepo(t *testing.T) {
	testutil.SkipIfNoIntegration(t)

	repoRoot := crushRepoRoot(t)
	ctx := context.Background()

	cfg, err := config.Init(repoRoot, "", false)
	require.NoError(t, err)
	cfg.Config().Options.RepoMap = &config.RepoMapOptions{
		RefreshMode: "always",
		ExcludeGlobs: []string{
			"vendor/**",
			"node_modules/**",
		},
	}

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	svc := NewService(cfg.Config(), q, conn, repoRoot, ctx)
	t.Cleanup(func() { svc.Close() })

	sessSvc := session.NewService(q, conn)
	sess, err := sessSvc.Create(t.Context(), "integration-repomap-test")
	require.NoError(t, err)

	const tokenBudget = 2048

	mapText, tokenCount, err := svc.Generate(ctx, GenerateOpts{
		SessionID:   sess.ID,
		TokenBudget: tokenBudget,
		ChatFiles:   []string{},
	})
	require.NoError(t, err)
	require.NotEmpty(t, mapText, "repo map should not be empty")
	require.Greater(t, tokenCount, 0, "token count should be positive")
	require.LessOrEqual(t, tokenCount, tokenBudget,
		"token count should not exceed budget")

	require.Contains(t, mapText, "internal/",
		"repo map should include internal/ paths from the crush codebase")

	t.Logf("RepoMap tokens: %d / %d budget", tokenCount, tokenBudget)
	t.Logf("RepoMap output (%d chars):\n%s", len(mapText), mapText)
}

func TestIntegration_Generate_WithChatFiles(t *testing.T) {
	testutil.SkipIfNoIntegration(t)

	repoRoot := crushRepoRoot(t)
	ctx := context.Background()

	cfg, err := config.Init(repoRoot, "", false)
	require.NoError(t, err)
	cfg.Config().Options.RepoMap = &config.RepoMapOptions{RefreshMode: "always"}

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	svc := NewService(cfg.Config(), q, conn, repoRoot, ctx)
	t.Cleanup(func() { svc.Close() })

	sessSvc := session.NewService(q, conn)
	sess, err := sessSvc.Create(t.Context(), "integration-repomap-chatfiles")
	require.NoError(t, err)

	const tokenBudget = 4096

	mapText, tokenCount, err := svc.Generate(ctx, GenerateOpts{
		SessionID:   sess.ID,
		TokenBudget: tokenBudget,
		ChatFiles:   []string{"internal/repomap/repomap.go"},
		MentionedIdents: []string{
			"compactLocked",
			"Summarizer",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, mapText)
	require.Greater(t, tokenCount, 0)
	require.LessOrEqual(t, tokenCount, tokenBudget)

	require.Contains(t, mapText, "repomap",
		"repo map should include repomap-related files when given repomap chat context")

	t.Logf("RepoMap with chat files — tokens: %d / %d", tokenCount, tokenBudget)
	t.Logf("RepoMap output:\n%s", mapText)
}

// TestIntegration_BlameProximityDiff exercises the full pipeline with blame,
// proximity, and diff watcher all wired into Service.Generate.
func TestIntegration_BlameProximityDiff(t *testing.T) {
	testutil.SkipIfNoIntegration(t)

	tmpDir := t.TempDir()
	ctx := context.Background()

	require.NoError(t, exec.Command("git", "init", tmpDir).Run())
	require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run())

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "calc.go"), []byte(`package calc

func Add(a, b int) int { return a + b }
func Sub(a, b int) int { return a - b }
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "calc_test.go"), []byte(`package calc

import "testing"

func TestAdd(t *testing.T) { }
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "util.go"), []byte(`package calc

func Helper() string { return "ok" }
`), 0o644))

	require.NoError(t, exec.Command("git", "-C", tmpDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", tmpDir, "commit", "-m", "initial").Run())

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	cfg := &config.Config{
		Options: &config.Options{
			RepoMap: &config.RepoMapOptions{
				RefreshMode: "always",
			},
		},
	}

	renderCaches := NewSessionRenderCacheSet()
	sessionCaches := NewSessionCacheSet()

	dw := NewDiffWatcher(DiffWatcherConfig{
		RootDir:       tmpDir,
		Interval:      1 * time.Hour,
		RenderCaches:  renderCaches,
		SessionCaches: sessionCaches,
	})

	svc := NewService(cfg, q, conn, tmpDir, ctx,
		WithDiffWatcher(dw),
		WithProximityScorer(),
	)
	t.Cleanup(func() { svc.Close() })

	sessSvc := session.NewService(q, conn)
	sess, err := sessSvc.Create(t.Context(), "integration-blame-prox-diff")
	require.NoError(t, err)

	mapText, tokenCount, err := svc.Generate(ctx, GenerateOpts{
		SessionID:     sess.ID,
		TokenBudget:   4096,
		ChatFiles:     []string{"calc.go"},
		WithBlameInfo: true,
		ForceRefresh:  true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, mapText, "repo map should not be empty with blame+proximity+diff")
	require.Greater(t, tokenCount, 0)

	mapText2, tokenCount2, err := svc.Generate(ctx, GenerateOpts{
		SessionID:     sess.ID,
		TokenBudget:   4096,
		ChatFiles:     []string{"calc.go"},
		WithBlameInfo: true,
	})
	require.NoError(t, err)
	require.Equal(t, mapText, mapText2, "cached result should match")
	require.Equal(t, tokenCount, tokenCount2)

	t.Logf("RepoMap (blame+prox+diff) tokens: %d", tokenCount)
	t.Logf("RepoMap output:\n%s", mapText)
}

// TestIntegration_ProximityWithoutGit verifies that proximity scoring works
// even without git (graceful degradation).
func TestIntegration_ProximityWithoutGit(t *testing.T) {
	testutil.SkipIfNoIntegration(t)

	tmpDir := t.TempDir()
	ctx := context.Background()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "handler.go"), []byte(`package handler

func Handle() error { return nil }
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "handler_test.go"), []byte(`package handler

import "testing"

func TestHandle(t *testing.T) { }
`), 0o644))

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	cfg := &config.Config{
		Options: &config.Options{
			RepoMap: &config.RepoMapOptions{
				RefreshMode: "always",
			},
		},
	}

	svc := NewService(cfg, q, conn, tmpDir, ctx,
		WithProximityScorer(),
	)
	t.Cleanup(func() { svc.Close() })

	sessSvc := session.NewService(q, conn)
	sess, err := sessSvc.Create(t.Context(), "integration-prox-no-git")
	require.NoError(t, err)

	mapText, tokenCount, err := svc.Generate(ctx, GenerateOpts{
		SessionID:     sess.ID,
		TokenBudget:   4096,
		WithBlameInfo: true,
		ForceRefresh:  true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, mapText, "repo map should still generate without git")
	require.Greater(t, tokenCount, 0)

	t.Logf("RepoMap (proximity, no git) tokens: %d", tokenCount)
	t.Logf("RepoMap output:\n%s", mapText)
}

// TestIntegration_DiffWatcherStopOnClose verifies that DiffWatcher is stopped
// when the Service is closed.
func TestIntegration_DiffWatcherStopOnClose(t *testing.T) {
	testutil.SkipIfNoIntegration(t)

	tmpDir := t.TempDir()
	ctx := context.Background()

	require.NoError(t, exec.Command("git", "init", tmpDir).Run())
	require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run())

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(`package main

func main() { }
`), 0o644))
	require.NoError(t, exec.Command("git", "-C", tmpDir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", tmpDir, "commit", "-m", "initial").Run())

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)

	q := db.New(conn)
	cfg := &config.Config{
		Options: &config.Options{
			RepoMap: &config.RepoMapOptions{
				RefreshMode: "always",
			},
		},
	}

	renderCaches := NewSessionRenderCacheSet()
	sessionCaches := NewSessionCacheSet()

	dw := NewDiffWatcher(DiffWatcherConfig{
		RootDir:       tmpDir,
		Interval:      100 * time.Millisecond,
		RenderCaches:  renderCaches,
		SessionCaches: sessionCaches,
	})

	svc := NewService(cfg, q, conn, tmpDir, ctx,
		WithDiffWatcher(dw),
	)

	svc.PreIndex()

	require.NoError(t, svc.Close())

	dw.mu.Lock()
	running := dw.running
	dw.mu.Unlock()
	require.False(t, running, "DiffWatcher should not be running after Service.Close()")

	require.NoError(t, conn.Close())
}
