package lcm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

// setupReversibleTest creates a store, compactor, session, and a leaf summary
// with linked messages. Returns the store, compactor, context, session ID,
// summary ID, and block ID.
func setupReversibleTest(t *testing.T) (*Store, *ReversibleCompactor, context.Context, string, string, string) {
	t.Helper()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	rc := NewReversibleCompactor(store)
	ctx := context.Background()

	sessionID := "sess-reversible"
	createTestSession(t, queries, sessionID)

	msgIDs := []string{"rmsg-0", "rmsg-1", "rmsg-2"}
	for i, id := range msgIDs {
		createTestMessage(t, queries, sessionID, id, "user", fmt.Sprintf("Original message %d", i))
	}

	summaryID := "sum_reversible_test_01"
	blockID := "blk-reversible-01"
	err := store.InsertLcmSummaryWithBlock(ctx, summaryID, sessionID, KindLeaf, "Compressed summary", 10, nil, blockID, "")
	require.NoError(t, err)

	for i, id := range msgIDs {
		err = queries.InsertLcmSummaryMessage(ctx, db.InsertLcmSummaryMessageParams{
			SummaryID: summaryID,
			MessageID: id,
			Ord:       int64(i),
		})
		require.NoError(t, err)
	}

	return store, rc, ctx, sessionID, summaryID, blockID
}

func TestReversibleCompactor_SaveAndDecompress_RoundTrip(t *testing.T) {
	t.Parallel()
	_, rc, ctx, sessionID, summaryID, blockID := setupReversibleTest(t)

	originals := []MessageForSummary{
		{ID: "rmsg-0", SessionID: "sess-reversible", Seq: 0, Role: "user", Content: "Original message 0"},
		{ID: "rmsg-1", SessionID: "sess-reversible", Seq: 1, Role: "user", Content: "Original message 1"},
		{ID: "rmsg-2", SessionID: "sess-reversible", Seq: 2, Role: "user", Content: "Original message 2"},
	}

	// Delete the summary created by setup so SaveReversibleState can INSERT.
	_, err := rc.store.rawDB.ExecContext(ctx, `DELETE FROM lcm_summaries WHERE summary_id = ?`, summaryID)
	require.NoError(t, err)

	err = rc.SaveReversibleState(ctx, summaryID, sessionID, KindLeaf, "Compressed summary", 10, originals, blockID)
	require.NoError(t, err)

	reversible, err := rc.IsReversible(ctx, blockID)
	require.NoError(t, err)
	require.True(t, reversible)

	msgs, err := rc.Decompress(ctx, DecompressCommand{
		SummaryID:    summaryID,
		TargetDetail: TargetFull,
	})
	require.NoError(t, err)
	require.Len(t, msgs, 3)

	for i, m := range msgs {
		require.Equal(t, originals[i].ID, m.ID)
		require.Equal(t, originals[i].SessionID, m.SessionID)
		require.Equal(t, originals[i].Seq, m.Seq)
		require.Equal(t, originals[i].Role, m.Role)
		require.Equal(t, originals[i].Content, m.Content)
	}
}

func TestReversibleCompactor_PartialDetail(t *testing.T) {
	t.Parallel()
	_, rc, ctx, sessionID, summaryID, blockID := setupReversibleTest(t)

	longContent := strings.Repeat("x", 300)
	originals := []MessageForSummary{
		{ID: "rmsg-0", SessionID: "sess-reversible", Seq: 0, Role: "user", Content: longContent},
	}

	_, err := rc.store.rawDB.ExecContext(ctx, `DELETE FROM lcm_summaries WHERE summary_id = ?`, summaryID)
	require.NoError(t, err)

	err = rc.SaveReversibleState(ctx, summaryID, sessionID, KindLeaf, "Compressed summary", 10, originals, blockID)
	require.NoError(t, err)

	msgs, err := rc.Decompress(ctx, DecompressCommand{
		SummaryID:    summaryID,
		TargetDetail: TargetPartial,
	})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, 200, len(msgs[0].Content))
	require.Equal(t, longContent[:200], msgs[0].Content)
	require.Equal(t, "rmsg-0", msgs[0].ID)
}

func TestReversibleCompactor_MetadataDetail(t *testing.T) {
	t.Parallel()
	_, rc, ctx, sessionID, summaryID, blockID := setupReversibleTest(t)

	originals := []MessageForSummary{
		{ID: "rmsg-0", SessionID: "sess-reversible", Seq: 0, Role: "user", Content: "Should be stripped"},
		{ID: "rmsg-1", SessionID: "sess-reversible", Seq: 1, Role: "assistant", Content: "Also stripped"},
	}

	_, err := rc.store.rawDB.ExecContext(ctx, `DELETE FROM lcm_summaries WHERE summary_id = ?`, summaryID)
	require.NoError(t, err)

	err = rc.SaveReversibleState(ctx, summaryID, sessionID, KindLeaf, "Compressed summary", 10, originals, blockID)
	require.NoError(t, err)

	msgs, err := rc.Decompress(ctx, DecompressCommand{
		SummaryID:    summaryID,
		TargetDetail: TargetMetadata,
	})
	require.NoError(t, err)
	require.Len(t, msgs, 2)

	require.Equal(t, "rmsg-0", msgs[0].ID)
	require.Equal(t, "user", msgs[0].Role)
	require.Empty(t, msgs[0].Content)

	require.Equal(t, "rmsg-1", msgs[1].ID)
	require.Equal(t, "assistant", msgs[1].Role)
	require.Empty(t, msgs[1].Content)
}

func TestReversibleCompactor_NonReversibleFallback(t *testing.T) {
	t.Parallel()
	_, rc, ctx, _, summaryID, blockID := setupReversibleTest(t)

	// The setup creates a summary with blockID but empty original_content,
	// so IsReversible returns false.
	reversible, err := rc.IsReversible(ctx, blockID)
	require.NoError(t, err)
	require.False(t, reversible, "summary with empty original_content should not be reversible")

	msgs, err := rc.Decompress(ctx, DecompressCommand{
		SummaryID:    summaryID,
		TargetDetail: TargetFull,
	})
	require.NoError(t, err)
	require.Len(t, msgs, 3, "should fall back to ExpandSummary and return messages")
	require.Equal(t, "Original message 0", msgs[0].Content)
	require.Equal(t, "Original message 1", msgs[1].Content)
	require.Equal(t, "Original message 2", msgs[2].Content)
}

func TestReversibleCompactor_NonReversibleFallback_CondensedSummary(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	rc := NewReversibleCompactor(store)
	ctx := context.Background()

	sessionID := "sess-condensed-fallback"
	createTestSession(t, queries, sessionID)

	for i := range 4 {
		createTestMessage(t, queries, sessionID, fmt.Sprintf("cmsg-%d", i), "user", fmt.Sprintf("Condensed msg %d", i))
	}

	leafID := "sum_condensed_leaf_01"
	err := queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  leafID,
		SessionID:  sessionID,
		Kind:       KindLeaf,
		Content:    "Leaf summary",
		TokenCount: 10,
		FileIds:    "[]",
	})
	require.NoError(t, err)
	for i := range 2 {
		err = queries.InsertLcmSummaryMessage(ctx, db.InsertLcmSummaryMessageParams{
			SummaryID: leafID,
			MessageID: fmt.Sprintf("cmsg-%d", i),
			Ord:       int64(i),
		})
		require.NoError(t, err)
	}

	condensedID := "sum_condensed_parent_01"
	err = queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  condensedID,
		SessionID:  sessionID,
		Kind:       KindCondensed,
		Content:    "Condensed summary",
		TokenCount: 5,
		FileIds:    "[]",
	})
	require.NoError(t, err)
	err = queries.InsertLcmSummaryMessage(ctx, db.InsertLcmSummaryMessageParams{
		SummaryID: condensedID,
		MessageID: "cmsg-2",
		Ord:       0,
	})
	require.NoError(t, err)
	err = queries.InsertLcmSummaryMessage(ctx, db.InsertLcmSummaryMessageParams{
		SummaryID: condensedID,
		MessageID: "cmsg-3",
		Ord:       1,
	})
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO lcm_summary_parents (summary_id, parent_summary_id, ord) VALUES (?, ?, 0)
	`, condensedID, leafID)
	require.NoError(t, err)

	msgs, err := rc.Decompress(ctx, DecompressCommand{
		SummaryID:    condensedID,
		TargetDetail: TargetFull,
	})
	require.NoError(t, err)
	require.Len(t, msgs, 4, "should expand all messages from leaf + condensed")
}

func TestReversibleCompactor_Decompress_NonexistentSummary(t *testing.T) {
	t.Parallel()
	_, rc, ctx, _, _, _ := setupReversibleTest(t)

	msgs, err := rc.Decompress(ctx, DecompressCommand{
		SummaryID:    "sum_nonexistent_summary",
		TargetDetail: TargetFull,
	})
	require.NoError(t, err)
	require.Empty(t, msgs, "nonexistent summary should return empty messages via fallback")
}

func TestReversibleCompactor_SaveReversibleState_Overwrite(t *testing.T) {
	t.Parallel()
	_, rc, ctx, sessionID, summaryID, blockID := setupReversibleTest(t)

	v1 := []MessageForSummary{
		{ID: "rmsg-0", SessionID: "sess-reversible", Seq: 0, Role: "user", Content: "Version 1"},
	}
	_, err := rc.store.rawDB.ExecContext(ctx, `DELETE FROM lcm_summaries WHERE summary_id = ?`, summaryID)
	require.NoError(t, err)

	err = rc.SaveReversibleState(ctx, summaryID, sessionID, KindLeaf, "Compressed summary", 10, v1, blockID)
	require.NoError(t, err)

	v2 := []MessageForSummary{
		{ID: "rmsg-0", SessionID: "sess-reversible", Seq: 0, Role: "user", Content: "Version 2"},
		{ID: "rmsg-1", SessionID: "sess-reversible", Seq: 1, Role: "assistant", Content: "New message"},
	}
	// Delete old row before re-inserting with same blockID.
	_, err = rc.store.rawDB.ExecContext(ctx, `DELETE FROM lcm_summaries WHERE summary_id = ?`, summaryID)
	require.NoError(t, err)

	err = rc.SaveReversibleState(ctx, summaryID, sessionID, KindLeaf, "Compressed summary v2", 12, v2, blockID)
	require.NoError(t, err)

	msgs, err := rc.Decompress(ctx, DecompressCommand{
		SummaryID:    summaryID,
		TargetDetail: TargetFull,
	})
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.Equal(t, "Version 2", msgs[0].Content)
	require.Equal(t, "New message", msgs[1].Content)
}

func TestBlockIDTracker_Sequential(t *testing.T) {
	t.Parallel()
	tracker := NewBlockIDTracker("blk")

	require.Equal(t, "blk-1", tracker.NextBlockID())
	require.Equal(t, "blk-2", tracker.NextBlockID())
	require.Equal(t, "blk-3", tracker.NextBlockID())
	require.Equal(t, int64(3), tracker.Current())
}

func TestBlockIDTracker_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	tracker := NewBlockIDTracker("race")

	const goroutines = 100
	ids := make(chan string, goroutines)

	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			ids <- tracker.NextBlockID()
		})
	}
	wg.Wait()
	close(ids)

	seen := make(map[string]struct{}, goroutines)
	for id := range ids {
		require.NotContains(t, seen, id, "duplicate block ID detected: %s", id)
		seen[id] = struct{}{}
	}
	require.Len(t, seen, goroutines)
	require.Equal(t, int64(goroutines), tracker.Current())
}

func TestBlockIDTracker_Prefix(t *testing.T) {
	t.Parallel()
	tracker := NewBlockIDTracker("ctx")

	require.Equal(t, "ctx-1", tracker.NextBlockID())
}

func setupBlockTestStore(t *testing.T) (*Store, context.Context, string) {
	t.Helper()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-block-test"
	createTestSession(t, queries, sessionID)

	return store, ctx, sessionID
}

func TestExpandLossless_Found(t *testing.T) {
	t.Parallel()
	store, ctx, sessionID := setupBlockTestStore(t)

	blockID := "blk-1"
	originalContent := "This is the original content that was compressed."

	err := store.InsertLcmSummaryWithBlock(ctx, "sum_blk_test_01", sessionID, KindLeaf, "compressed", 5, nil, blockID, originalContent)
	require.NoError(t, err)

	content, found, err := store.ExpandLossless(ctx, blockID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, originalContent, content)
}

func TestExpandLossless_NotFound(t *testing.T) {
	t.Parallel()
	store, ctx, _ := setupBlockTestStore(t)

	content, found, err := store.ExpandLossless(ctx, "nonexistent-block")
	require.NoError(t, err)
	require.False(t, found)
	require.Empty(t, content)
}

func TestRecompress_RoundTripPreservesContent(t *testing.T) {
	t.Parallel()
	_, rc, ctx, sessionID, summaryID, blockID := setupReversibleTest(t)

	originals := []MessageForSummary{
		{ID: "rmsg-0", SessionID: "sess-reversible", Seq: 0, Role: "user", Content: "Original message 0"},
		{ID: "rmsg-1", SessionID: "sess-reversible", Seq: 1, Role: "user", Content: "Original message 1"},
		{ID: "rmsg-2", SessionID: "sess-reversible", Seq: 2, Role: "user", Content: "Original message 2"},
	}

	_, err := rc.store.rawDB.ExecContext(ctx, `DELETE FROM lcm_summaries WHERE summary_id = ?`, summaryID)
	require.NoError(t, err)

	err = rc.SaveReversibleState(ctx, summaryID, sessionID, KindLeaf, "Compressed summary", 10, originals, blockID)
	require.NoError(t, err)

	// Re-link messages (cascade-deleted with the summary row above).
	msgIDs := []string{"rmsg-0", "rmsg-1", "rmsg-2"}
	for i, id := range msgIDs {
		err = rc.store.q.InsertLcmSummaryMessage(ctx, db.InsertLcmSummaryMessageParams{
			SummaryID: summaryID,
			MessageID: id,
			Ord:       int64(i),
		})
		require.NoError(t, err)
	}

	msgs, err := rc.Decompress(ctx, DecompressCommand{
		SummaryID:    summaryID,
		TargetDetail: TargetFull,
	})
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	require.Equal(t, "Original message 0", msgs[0].Content)

	err = rc.Recompress(ctx, RecompressCommand{SummaryID: summaryID})
	require.NoError(t, err)

	reversible, err := rc.IsReversible(ctx, blockID)
	require.NoError(t, err)
	require.False(t, reversible, "after recompress, block should no longer be reversible")

	decompressed, err := rc.IsDecompressed(ctx, summaryID)
	require.NoError(t, err)
	require.False(t, decompressed, "after recompress, summary should not be in decompressed state")

	msgs2, err := rc.Decompress(ctx, DecompressCommand{
		SummaryID:    summaryID,
		TargetDetail: TargetFull,
	})
	require.NoError(t, err)
	require.Len(t, msgs2, 3, "fallback to ExpandSummary should return messages from ancestry")
	require.Equal(t, "Original message 0", msgs2[0].Content)
	require.Equal(t, "Original message 1", msgs2[1].Content)
	require.Equal(t, "Original message 2", msgs2[2].Content)
}

func TestRecompress_NonDecompressedBlockReturnsError(t *testing.T) {
	t.Parallel()
	_, rc, ctx, _, summaryID, _ := setupReversibleTest(t)

	err := rc.Recompress(ctx, RecompressCommand{SummaryID: summaryID})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in a decompressed state")
}

func TestRecompress_NonexistentSummaryReturnsError(t *testing.T) {
	t.Parallel()
	_, rc, ctx, _, _, _ := setupReversibleTest(t)

	err := rc.Recompress(ctx, RecompressCommand{SummaryID: "sum_nonexistent"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestRecompress_AncestryChainMaintained(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	rc := NewReversibleCompactor(store)
	ctx := context.Background()

	sessionID := "sess-recompress-ancestry"
	createTestSession(t, queries, sessionID)

	msgIDs := []string{"amsg-0", "amsg-1"}
	for i, id := range msgIDs {
		createTestMessage(t, queries, sessionID, id, "user", fmt.Sprintf("Ancestry msg %d", i))
	}

	leafID := "sum_ancestry_leaf"
	leafBlockID := "blk-ancestry-leaf"
	originals := []MessageForSummary{
		{ID: "amsg-0", SessionID: sessionID, Seq: 0, Role: "user", Content: "Ancestry msg 0"},
	}
	err := store.InsertLcmSummaryWithBlock(ctx, leafID, sessionID, KindLeaf, "Leaf compressed", 5, nil, leafBlockID, "")
	require.NoError(t, err)
	err = queries.InsertLcmSummaryMessage(ctx, db.InsertLcmSummaryMessageParams{
		SummaryID: leafID,
		MessageID: "amsg-0",
		Ord:       0,
	})
	require.NoError(t, err)

	condensedID := "sum_ancestry_condensed"
	err = queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  condensedID,
		SessionID:  sessionID,
		Kind:       KindCondensed,
		Content:    "Condensed summary",
		TokenCount: 3,
		FileIds:    "[]",
	})
	require.NoError(t, err)
	err = queries.InsertLcmSummaryMessage(ctx, db.InsertLcmSummaryMessageParams{
		SummaryID: condensedID,
		MessageID: "amsg-1",
		Ord:       0,
	})
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO lcm_summary_parents (summary_id, parent_summary_id, ord) VALUES (?, ?, 0)
	`, condensedID, leafID)
	require.NoError(t, err)

	// Set original_content directly to make the leaf reversible (no delete/re-insert).
	msgsJSON, err := json.Marshal(originals)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx,
		`UPDATE lcm_summaries SET original_content = ? WHERE summary_id = ?`,
		string(msgsJSON), leafID,
	)
	require.NoError(t, err)

	msgsBefore, err := rc.Decompress(ctx, DecompressCommand{
		SummaryID:    condensedID,
		TargetDetail: TargetFull,
	})
	require.NoError(t, err)
	require.Len(t, msgsBefore, 2)

	err = rc.Recompress(ctx, RecompressCommand{SummaryID: leafID})
	require.NoError(t, err)

	msgsAfter, err := rc.Decompress(ctx, DecompressCommand{
		SummaryID:    condensedID,
		TargetDetail: TargetFull,
	})
	require.NoError(t, err)
	require.Equal(t, msgsBefore, msgsAfter, "ancestry chain should still expand to same messages after recompress")
	require.Len(t, msgsAfter, 2)
}

func TestIsDecompressed(t *testing.T) {
	t.Parallel()
	_, rc, ctx, sessionID, summaryID, blockID := setupReversibleTest(t)

	decompressed, err := rc.IsDecompressed(ctx, summaryID)
	require.NoError(t, err)
	require.False(t, decompressed, "empty original_content should not be decompressed")

	originals := []MessageForSummary{
		{ID: "rmsg-0", SessionID: "sess-reversible", Seq: 0, Role: "user", Content: "Hello"},
	}
	_, err = rc.store.rawDB.ExecContext(ctx, `DELETE FROM lcm_summaries WHERE summary_id = ?`, summaryID)
	require.NoError(t, err)
	err = rc.SaveReversibleState(ctx, summaryID, sessionID, KindLeaf, "Compressed", 5, originals, blockID)
	require.NoError(t, err)

	decompressed, err = rc.IsDecompressed(ctx, summaryID)
	require.NoError(t, err)
	require.True(t, decompressed)

	err = rc.Recompress(ctx, RecompressCommand{SummaryID: summaryID})
	require.NoError(t, err)

	decompressed, err = rc.IsDecompressed(ctx, summaryID)
	require.NoError(t, err)
	require.False(t, decompressed, "after recompress, should not be decompressed")
}

func TestExpandLossless_EmptyContent(t *testing.T) {
	t.Parallel()
	store, ctx, sessionID := setupBlockTestStore(t)

	err := store.InsertLcmSummaryWithBlock(ctx, "sum_blk_empty_01", sessionID, KindLeaf, "compressed", 5, nil, "blk-empty", "")
	require.NoError(t, err)

	content, found, err := store.ExpandLossless(ctx, "blk-empty")
	require.NoError(t, err)
	require.False(t, found, "empty original_content should not be found")
	require.Empty(t, content)
}
