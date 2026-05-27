package lcm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

func TestFullCompactor_ImplementsCompactionLayer(t *testing.T) {
	t.Parallel()
	var _ CompactionLayer = (*FullCompactor)(nil)
}

// ---------------------------------------------------------------------------
// Name and Priority
// ---------------------------------------------------------------------------

func TestFullCompactor_Name(t *testing.T) {
	t.Parallel()
	f := NewFullCompactor(FullCompactorConfig{})
	require.Equal(t, "full-compactor", f.Name())
}

func TestFullCompactor_Priority(t *testing.T) {
	t.Parallel()
	f := NewFullCompactor(FullCompactorConfig{})
	require.Equal(t, 30, f.Priority())
}

// ---------------------------------------------------------------------------
// ShouldCompact
// ---------------------------------------------------------------------------

func TestFullCompactor_ShouldCompact_NilLLM(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	f := NewFullCompactor(FullCompactorConfig{Store: store, SessionID: "s1"})
	require.False(t, f.ShouldCompact(context.Background(), Budget{}))
}

func TestFullCompactor_ShouldCompact_NilStore(t *testing.T) {
	t.Parallel()
	f := NewFullCompactor(FullCompactorConfig{
		LLM:       &mockLLMClient{},
		SessionID: "s1",
	})
	require.False(t, f.ShouldCompact(context.Background(), Budget{}))
}

func TestFullCompactor_ShouldCompact_EmptySessionID(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	f := NewFullCompactor(FullCompactorConfig{LLM: &mockLLMClient{}, Store: store})
	require.False(t, f.ShouldCompact(context.Background(), Budget{}))
}

func TestFullCompactor_ShouldCompact_UnderThreshold(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-full-under"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a small message (100 tokens, well under 5000 default).
	msgID := "msg-small-fc"
	createTestMessage(t, queries, sessionID, msgID, "assistant", "small")
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 100,
	})
	require.NoError(t, err)

	f := NewFullCompactor(FullCompactorConfig{
		LLM:       &mockLLMClient{},
		Store:     store,
		SessionID: sessionID,
	})
	require.False(t, f.ShouldCompact(ctx, Budget{}))
}

func TestFullCompactor_ShouldCompact_OverThreshold(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-full-over"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a message with 6000 tokens (over the 5000 default).
	msgID := "msg-large-fc"
	createTestMessage(t, queries, sessionID, msgID, "assistant", strings.Repeat("x ", 24000))
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 6000,
	})
	require.NoError(t, err)

	f := NewFullCompactor(FullCompactorConfig{
		LLM:       &mockLLMClient{},
		Store:     store,
		SessionID: sessionID,
	})
	require.True(t, f.ShouldCompact(ctx, Budget{}))
}

func TestFullCompactor_ShouldCompact_CustomMinTokens(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-full-custom"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a message at 500 tokens — under default but over custom min of 200.
	msgID := "msg-med-fc"
	createTestMessage(t, queries, sessionID, msgID, "assistant", "medium content")
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 500,
	})
	require.NoError(t, err)

	f := NewFullCompactor(FullCompactorConfig{
		LLM:            &mockLLMClient{},
		Store:          store,
		SessionID:      sessionID,
		MinTotalTokens: 200,
	})
	require.True(t, f.ShouldCompact(ctx, Budget{}))
}

// ---------------------------------------------------------------------------
// Compact — validation errors
// ---------------------------------------------------------------------------

func TestFullCompactor_Compact_NilLLM_ReturnsError(t *testing.T) {
	t.Parallel()
	f := NewFullCompactor(FullCompactorConfig{SessionID: "s1"})
	_, err := f.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrLLMClientNil))
}

func TestFullCompactor_Compact_NilStore_ReturnsError(t *testing.T) {
	t.Parallel()
	f := NewFullCompactor(FullCompactorConfig{LLM: &mockLLMClient{}, SessionID: "s1"})
	_, err := f.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrStoreIsNil))
}

func TestFullCompactor_Compact_EmptySessionID_ReturnsError(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	f := NewFullCompactor(FullCompactorConfig{LLM: &mockLLMClient{}, Store: store})
	_, err := f.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSessionIDEmpty))
}

func TestFullCompactor_Compact_EmptyEntries_NoAction(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-full-empty"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)
	f := NewFullCompactor(FullCompactorConfig{
		LLM:       &mockLLMClient{response: "summary"},
		Store:     store,
		SessionID: sessionID,
	})

	result, err := f.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
}

// ---------------------------------------------------------------------------
// Compact — token reduction
// ---------------------------------------------------------------------------

func TestFullCompactor_Compact_ProducesTokenReduction(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-full-reduce"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert 5 messages with high token counts (total 25000 tokens).
	for i := range 5 {
		msgID := fmt.Sprintf("msg-fc-%d", i)
		content := fmt.Sprintf("Full compaction test message %d: %s", i, strings.Repeat("data ", 1000))
		createTestMessage(t, queries, sessionID, msgID, "user", content)
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 5000,
		})
		require.NoError(t, err)
	}

	// Mock LLM returns a dense summary (much smaller than original).
	denseSummary := "Key decisions: implemented X, refactored Y. Files: foo.go, bar.go. Pending: tests for Z."
	mockLLM := &mockLLMClient{response: denseSummary}

	f := NewFullCompactor(FullCompactorConfig{
		LLM:       mockLLM,
		Store:     store,
		SessionID: sessionID,
	})

	result, err := f.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 5, result.ItemsAffected)
	require.True(t, result.TokensFreed > 0, "should have freed tokens")

	// Verify the mock was called (the "forked agent" LLM call).
	require.Equal(t, 1, mockLLM.callCount, "should have made exactly one LLM call")

	// Verify token reduction ratio > 80%.
	// Original: 25000 tokens. Summary is ~30 words ≈ ~40 tokens.
	// Freed should be ~24960 which is >80% of 25000.
	reductionRatio := float64(result.TokensFreed) / float64(25000)
	require.True(t, reductionRatio > 0.80, "token reduction ratio should exceed 80%%, got %.2f%%", reductionRatio*100)
}

func TestFullCompactor_Compact_MixedEntries(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-full-mixed"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a message entry.
	msgID := "msg-fc-mix"
	createTestMessage(t, queries, sessionID, msgID, "user", "message content for full compaction")
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 3000,
	})
	require.NoError(t, err)

	// Insert a summary entry.
	sumID := "sum_fullmix00010001"
	err = queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  sumID,
		SessionID:  sessionID,
		Kind:       KindLeaf,
		Content:    "Existing summary content about previous work.",
		TokenCount: 2000,
		FileIds:    "[]",
	})
	require.NoError(t, err)

	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   1,
		ItemType:   "summary",
		SummaryID:  sql.NullString{String: sumID, Valid: true},
		TokenCount: 2000,
	})
	require.NoError(t, err)

	denseSummary := "Condensed: completed feature A. Working on feature B."
	mockLLM := &mockLLMClient{response: denseSummary}

	f := NewFullCompactor(FullCompactorConfig{
		LLM:       mockLLM,
		Store:     store,
		SessionID: sessionID,
	})

	result, err := f.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 2, result.ItemsAffected)

	// Original total: 3000 + 2000 = 5000 tokens.
	// Summary is ~10 words ≈ ~13 tokens. Freed ≈ 4987.
	require.True(t, result.TokensFreed > 0)
}

func TestFullCompactor_Compact_LLMError_Propagates(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-full-err"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	msgID := "msg-fc-err"
	createTestMessage(t, queries, sessionID, msgID, "user", strings.Repeat("content ", 5000))
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 10000,
	})
	require.NoError(t, err)

	mockLLM := &mockLLMClient{err: fmt.Errorf("LLM service unavailable")}

	f := NewFullCompactor(FullCompactorConfig{
		LLM:       mockLLM,
		Store:     store,
		SessionID: sessionID,
	})

	_, err = f.Compact(ctx, Budget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "full compaction LLM call")
	require.Contains(t, err.Error(), "LLM service unavailable")
}

// ---------------------------------------------------------------------------
// Compact — summary truncation when too large
// ---------------------------------------------------------------------------

func TestFullCompactor_Compact_TruncatesOversizedSummary(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-full-trunc"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a message with moderate tokens.
	msgID := "msg-fc-trunc"
	createTestMessage(t, queries, sessionID, msgID, "user", "test content")
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 6000,
	})
	require.NoError(t, err)

	// Mock LLM returns a very long summary (exceeds FullCompactorMaxSummaryTokens).
	largeSummary := strings.Repeat("word ", int(FullCompactorMaxSummaryTokens*CharsPerToken)+1000)
	mockLLM := &mockLLMClient{response: largeSummary}

	f := NewFullCompactor(FullCompactorConfig{
		LLM:       mockLLM,
		Store:     store,
		SessionID: sessionID,
	})

	result, err := f.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)

	// Even with truncation, the summary should be smaller than the original.
	require.True(t, result.TokensFreed > 0)
}

// ---------------------------------------------------------------------------
// Integration: CompactionLayerManager with FullCompactor
// ---------------------------------------------------------------------------

func TestLayerManager_Integration_WithFullCompactor(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-full-integration"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a large message.
	msgID := "msg-int-fc"
	largeContent := strings.Repeat("integration test data for full compaction ", 3000)
	createTestMessage(t, queries, sessionID, msgID, "tool", largeContent)
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 30000,
	})
	require.NoError(t, err)

	denseSummary := "Completed: feature X. Files modified: a.go, b.go."
	mockLLM := &mockLLMClient{response: denseSummary}

	fc := NewFullCompactor(FullCompactorConfig{
		LLM:       mockLLM,
		Store:     store,
		SessionID: sessionID,
	})
	mgr := NewCompactionLayerManager(fc)

	budget := Budget{SoftThreshold: 50000, HardLimit: 60000, ContextWindow: 128000}
	result, err := mgr.RunAll(ctx, budget)
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
}

func TestLayerManager_Integration_MicroAndFullCompactor(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-both-layers"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a large message that both layers should process.
	msgID := "msg-both"
	largeContent := strings.Repeat("large output ", 10000)
	createTestMessage(t, queries, sessionID, msgID, "tool", largeContent)
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 30000,
	})
	require.NoError(t, err)

	denseSummary := "Dense summary replacing everything."
	mockLLM := &mockLLMClient{response: denseSummary}

	micro := NewMicroCompactor(MicroCompactorConfig{
		Store:     store,
		SessionID: sessionID,
	})
	full := NewFullCompactor(FullCompactorConfig{
		LLM:       mockLLM,
		Store:     store,
		SessionID: sessionID,
	})

	mgr := NewCompactionLayerManager(micro, full)
	budget := Budget{SoftThreshold: 50000, HardLimit: 60000, ContextWindow: 128000}
	result, err := mgr.RunAll(ctx, budget)
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
}

// ---------------------------------------------------------------------------
// formatEntriesForFullSummary
// ---------------------------------------------------------------------------

func TestFormatEntriesForFullSummary_Messages(t *testing.T) {
	t.Parallel()
	f := NewFullCompactor(FullCompactorConfig{})
	entries := []ContextEntry{
		{ItemType: "message", MessageID: "msg1", TokenCount: 100},
		{ItemType: "message", MessageID: "msg2", TokenCount: 200},
	}

	result := f.formatEntriesForFullSummary(entries)
	require.Contains(t, result, "<conversation-context>")
	require.Contains(t, result, "</conversation-context>")
	require.Contains(t, result, "msg1")
	require.Contains(t, result, "msg2")
}

func TestFormatEntriesForFullSummary_Summaries(t *testing.T) {
	t.Parallel()
	f := NewFullCompactor(FullCompactorConfig{})
	entries := []ContextEntry{
		{
			ItemType:       "summary",
			SummaryID:      "sum_abc",
			SummaryKind:    KindLeaf,
			SummaryContent: "Summary text here.",
			TokenCount:     50,
			ParentIDs:      []string{"sum_parent1"},
		},
	}

	result := f.formatEntriesForFullSummary(entries)
	require.Contains(t, result, "Summary text here.")
	require.Contains(t, result, "sum_parent1")
	require.Contains(t, result, KindLeaf)
}

func TestFormatEntriesForFullSummary_Empty(t *testing.T) {
	t.Parallel()
	f := NewFullCompactor(FullCompactorConfig{})
	result := f.formatEntriesForFullSummary(nil)
	require.Contains(t, result, "<conversation-context>")
	require.Contains(t, result, "</conversation-context>")
}
