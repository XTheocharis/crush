package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func setupManagerForRestore(t *testing.T) (Manager, *db.Queries, *sql.DB, context.Context) {
	t.Helper()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()
	return mgr, queries, sqlDB, ctx
}

func TestRestoreReplacement(t *testing.T) {
	t.Parallel()
	mgr, queries, _, ctx := setupManagerForRestore(t)

	sessionID := "sess-restore-single"
	createTestSession(t, queries, sessionID)
	insertContextItemForPosition(t, queries, sessionID, 5)

	crStore := newContentReplacementStore(queries, nil)
	id, err := crStore.RecordReplacement(ctx, ContentReplacement{
		SessionID:             sessionID,
		Position:              5,
		MessageID:             sql.NullString{String: "msg_1", Valid: true},
		State:                 ReplacementActive,
		Round:                 1,
		OriginalTokenCount:    500,
		ReplacementTokenCount: 50,
	})
	require.NoError(t, err)
	require.Positive(t, id)

	err = mgr.RestoreReplacement(ctx, id)
	require.NoError(t, err)

	results, err := crStore.ListByState(ctx, sessionID, ReplacementRestored)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, id, results[0].ID)
	require.Equal(t, ReplacementRestored, results[0].State)
}

func TestRestoreReplacementAlreadyRestored(t *testing.T) {
	t.Parallel()
	mgr, queries, _, ctx := setupManagerForRestore(t)

	sessionID := "sess-restore-already"
	createTestSession(t, queries, sessionID)
	insertContextItemForPosition(t, queries, sessionID, 3)

	crStore := newContentReplacementStore(queries, nil)
	id, err := crStore.RecordReplacement(ctx, ContentReplacement{
		SessionID:             sessionID,
		Position:              3,
		State:                 ReplacementActive,
		Round:                 1,
		OriginalTokenCount:    200,
		ReplacementTokenCount: 20,
	})
	require.NoError(t, err)

	err = mgr.RestoreReplacement(ctx, id)
	require.NoError(t, err)

	err = mgr.RestoreReplacement(ctx, id)
	require.ErrorIs(t, err, ErrNoActiveReplacement)
}

func TestRestoreAllByRound(t *testing.T) {
	t.Parallel()
	mgr, queries, _, ctx := setupManagerForRestore(t)

	sessionID := "sess-restore-round"
	createTestSession(t, queries, sessionID)

	crStore := newContentReplacementStore(queries, nil)
	for i := range 3 {
		insertContextItemForPosition(t, queries, sessionID, int64(i))
		_, err := crStore.RecordReplacement(ctx, ContentReplacement{
			SessionID:             sessionID,
			Position:              int64(i),
			State:                 ReplacementActive,
			Round:                 1,
			OriginalTokenCount:    100,
			ReplacementTokenCount: 10,
		})
		require.NoError(t, err)
	}

	err := mgr.RestoreAllByRound(ctx, sessionID, 1)
	require.NoError(t, err)

	restored, err := crStore.ListByState(ctx, sessionID, ReplacementRestored)
	require.NoError(t, err)
	require.Len(t, restored, 3)

	active, err := crStore.ListByState(ctx, sessionID, ReplacementActive)
	require.NoError(t, err)
	require.Empty(t, active)
}

func TestRestoreAllByRoundSkipsAlreadyRestored(t *testing.T) {
	t.Parallel()
	mgr, queries, _, ctx := setupManagerForRestore(t)

	sessionID := "sess-restore-round-skip"
	createTestSession(t, queries, sessionID)

	crStore := newContentReplacementStore(queries, nil)
	for i := range 3 {
		insertContextItemForPosition(t, queries, sessionID, int64(i))
		state := ReplacementActive
		if i == 1 {
			state = ReplacementRestored
		}
		_, err := crStore.RecordReplacement(ctx, ContentReplacement{
			SessionID:             sessionID,
			Position:              int64(i),
			State:                 state,
			Round:                 1,
			OriginalTokenCount:    100,
			ReplacementTokenCount: 10,
		})
		require.NoError(t, err)
	}

	err := mgr.RestoreAllByRound(ctx, sessionID, 1)
	require.NoError(t, err)

	restored, err := crStore.ListByState(ctx, sessionID, ReplacementRestored)
	require.NoError(t, err)
	require.Len(t, restored, 3)
}

func TestRestoreAllByRoundIgnoresOtherRounds(t *testing.T) {
	t.Parallel()
	mgr, queries, _, ctx := setupManagerForRestore(t)

	sessionID := "sess-restore-round-other"
	createTestSession(t, queries, sessionID)

	crStore := newContentReplacementStore(queries, nil)
	for i := range 4 {
		insertContextItemForPosition(t, queries, sessionID, int64(i))
		round := 1
		if i >= 2 {
			round = 2
		}
		_, err := crStore.RecordReplacement(ctx, ContentReplacement{
			SessionID:             sessionID,
			Position:              int64(i),
			State:                 ReplacementActive,
			Round:                 round,
			OriginalTokenCount:    100,
			ReplacementTokenCount: 10,
		})
		require.NoError(t, err)
	}

	err := mgr.RestoreAllByRound(ctx, sessionID, 1)
	require.NoError(t, err)

	round1, err := crStore.ListByRound(ctx, sessionID, 1)
	require.NoError(t, err)
	for _, r := range round1 {
		require.Equal(t, ReplacementRestored, r.State)
	}

	round2, err := crStore.ListByRound(ctx, sessionID, 2)
	require.NoError(t, err)
	for _, r := range round2 {
		require.Equal(t, ReplacementActive, r.State)
	}
}

func TestPinEntry(t *testing.T) {
	t.Parallel()
	mgr, queries, _, ctx := setupManagerForRestore(t)

	sessionID := "sess-pin"
	createTestSession(t, queries, sessionID)
	insertContextItemForPosition(t, queries, sessionID, 7)

	err := mgr.PinEntry(ctx, sessionID, 7)
	require.NoError(t, err)

	crStore := newContentReplacementStore(queries, nil)
	pinned, err := crStore.ListByState(ctx, sessionID, ReplacementPinned)
	require.NoError(t, err)
	require.Len(t, pinned, 1)
	require.Equal(t, int64(7), pinned[0].Position)
}

func TestCleanOrphanedReplacements(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-orphan"
	createTestSession(t, queries, sessionID)

	crStore := newContentReplacementStore(queries, sqlDB)
	for i := range 3 {
		insertContextItemForPosition(t, queries, sessionID, int64(i))
		_, err := crStore.RecordReplacement(ctx, ContentReplacement{
			SessionID:             sessionID,
			Position:              int64(i),
			State:                 ReplacementActive,
			Round:                 1,
			OriginalTokenCount:    100,
			ReplacementTokenCount: 10,
		})
		require.NoError(t, err)
	}

	// Disable FK checks so we can delete the context item without cascade.
	_, err := sqlDB.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM lcm_context_items WHERE session_id = '%s' AND position = 1", sessionID),
	)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	cleaned, err := mgr.CleanOrphanedReplacements(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, 1, cleaned)

	remaining, err := crStore.ListByState(ctx, sessionID, ReplacementActive)
	require.NoError(t, err)
	require.Len(t, remaining, 2)
}

func TestCleanOrphanedReplacementsNoOrphans(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-no-orphan"
	createTestSession(t, queries, sessionID)

	crStore := newContentReplacementStore(queries, sqlDB)
	for i := range 2 {
		insertContextItemForPosition(t, queries, sessionID, int64(i))
		_, err := crStore.RecordReplacement(ctx, ContentReplacement{
			SessionID:             sessionID,
			Position:              int64(i),
			State:                 ReplacementActive,
			Round:                 1,
			OriginalTokenCount:    100,
			ReplacementTokenCount: 10,
		})
		require.NoError(t, err)
	}

	cleaned, err := mgr.CleanOrphanedReplacements(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, 0, cleaned)
}
