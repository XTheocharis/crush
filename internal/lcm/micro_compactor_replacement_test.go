package lcm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

// mockReplacementStore is a test double for ContentReplacementStore.
type mockReplacementStore struct {
	mu          sync.Mutex
	recorded    []ContentReplacement
	byPosition  map[int64][]ContentReplacement
	recordErr   error
	recordCalls int
}

func newMockReplacementStore() *mockReplacementStore {
	return &mockReplacementStore{
		byPosition: make(map[int64][]ContentReplacement),
	}
}

func (m *mockReplacementStore) RecordReplacement(_ context.Context, r ContentReplacement) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCalls++
	if m.recordErr != nil {
		return 0, m.recordErr
	}
	id := int64(len(m.recorded) + 1)
	r.ID = id
	m.recorded = append(m.recorded, r)
	m.byPosition[r.Position] = append(m.byPosition[r.Position], r)
	return id, nil
}

func (m *mockReplacementStore) GetBySessionPosition(_ context.Context, _ string, position int64) ([]ContentReplacement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byPosition[position], nil
}

func (m *mockReplacementStore) GetByFileID(_ context.Context, _ string, _ string) ([]ContentReplacement, error) {
	return nil, nil
}

func (m *mockReplacementStore) ListByState(_ context.Context, _ string, _ ReplacementState) ([]ContentReplacement, error) {
	return nil, nil
}

func (m *mockReplacementStore) UpdateState(_ context.Context, _ int64, _ ReplacementState) error {
	return nil
}

func (m *mockReplacementStore) ListByRound(_ context.Context, _ string, _ int) ([]ContentReplacement, error) {
	return nil, nil
}

func (m *mockReplacementStore) getRecorded() []ContentReplacement {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ContentReplacement, len(m.recorded))
	copy(out, m.recorded)
	return out
}

// setupOversizedMessage creates a session with a single oversized message entry
// and returns the store, sessionID, and msgID.
func setupOversizedMessage(t *testing.T) (*Store, string, string) {
	t.Helper()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := fmt.Sprintf("sess-repl-%s", t.Name())
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	msgID := "msg-big"
	largeContent := strings.Repeat("large output data ", 5000)
	createTestMessage(t, queries, sessionID, msgID, "tool", largeContent)
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 25000,
	})
	require.NoError(t, err)

	return store, sessionID, msgID
}

func TestMicroCompactor_Compact_RecordsReplacement(t *testing.T) {
	t.Parallel()
	store, sessionID, _ := setupOversizedMessage(t)
	ctx := context.Background()

	mockStore := newMockReplacementStore()

	m := NewMicroCompactor(MicroCompactorConfig{
		Store:            store,
		SessionID:        sessionID,
		ReplacementStore: mockStore,
		Round:            3,
	})

	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)

	recorded := mockStore.getRecorded()
	require.Len(t, recorded, 1)

	r := recorded[0]
	require.Equal(t, sessionID, r.SessionID)
	require.Equal(t, int64(0), r.Position)
	require.Equal(t, ReplacementActive, r.State)
	require.Equal(t, 3, r.Round)
	require.Equal(t, 25000, r.OriginalTokenCount)
	require.Greater(t, r.ReplacementTokenCount, 0)
	require.Less(t, r.ReplacementTokenCount, r.OriginalTokenCount)
	require.True(t, r.MessageID.Valid)
	require.True(t, r.FileID.Valid)
	require.NotEmpty(t, r.FileID.String)
}

func TestMicroCompactor_Compact_NilReplacementStore_NoPanic(t *testing.T) {
	t.Parallel()
	store, sessionID, _ := setupOversizedMessage(t)
	ctx := context.Background()

	m := NewMicroCompactor(MicroCompactorConfig{
		Store:            store,
		SessionID:        sessionID,
		ReplacementStore: nil,
	})

	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
}

func TestMicroCompactor_Compact_PinnedEntrySkipped(t *testing.T) {
	t.Parallel()
	store, sessionID, _ := setupOversizedMessage(t)
	ctx := context.Background()

	mockStore := newMockReplacementStore()
	mockStore.byPosition[0] = []ContentReplacement{
		{
			SessionID: sessionID,
			Position:  0,
			State:     ReplacementPinned,
		},
	}

	m := NewMicroCompactor(MicroCompactorConfig{
		Store:            store,
		SessionID:        sessionID,
		ReplacementStore: mockStore,
		Round:            1,
	})

	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
	require.Equal(t, 0, result.ItemsAffected)

	recorded := mockStore.getRecorded()
	require.Empty(t, recorded)
}

func TestMicroCompactor_Compact_ActiveEntryNotSkipped(t *testing.T) {
	t.Parallel()
	store, sessionID, _ := setupOversizedMessage(t)
	ctx := context.Background()

	mockStore := newMockReplacementStore()
	mockStore.byPosition[0] = []ContentReplacement{
		{
			SessionID: sessionID,
			Position:  0,
			State:     ReplacementActive,
		},
	}

	m := NewMicroCompactor(MicroCompactorConfig{
		Store:            store,
		SessionID:        sessionID,
		ReplacementStore: mockStore,
		Round:            1,
	})

	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
}

func TestMicroCompactor_Compact_RecordingFailureDoesNotAbort(t *testing.T) {
	t.Parallel()
	store, sessionID, _ := setupOversizedMessage(t)
	ctx := context.Background()

	mockStore := newMockReplacementStore()
	mockStore.recordErr = errors.New("database is full")

	m := NewMicroCompactor(MicroCompactorConfig{
		Store:            store,
		SessionID:        sessionID,
		ReplacementStore: mockStore,
		Round:            1,
	})

	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
	require.True(t, result.TokensFreed > 0)
	require.Equal(t, 1, mockStore.recordCalls)
}

func TestMicroCompactor_Compact_MultipleOversized_RecordsAll(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := fmt.Sprintf("sess-repl-multi-%s", t.Name())
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	for i := range 3 {
		msgID := fmt.Sprintf("msg-big-%d", i)
		content := strings.Repeat(fmt.Sprintf("output %d ", i), 5000)
		createTestMessage(t, queries, sessionID, msgID, "tool", content)
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 12000,
		})
		require.NoError(t, err)
	}

	mockStore := newMockReplacementStore()

	m := NewMicroCompactor(MicroCompactorConfig{
		Store:            store,
		SessionID:        sessionID,
		ReplacementStore: mockStore,
		Round:            5,
	})

	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 3, result.ItemsAffected)

	recorded := mockStore.getRecorded()
	require.Len(t, recorded, 3)

	for i, r := range recorded {
		require.Equal(t, int64(i), r.Position)
		require.Equal(t, ReplacementActive, r.State)
		require.Equal(t, 5, r.Round)
	}
}

func TestMicroCompactor_Compact_MixedPinnedAndUnpinned(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := fmt.Sprintf("sess-repl-mixed-%s", t.Name())
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	for i := range 3 {
		msgID := fmt.Sprintf("msg-mix-%d", i)
		content := strings.Repeat(fmt.Sprintf("content %d ", i), 5000)
		createTestMessage(t, queries, sessionID, msgID, "tool", content)
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 12000,
		})
		require.NoError(t, err)
	}

	mockStore := newMockReplacementStore()
	mockStore.byPosition[1] = []ContentReplacement{
		{SessionID: sessionID, Position: 1, State: ReplacementPinned},
	}

	m := NewMicroCompactor(MicroCompactorConfig{
		Store:            store,
		SessionID:        sessionID,
		ReplacementStore: mockStore,
		Round:            1,
	})

	result, err := m.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 2, result.ItemsAffected)

	recorded := mockStore.getRecorded()
	require.Len(t, recorded, 2)

	require.Equal(t, int64(0), recorded[0].Position)
	require.Equal(t, int64(2), recorded[1].Position)
}
