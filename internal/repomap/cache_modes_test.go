package repomap

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBuildRenderCacheKeyModes(t *testing.T) {
	t.Parallel()

	opts := GenerateOpts{
		ChatFiles:       []string{"b.go", "a.go"},
		MentionedFnames: []string{"z.go", "m.go"},
		MentionedIdents: []string{"Foo", "Bar"},
		TokenBudget:     1024,
	}

	auto := buildRenderCacheKey("auto", opts)
	files := buildRenderCacheKey("files", opts)
	manual := buildRenderCacheKey("manual", opts)
	always := buildRenderCacheKey("always", opts)

	require.NotEmpty(t, auto)
	require.NotEmpty(t, files)
	require.Equal(t, "manual", manual)
	require.Empty(t, always)
	require.NotEqual(t, auto, files)
}

func TestGenerateUsesRenderCacheByMode(t *testing.T) {
	t.Parallel()

	mkSvc := func(mode string, withLast bool) *Service {
		cfg := &config.Config{Options: &config.Options{RepoMap: &config.RepoMapOptions{RefreshMode: mode}}}
		svc := NewService(cfg, nil, nil, ".", context.Background())
		if withLast {
			svc.sessionCaches.Store("s", "last", 1)
		}
		key := buildRenderCacheKey(mode, GenerateOpts{SessionID: "s", ChatFiles: []string{"a.go"}, TokenBudget: 100})
		if key != "" {
			svc.renderCaches.GetOrCreate("s").Set(key, "cached", 2)
		}
		return svc
	}

	t.Run("last-good takes precedence", func(t *testing.T) {
		t.Parallel()
		for _, mode := range []string{"auto", "files", "manual", "always"} {
			svc := mkSvc(mode, true)
			m, tok, err := svc.Generate(context.Background(), GenerateOpts{SessionID: "s", ChatFiles: []string{"a.go"}, TokenBudget: 100})
			require.NoError(t, err)
			require.Equal(t, "last", m)
			require.Equal(t, 1, tok)
		}
	})

	t.Run("auto and files fall back to render cache when last-good empty", func(t *testing.T) {
		t.Parallel()
		for _, mode := range []string{"auto", "files"} {
			svc := mkSvc(mode, false)
			m, tok, err := svc.Generate(context.Background(), GenerateOpts{SessionID: "s", ChatFiles: []string{"a.go"}, TokenBudget: 100})
			require.NoError(t, err)
			require.Equal(t, "cached", m)
			require.Equal(t, 2, tok)
		}
	})

	t.Run("manual returns empty on cold start without last-good", func(t *testing.T) {
		t.Parallel()
		svc := mkSvc("manual", false)
		m, tok, err := svc.Generate(context.Background(), GenerateOpts{SessionID: "s", ChatFiles: []string{"a.go"}, TokenBudget: 100})
		require.NoError(t, err)
		require.Empty(t, m)
		require.Zero(t, tok)
	})
}

func TestRefreshStoresRenderCacheForKeyedModes(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Options: &config.Options{RepoMap: &config.RepoMapOptions{RefreshMode: "files"}}}
	svc := NewService(cfg, nil, nil, ".", context.Background())
	svc.sessionCaches.Store("s", "last", 7)

	m, tok, err := svc.Refresh(context.Background(), "s", GenerateOpts{SessionID: "s", ChatFiles: []string{"a.go"}, TokenBudget: 100})
	require.NoError(t, err)
	require.Equal(t, "last", m)
	require.Equal(t, 7, tok)

	key := buildRenderCacheKey("files", GenerateOpts{SessionID: "s", ChatFiles: []string{"a.go"}, TokenBudget: 100})
	cachedMap, cachedTok, ok := svc.renderCaches.GetOrCreate("s").Get(key)
	require.True(t, ok)
	require.Equal(t, "last", cachedMap)
	require.Equal(t, 7, cachedTok)
}

func TestGenerateForceRefreshInvalidatesSessionAndRenderCaches(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Options: &config.Options{RepoMap: &config.RepoMapOptions{RefreshMode: "auto"}}}
	svc := NewService(cfg, nil, nil, ".", context.Background())

	opts := GenerateOpts{
		SessionID:       "s",
		ChatFiles:       []string{"a.go"},
		MentionedFnames: []string{"b.go"},
		MentionedIdents: []string{"Foo"},
		TokenBudget:     100,
	}
	key := buildRenderCacheKey("auto", opts)
	svc.sessionCaches.Store("s", "last", 9)
	svc.renderCaches.GetOrCreate("s").Set(key, "cached", 7)

	m, tok, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:       "s",
		ChatFiles:       []string{"a.go"},
		MentionedFnames: []string{"b.go"},
		MentionedIdents: []string{"Foo"},
		TokenBudget:     100,
		ForceRefresh:    true,
	})
	require.NoError(t, err)
	require.Empty(t, m)
	require.Zero(t, tok)

	lastMap, lastTok := svc.sessionCaches.Load("s")
	require.Empty(t, lastMap)
	require.Zero(t, lastTok)

	_, _, ok := svc.renderCaches.GetOrCreate("s").Get(key)
	require.False(t, ok)
}

func TestRefreshForceRefreshInvalidatesCachesBeforeGenerate(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Options: &config.Options{RepoMap: &config.RepoMapOptions{RefreshMode: "files"}}}
	svc := NewService(cfg, nil, nil, ".", context.Background())
	opts := GenerateOpts{SessionID: "s", ChatFiles: []string{"a.go"}, TokenBudget: 100}
	key := buildRenderCacheKey("files", opts)

	svc.sessionCaches.Store("s", "last", 5)
	svc.renderCaches.GetOrCreate("s").Set(key, "cached", 6)

	m, tok, err := svc.Refresh(context.Background(), "s", GenerateOpts{SessionID: "s", ChatFiles: []string{"a.go"}, TokenBudget: 100, ForceRefresh: true})
	require.NoError(t, err)
	require.Empty(t, m)
	require.Zero(t, tok)

	lastMap, lastTok := svc.sessionCaches.Load("s")
	require.Empty(t, lastMap)
	require.Zero(t, lastTok)

	_, _, ok := svc.renderCaches.GetOrCreate("s").Get(key)
	require.False(t, ok)
}
