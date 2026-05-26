package filetracker

import (
	"database/sql"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func TestListRecentReadFiles(t *testing.T) {
	t.Parallel()
	env := setupTest(t)

	sessionID := "recent-session-1"
	env.createSession(t, sessionID)

	env.svc.RecordRead(env.ctx, sessionID, "/path/to/recent1.go")
	env.svc.RecordRead(env.ctx, sessionID, "/path/to/recent2.go")

	files, err := env.svc.ListRecentReadFiles(env.ctx, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, files, 2)
}

func TestListRecentReadFiles_Empty(t *testing.T) {
	t.Parallel()
	env := setupTest(t)

	files, err := env.svc.ListRecentReadFiles(env.ctx, 5*time.Minute)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestListRecentReadFiles_OldFilesExcluded(t *testing.T) {
	t.Parallel()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)
	svc := NewService(q)

	sessionID := "old-session"
	_, err = q.CreateSession(t.Context(), db.CreateSessionParams{
		ID:    sessionID,
		Title: "Test",
	})
	require.NoError(t, err)

	oldTimestamp := time.Now().Add(-2 * time.Hour).Unix()
	_, err = conn.ExecContext(t.Context(),
		"INSERT INTO read_files (session_id, path, read_at) VALUES (?, ?, ?)",
		sessionID, "/path/to/old.go", oldTimestamp,
	)
	require.NoError(t, err)

	files, err := svc.ListRecentReadFiles(t.Context(), 5*time.Minute)
	require.NoError(t, err)
	require.Empty(t, files, "files older than the since window should be excluded")
}

var _ *sql.DB
