package repomap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/testutil"
	"github.com/stretchr/testify/require"
)

// crushRepoRoot returns the crush repository root.
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

	// Create a session.
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

	// The map should contain files from the crush codebase.
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

	// Generate with chat files — the map should rank related files higher.
	mapText, tokenCount, err := svc.Generate(ctx, GenerateOpts{
		SessionID:   sess.ID,
		TokenBudget: tokenBudget,
		ChatFiles:   []string{"internal/lcm/compactor.go"},
		MentionedIdents: []string{
			"CompactOnce",
			"Summarizer",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, mapText)
	require.Greater(t, tokenCount, 0)
	require.LessOrEqual(t, tokenCount, tokenBudget)

	// With LCM-related chat files, the map should include related LCM files.
	require.Contains(t, mapText, "lcm",
		"repo map should include LCM-related files when given LCM chat context")

	t.Logf("RepoMap with chat files — tokens: %d / %d", tokenCount, tokenBudget)
	t.Logf("RepoMap output:\n%s", mapText)
}
