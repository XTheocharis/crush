package lcm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func TestTimeGapCompactor_ImplementsCompactionLayer(t *testing.T) {
	t.Parallel()
	var _ CompactionLayer = (*TimeGapCompactor)(nil)
}

func TestTimeGapCompactor_NameAndPriority(t *testing.T) {
	t.Parallel()
	c := NewTimeGapCompactor(TimeGapCompactorConfig{
		Store:     nil,
		SessionID: "s1",
	})
	require.Equal(t, "time-gap-compactor", c.Name())
	require.Equal(t, 15, c.Priority())
}

func TestTimeGapCompactor_DefaultGapThreshold(t *testing.T) {
	t.Parallel()
	c := NewTimeGapCompactor(TimeGapCompactorConfig{})
	require.Equal(t, 30*time.Second, c.gapThreshold)
}

func TestTimeGapCompactor_CustomGapThreshold(t *testing.T) {
	t.Parallel()
	c := NewTimeGapCompactor(TimeGapCompactorConfig{
		GapThreshold: 10 * time.Second,
	})
	require.Equal(t, 10*time.Second, c.gapThreshold)
}

func TestTimeGapCompactor_ShouldCompact_NilStore(t *testing.T) {
	t.Parallel()
	c := NewTimeGapCompactor(TimeGapCompactorConfig{SessionID: "s1"})
	require.False(t, c.ShouldCompact(context.Background(), Budget{}))
}

func TestTimeGapCompactor_ShouldCompact_EmptySessionID(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	c := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store})
	require.False(t, c.ShouldCompact(context.Background(), Budget{}))
}

func TestTimeGapCompactor_ShouldCompact_NoGap(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-tgap-no-gap"
	createTestSession(t, queries, sessionID)

	now := time.Now().Unix()

	msg1ID := "msg-no-gap-1"
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg1ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "user",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "content 1"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now, msg1ID)
	require.NoError(t, err)

	msg2ID := "msg-no-gap-2"
	_, err = queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg2ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "tool",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "content 2"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now+5, msg2ID)
	require.NoError(t, err)

	for i, id := range []string{msg1ID, msg2ID} {
		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: id, Valid: true},
			TokenCount: 500,
		})
		require.NoError(t, err)
	}

	c := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store, SessionID: sessionID})
	require.False(t, c.ShouldCompact(ctx, Budget{}))
}

func TestTimeGapCompactor_ShouldCompact_WithGap(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-tgap-with-gap"
	createTestSession(t, queries, sessionID)

	now := time.Now().Unix()

	msg1ID := "msg-gap-1"
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg1ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "tool",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "tool result 1"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now, msg1ID)
	require.NoError(t, err)

	msg2ID := "msg-gap-2"
	_, err = queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg2ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "user",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "after gap"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now+60, msg2ID)
	require.NoError(t, err)

	for i, id := range []string{msg1ID, msg2ID} {
		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: id, Valid: true},
			TokenCount: 500,
		})
		require.NoError(t, err)
	}

	c := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store, SessionID: sessionID})
	require.True(t, c.ShouldCompact(ctx, Budget{}))
}

func TestTimeGapCompactor_Compact_NilStore_ReturnsError(t *testing.T) {
	t.Parallel()
	c := NewTimeGapCompactor(TimeGapCompactorConfig{SessionID: "s1"})
	_, err := c.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrStoreIsNil))
}

func TestTimeGapCompactor_Compact_EmptySessionID_ReturnsError(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	c := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store})
	_, err := c.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSessionIDEmpty))
}

func TestTimeGapCompactor_Compact_NoGap_NoAction(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-tgap-noop"
	createTestSession(t, queries, sessionID)

	now := time.Now().Unix()
	msg1ID := "msg-noop-1"
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg1ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "user",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "close"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now, msg1ID)
	require.NoError(t, err)

	msg2ID := "msg-noop-2"
	_, err = queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg2ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "tool",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "close too"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now+5, msg2ID)
	require.NoError(t, err)

	for i, id := range []string{msg1ID, msg2ID} {
		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: id, Valid: true},
			TokenCount: 100,
		})
		require.NoError(t, err)
	}

	c := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store, SessionID: sessionID})
	result, err := c.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
	require.Equal(t, 0, result.ItemsAffected)
}

func TestTimeGapCompactor_Compact_WithGapToolResult(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-tgap-compact"
	createTestSession(t, queries, sessionID)

	now := time.Now().Unix()

	msg1ID := "msg-tool-before-gap"
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg1ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "tool",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "verbose tool output"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now, msg1ID)
	require.NoError(t, err)

	msg2ID := "msg-user-after-gap"
	_, err = queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg2ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "user",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "new request"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now+60, msg2ID)
	require.NoError(t, err)

	for i, id := range []string{msg1ID, msg2ID} {
		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: id, Valid: true},
			TokenCount: 800,
		})
		require.NoError(t, err)
	}

	c := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store, SessionID: sessionID})
	require.True(t, c.ShouldCompact(ctx, Budget{}))

	result, err := c.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
	require.Equal(t, int64(800), result.TokensFreed)
}

func TestTimeGapCompactor_Compact_SkipsNonToolMessages(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-tgap-non-tool"
	createTestSession(t, queries, sessionID)

	now := time.Now().Unix()

	msg1ID := "msg-user-before-gap"
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg1ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "user",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "user question"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now, msg1ID)
	require.NoError(t, err)

	msg2ID := "msg-after-gap"
	_, err = queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg2ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "assistant",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "after gap"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now+60, msg2ID)
	require.NoError(t, err)

	for i, id := range []string{msg1ID, msg2ID} {
		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: id, Valid: true},
			TokenCount: 300,
		})
		require.NoError(t, err)
	}

	c := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store, SessionID: sessionID})
	require.True(t, c.ShouldCompact(ctx, Budget{}))

	result, err := c.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
	require.Equal(t, 0, result.ItemsAffected)
}

func TestTimeGapCompactor_Compact_SkipsAlreadyReferenced(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-tgap-referenced"
	createTestSession(t, queries, sessionID)

	now := time.Now().Unix()

	msg1ID := "msg-referenced"
	referencedContent := "[Large File Stored: file_abc]\nLCM File ID: file_abc\n\nPreview:\nstuff"
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg1ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "tool",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, referencedContent),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now, msg1ID)
	require.NoError(t, err)

	msg2ID := "msg-after-gap"
	_, err = queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg2ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "user",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "new"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now+60, msg2ID)
	require.NoError(t, err)

	for i, id := range []string{msg1ID, msg2ID} {
		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: id, Valid: true},
			TokenCount: 1000,
		})
		require.NoError(t, err)
	}

	c := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store, SessionID: sessionID})
	result, err := c.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
	require.Equal(t, 0, result.ItemsAffected)
}

func TestTimeGapCompactor_Compact_MultipleGaps(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-tgap-multi"
	createTestSession(t, queries, sessionID)

	now := time.Now().Unix()

	toolMsg1ID := "msg-tool-1"
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          toolMsg1ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "tool",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "output 1"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now, toolMsg1ID)
	require.NoError(t, err)

	userMsg1ID := "msg-user-1"
	_, err = queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          userMsg1ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "user",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "prompt 1"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now+45, userMsg1ID)
	require.NoError(t, err)

	toolMsg2ID := "msg-tool-2"
	_, err = queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          toolMsg2ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "tool",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "output 2"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now+50, toolMsg2ID)
	require.NoError(t, err)

	userMsg2ID := "msg-user-2"
	_, err = queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          userMsg2ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "user",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "prompt 2"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now+120, userMsg2ID)
	require.NoError(t, err)

	msgs := []struct {
		id     string
		tokens int64
	}{
		{toolMsg1ID, 600},
		{userMsg1ID, 100},
		{toolMsg2ID, 600},
		{userMsg2ID, 100},
	}
	for i, m := range msgs {
		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: m.id, Valid: true},
			TokenCount: m.tokens,
		})
		require.NoError(t, err)
	}

	c := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store, SessionID: sessionID})
	result, err := c.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 2, result.ItemsAffected)
	require.Equal(t, int64(1200), result.TokensFreed)
}

func TestTimeGapCompactor_ShouldCompact_SingleMessage(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-tgap-single"
	createTestSession(t, queries, sessionID)

	msgID := "msg-only"
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msgID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "tool",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "only msg"),
	})
	require.NoError(t, err)

	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 500,
	})
	require.NoError(t, err)

	c := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store, SessionID: sessionID})
	require.False(t, c.ShouldCompact(ctx, Budget{}))
}

func TestTimeGapCompactor_Compact_WithLayerManager(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-tgap-lm"
	createTestSession(t, queries, sessionID)

	now := time.Now().Unix()

	msg1ID := "msg-lm-tool"
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg1ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "tool",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "tool output"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now, msg1ID)
	require.NoError(t, err)

	msg2ID := "msg-lm-after"
	_, err = queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          msg2ID,
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "user",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "after"),
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now+45, msg2ID)
	require.NoError(t, err)

	for i, id := range []string{msg1ID, msg2ID} {
		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: id, Valid: true},
			TokenCount: 700,
		})
		require.NoError(t, err)
	}

	tgLayer := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store, SessionID: sessionID})
	mgr := NewCompactionLayerManager(tgLayer)

	budget := Budget{SoftThreshold: 50000, HardLimit: 60000, ContextWindow: 128000}
	result, err := mgr.RunAll(ctx, budget)
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
}

func TestTimeGapCompactor_Compact_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		roles          []string
		timeOffsets    []int64
		tokenCounts    []int64
		expectAction   bool
		expectAffected int
		expectTokens   int64
	}{
		{
			name:           "no_gap_no_compact",
			roles:          []string{"tool", "user"},
			timeOffsets:    []int64{0, 5},
			tokenCounts:    []int64{500, 100},
			expectAction:   false,
			expectAffected: 0,
			expectTokens:   0,
		},
		{
			name:           "gap_tool_result_compacted",
			roles:          []string{"tool", "user"},
			timeOffsets:    []int64{0, 60},
			tokenCounts:    []int64{800, 100},
			expectAction:   true,
			expectAffected: 1,
			expectTokens:   800,
		},
		{
			name:           "gap_user_not_compacted",
			roles:          []string{"user", "assistant"},
			timeOffsets:    []int64{0, 60},
			tokenCounts:    []int64{300, 200},
			expectAction:   false,
			expectAffected: 0,
			expectTokens:   0,
		},
		{
			name:           "exact_threshold_no_compact",
			roles:          []string{"tool", "user"},
			timeOffsets:    []int64{0, 30},
			tokenCounts:    []int64{500, 100},
			expectAction:   false,
			expectAffected: 0,
			expectTokens:   0,
		},
		{
			name:           "just_over_threshold",
			roles:          []string{"tool", "user"},
			timeOffsets:    []int64{0, 31},
			tokenCounts:    []int64{500, 100},
			expectAction:   true,
			expectAffected: 1,
			expectTokens:   500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			queries, sqlDB := setupTestDB(t)
			store := newStore(queries, sqlDB)
			ctx := context.Background()

			sessionID := "sess-tgap-td-" + tt.name
			createTestSession(t, queries, sessionID)

			now := time.Now().Unix()

			for i, role := range tt.roles {
				msgID := fmt.Sprintf("msg-td-%d", i)
				_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
					ID:          msgID,
					SessionID:   sessionID,
					SessionID_2: sessionID,
					Role:        role,
					Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, fmt.Sprintf("content %d", i)),
				})
				require.NoError(t, err)
				_, err = sqlDB.ExecContext(ctx, "UPDATE messages SET created_at = ? WHERE id = ?", now+tt.timeOffsets[i], msgID)
				require.NoError(t, err)

				err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
					SessionID:  sessionID,
					Position:   int64(i),
					ItemType:   "message",
					MessageID:  sql.NullString{String: msgID, Valid: true},
					TokenCount: tt.tokenCounts[i],
				})
				require.NoError(t, err)
			}

			c := NewTimeGapCompactor(TimeGapCompactorConfig{Store: store, SessionID: sessionID})
			result, err := c.Compact(ctx, Budget{})
			require.NoError(t, err)
			require.Equal(t, tt.expectAction, result.ActionTaken, "ActionTaken mismatch")
			require.Equal(t, tt.expectAffected, result.ItemsAffected, "ItemsAffected mismatch")
			require.Equal(t, tt.expectTokens, result.TokensFreed, "TokensFreed mismatch")
		})
	}
}
