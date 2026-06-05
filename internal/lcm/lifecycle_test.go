package lcm

import (
	"context"
	"maps"
	"sync"
	"sync/atomic"
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
	maps.Copy(cp, sess)
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
	mgr.SetOperationalMemoryEnabled(true)

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
	mgr.SetOperationalMemoryEnabled(true)

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
	mgr.SetOperationalMemoryEnabled(true)

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

func TestOperationalMemoryConfig(t *testing.T) {
	t.Parallel()

	t.Run("enabled_allows_lifecycle_hooks", func(t *testing.T) {
		t.Parallel()
		queries, sqlDB := setupTestDB(t)
		mgr := NewManager(queries, sqlDB)
		om := newMockOMStore()
		mgr.SetOperationalMemory(om)
		mgr.SetOperationalMemoryEnabled(true)

		ctx := context.Background()
		sessionID := "sess-om-enabled"
		createTestSession(t, queries, sessionID)

		err := mgr.OnSessionStart(ctx, sessionID)
		require.NoError(t, err)

		val, ok, err := om.Get(ctx, sessionID, "session_started_at")
		require.NoError(t, err)
		require.True(t, ok)
		require.NotEmpty(t, val)

		entries, err := om.List(ctx, sessionID)
		require.NoError(t, err)
		require.NotEmpty(t, entries)

		err = mgr.OnSessionEnd(ctx, sessionID)
		require.NoError(t, err)

		val, ok, err = om.Get(ctx, sessionID, "session_ended_at")
		require.NoError(t, err)
		require.True(t, ok)
		require.NotEmpty(t, val)
	})

	t.Run("disabled_blocks_lifecycle_hooks", func(t *testing.T) {
		t.Parallel()
		queries, sqlDB := setupTestDB(t)
		mgr := NewManager(queries, sqlDB)
		om := newMockOMStore()
		mgr.SetOperationalMemory(om)
		// operationalMemEnabled defaults to false — do not call
		// SetOperationalMemoryEnabled.

		ctx := context.Background()
		sessionID := "sess-om-disabled"
		createTestSession(t, queries, sessionID)

		err := mgr.OnSessionStart(ctx, sessionID)
		require.NoError(t, err)

		_, ok, err := om.Get(ctx, sessionID, "session_started_at")
		require.NoError(t, err)
		require.False(t, ok, "expected no start marker when disabled")

		err = mgr.OnSessionEnd(ctx, sessionID)
		require.NoError(t, err)

		_, ok, err = om.Get(ctx, sessionID, "session_ended_at")
		require.NoError(t, err)
		require.False(t, ok, "expected no end marker when disabled")
	})

	t.Run("default_is_disabled", func(t *testing.T) {
		t.Parallel()
		queries, sqlDB := setupTestDB(t)
		mgr := NewManager(queries, sqlDB)
		cm := mgr.(*compactionManager)
		require.False(t, cm.operationalMemEnabled)
	})
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

func populateAllSyncMaps(cm *compactionManager, sessionID string) {
	cm.inFlight.Store(sessionID, struct{}{})
	cm.inFlight.Delete(sessionID)
	cm.budgetCache.Store(sessionID, Budget{SoftThreshold: 1000})
	cm.repoMapTokens.Store(sessionID, int64(500))
	cm.sessionMu.Store(sessionID, &ctxMutex{})
	cm.providerState.Store(sessionID, &providerTokenState{promptTokens: 42})
	cm.turnCounter.Store(sessionID, &atomic.Int64{})
	cm.iterationCounter.Store(sessionID, &atomic.Int64{})
}

func assertMapHasEntry(t *testing.T, m *sync.Map, sessionID, label string) {
	t.Helper()
	_, ok := m.Load(sessionID)
	require.True(t, ok, "expected %s entry for session %s", label, sessionID)
}

func assertMapNoEntry(t *testing.T, m *sync.Map, sessionID, label string) {
	t.Helper()
	_, ok := m.Load(sessionID)
	require.False(t, ok, "expected no %s entry for session %s", label, sessionID)
}

func TestOnSessionEnd_CleansUpSyncMaps(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	cm := mgr.(*compactionManager)

	sessionID := "sess-cleanup"
	ctx := context.Background()

	populateAllSyncMaps(cm, sessionID)

	assertMapHasEntry(t, &cm.budgetCache, sessionID, "budgetCache")
	assertMapHasEntry(t, &cm.repoMapTokens, sessionID, "repoMapTokens")
	assertMapHasEntry(t, &cm.sessionMu, sessionID, "sessionMu")
	assertMapHasEntry(t, &cm.providerState, sessionID, "providerState")
	assertMapHasEntry(t, &cm.turnCounter, sessionID, "turnCounter")
	assertMapHasEntry(t, &cm.iterationCounter, sessionID, "iterationCounter")

	err := cm.OnSessionEnd(ctx, sessionID)
	require.NoError(t, err)

	assertMapNoEntry(t, &cm.inFlight, sessionID, "inFlight")
	assertMapNoEntry(t, &cm.budgetCache, sessionID, "budgetCache")
	assertMapNoEntry(t, &cm.repoMapTokens, sessionID, "repoMapTokens")
	assertMapNoEntry(t, &cm.sessionMu, sessionID, "sessionMu")
	assertMapNoEntry(t, &cm.providerState, sessionID, "providerState")
	assertMapNoEntry(t, &cm.turnCounter, sessionID, "turnCounter")
	assertMapNoEntry(t, &cm.iterationCounter, sessionID, "iterationCounter")
}

func TestOnSessionEnd_PreservesOtherSessions(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	cm := mgr.(*compactionManager)

	sessionA := "sess-preserve-a"
	sessionB := "sess-preserve-b"
	ctx := context.Background()

	populateAllSyncMaps(cm, sessionA)
	populateAllSyncMaps(cm, sessionB)

	err := cm.OnSessionEnd(ctx, sessionA)
	require.NoError(t, err)

	assertMapNoEntry(t, &cm.budgetCache, sessionA, "budgetCache")
	assertMapNoEntry(t, &cm.providerState, sessionA, "providerState")

	assertMapHasEntry(t, &cm.budgetCache, sessionB, "budgetCache")
	assertMapHasEntry(t, &cm.repoMapTokens, sessionB, "repoMapTokens")
	assertMapHasEntry(t, &cm.sessionMu, sessionB, "sessionMu")
	assertMapHasEntry(t, &cm.providerState, sessionB, "providerState")
	assertMapHasEntry(t, &cm.turnCounter, sessionB, "turnCounter")
	assertMapHasEntry(t, &cm.iterationCounter, sessionB, "iterationCounter")
}

func TestOnSessionEnd_SkipsIfInFlight(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	cm := mgr.(*compactionManager)

	sessionID := "sess-inflight"
	ctx := context.Background()

	populateAllSyncMaps(cm, sessionID)

	cm.inFlight.Store(sessionID, struct{}{})

	err := cm.OnSessionEnd(ctx, sessionID)
	require.NoError(t, err)

	assertMapHasEntry(t, &cm.budgetCache, sessionID, "budgetCache")
	assertMapHasEntry(t, &cm.repoMapTokens, sessionID, "repoMapTokens")
	assertMapHasEntry(t, &cm.sessionMu, sessionID, "sessionMu")
	assertMapHasEntry(t, &cm.providerState, sessionID, "providerState")
	assertMapHasEntry(t, &cm.turnCounter, sessionID, "turnCounter")
	assertMapHasEntry(t, &cm.iterationCounter, sessionID, "iterationCounter")
}
