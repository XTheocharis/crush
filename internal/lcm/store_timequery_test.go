package lcm

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"charm.land/fantasy"

	"github.com/stretchr/testify/require"
)

func TestQueryByTime_RangeFiltering(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_time_range"
	createTestSession(t, q, sessionID)

	baseTime := int64(1000)
	for i := 0; i < 5; i++ {
		msgID := fmt.Sprintf("msg_%d", i)
		createTestMessage(t, q, sessionID, msgID, "user", fmt.Sprintf("message %d", i))
		_, err := rawDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", baseTime+int64(i*100), msgID)
		require.NoError(t, err)
	}

	msgs, err := store.QueryByTime(ctx, sessionID, 1100, 1300)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	require.Equal(t, "msg_1", msgs[0].ID)
	require.Equal(t, "msg_2", msgs[1].ID)
	require.Equal(t, "msg_3", msgs[2].ID)
}

func TestQueryByTime_BoundaryInclusivity(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_time_boundary"
	createTestSession(t, q, sessionID)

	for i, ts := range []int64{500, 1000, 1500} {
		msgID := fmt.Sprintf("msg_boundary_%d", i)
		createTestMessage(t, q, sessionID, msgID, "user", fmt.Sprintf("at %d", ts))
		_, err := rawDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", ts, msgID)
		require.NoError(t, err)
	}

	msgs, err := store.QueryByTime(ctx, sessionID, 500, 1500)
	require.NoError(t, err)
	require.Len(t, msgs, 3)

	msgs, err = store.QueryByTime(ctx, sessionID, 500, 500)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, int64(500), msgs[0].CreatedAt)

	msgs, err = store.QueryByTime(ctx, sessionID, 1500, 1500)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, int64(1500), msgs[0].CreatedAt)
}

func TestQueryByTime_EmptyRange(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_time_empty"
	createTestSession(t, q, sessionID)

	for i, ts := range []int64{100, 200, 300} {
		msgID := fmt.Sprintf("msg_empty_%d", i)
		createTestMessage(t, q, sessionID, msgID, "user", "data")
		_, err := rawDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", ts, msgID)
		require.NoError(t, err)
	}

	msgs, err := store.QueryByTime(ctx, sessionID, 500, 600)
	require.NoError(t, err)
	require.Len(t, msgs, 0)
}

func TestQueryByTime_FullRange(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_time_full"
	createTestSession(t, q, sessionID)

	timestamps := []int64{100, 200, 300, 400, 500}
	for i, ts := range timestamps {
		msgID := fmt.Sprintf("msg_full_%d", i)
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		createTestMessage(t, q, sessionID, msgID, role, fmt.Sprintf("content %d", i))
		_, err := rawDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", ts, msgID)
		require.NoError(t, err)
	}

	msgs, err := store.QueryByTime(ctx, sessionID, 0, 1000)
	require.NoError(t, err)
	require.Len(t, msgs, 5)

	for i, m := range msgs {
		require.Equal(t, timestamps[i], m.CreatedAt, "messages should be ordered by created_at")
		require.Equal(t, fmt.Sprintf("msg_full_%d", i), m.ID)
	}
}

func TestQueryByTime_DifferentSessions(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessA := "sess_time_a"
	sessB := "sess_time_b"
	createTestSession(t, q, sessA)
	createTestSession(t, q, sessB)

	createTestMessage(t, q, sessA, "msg_a1", "user", "from A")
	_, err := rawDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", int64(100), "msg_a1")
	require.NoError(t, err)

	createTestMessage(t, q, sessB, "msg_b1", "user", "from B")
	_, err = rawDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", int64(200), "msg_b1")
	require.NoError(t, err)

	msgs, err := store.QueryByTime(ctx, sessA, 0, 300)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, "msg_a1", msgs[0].ID)
}

func TestQueryByTime_ExtractsContent(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_time_content"
	createTestSession(t, q, sessionID)

	createTestMessage(t, q, sessionID, "msg_content", "assistant", "hello world")
	_, err := rawDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", int64(42), "msg_content")
	require.NoError(t, err)

	msgs, err := store.QueryByTime(ctx, sessionID, 0, 100)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, "hello world", msgs[0].Content)
	require.Equal(t, "assistant", msgs[0].Role)
}

func TestTimeQueryTool_Creation(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)

	tool := newTimeQueryTool(store)
	info := tool.Info()
	require.Equal(t, "lcm_time_query", info.Name)
	require.NotEmpty(t, info.Description)
}

func TestTimeQueryTool_MissingParams(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	tool := newTimeQueryTool(store)

	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_time_query", Input: `{}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "session_id is required")

	resp, err = tool.Run(ctx, fantasy.ToolCall{ID: "2", Name: "lcm_time_query", Input: `{"session_id":"s1","start_time":0,"end_time":0}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "start_time and end_time are required")
}

func TestTimeQueryTool_WithData(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_tool_time"
	createTestSession(t, q, sessionID)

	createTestMessage(t, q, sessionID, "msg_t1", "user", "first")
	_, err := rawDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", int64(100), "msg_t1")
	require.NoError(t, err)

	createTestMessage(t, q, sessionID, "msg_t2", "assistant", "second")
	_, err = rawDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", int64(200), "msg_t2")
	require.NoError(t, err)

	tool := newTimeQueryTool(store)
	input, _ := json.Marshal(timeQueryParams{SessionID: sessionID, StartTime: 50, EndTime: 250})
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_time_query", Input: string(input)})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Found 2 messages")
	require.Contains(t, resp.Content, "user")
	require.Contains(t, resp.Content, "assistant")
}

func TestTimeQueryTool_NoResults(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	sessionID := "sess_tool_empty"
	createTestSession(t, q, sessionID)

	tool := newTimeQueryTool(store)
	input, _ := json.Marshal(timeQueryParams{SessionID: sessionID, StartTime: 0, EndTime: 1000})
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_time_query", Input: string(input)})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "No messages found")
}
