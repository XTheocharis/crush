package tools

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

var lcmToolsGooseOnce sync.Once

func initLcmToolsGoose() {
	lcmToolsGooseOnce.Do(func() {
		goose.SetBaseFS(db.FS)
		if err := goose.SetDialect("sqlite3"); err != nil {
			panic(fmt.Sprintf("goose.SetDialect: %v", err))
		}
	})
}

func setupLcmToolsTestDB(t *testing.T) (*db.Queries, *sql.DB) {
	t.Helper()
	initLcmToolsGoose()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=recursive_triggers(ON)", dbPath)

	sqlDB, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	err = sqlDB.PingContext(t.Context())
	require.NoError(t, err)

	err = goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	return db.New(sqlDB), sqlDB
}

func createSession(t *testing.T, q *db.Queries, sessionID, parentSessionID string) {
	t.Helper()

	parent := sql.NullString{}
	if parentSessionID != "" {
		parent = sql.NullString{String: parentSessionID, Valid: true}
	}

	_, err := q.CreateSession(t.Context(), db.CreateSessionParams{
		ID:              sessionID,
		ParentSessionID: parent,
		Title:           sessionID,
	})
	require.NoError(t, err)
}

func createMessage(t *testing.T, q *db.Queries, msgID, sessionID, role, text string) {
	t.Helper()

	parts := fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, text)
	_, err := q.CreateMessage(t.Context(), db.CreateMessageParams{
		ID:        msgID,
		SessionID: sessionID,
		Role:      role,
		Parts:     parts,
	})
	require.NoError(t, err)
}

func TestLcmDescribeFileSessionLineageScope(t *testing.T) {
	t.Parallel()

	q, sqlDB := setupLcmToolsTestDB(t)

	createSession(t, q, "sess_main", "")
	createSession(t, q, "sess_sub", "sess_main")
	createSession(t, q, "sess_other", "")

	require.NoError(t, q.InsertLcmLargeFile(t.Context(), db.InsertLcmLargeFileParams{
		FileID:       "file_self",
		SessionID:    "sess_sub",
		OriginalPath: "/tmp/self.txt",
		Content:      sql.NullString{String: "self-content", Valid: true},
		TokenCount:   10,
	}))
	require.NoError(t, q.InsertLcmLargeFile(t.Context(), db.InsertLcmLargeFileParams{
		FileID:       "file_ancestor",
		SessionID:    "sess_main",
		OriginalPath: "/tmp/ancestor.txt",
		Content:      sql.NullString{String: "ancestor-content", Valid: true},
		TokenCount:   20,
	}))
	require.NoError(t, q.InsertLcmLargeFile(t.Context(), db.InsertLcmLargeFileParams{
		FileID:       "file_unrelated",
		SessionID:    "sess_other",
		OriginalPath: "/tmp/other.txt",
		Content:      sql.NullString{String: "other-content", Valid: true},
		TokenCount:   30,
	}))

	tool := NewLcmDescribeTool(sqlDB)
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "sess_sub")

	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Input: `{"id":"file_self"}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "File ID: file_self")
	require.Contains(t, resp.Content, "Content preview:")

	resp, err = tool.Run(ctx, fantasy.ToolCall{ID: "2", Input: `{"id":"file_ancestor"}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "File ID: file_ancestor")

	resp, err = tool.Run(ctx, fantasy.ToolCall{ID: "3", Input: `{"id":"file_unrelated"}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "Access denied: file_unrelated is outside this session lineage")
}

func TestLcmDescribeSummarySessionLineageScope(t *testing.T) {
	t.Parallel()

	q, sqlDB := setupLcmToolsTestDB(t)

	createSession(t, q, "sess_main", "")
	createSession(t, q, "sess_sub", "sess_main")
	createSession(t, q, "sess_other", "")

	require.NoError(t, q.InsertLcmSummary(t.Context(), db.InsertLcmSummaryParams{
		SummaryID:  "sum_ancestor",
		SessionID:  "sess_main",
		Kind:       "leaf",
		Content:    "ancestor summary",
		TokenCount: 11,
		FileIds:    "[]",
	}))
	require.NoError(t, q.InsertLcmSummary(t.Context(), db.InsertLcmSummaryParams{
		SummaryID:  "sum_unrelated",
		SessionID:  "sess_other",
		Kind:       "leaf",
		Content:    "other summary",
		TokenCount: 22,
		FileIds:    "[]",
	}))

	tool := NewLcmDescribeTool(sqlDB)
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "sess_sub")

	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Input: `{"id":"sum_ancestor"}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "Summary ID: sum_ancestor")
	require.Contains(t, resp.Content, "Content:\nancestor summary")

	resp, err = tool.Run(ctx, fantasy.ToolCall{ID: "2", Input: `{"id":"sum_unrelated"}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "Access denied: sum_unrelated is outside this session lineage")
}

func TestLcmExpandScopeAndSubAgentGate(t *testing.T) {
	t.Parallel()

	q, sqlDB := setupLcmToolsTestDB(t)

	createSession(t, q, "sess_main", "")
	createSession(t, q, "sess_sub", "sess_main")
	createSession(t, q, "sess_other", "")

	createMessage(t, q, "msg_main_1", "sess_main", "user", "from main session")
	createMessage(t, q, "msg_other_1", "sess_other", "user", "from other session")

	require.NoError(t, q.InsertLcmSummary(t.Context(), db.InsertLcmSummaryParams{
		SummaryID:  "sum_main",
		SessionID:  "sess_main",
		Kind:       "leaf",
		Content:    "main summary",
		TokenCount: 15,
		FileIds:    "[]",
	}))
	require.NoError(t, q.InsertLcmSummaryMessage(t.Context(), db.InsertLcmSummaryMessageParams{
		SummaryID: "sum_main",
		MessageID: "msg_main_1",
		Ord:       0,
	}))

	require.NoError(t, q.InsertLcmSummary(t.Context(), db.InsertLcmSummaryParams{
		SummaryID:  "sum_other",
		SessionID:  "sess_other",
		Kind:       "leaf",
		Content:    "other summary",
		TokenCount: 16,
		FileIds:    "[]",
	}))
	require.NoError(t, q.InsertLcmSummaryMessage(t.Context(), db.InsertLcmSummaryMessageParams{
		SummaryID: "sum_other",
		MessageID: "msg_other_1",
		Ord:       0,
	}))

	tool := NewLcmExpandTool(sqlDB)

	// Main session is denied by sub-agent gate (Volt-strict behavior).
	mainCtx := context.WithValue(t.Context(), SessionIDContextKey, "sess_main")
	resp, err := tool.Run(mainCtx, fantasy.ToolCall{ID: "1", Input: `{"summary_id":"sum_main"}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, lcmExpandMainSessionDenied)

	// Sub-agent can expand ancestor session summary.
	subCtx := context.WithValue(t.Context(), SessionIDContextKey, "sess_sub")
	resp, err = tool.Run(subCtx, fantasy.ToolCall{ID: "2", Input: `{"summary_id":"sum_main"}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "Expanded 1 messages from summary sum_main:")
	require.Contains(t, resp.Content, "from main session")

	// Sub-agent cannot expand unrelated session summary.
	resp, err = tool.Run(subCtx, fantasy.ToolCall{ID: "3", Input: `{"summary_id":"sum_other"}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "Access denied: sum_other is outside this session lineage")
}

func TestLcmDescribeReadbackPersistedAndNonPersistedExploration(t *testing.T) {
	t.Parallel()

	q, sqlDB := setupLcmToolsTestDB(t)
	createSession(t, q, "sess_main", "")
	createSession(t, q, "sess_sub", "sess_main")

	require.NoError(t, q.InsertLcmLargeFile(t.Context(), db.InsertLcmLargeFileParams{
		FileID:             "file_persisted",
		SessionID:          "sess_main",
		OriginalPath:       "/tmp/persisted.txt",
		Content:            sql.NullString{String: "persisted-content", Valid: true},
		TokenCount:         10,
		ExplorationSummary: sql.NullString{String: "persisted summary", Valid: true},
		ExplorerUsed:       sql.NullString{String: "text", Valid: true},
	}))

	require.NoError(t, q.InsertLcmLargeFile(t.Context(), db.InsertLcmLargeFileParams{
		FileID:       "file_nonpersisted",
		SessionID:    "sess_main",
		OriginalPath: "/tmp/nonpersisted.txt",
		Content:      sql.NullString{String: "nonpersisted-content", Valid: true},
		TokenCount:   11,
	}))

	tool := NewLcmDescribeTool(sqlDB)
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "sess_sub")

	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "persisted", Input: `{"id":"file_persisted"}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "File ID: file_persisted")
	require.Contains(t, resp.Content, "Explorer: text")
	require.Contains(t, resp.Content, "Exploration summary:\npersisted summary")

	resp, err = tool.Run(ctx, fantasy.ToolCall{ID: "nonpersisted", Input: `{"id":"file_nonpersisted"}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "File ID: file_nonpersisted")
	require.NotContains(t, resp.Content, "Explorer:")
	require.NotContains(t, resp.Content, "Exploration summary:")
}
