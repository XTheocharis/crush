package lcm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// CompactionLayer interface compliance
// ---------------------------------------------------------------------------

func TestMicroCompactor_ImplementsCompactionLayer(t *testing.T) {
	t.Parallel()
	var _ CompactionLayer = (*MicroCompactor)(nil)
}

// ---------------------------------------------------------------------------
// CompactionLayerManager
// ---------------------------------------------------------------------------

func TestNewCompactionLayerManager_SortsByPriority(t *testing.T) {
	t.Parallel()

	layer3 := &stubLayer{name: "three", priority: 30}
	layer1 := &stubLayer{name: "one", priority: 10}
	layer2 := &stubLayer{name: "two", priority: 20}

	mgr := NewCompactionLayerManager(layer3, layer1, layer2)
	layers := mgr.Layers()

	require.Len(t, layers, 3)
	require.Equal(t, "one", layers[0].Name())
	require.Equal(t, "two", layers[1].Name())
	require.Equal(t, "three", layers[2].Name())
}

func TestNewCompactionLayerManager_Empty(t *testing.T) {
	t.Parallel()

	mgr := NewCompactionLayerManager()
	require.Empty(t, mgr.Layers())
}

func TestCompactionLayerManager_Layers_ReturnsSnapshot(t *testing.T) {
	t.Parallel()

	layer := &stubLayer{name: "a", priority: 1}
	mgr := NewCompactionLayerManager(layer)

	snapshot := mgr.Layers()
	require.Len(t, snapshot, 1)

	// Mutating the snapshot should not affect the manager.
	snapshot[0] = nil
	require.NotNil(t, mgr.Layers()[0])
}

func TestCompactionLayerManager_RunAll_NoEligibleLayers(t *testing.T) {
	t.Parallel()

	never := &stubLayer{
		name:     "never",
		priority: 1,
		should:   false,
	}

	mgr := NewCompactionLayerManager(never)
	result, err := mgr.RunAll(context.Background(), Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
}

func TestCompactionLayerManager_RunAll_AllEligible(t *testing.T) {
	t.Parallel()

	a := &stubLayer{
		name:     "a",
		priority: 1,
		should:   true,
		result: &CompactionLayerResult{
			LayerName:     "a",
			TokensFreed:   100,
			ItemsAffected: 1,
			ActionTaken:   true,
		},
	}
	b := &stubLayer{
		name:     "b",
		priority: 2,
		should:   true,
		result: &CompactionLayerResult{
			LayerName:     "b",
			TokensFreed:   200,
			ItemsAffected: 2,
			ActionTaken:   true,
		},
	}

	mgr := NewCompactionLayerManager(a, b)
	result, err := mgr.RunAll(context.Background(), Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, int64(300), result.TokensFreed)
	require.Equal(t, 3, result.ItemsAffected)
}

func TestCompactionLayerManager_RunAll_StopsOnError(t *testing.T) {
	t.Parallel()

	failing := &stubLayer{
		name:     "fail",
		priority: 1,
		should:   true,
		err:      fmt.Errorf("boom"),
	}
	after := &stubLayer{
		name:     "after",
		priority: 2,
		should:   true,
		result:   &CompactionLayerResult{ActionTaken: true},
	}

	mgr := NewCompactionLayerManager(failing, after)
	_, err := mgr.RunAll(context.Background(), Budget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "layer fail")
	require.Contains(t, err.Error(), "boom")
}

func TestCompactionLayerManager_RunAll_SkipsNilResult(t *testing.T) {
	t.Parallel()

	layer := &stubLayer{
		name:     "nil-result",
		priority: 1,
		should:   true,
		result:   nil,
	}

	mgr := NewCompactionLayerManager(layer)
	result, err := mgr.RunAll(context.Background(), Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
}

func TestCompactionLayerManager_RunAll_MixedEligibility(t *testing.T) {
	t.Parallel()

	yes := &stubLayer{
		name:     "yes",
		priority: 1,
		should:   true,
		result: &CompactionLayerResult{
			TokensFreed:   50,
			ItemsAffected: 1,
			ActionTaken:   true,
		},
	}
	no := &stubLayer{
		name:     "no",
		priority: 2,
		should:   false,
	}

	mgr := NewCompactionLayerManager(yes, no)
	result, err := mgr.RunAll(context.Background(), Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, int64(50), result.TokensFreed)
}

// ---------------------------------------------------------------------------
// MicroCompactor unit tests
// ---------------------------------------------------------------------------

func TestMicroCompactor_Name(t *testing.T) {
	t.Parallel()
	m := NewMicroCompactor(MicroCompactorConfig{})
	require.Equal(t, "micro-compactor", m.Name())
}

func TestMicroCompactor_Priority(t *testing.T) {
	t.Parallel()
	m := NewMicroCompactor(MicroCompactorConfig{})
	require.Equal(t, 1, m.Priority())
}

func TestMicroCompactor_ShouldCompact_NoStore(t *testing.T) {
	t.Parallel()
	m := NewMicroCompactor(MicroCompactorConfig{SessionID: "s1"})
	require.False(t, m.ShouldCompact(context.Background(), Budget{}))
}

func TestMicroCompactor_ShouldCompact_EmptySessionID(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	m := NewMicroCompactor(MicroCompactorConfig{Store: store})
	require.False(t, m.ShouldCompact(context.Background(), Budget{}))
}

func TestMicroCompactor_ShouldCompact_UnderThreshold(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-micro-under"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a small message (500 tokens, well under the 10000 default).
	msgID := "msg-small"
	createTestMessage(t, queries, sessionID, msgID, "assistant", strings.Repeat("x", 2000))
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 500,
	})
	require.NoError(t, err)

	m := NewMicroCompactor(MicroCompactorConfig{Store: store, SessionID: sessionID})
	require.False(t, m.ShouldCompact(ctx, Budget{}))
}

func TestMicroCompactor_ShouldCompact_OverThreshold(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-micro-over"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a large message (55000 tokens, over the 50000 threshold).
	msgID := "msg-large"
	largeContent := strings.Repeat("content word ", 220000)
	createTestMessage(t, queries, sessionID, msgID, "assistant", largeContent)
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 55000,
	})
	require.NoError(t, err)

	m := NewMicroCompactor(MicroCompactorConfig{Store: store, SessionID: sessionID})
	require.True(t, m.ShouldCompact(ctx, Budget{}))
}

func TestMicroCompactor_Compact_StoresLargeContent(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-micro-compact"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a large message.
	msgID := "msg-big"
	largeContent := strings.Repeat("large output data ", 22000)
	createTestMessage(t, queries, sessionID, msgID, "tool", largeContent)
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 55000,
	})
	require.NoError(t, err)

	m := NewMicroCompactor(MicroCompactorConfig{
		Store:     store,
		SessionID: sessionID,
	})

	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
	require.True(t, result.TokensFreed > 0, "should have freed some tokens")
}

func TestMicroCompactor_Compact_NoOversizedMessages(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-micro-noop"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	msgID := "msg-tiny"
	createTestMessage(t, queries, sessionID, msgID, "assistant", "small content")
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 5,
	})
	require.NoError(t, err)

	m := NewMicroCompactor(MicroCompactorConfig{Store: store, SessionID: sessionID})
	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
	require.Equal(t, 0, result.ItemsAffected)
}

func TestMicroCompactor_Compact_NilStore_ReturnsError(t *testing.T) {
	t.Parallel()
	m := NewMicroCompactor(MicroCompactorConfig{SessionID: "s1"})
	_, err := m.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrStoreIsNil))
}

func TestMicroCompactor_Compact_EmptySessionID_ReturnsError(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	m := NewMicroCompactor(MicroCompactorConfig{Store: store})
	_, err := m.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSessionIDEmpty))
}

func TestMicroCompactor_Compact_SkipsAlreadyReferenced(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-micro-skip"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a message that already contains a reference marker.
	msgID := "msg-referenced"
	referencedContent := "[Large File Stored: file_abc123def456]\nLCM File ID: file_abc123def456\n\nPreview:\nstuff"
	createTestMessage(t, queries, sessionID, msgID, "tool", referencedContent)
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 15000,
	})
	require.NoError(t, err)

	m := NewMicroCompactor(MicroCompactorConfig{Store: store, SessionID: sessionID})
	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
	require.Equal(t, 0, result.ItemsAffected)
}

func TestMicroCompactor_Compact_CustomThreshold(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-micro-custom"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Message at 1000 tokens — under default threshold but over custom threshold of 500.
	msgID := "msg-med"
	content := strings.Repeat("medium content ", 2000)
	createTestMessage(t, queries, sessionID, msgID, "assistant", content)
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 1000,
	})
	require.NoError(t, err)

	m := NewMicroCompactor(MicroCompactorConfig{
		Store:          store,
		SessionID:      sessionID,
		TokenThreshold: 500,
	})

	// Should detect the message as oversized with the custom threshold.
	require.True(t, m.ShouldCompact(ctx, Budget{}))

	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
}

func TestMicroCompactor_Compact_MultipleOversized(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-micro-multi"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	for i := range 3 {
		msgID := fmt.Sprintf("msg-big-%d", i)
		content := strings.Repeat(fmt.Sprintf("output %d ", i), 22000)
		createTestMessage(t, queries, sessionID, msgID, "tool", content)
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 55000,
		})
		require.NoError(t, err)
	}

	m := NewMicroCompactor(MicroCompactorConfig{Store: store, SessionID: sessionID})
	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 3, result.ItemsAffected)
	require.True(t, result.TokensFreed > 0)
}

// ---------------------------------------------------------------------------
// isAlreadyReferenced
// ---------------------------------------------------------------------------

func TestIsAlreadyReferenced_Positive(t *testing.T) {
	t.Parallel()
	require.True(t, isAlreadyReferenced("some text [Large File Stored: file_abc] more"))
	require.True(t, isAlreadyReferenced("[Large Tool Output Stored: file_abc]"))
	require.True(t, isAlreadyReferenced("[Large User Text Stored: file_abc]"))
}

func TestIsAlreadyReferenced_Negative(t *testing.T) {
	t.Parallel()
	require.False(t, isAlreadyReferenced("just some regular text"))
	require.False(t, isAlreadyReferenced(""))
	require.False(t, isAlreadyReferenced("file_abc123 without markers"))
}

// ---------------------------------------------------------------------------
// Integration: LayerManager + MicroCompactor with real DB
// ---------------------------------------------------------------------------

func TestLayerManager_Integration_WithMicroCompactor(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-integration"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	// Insert a large message.
	msgID := "msg-int-big"
	largeContent := strings.Repeat("integration test data ", 22000)
	createTestMessage(t, queries, sessionID, msgID, "tool", largeContent)
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 55000,
	})
	require.NoError(t, err)

	micro := NewMicroCompactor(MicroCompactorConfig{Store: store, SessionID: sessionID})
	mgr := NewCompactionLayerManager(micro)

	budget := Budget{SoftThreshold: 50000, HardLimit: 60000, ContextWindow: 128000}
	result, err := mgr.RunAll(ctx, budget)
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// stubLayer is a test double for CompactionLayer.
type stubLayer struct {
	name     string
	priority int
	should   bool
	result   *CompactionLayerResult
	err      error
	called   bool
}

func (s *stubLayer) Name() string                                   { return s.name }
func (s *stubLayer) Priority() int                                  { return s.priority }
func (s *stubLayer) ShouldCompact(_ context.Context, _ Budget) bool { return s.should }
func (s *stubLayer) Compact(_ context.Context, _ Budget) (*CompactionLayerResult, error) {
	s.called = true
	return s.result, s.err
}

// ---------------------------------------------------------------------------
// Layer 2: DedupCompactionLayer tests
// ---------------------------------------------------------------------------

func TestDedupCompactionLayer_ImplementsCompactionLayer(t *testing.T) {
	t.Parallel()
	var _ CompactionLayer = (*DedupCompactionLayer)(nil)
}

func TestDedupLayerNameAndPriority(t *testing.T) {
	t.Parallel()
	layer := NewDedupCompactionLayer(nil, "sess")
	require.Equal(t, "dedup-compaction", layer.Name())
	require.Equal(t, 2, layer.Priority())
}

func TestDedupLayerShouldCompact_NoDuplicates(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-dedup-no"
	createTestSession(t, queries, sessionID)

	createTestMessage(t, queries, sessionID, "msg-1", "user", "unique content A")
	createTestMessage(t, queries, sessionID, "msg-2", "user", "unique content B")

	for i, id := range []string{"msg-1", "msg-2"} {
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: id, Valid: true},
			TokenCount: 100,
		})
		require.NoError(t, err)
	}

	layer := NewDedupCompactionLayer(store, sessionID)
	require.False(t, layer.ShouldCompact(ctx, Budget{}))
}

func TestDedupLayerShouldCompact_WithDuplicates(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-dedup-yes"
	createTestSession(t, queries, sessionID)

	content := "duplicate content here"
	createTestMessage(t, queries, sessionID, "msg-1", "user", content)
	createTestMessage(t, queries, sessionID, "msg-2", "user", content)

	for i, id := range []string{"msg-1", "msg-2"} {
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: id, Valid: true},
			TokenCount: 200,
		})
		require.NoError(t, err)
	}

	layer := NewDedupCompactionLayer(store, sessionID)
	require.True(t, layer.ShouldCompact(ctx, Budget{}))
}

func TestDedupLayerCompact(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-dedup-compact"
	createTestSession(t, queries, sessionID)

	content := "exact same content"
	createTestMessage(t, queries, sessionID, "msg-a", "user", content)
	createTestMessage(t, queries, sessionID, "msg-b", "user", content)
	createTestMessage(t, queries, sessionID, "msg-c", "user", "different")

	for i, id := range []string{"msg-a", "msg-b", "msg-c"} {
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: id, Valid: true},
			TokenCount: 300,
		})
		require.NoError(t, err)
	}

	layer := NewDedupCompactionLayer(store, sessionID)
	result, err := layer.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
	require.Equal(t, int64(300), result.TokensFreed)

	summaries, err := queries.ListLcmSummariesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.Equal(t, KindArchiveStub, summaries[0].Kind)
}

// ---------------------------------------------------------------------------
// Layer 3: StaleEvictionLayer tests
// ---------------------------------------------------------------------------

func TestStaleEvictionLayer_ImplementsCompactionLayer(t *testing.T) {
	t.Parallel()
	var _ CompactionLayer = (*StaleEvictionLayer)(nil)
}

func TestStaleEvictionLayerNameAndPriority(t *testing.T) {
	t.Parallel()
	layer := NewStaleEvictionLayer(nil, "sess", 0)
	require.Equal(t, "stale-eviction", layer.Name())
	require.Equal(t, 3, layer.Priority())
}

func TestStaleEvictionLayerShouldCompact_NoStale(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-stale-no"
	createTestSession(t, queries, sessionID)

	createTestMessage(t, queries, sessionID, "msg-recent", "user", "recent content")

	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: "msg-recent", Valid: true},
		TokenCount: 100,
	})
	require.NoError(t, err)

	layer := NewStaleEvictionLayer(store, sessionID, 30*time.Minute)
	require.False(t, layer.ShouldCompact(ctx, Budget{}))
}

func TestStaleEvictionLayerShouldCompact_WithStale(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-stale-yes"
	createTestSession(t, queries, sessionID)

	staleTime := time.Now().Add(-2 * time.Hour).Unix()
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          "msg-stale",
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "user",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "stale content"),
	})
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx,
		"UPDATE messages SET created_at = ? WHERE id = ?",
		staleTime, "msg-stale",
	)
	require.NoError(t, err)

	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: "msg-stale", Valid: true},
		TokenCount: 150,
	})
	require.NoError(t, err)

	layer := NewStaleEvictionLayer(store, sessionID, 30*time.Minute)
	require.True(t, layer.ShouldCompact(ctx, Budget{}))
}

func TestStaleEvictionLayerCompact(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-stale-compact"
	createTestSession(t, queries, sessionID)

	staleTime := time.Now().Add(-2 * time.Hour).Unix()
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:          "msg-old",
		SessionID:   sessionID,
		SessionID_2: sessionID,
		Role:        "user",
		Parts:       fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, "old content"),
	})
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx,
		"UPDATE messages SET created_at = ? WHERE id = ?",
		staleTime, "msg-old",
	)
	require.NoError(t, err)

	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: "msg-old", Valid: true},
		TokenCount: 500,
	})
	require.NoError(t, err)

	layer := NewStaleEvictionLayer(store, sessionID, 30*time.Minute)
	result, err := layer.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
	require.Equal(t, int64(500), result.TokensFreed)
}

// ---------------------------------------------------------------------------
// Layer 5: AdjacentCondensationLayer tests
// ---------------------------------------------------------------------------

func TestAdjacentCondensationLayer_ImplementsCompactionLayer(t *testing.T) {
	t.Parallel()
	var _ CompactionLayer = (*AdjacentCondensationLayer)(nil)
}

func TestAdjacentCondensationLayerNameAndPriority(t *testing.T) {
	t.Parallel()
	layer := NewAdjacentCondensationLayer(nil, "sess", 0)
	require.Equal(t, "adjacent-condensation", layer.Name())
	require.Equal(t, 5, layer.Priority())
}

func TestAdjacentCondensationLayerShouldCompact_NoAdjacent(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-adj-no"
	createTestSession(t, queries, sessionID)

	summaryID := "sum_solo1234567890"
	err := queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  summaryID,
		SessionID:  sessionID,
		Kind:       KindLeaf,
		Content:    "solo summary",
		TokenCount: 100,
		FileIds:    "[]",
	})
	require.NoError(t, err)

	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "summary",
		SummaryID:  sql.NullString{String: summaryID, Valid: true},
		TokenCount: 3000,
	})
	require.NoError(t, err)

	layer := NewAdjacentCondensationLayer(store, sessionID, 4000)
	require.False(t, layer.ShouldCompact(ctx, Budget{}))
}

func TestAdjacentCondensationLayerShouldCompact_WithAdjacent(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-adj-yes"
	createTestSession(t, queries, sessionID)

	for i, suffix := range []string{"aaa000000000000", "bbb000000000000"} {
		sid := "sum_" + suffix
		err := queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
			SummaryID:  sid,
			SessionID:  sessionID,
			Kind:       KindLeaf,
			Content:    fmt.Sprintf("summary %d", i),
			TokenCount: 500,
			FileIds:    "[]",
		})
		require.NoError(t, err)

		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "summary",
			SummaryID:  sql.NullString{String: sid, Valid: true},
			TokenCount: 500,
		})
		require.NoError(t, err)
	}

	layer := NewAdjacentCondensationLayer(store, sessionID, 4000)
	require.True(t, layer.ShouldCompact(ctx, Budget{}))
}

func TestAdjacentCondensationLayerCompact(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-adj-compact"
	createTestSession(t, queries, sessionID)

	sums := []struct {
		id, content string
		tokens      int64
	}{
		{"sum_ccc000000000000", "first part of summary", 200},
		{"sum_ddd000000000000", "second part of summary", 200},
	}

	for i, s := range sums {
		err := queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
			SummaryID:  s.id,
			SessionID:  sessionID,
			Kind:       KindLeaf,
			Content:    s.content,
			TokenCount: s.tokens,
			FileIds:    "[]",
		})
		require.NoError(t, err)

		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "summary",
			SummaryID:  sql.NullString{String: s.id, Valid: true},
			TokenCount: s.tokens,
		})
		require.NoError(t, err)
	}

	layer := NewAdjacentCondensationLayer(store, sessionID, 4000)
	result, err := layer.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 2, result.ItemsAffected)

	summariesAfter, err := queries.ListLcmSummariesBySession(ctx, sessionID)
	require.NoError(t, err)

	var stubCount, leafCount int
	for _, s := range summariesAfter {
		switch s.Kind {
		case KindArchiveStub:
			stubCount++
		case KindLeaf:
			leafCount++
		}
	}
	require.Equal(t, 2, stubCount)
	require.Equal(t, 1, leafCount)
}
