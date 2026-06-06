package rewind_test

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

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/rewind"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

var doomGooseOnce sync.Once

func doomTestOpenDB(t *testing.T) *sql.DB {
	t.Helper()
	doomGooseOnce.Do(func() {
		goose.SetBaseFS(db.FS)
		if err := goose.SetDialect("sqlite3"); err != nil {
			t.Fatalf("goose.SetDialect: %v", err)
		}
	})
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

type doomTestEnv struct {
	svc        rewind.Service
	sqlDB      *sql.DB
	q          *db.Queries
	sessions   session.Service
	workingDir string
}

func doomTestSetup(t *testing.T) *doomTestEnv {
	t.Helper()
	sqlDB := doomTestOpenDB(t)
	q := db.New(sqlDB)
	sessions := session.NewService(q, sqlDB)
	workingDir := t.TempDir()
	svc := rewind.NewService(q, sessions, workingDir)
	return &doomTestEnv{svc: svc, sqlDB: sqlDB, q: q, sessions: sessions, workingDir: workingDir}
}

func (e *doomTestEnv) createSession(t *testing.T, ctx context.Context, title string) string {
	t.Helper()
	sess, err := e.sessions.Create(ctx, title)
	require.NoError(t, err)
	return sess.ID
}

var doomMsgSeq atomic.Uint64

func (e *doomTestEnv) insertMessage(t *testing.T, ctx context.Context, sessionID, role string, parts string) string {
	t.Helper()
	n := doomMsgSeq.Add(1)
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

var doomFileSeq atomic.Uint64

func (e *doomTestEnv) insertFile(t *testing.T, ctx context.Context, sessionID, path, content string, version int) {
	t.Helper()
	n := doomFileSeq.Add(1)
	id := fmt.Sprintf("file-%d-%s-v%d", n, filepath.Base(path), version)
	_, err := e.sqlDB.ExecContext(ctx,
		`INSERT INTO files (id, session_id, path, content, version, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, strftime('%s','now'), strftime('%s','now'))`,
		id, sessionID, path, content, version,
	)
	require.NoError(t, err)
}

func doomTextParts(text string) string {
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

func makeDoomStep(toolName, input, output string) fantasy.StepResult {
	callID := fmt.Sprintf("call_%s_%s", toolName, input)
	var content fantasy.ResponseContent
	content = append(content, fantasy.ToolCallContent{
		ToolCallID: callID,
		ToolName:   toolName,
		Input:      input,
	})
	content = append(content, fantasy.ToolResultContent{
		ToolCallID: callID,
		ToolName:   toolName,
		Result:     fantasy.ToolResultOutputContentText{Text: output},
	})
	return fantasy.StepResult{Response: fantasy.Response{Content: content}}
}

// TestDoomInteraction_SnapshotIntegrityAfterDoom verifies that snapshot data
// remains intact after a doom-loop intervention.
func TestDoomInteraction_SnapshotIntegrityAfterDoom(t *testing.T) {
	t.Parallel()

	env := doomTestSetup(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "doom-snapshot-integrity")

	env.insertMessage(t, ctx, sid, "user", doomTextParts("create initial code"))
	env.insertFile(t, ctx, sid, "server.go", "package main\n\nfunc handle() {}", 1)
	require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, 0))

	snap, err := env.svc.GetSnapshotAtOrBeforeSeq(ctx, sid, 0)
	require.NoError(t, err)
	require.Equal(t, sid, snap.SessionID)

	files, err := env.svc.GetSnapshotFiles(ctx, snap.ID)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "server.go", files[0].Path)
	require.Equal(t, "package main\n\nfunc handle() {}", files[0].Content)

	detector := agent.NewDoomLoopDetector(agent.DefaultDoomLoopThresholds, 10)
	steps := make([]fantasy.StepResult, 10)
	for i := range 7 {
		steps[i] = makeDoomStep("edit",
			`{"file":"server.go","old":"handle","new":"process"}`,
			"error: string not found",
		)
	}
	for i := 7; i < 10; i++ {
		steps[i] = makeDoomStep("read",
			fmt.Sprintf(`{"file":"%d.go"}`, i),
			fmt.Sprintf("data-%d", i),
		)
	}

	result := detector.Detect(steps)
	require.Equal(t, agent.EscalationHard, result.Level)
	require.Equal(t, 7, result.RepeatCount)
	require.Equal(t, "edit", result.ToolName)
	require.Contains(t, result.Message, "HARD LOOP")

	snapAfter, err := env.svc.GetSnapshotAtOrBeforeSeq(ctx, sid, 0)
	require.NoError(t, err)
	require.Equal(t, snap.ID, snapAfter.ID)

	filesAfter, err := env.svc.GetSnapshotFiles(ctx, snapAfter.ID)
	require.NoError(t, err)
	require.Len(t, filesAfter, 1)
	require.Equal(t, "server.go", filesAfter[0].Path)
	require.Equal(t, "package main\n\nfunc handle() {}", filesAfter[0].Content)
}

// TestDoomInteraction_RewindAfterDoom verifies that rewind restores correct
// file state after a doom-loop intervention corrupts files on disk.
func TestDoomInteraction_RewindAfterDoom(t *testing.T) {
	t.Parallel()

	env := doomTestSetup(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "doom-rewind")

	env.insertMessage(t, ctx, sid, "user", doomTextParts("setup project"))
	env.insertFile(t, ctx, sid, "handler.go", "func original() {}", 1)
	env.insertFile(t, ctx, sid, "config.yaml", "key: original-value", 1)
	require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, 0))

	handlerPath := filepath.Join(env.workingDir, "handler.go")
	configPath := filepath.Join(env.workingDir, "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(handlerPath), 0o755))
	require.NoError(t, os.WriteFile(handlerPath, []byte("func original() {}"), 0o644))
	require.NoError(t, os.WriteFile(configPath, []byte("key: original-value"), 0o644))

	detector := agent.NewDoomLoopDetector(agent.DefaultDoomLoopThresholds, 10)

	require.NoError(t, os.WriteFile(handlerPath, []byte("func broken() {}"), 0o644))
	require.NoError(t, os.WriteFile(configPath, []byte("key: corrupted-value"), 0o644))

	steps := make([]fantasy.StepResult, 10)
	for i := range 7 {
		steps[i] = makeDoomStep("edit",
			`{"file":"handler.go","old":"original","new":"fixed"}`,
			"error: string not found",
		)
	}
	for i := 7; i < 10; i++ {
		steps[i] = makeDoomStep("read",
			fmt.Sprintf(`{"file":"file%d.go"}`, i),
			"data",
		)
	}
	doomResult := detector.Detect(steps)
	require.Equal(t, agent.EscalationHard, doomResult.Level)

	rewindResult, err := env.svc.Rewind(ctx, sid, 0, rewind.RewindCodeOnly)
	require.NoError(t, err)
	require.Equal(t, 2, rewindResult.FilesRestored)
	require.NotNil(t, rewindResult.Snapshot)

	handlerData, err := os.ReadFile(handlerPath)
	require.NoError(t, err)
	require.Equal(t, "func original() {}", string(handlerData))

	configData, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, "key: original-value", string(configData))

	var msgCount int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ?", sid,
	).Scan(&msgCount))
	require.Equal(t, 1, msgCount)
}

// TestDoomInteraction_CombinedRewindAfterDoom verifies that a combined rewind
// restores both file state and conversation after doom-loop detection.
func TestDoomInteraction_CombinedRewindAfterDoom(t *testing.T) {
	t.Parallel()

	env := doomTestSetup(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "doom-combined-rewind")

	env.insertMessage(t, ctx, sid, "user", doomTextParts("KEEP_THIS_MESSAGE"))
	env.insertMessage(t, ctx, sid, "assistant", doomTextParts("response-keep"))
	env.insertFile(t, ctx, sid, "main.go", "package main\n\nfunc init() {}", 1)
	require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, 0))

	env.insertMessage(t, ctx, sid, "user", doomTextParts("DOOM_RETRY_1"))
	env.insertMessage(t, ctx, sid, "assistant", doomTextParts("retry-response-1"))
	env.insertMessage(t, ctx, sid, "user", doomTextParts("DOOM_RETRY_2"))
	env.insertMessage(t, ctx, sid, "assistant", doomTextParts("retry-response-2"))

	var countBefore int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ?", sid,
	).Scan(&countBefore))
	require.Equal(t, 6, countBefore)

	detector := agent.NewDoomLoopDetector(agent.DefaultDoomLoopThresholds, 10)
	steps := make([]fantasy.StepResult, 10)
	for i := range 7 {
		steps[i] = makeDoomStep("edit",
			`{"file":"main.go","old":"init","new":"setup"}`,
			"error: string not found",
		)
	}
	for i := 7; i < 10; i++ {
		steps[i] = makeDoomStep("read",
			fmt.Sprintf(`{"file":"file%d.go"}`, i),
			"data",
		)
	}
	doomResult := detector.Detect(steps)
	require.Equal(t, agent.EscalationHard, doomResult.Level)

	mainPath := filepath.Join(env.workingDir, "main.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(mainPath), 0o755))
	require.NoError(t, os.WriteFile(mainPath, []byte("package main\n\nfunc broken() {}"), 0o644))

	result, err := env.svc.Rewind(ctx, sid, 0, rewind.RewindBoth)
	require.NoError(t, err)
	require.Equal(t, 1, result.FilesRestored)
	require.Greater(t, result.MessagesDeleted, 0)
	require.NotNil(t, result.Snapshot)

	data, err := os.ReadFile(mainPath)
	require.NoError(t, err)
	require.Equal(t, "package main\n\nfunc init() {}", string(data))

	var totalCount int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ?", sid,
	).Scan(&totalCount))
	require.Equal(t, 0, totalCount)
}

// TestDoomInteraction_SnapshotSurvivesMultipleDoomCycles verifies snapshot
// integrity is preserved across multiple doom detection cycles.
func TestDoomInteraction_SnapshotSurvivesMultipleDoomCycles(t *testing.T) {
	t.Parallel()

	env := doomTestSetup(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "doom-multi-cycle")

	env.insertMessage(t, ctx, sid, "user", doomTextParts("initial setup"))
	env.insertFile(t, ctx, sid, "app.go", "package app", 1)
	require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, 0))

	snap, err := env.svc.GetSnapshotAtOrBeforeSeq(ctx, sid, 0)
	require.NoError(t, err)
	originalSnapID := snap.ID

	detector := agent.NewDoomLoopDetector(agent.DefaultDoomLoopThresholds, 10)
	for cycle := range 3 {
		steps := make([]fantasy.StepResult, 10)
		for i := range 7 {
			steps[i] = makeDoomStep("edit",
				fmt.Sprintf(`{"file":"app.go","old":"v%d","new":"v%d"}`, cycle, cycle+1),
				"error: string not found",
			)
		}
		for i := 7; i < 10; i++ {
			steps[i] = makeDoomStep("read", `{"file":"other.go"}`, "data")
		}
		result := detector.Detect(steps)
		require.Equal(t, agent.EscalationHard, result.Level,
			"doom cycle %d must trigger hard escalation", cycle)
	}

	snapFinal, err := env.svc.GetSnapshotAtOrBeforeSeq(ctx, sid, 0)
	require.NoError(t, err)
	require.Equal(t, originalSnapID, snapFinal.ID)

	filesFinal, err := env.svc.GetSnapshotFiles(ctx, snapFinal.ID)
	require.NoError(t, err)
	require.Len(t, filesFinal, 1)
	require.Equal(t, "app.go", filesFinal[0].Path)
	require.Equal(t, "package app", filesFinal[0].Content)
}

// TestDoomInteraction_RewindAfterSoftEscalation verifies that rewind works
// correctly after a soft doom-loop escalation.
func TestDoomInteraction_RewindAfterSoftEscalation(t *testing.T) {
	t.Parallel()

	env := doomTestSetup(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "doom-soft-rewind")

	env.insertMessage(t, ctx, sid, "user", doomTextParts("build feature"))
	env.insertFile(t, ctx, sid, "feature.go", "func feature() {}", 1)
	require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, 0))

	detector := agent.NewDoomLoopDetector(agent.DefaultDoomLoopThresholds, 10)
	steps := make([]fantasy.StepResult, 10)
	for i := range 3 {
		steps[i] = makeDoomStep("edit",
			`{"file":"feature.go","old":"feature","new":"better"}`,
			"error: not found",
		)
	}
	for i := 3; i < 10; i++ {
		steps[i] = makeDoomStep("read",
			fmt.Sprintf(`{"file":"file%d.go"}`, i),
			"data",
		)
	}

	result := detector.Detect(steps)
	require.Equal(t, agent.EscalationSoft, result.Level)
	require.Equal(t, 3, result.RepeatCount)

	featurePath := filepath.Join(env.workingDir, "feature.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(featurePath), 0o755))
	require.NoError(t, os.WriteFile(featurePath, []byte("func modified() {}"), 0o644))

	rewindResult, err := env.svc.Rewind(ctx, sid, 0, rewind.RewindCodeOnly)
	require.NoError(t, err)
	require.Equal(t, 1, rewindResult.FilesRestored)

	data, err := os.ReadFile(featurePath)
	require.NoError(t, err)
	require.Equal(t, "func feature() {}", string(data))
}
