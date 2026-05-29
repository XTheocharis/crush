package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func setupContentReplacementStore(t *testing.T) (*contentReplacementStore, *db.Queries, context.Context) {
	t.Helper()
	queries, sqlDB := setupTestDB(t)
	store := newContentReplacementStore(queries, sqlDB)
	ctx := context.Background()
	return store, queries, ctx
}

func insertContextItemForPosition(t *testing.T, queries *db.Queries, sessionID string, position int64) {
	t.Helper()
	msgID := fmt.Sprintf("msg-cr-pos-%d", position)
	createTestMessage(t, queries, sessionID, msgID, "user", "test content")
	err := queries.InsertLcmContextItem(context.Background(), db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   position,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 100,
	})
	require.NoError(t, err)
}

func TestContentReplacementStore_RecordReplacement(t *testing.T) {
	t.Parallel()
	store, queries, ctx := setupContentReplacementStore(t)

	sessionID := "sess-cr-record"
	createTestSession(t, queries, sessionID)
	insertContextItemForPosition(t, queries, sessionID, 5)

	id, err := store.RecordReplacement(ctx, ContentReplacement{
		SessionID:             sessionID,
		Position:              5,
		MessageID:             sql.NullString{String: "msg_123", Valid: true},
		FileID:                sql.NullString{String: "file_abc1234567890123", Valid: true},
		State:                 ReplacementActive,
		Round:                 1,
		OriginalTokenCount:    500,
		ReplacementTokenCount: 50,
	})
	require.NoError(t, err)
	require.Positive(t, id)
}

func TestContentReplacementStore_GetBySessionPosition(t *testing.T) {
	t.Parallel()
	store, queries, ctx := setupContentReplacementStore(t)

	sessionID := "sess-cr-pos"
	createTestSession(t, queries, sessionID)
	insertContextItemForPosition(t, queries, sessionID, 10)

	_, err := store.RecordReplacement(ctx, ContentReplacement{
		SessionID:             sessionID,
		Position:              10,
		State:                 ReplacementActive,
		Round:                 2,
		OriginalTokenCount:    300,
		ReplacementTokenCount: 30,
	})
	require.NoError(t, err)

	results, err := store.GetBySessionPosition(ctx, sessionID, 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, int64(10), results[0].Position)
	require.Equal(t, ReplacementActive, results[0].State)
	require.Equal(t, 2, results[0].Round)
	require.Equal(t, 300, results[0].OriginalTokenCount)
	require.Equal(t, 30, results[0].ReplacementTokenCount)
}

func TestContentReplacementStore_GetByFileID(t *testing.T) {
	t.Parallel()
	store, queries, ctx := setupContentReplacementStore(t)

	sessionID := "sess-cr-file"
	fileID := "file_aabbccdd11223344"
	createTestSession(t, queries, sessionID)
	insertContextItemForPosition(t, queries, sessionID, 1)

	_, err := store.RecordReplacement(ctx, ContentReplacement{
		SessionID:             sessionID,
		Position:              1,
		FileID:                sql.NullString{String: fileID, Valid: true},
		State:                 ReplacementActive,
		Round:                 0,
		OriginalTokenCount:    1000,
		ReplacementTokenCount: 100,
	})
	require.NoError(t, err)

	results, err := store.GetByFileID(ctx, sessionID, fileID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, fileID, results[0].FileID.String)
}

func TestContentReplacementStore_ListByState(t *testing.T) {
	t.Parallel()
	store, queries, ctx := setupContentReplacementStore(t)

	sessionID := "sess-cr-state"
	createTestSession(t, queries, sessionID)

	for i := range 3 {
		insertContextItemForPosition(t, queries, sessionID, int64(i))
		state := ReplacementActive
		if i == 2 {
			state = ReplacementRestored
		}
		_, err := store.RecordReplacement(ctx, ContentReplacement{
			SessionID:             sessionID,
			Position:              int64(i),
			State:                 state,
			Round:                 1,
			OriginalTokenCount:    100,
			ReplacementTokenCount: 10,
		})
		require.NoError(t, err)
	}

	active, err := store.ListByState(ctx, sessionID, ReplacementActive)
	require.NoError(t, err)
	require.Len(t, active, 2)

	restored, err := store.ListByState(ctx, sessionID, ReplacementRestored)
	require.NoError(t, err)
	require.Len(t, restored, 1)
}

func TestContentReplacementStore_UpdateState(t *testing.T) {
	t.Parallel()
	store, queries, ctx := setupContentReplacementStore(t)

	sessionID := "sess-cr-update"
	createTestSession(t, queries, sessionID)
	insertContextItemForPosition(t, queries, sessionID, 0)

	id, err := store.RecordReplacement(ctx, ContentReplacement{
		SessionID:             sessionID,
		Position:              0,
		State:                 ReplacementActive,
		Round:                 0,
		OriginalTokenCount:    200,
		ReplacementTokenCount: 20,
	})
	require.NoError(t, err)

	err = store.UpdateState(ctx, id, ReplacementRestored)
	require.NoError(t, err)

	results, err := store.GetBySessionPosition(ctx, sessionID, 0)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, ReplacementRestored, results[0].State)
}

func TestContentReplacementStore_UpdateStateInvalidTransition(t *testing.T) {
	t.Parallel()
	store, queries, ctx := setupContentReplacementStore(t)

	sessionID := "sess-cr-bad-trans"
	createTestSession(t, queries, sessionID)
	insertContextItemForPosition(t, queries, sessionID, 0)

	id, err := store.RecordReplacement(ctx, ContentReplacement{
		SessionID:             sessionID,
		Position:              0,
		State:                 ReplacementActive,
		Round:                 0,
		OriginalTokenCount:    100,
		ReplacementTokenCount: 10,
	})
	require.NoError(t, err)

	err = store.UpdateState(ctx, id, ReplacementActive)
	require.Error(t, err)
	var badTrans ErrInvalidStateTransition
	require.ErrorAs(t, err, &badTrans)
	require.Equal(t, ReplacementActive, badTrans.From)
	require.Equal(t, ReplacementActive, badTrans.To)

	results, err := store.GetBySessionPosition(ctx, sessionID, 0)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, ReplacementActive, results[0].State)
}

func TestContentReplacementStore_ListByRound(t *testing.T) {
	t.Parallel()
	store, queries, ctx := setupContentReplacementStore(t)

	sessionID := "sess-cr-round"
	createTestSession(t, queries, sessionID)

	for i := range 4 {
		insertContextItemForPosition(t, queries, sessionID, int64(i))
		round := 1
		if i >= 2 {
			round = 2
		}
		_, err := store.RecordReplacement(ctx, ContentReplacement{
			SessionID:             sessionID,
			Position:              int64(i),
			State:                 ReplacementActive,
			Round:                 round,
			OriginalTokenCount:    50,
			ReplacementTokenCount: 5,
		})
		require.NoError(t, err)
	}

	round1, err := store.ListByRound(ctx, sessionID, 1)
	require.NoError(t, err)
	require.Len(t, round1, 2)

	round2, err := store.ListByRound(ctx, sessionID, 2)
	require.NoError(t, err)
	require.Len(t, round2, 2)

	require.Equal(t, int64(0), round1[0].Position)
	require.Equal(t, int64(1), round1[1].Position)
	require.Equal(t, int64(2), round2[0].Position)
	require.Equal(t, int64(3), round2[1].Position)
}

func TestContentReplacementStore_RecordReplacementNonFatal(t *testing.T) {
	t.Parallel()
	store, queries, ctx := setupContentReplacementStore(t)

	sessionID := "sess-cr-nonfatal"
	createTestSession(t, queries, sessionID)
	// Deliberately NOT inserting a context item — FK violation.

	_, err := store.RecordReplacement(ctx, ContentReplacement{
		SessionID:             sessionID,
		Position:              99,
		State:                 ReplacementActive,
		Round:                 0,
		OriginalTokenCount:    100,
		ReplacementTokenCount: 10,
	})
	require.Error(t, err)
}

func TestContentReplacementStore_EmptyResults(t *testing.T) {
	t.Parallel()
	store, queries, ctx := setupContentReplacementStore(t)

	sessionID := "sess-cr-empty"
	createTestSession(t, queries, sessionID)

	results, err := store.GetBySessionPosition(ctx, sessionID, 0)
	require.NoError(t, err)
	require.Empty(t, results)

	results, err = store.ListByState(ctx, sessionID, ReplacementActive)
	require.NoError(t, err)
	require.Empty(t, results)

	results, err = store.ListByRound(ctx, sessionID, 0)
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestContentReplacementStore_UpdateStatePinned(t *testing.T) {
	t.Parallel()
	store, queries, ctx := setupContentReplacementStore(t)

	sessionID := "sess-cr-pin"
	createTestSession(t, queries, sessionID)
	insertContextItemForPosition(t, queries, sessionID, 7)

	id, err := store.RecordReplacement(ctx, ContentReplacement{
		SessionID:             sessionID,
		Position:              7,
		State:                 ReplacementActive,
		Round:                 1,
		OriginalTokenCount:    800,
		ReplacementTokenCount: 80,
	})
	require.NoError(t, err)

	err = store.UpdateState(ctx, id, ReplacementPinned)
	require.NoError(t, err)

	pinned, err := store.ListByState(ctx, sessionID, ReplacementPinned)
	require.NoError(t, err)
	require.Len(t, pinned, 1)
	require.Equal(t, int64(id), pinned[0].ID)

	err = store.UpdateState(ctx, id, ReplacementActive)
	require.NoError(t, err)

	active, err := store.ListByState(ctx, sessionID, ReplacementActive)
	require.NoError(t, err)
	require.Len(t, active, 1)
}

func TestContentReplacementStore_SupersededIsTerminal(t *testing.T) {
	t.Parallel()
	store, queries, ctx := setupContentReplacementStore(t)

	sessionID := "sess-cr-supersede"
	createTestSession(t, queries, sessionID)
	insertContextItemForPosition(t, queries, sessionID, 3)

	id, err := store.RecordReplacement(ctx, ContentReplacement{
		SessionID:             sessionID,
		Position:              3,
		State:                 ReplacementActive,
		Round:                 1,
		OriginalTokenCount:    400,
		ReplacementTokenCount: 40,
	})
	require.NoError(t, err)

	err = store.UpdateState(ctx, id, ReplacementSuperseded)
	require.NoError(t, err)

	for _, invalidTarget := range []ReplacementState{ReplacementActive, ReplacementRestored, ReplacementPinned} {
		err = store.UpdateState(ctx, id, invalidTarget)
		require.Error(t, err)
		var badTrans ErrInvalidStateTransition
		require.ErrorAs(t, err, &badTrans)
	}
}
