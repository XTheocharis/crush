package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/filetracker"
	"github.com/charmbracelet/crush/internal/repomap"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/require"
)

func testRepoMapController(t *testing.T, refreshMode string) (*RepoMapController, *repomap.Service) {
	t.Helper()
	cfg, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)
	cfg.Options.RepoMap = &config.RepoMapOptions{RefreshMode: refreshMode}

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	svc := repomap.NewService(cfg, q, conn, cfg.WorkingDir(), context.Background())
	ft := filetracker.NewService(q, cfg.WorkingDir())
	ctl := newRepoMapController(svc, cfg, ft)

	return ctl, svc
}

func ensureSession(t *testing.T, svc *repomap.Service, cfg *config.Config) string {
	t.Helper()
	conn, err := db.Connect(t.Context(), cfg.Options.DataDirectory)
	require.NoError(t, err)
	defer conn.Close()
	q := db.New(conn)
	sessSvc := session.NewService(q, conn)
	sess, err := sessSvc.Create(t.Context(), "test session")
	require.NoError(t, err)
	require.NotEmpty(t, sess.ID)
	return sess.ID
}

func TestRunRepoMapControlProjectRefreshAndReset(t *testing.T) {
	cfg, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)
	cfg.Options.RepoMap = &config.RepoMapOptions{RefreshMode: "manual"}
	cfg.SetupAgents()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	app := &App{config: cfg, FileTracker: filetracker.NewService(q, cfg.WorkingDir())}
	opt := app.initRepoMap(t.Context(), conn)
	require.NotNil(t, opt)
	require.NotNil(t, app.repoMapCtl)
	require.NotNil(t, app.repoMapSvc)

	sessSvc := session.NewService(q, conn)
	sess, err := sessSvc.Create(t.Context(), "test")
	require.NoError(t, err)

	handled, msg, err := app.RunRepoMapControl(t.Context(), "project:map-refresh", sess.ID)
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, "Repository map refreshed.", msg)

	handled, msg, err = app.RunRepoMapControl(t.Context(), "project:map-reset", sess.ID)
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, "Repository map reset and rebuilt.", msg)

	handled, msg, err = app.RunRepoMapControl(t.Context(), "project:unknown", sess.ID)
	require.False(t, handled)
	require.NoError(t, err)
	require.Empty(t, msg)
}

func TestRunRepoMapControlReportsUnavailableWhenNoService(t *testing.T) {
	t.Parallel()

	app := &App{}
	handled, _, err := app.RunRepoMapControl(t.Context(), "project:map-refresh", "sess-1")
	require.True(t, handled)
	require.ErrorContains(t, err, "not available")

	handled, _, err = app.RunRepoMapControl(t.Context(), "project:map-reset", "sess-1")
	require.True(t, handled)
	require.ErrorContains(t, err, "not available")
}

func TestMapRefreshSyncAndAsyncRequireSession(t *testing.T) {
	cfg, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)
	cfg.Options.RepoMap = &config.RepoMapOptions{RefreshMode: "manual"}

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	q := db.New(conn)
	app := &App{config: cfg, FileTracker: filetracker.NewService(q, cfg.WorkingDir())}
	_ = app.initRepoMap(t.Context(), conn)

	err = app.mapRefreshSync(t.Context(), "")
	require.ErrorContains(t, err, "session ID is required")

	err = app.mapRefreshAsync(t.Context(), "")
	require.ErrorContains(t, err, "session ID is required")
}

func TestMapResetClearsInjectedRunState(t *testing.T) {
	cfg, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)
	cfg.Options.RepoMap = &config.RepoMapOptions{RefreshMode: "manual"}

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	q := db.New(conn)
	app := &App{config: cfg, FileTracker: filetracker.NewService(q, cfg.WorkingDir())}
	_ = app.initRepoMap(t.Context(), conn)

	sessSvc := session.NewService(q, conn)
	sess, err := sessSvc.Create(t.Context(), "test")
	require.NoError(t, err)

	runKey := repomap.RunInjectionKey{RootUserMessageID: "m1", QueueGeneration: 0}
	require.True(t, app.repoMapSvc.ShouldInject(sess.ID, runKey))
	require.False(t, app.repoMapSvc.ShouldInject(sess.ID, runKey))

	err = app.mapReset(t.Context(), sess.ID)
	require.NoError(t, err)
	require.True(t, app.repoMapSvc.ShouldInject(sess.ID, runKey))
}

func TestRunRepoMapControlRefreshResetObservableMapChanges(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/repomap\n\ngo 1.23\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte("package main\n\nfunc MainA() {}\n"), 0o644))

	cfg, err := config.Init(repoRoot, "", false)
	require.NoError(t, err)
	cfg.Options.RepoMap = &config.RepoMapOptions{RefreshMode: "manual", MaxTokens: 4096}
	cfg.SetupAgents()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	q := db.New(conn)
	app := &App{config: cfg, FileTracker: filetracker.NewService(q, cfg.WorkingDir())}
	_ = app.initRepoMap(t.Context(), conn)
	require.NotNil(t, app.repoMapSvc)

	sessSvc := session.NewService(q, conn)
	sess, err := sessSvc.Create(t.Context(), "map-observable")
	require.NoError(t, err)

	handled, msg, err := app.RunRepoMapControl(t.Context(), "project:map-refresh", sess.ID)
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, "Repository map refreshed.", msg)

	firstMap := app.repoMapSvc.LastGoodMap(sess.ID)
	require.NotEmpty(t, firstMap)
	require.Contains(t, firstMap, "main.go")
	require.Contains(t, firstMap, "MainA")

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte("package main\n\nfunc MainB() {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "added.go"), []byte("package main\n\nfunc AddedFromRefresh() {}\n"), 0o644))

	handled, msg, err = app.RunRepoMapControl(t.Context(), "project:map-refresh", sess.ID)
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, "Repository map refreshed.", msg)

	refreshedMap := app.repoMapSvc.LastGoodMap(sess.ID)
	require.NotEmpty(t, refreshedMap)
	require.Contains(t, refreshedMap, "MainB")
	require.NotContains(t, refreshedMap, "MainA")

	prevTokenCount := app.repoMapSvc.LastTokenCount(sess.ID)
	require.Greater(t, prevTokenCount, 0)

	handled, msg, err = app.RunRepoMapControl(t.Context(), "project:map-reset", sess.ID)
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, "Repository map reset and rebuilt.", msg)

	resetMap := app.repoMapSvc.LastGoodMap(sess.ID)
	require.NotEmpty(t, resetMap)
	require.Contains(t, resetMap, "MainB")
	require.Equal(t, resetMap, app.repoMapSvc.LastGoodMap(sess.ID))
	require.Greater(t, app.repoMapSvc.LastTokenCount(sess.ID), 0)
}

func TestMapRefreshAsyncSchedulesAndUpdatesObservableMap(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/repomap\n\ngo 1.23\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "async.go"), []byte("package main\n\nfunc AsyncBefore() {}\n"), 0o644))

	cfg, err := config.Init(repoRoot, "", false)
	require.NoError(t, err)
	cfg.Options.RepoMap = &config.RepoMapOptions{RefreshMode: "always", MaxTokens: 4096}

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	q := db.New(conn)
	app := &App{config: cfg, FileTracker: filetracker.NewService(q, cfg.WorkingDir())}
	_ = app.initRepoMap(t.Context(), conn)

	sessSvc := session.NewService(q, conn)
	sess, err := sessSvc.Create(t.Context(), "map-async")
	require.NoError(t, err)

	require.NoError(t, app.mapRefreshSync(t.Context(), sess.ID))
	initial := app.repoMapSvc.LastGoodMap(sess.ID)
	require.Contains(t, initial, "AsyncBefore")

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "async.go"), []byte("package main\n\nfunc AsyncAfter() {}\n"), 0o644))

	require.NoError(t, app.mapRefreshAsync(t.Context(), sess.ID))

	deadline := time.Now().Add(2 * time.Second)
	for {
		updated := app.repoMapSvc.LastGoodMap(sess.ID)
		if strings.Contains(updated, "AsyncAfter") {
			require.NotContains(t, updated, "AsyncBefore")
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for async map refresh to update map; latest map: %q", updated)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestIsRepoMapResetCommand(t *testing.T) {
	t.Parallel()

	require.True(t, IsRepoMapResetCommand("project:map-reset"))
	require.True(t, IsRepoMapResetCommand("  project:map-reset  "))
	require.False(t, IsRepoMapResetCommand("project:map-refresh"))
	require.False(t, IsRepoMapResetCommand("map_reset"))
}
