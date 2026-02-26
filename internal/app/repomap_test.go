package app

import (
	"context"
	"testing"

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

func TestIsRepoMapResetCommand(t *testing.T) {
	t.Parallel()

	require.True(t, IsRepoMapResetCommand("project:map-reset"))
	require.True(t, IsRepoMapResetCommand("  project:map-reset  "))
	require.False(t, IsRepoMapResetCommand("project:map-refresh"))
	require.False(t, IsRepoMapResetCommand("map_reset"))
}
