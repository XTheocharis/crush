package rewind

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

var integrationGooseOnce sync.Once

func integrationTestInitGoose(t *testing.T) {
	t.Helper()
	integrationGooseOnce.Do(func() {
		goose.SetBaseFS(db.FS)
		if err := goose.SetDialect("sqlite3"); err != nil {
			t.Fatalf("goose.SetDialect: %v", err)
		}
	})
}

func integrationTestOpenDB(t *testing.T) *sql.DB {
	t.Helper()
	integrationTestInitGoose(t)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=recursive_triggers(ON)",
		dbPath,
	)
	sqlDB, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	require.NoError(t, sqlDB.PingContext(context.Background()))
	require.NoError(t, goose.Up(sqlDB, "migrations"))
	return sqlDB
}

type testEnv struct {
	svc        Service
	sqlDB      *sql.DB
	q          *db.Queries
	sessions   session.Service
	workingDir string
}

func integrationTestEnv(t *testing.T, maxSnapshots ...int) *testEnv {
	t.Helper()
	sqlDB := integrationTestOpenDB(t)
	q := db.New(sqlDB)
	sessions := session.NewService(q, sqlDB)
	workingDir := t.TempDir()

	var opts []SnapshotterOption
	if len(maxSnapshots) > 0 {
		opts = append(opts, WithMaxPerSession(maxSnapshots[0]))
	}
	svc := NewService(q, sessions, workingDir, opts...)

	return &testEnv{svc: svc, sqlDB: sqlDB, q: q, sessions: sessions, workingDir: workingDir}
}

func (e *testEnv) createSession(t *testing.T, ctx context.Context, title string) string {
	t.Helper()
	sess, err := e.sessions.Create(ctx, title)
	require.NoError(t, err)
	return sess.ID
}

var integrationMsgSeq uint64

func (e *testEnv) insertMessage(t *testing.T, ctx context.Context, sessionID, role, parts string) string {
	t.Helper()
	n := atomic.AddUint64(&integrationMsgSeq, 1)
	msgID := fmt.Sprintf("msg-%d-%s-%d", os.Getpid(), role, n)
	var id string
	err := e.sqlDB.QueryRowContext(ctx,
		`INSERT INTO messages (id, session_id, role, parts, seq, created_at, updated_at)
		 VALUES (?, ?, ?, ?,
		         (SELECT COALESCE(MAX(seq)+1,0) FROM messages m WHERE m.session_id=?),
		         strftime('%s','now'), strftime('%s','now'))
		 RETURNING id`,
		msgID, sessionID, role, parts, sessionID,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

var integrationFileSeq uint64

func (e *testEnv) insertFile(t *testing.T, ctx context.Context, sessionID, path, content string, version int) {
	t.Helper()
	n := atomic.AddUint64(&integrationFileSeq, 1)
	id := fmt.Sprintf("file-%d-%s-v%d", n, filepath.Base(path), version)
	_, err := e.sqlDB.ExecContext(ctx,
		`INSERT INTO files (id, session_id, path, content, version, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, strftime('%s','now'), strftime('%s','now'))`,
		id, sessionID, path, content, version,
	)
	require.NoError(t, err)
}

func textParts(text string) string {
	type pw struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	type td struct {
		Text string `json:"text"`
	}
	data, _ := json.Marshal(td{Text: text})
	parts, _ := json.Marshal([]pw{{Type: "text", Data: data}})
	return string(parts)
}

func TestIntegration_SnapshotLifecycle(t *testing.T) {
	t.Parallel()
	env := integrationTestEnv(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "snapshot-lifecycle")

	env.insertMessage(t, ctx, sid, "user", textParts("hello"))
	env.insertFile(t, ctx, sid, "main.go", "package main", 1)
	env.insertFile(t, ctx, sid, "util.go", "package util", 1)

	require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, 0))

	snap, err := env.svc.GetSnapshotAtOrBeforeSeq(ctx, sid, 0)
	require.NoError(t, err)
	require.Equal(t, sid, snap.SessionID)
	require.Equal(t, 0, snap.UserMessageSeq)
	require.NotEmpty(t, snap.ID)

	files, err := env.svc.GetSnapshotFiles(ctx, snap.ID)
	require.NoError(t, err)
	require.Len(t, files, 2)

	fm := map[string]SnapshotFile{}
	for _, f := range files {
		fm[f.Path] = f
	}
	require.Equal(t, "package main", fm["main.go"].Content)
	require.Equal(t, "package util", fm["util.go"].Content)
}

func TestIntegration_RewindCodeOnly(t *testing.T) {
	t.Parallel()
	env := integrationTestEnv(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "rewind-code")

	env.insertMessage(t, ctx, sid, "user", textParts("write code"))
	env.insertFile(t, ctx, sid, "main.go", "package main\n\nfunc main() {}", 1)

	require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, 0))

	mainPath := filepath.Join(env.workingDir, "main.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(mainPath), 0o755))
	require.NoError(t, os.WriteFile(mainPath, []byte("package main\n\nfunc modified() {}"), 0o644))

	result, err := env.svc.Rewind(ctx, sid, 0, RewindCodeOnly)
	require.NoError(t, err)
	require.Equal(t, 1, result.FilesRestored)
	require.NotNil(t, result.Snapshot)

	data, err := os.ReadFile(mainPath)
	require.NoError(t, err)
	require.Equal(t, "package main\n\nfunc main() {}", string(data))
}

func TestIntegration_RewindConvoOnly(t *testing.T) {
	t.Parallel()
	env := integrationTestEnv(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "rewind-convo")

	env.insertMessage(t, ctx, sid, "user", textParts("first"))
	env.insertMessage(t, ctx, sid, "assistant", textParts("response"))
	env.insertMessage(t, ctx, sid, "user", textParts("second"))

	require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, 0))

	result, err := env.svc.Rewind(ctx, sid, 0, RewindConvoOnly)
	require.NoError(t, err)
	require.Equal(t, 3, result.MessagesDeleted)
	require.Equal(t, "first", result.ExtractedText)

	var count int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ?", sid,
	).Scan(&count))
	require.Equal(t, 0, count)
}

func TestIntegration_RewindBoth(t *testing.T) {
	t.Parallel()
	env := integrationTestEnv(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "rewind-both")

	env.insertMessage(t, ctx, sid, "user", textParts("write"))
	env.insertMessage(t, ctx, sid, "assistant", textParts("done"))
	env.insertMessage(t, ctx, sid, "user", textParts("more"))
	env.insertFile(t, ctx, sid, "app.go", "original content", 1)

	require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, 0))

	appPath := filepath.Join(env.workingDir, "app.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(appPath), 0o755))
	require.NoError(t, os.WriteFile(appPath, []byte("modified content"), 0o644))

	result, err := env.svc.Rewind(ctx, sid, 0, RewindBoth)
	require.NoError(t, err)
	require.Equal(t, 1, result.FilesRestored)
	require.Equal(t, 3, result.MessagesDeleted)
	require.Equal(t, "write", result.ExtractedText)

	data, err := os.ReadFile(appPath)
	require.NoError(t, err)
	require.Equal(t, "original content", string(data))

	var count int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ?", sid,
	).Scan(&count))
	require.Equal(t, 0, count)
}

func TestIntegration_Fork(t *testing.T) {
	t.Parallel()
	env := integrationTestEnv(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "original-session")

	env.insertMessage(t, ctx, sid, "user", textParts("hello"))
	env.insertFile(t, ctx, sid, "code.go", "package code", 1)

	result, err := env.svc.Fork(ctx, sid, 0)
	require.NoError(t, err)
	require.NotEmpty(t, result.NewSessionID)
	require.Contains(t, result.NewSessionTitle, "(fork)")

	var forkFileCount int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM files WHERE session_id = ?", result.NewSessionID,
	).Scan(&forkFileCount))
	require.Equal(t, 1, forkFileCount)

	var origMsgCount int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ?", sid,
	).Scan(&origMsgCount))
	require.Equal(t, 1, origMsgCount)
}

func TestIntegration_EditMessage(t *testing.T) {
	t.Parallel()
	env := integrationTestEnv(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "edit-msg")

	env.insertMessage(t, ctx, sid, "user", textParts("original prompt"))
	env.insertMessage(t, ctx, sid, "assistant", textParts("response"))
	env.insertMessage(t, ctx, sid, "user", textParts("follow-up"))

	result, err := env.svc.EditMessage(ctx, sid, 2)
	require.NoError(t, err)
	require.Equal(t, "follow-up", result.ExtractedText)
	require.Equal(t, 1, result.MessagesDeleted)
	require.NotEmpty(t, result.NewMessageID)

	var count int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ?", sid,
	).Scan(&count))
	require.Equal(t, 2, count)
}

func TestIntegration_CleanupRetention(t *testing.T) {
	t.Parallel()
	env := integrationTestEnv(t, 3)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "cleanup")

	for i := 0; i < 5; i++ {
		env.insertMessage(t, ctx, sid, "user", textParts(fmt.Sprintf("msg %d", i)))
		env.insertFile(t, ctx, sid, "file.txt", fmt.Sprintf("content v%d", i), i+1)
		require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, i))
	}

	var snapCount int64
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM turn_snapshots WHERE session_id = ?", sid,
	).Scan(&snapCount))
	require.Equal(t, int64(5), snapCount)

	require.NoError(t, env.svc.CleanupOldSnapshots(ctx, sid))

	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM turn_snapshots WHERE session_id = ?", sid,
	).Scan(&snapCount))
	require.Equal(t, int64(3), snapCount)

	var minSeq int64
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT MIN(user_message_seq) FROM turn_snapshots WHERE session_id = ?", sid,
	).Scan(&minSeq))
	require.Equal(t, int64(2), minSeq)
}
