package lcm

import (
	"context"
	"sync"
	"testing"

	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/require"
)

type mockOMStore struct {
	mu      sync.Mutex
	entries map[string]map[string]string
}

func newMockOMStore() *mockOMStore {
	return &mockOMStore{
		entries: make(map[string]map[string]string),
	}
}

func (m *mockOMStore) Get(_ context.Context, sessionID, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.entries[sessionID]
	if !ok {
		return "", false, nil
	}
	v, ok := sess[key]
	return v, ok, nil
}

func (m *mockOMStore) Set(_ context.Context, sessionID, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.entries[sessionID]; !ok {
		m.entries[sessionID] = make(map[string]string)
	}
	m.entries[sessionID][key] = value
	return nil
}

func (m *mockOMStore) Delete(_ context.Context, sessionID, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries[sessionID], key)
	return nil
}

func (m *mockOMStore) List(_ context.Context, sessionID string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.entries[sessionID]
	if !ok {
		return map[string]string{}, nil
	}
	cp := make(map[string]string, len(sess))
	for k, v := range sess {
		cp[k] = v
	}
	return cp, nil
}

func (m *mockOMStore) ListByPriority(_ context.Context, _ string) ([]session.OMEntry, error) {
	return nil, nil
}

func TestOnSessionStart_SetsMarker(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	om := newMockOMStore()
	mgr.SetOperationalMemory(om)

	ctx := context.Background()
	sessionID := "sess-lifecycle-start"
	createTestSession(t, queries, sessionID)

	err := mgr.OnSessionStart(ctx, sessionID)
	require.NoError(t, err)

	val, ok, err := om.Get(ctx, sessionID, "session_started_at")
	require.NoError(t, err)
	require.True(t, ok, "expected session_started_at key to be set")
	require.NotEmpty(t, val)
}

func TestOnSessionStart_NilOM_NoPanic(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)

	ctx := context.Background()
	sessionID := "sess-lifecycle-nil"

	err := mgr.OnSessionStart(ctx, sessionID)
	require.NoError(t, err)
}

func TestOnSessionEnd_SetsEndMarker(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	om := newMockOMStore()
	mgr.SetOperationalMemory(om)

	ctx := context.Background()
	sessionID := "sess-lifecycle-end"

	err := mgr.OnSessionStart(ctx, sessionID)
	require.NoError(t, err)

	err = mgr.OnSessionEnd(ctx, sessionID)
	require.NoError(t, err)

	val, ok, err := om.Get(ctx, sessionID, "session_ended_at")
	require.NoError(t, err)
	require.True(t, ok, "expected session_ended_at key to be set")
	require.NotEmpty(t, val)
}

func TestOnSessionEnd_EmptyEntries_NoEndMarker(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	om := newMockOMStore()
	mgr.SetOperationalMemory(om)

	ctx := context.Background()
	sessionID := "sess-lifecycle-empty-end"

	err := mgr.OnSessionEnd(ctx, sessionID)
	require.NoError(t, err)

	_, ok, err := om.Get(ctx, sessionID, "session_ended_at")
	require.NoError(t, err)
	require.False(t, ok, "expected no session_ended_at key when entries are empty")
}

func TestOnSessionEnd_NilOM_NoPanic(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)

	ctx := context.Background()
	sessionID := "sess-lifecycle-nil-end"

	err := mgr.OnSessionEnd(ctx, sessionID)
	require.NoError(t, err)
}

func TestBuildObservationContextPrompt_Empty(t *testing.T) {
	t.Parallel()
	result := BuildObservationContextPrompt(map[string]string{})
	require.Equal(t, "", result)
}

func TestBuildObservationContextPrompt_Nil(t *testing.T) {
	t.Parallel()
	result := BuildObservationContextPrompt(nil)
	require.Equal(t, "", result)
}

func TestBuildObservationContextPrompt_WithEntries(t *testing.T) {
	t.Parallel()
	entries := map[string]string{
		"zeta_key":  "value_z",
		"alpha_key": "value_a",
		"mid_key":   "value_m",
	}
	result := BuildObservationContextPrompt(entries)
	require.Contains(t, result, "## Operational Memory")
	require.Contains(t, result, "- **alpha_key**: value_a")
	require.Contains(t, result, "- **mid_key**: value_m")
	require.Contains(t, result, "- **zeta_key**: value_z")

	expected := "## Operational Memory\n\n" +
		"- **alpha_key**: value_a\n" +
		"- **mid_key**: value_m\n" +
		"- **zeta_key**: value_z\n"
	require.Equal(t, expected, result)
}
