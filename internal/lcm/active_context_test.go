package lcm

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func TestGetActiveContext_AllEntries(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_active_ctx"
	createTestSession(t, q, sessionID)

	msgID1 := "msg_ac_1"
	msgID2 := "msg_ac_2"
	createTestMessage(t, q, sessionID, msgID1, "user", "hello")
	createTestMessage(t, q, sessionID, msgID2, "assistant", "world")

	require.NoError(t, q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID1, Valid: true},
		TokenCount: 100,
	}))
	require.NoError(t, q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   1,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID2, Valid: true},
		TokenCount: 200,
	}))

	ac, err := store.GetActiveContext(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, sessionID, ac.SessionID)
	require.Equal(t, 2, ac.EntryCount)
	require.Equal(t, int64(300), ac.TotalTokens)
	require.Len(t, ac.Entries, 2)
	require.Equal(t, int64(0), ac.Entries[0].Position)
	require.Equal(t, "message", ac.Entries[0].ItemType)
	require.Equal(t, int64(100), ac.Entries[0].TokenCount)
	require.Equal(t, msgID1, ac.Entries[0].MessageID)
}

func TestGetActiveContext_WithSummary(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_ac_summary"
	createTestSession(t, q, sessionID)

	msgID := "msg_ac_s1"
	createTestMessage(t, q, sessionID, msgID, "user", "hello")

	summaryID := "sum_ac_1"
	require.NoError(t, q.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID: summaryID, SessionID: sessionID, Kind: KindLeaf, Content: "summary text", TokenCount: 50, FileIds: "[]",
	}))
	require.NoError(t, q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID: sessionID, Position: 0, ItemType: "summary",
		SummaryID:  sql.NullString{String: summaryID, Valid: true},
		TokenCount: 50,
	}))
	require.NoError(t, q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID: sessionID, Position: 1, ItemType: "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 100,
	}))

	ac, err := store.GetActiveContext(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, 2, ac.EntryCount)
	require.Equal(t, int64(150), ac.TotalTokens)
	require.Equal(t, "summary", ac.Entries[0].ItemType)
	require.Equal(t, summaryID, ac.Entries[0].SummaryID)
	require.Equal(t, "summary text", ac.Entries[0].SummaryContent)
	require.Equal(t, KindLeaf, ac.Entries[0].SummaryKind)
}

func TestGetActiveContext_EmptySession(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_ac_empty"
	createTestSession(t, q, sessionID)

	ac, err := store.GetActiveContext(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, sessionID, ac.SessionID)
	require.Equal(t, 0, ac.EntryCount)
	require.Equal(t, int64(0), ac.TotalTokens)
	require.Empty(t, ac.Entries)
}

func TestGetActiveContextFiltered_FilterByType(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_ac_filter"
	createTestSession(t, q, sessionID)

	msgID := "msg_ac_f1"
	createTestMessage(t, q, sessionID, msgID, "user", "hello")

	summaryID := "sum_ac_f1"
	require.NoError(t, q.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID: summaryID, SessionID: sessionID, Kind: KindLeaf, Content: "summary text", TokenCount: 50, FileIds: "[]",
	}))
	require.NoError(t, q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID: sessionID, Position: 0, ItemType: "summary",
		SummaryID:  sql.NullString{String: summaryID, Valid: true},
		TokenCount: 50,
	}))
	require.NoError(t, q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID: sessionID, Position: 1, ItemType: "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 200,
	}))

	msgType := "message"
	ac, err := store.GetActiveContextFiltered(ctx, sessionID, ContextFilter{Type: &msgType})
	require.NoError(t, err)
	require.Equal(t, 1, ac.EntryCount)
	require.Equal(t, int64(200), ac.TotalTokens)
	require.Equal(t, "message", ac.Entries[0].ItemType)
	require.Equal(t, msgID, ac.Entries[0].MessageID)

	sumType := "summary"
	ac, err = store.GetActiveContextFiltered(ctx, sessionID, ContextFilter{Type: &sumType})
	require.NoError(t, err)
	require.Equal(t, 1, ac.EntryCount)
	require.Equal(t, int64(50), ac.TotalTokens)
	require.Equal(t, "summary", ac.Entries[0].ItemType)
}

func TestGetActiveContextFiltered_MinTokens(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_ac_mintok"
	createTestSession(t, q, sessionID)

	msgID1 := "msg_ac_mt1"
	msgID2 := "msg_ac_mt2"
	createTestMessage(t, q, sessionID, msgID1, "user", "hello")
	createTestMessage(t, q, sessionID, msgID2, "assistant", "world")

	require.NoError(t, q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID: sessionID, Position: 0, ItemType: "message",
		MessageID:  sql.NullString{String: msgID1, Valid: true},
		TokenCount: 50,
	}))
	require.NoError(t, q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID: sessionID, Position: 1, ItemType: "message",
		MessageID:  sql.NullString{String: msgID2, Valid: true},
		TokenCount: 200,
	}))

	minTokens := 100
	ac, err := store.GetActiveContextFiltered(ctx, sessionID, ContextFilter{MinTokens: &minTokens})
	require.NoError(t, err)
	require.Equal(t, 1, ac.EntryCount)
	require.Equal(t, int64(200), ac.TotalTokens)
	require.Equal(t, msgID2, ac.Entries[0].MessageID)
}

func TestActiveContextTool_MissingSessionID(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)

	tool := newActiveContextTool(store)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{ID: "1", Name: "lcm_active_context", Input: `{}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "session_id is required")
}

func TestActiveContextTool_ToolOutput(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_ac_tool"
	createTestSession(t, q, sessionID)

	msgID := "msg_ac_tool"
	createTestMessage(t, q, sessionID, msgID, "user", "hello")
	require.NoError(t, q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID: sessionID, Position: 0, ItemType: "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 150,
	}))

	tool := newActiveContextTool(store)
	input, _ := json.Marshal(activeContextParams{SessionID: sessionID})
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_active_context", Input: string(input)})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Active Context for session sess_ac_tool")
	require.Contains(t, resp.Content, "Entries: 1 | Total tokens: 150")
	require.Contains(t, resp.Content, "message_id="+msgID)
}

func TestActiveContextTool_FilterByType(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_ac_tool_filter"
	createTestSession(t, q, sessionID)

	msgID := "msg_ac_tf1"
	createTestMessage(t, q, sessionID, msgID, "user", "hello")

	summaryID := "sum_ac_tf1"
	require.NoError(t, q.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID: summaryID, SessionID: sessionID, Kind: KindLeaf, Content: "s", TokenCount: 50, FileIds: "[]",
	}))
	require.NoError(t, q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID: sessionID, Position: 0, ItemType: "summary",
		SummaryID:  sql.NullString{String: summaryID, Valid: true},
		TokenCount: 50,
	}))
	require.NoError(t, q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID: sessionID, Position: 1, ItemType: "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 200,
	}))

	tool := newActiveContextTool(store)
	input, _ := json.Marshal(activeContextParams{SessionID: sessionID, FilterType: "message"})
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_active_context", Input: string(input)})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Entries: 1 | Total tokens: 200")
	require.NotContains(t, resp.Content, "summary_id="+summaryID)
}

func TestActiveContextTool_EmptySession(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_ac_tool_empty"
	createTestSession(t, q, sessionID)

	tool := newActiveContextTool(store)
	input, _ := json.Marshal(activeContextParams{SessionID: sessionID})
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_active_context", Input: string(input)})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Entries: 0 | Total tokens: 0")
}
