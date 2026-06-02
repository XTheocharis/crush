package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/require"
)

func TestIntegration_LayeredCompaction(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	llm := &mockLLMClient{response: `[{"event":"test","context":"ctx","implication":"impl"}]`}
	mgr := NewManagerWithLLM(queries, sqlDB, llm)
	cm := mgr.(*compactionManager)
	ctx := context.Background()

	sessionID := "sess-layered-integration"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// Insert messages that will create enough tokens to be visible.
	for i := range 5 {
		msgID := fmt.Sprintf("msg-layer-%d", i)
		text := fmt.Sprintf("Layer integration test message %d with enough content for token estimation", i)
		createTestMessage(t, queries, sessionID, msgID, "user", text)
		require.NoError(t, queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 500,
		}))
	}

	t.Run("RunLayeredCompaction", func(t *testing.T) {
		t.Parallel()
		result, err := mgr.RunLayeredCompaction(ctx, sessionID)
		require.NoError(t, err)
		require.NotNil(t, result)
		// Result may or may not have action taken depending on thresholds,
		// but the call should succeed without error.
	})

	t.Run("InjectCuesIntoPrompt", func(t *testing.T) {
		t.Parallel()
		cueInjector := NewCueInjector()
		cues := []GhostCue{
			cueInjector.NewCue(CueTypeSummaryID, 10, map[string]string{
				"SummaryID": "sum_test123",
				"Snippet":   "test snippet",
			}),
			cueInjector.NewCue(CueTypeLineagePointer, 5, map[string]string{
				"ParentIDs": "sum_parent1,sum_parent2",
				"Depth":     "2",
			}),
		}

		injected := mgr.InjectCuesIntoPrompt("base prompt", cues, 1000)
		require.Contains(t, injected, "base prompt")
		require.Contains(t, injected, "sum_test123")
	})

	t.Run("BuildCompactPrompt", func(t *testing.T) {
		t.Parallel()
		prompt, err := mgr.BuildCompactPrompt(ctx, sessionID)
		require.NoError(t, err)
		require.Contains(t, prompt, "system-instructions")
	})

	t.Run("PostCompactionHook", func(t *testing.T) {
		t.Parallel()
		// Should not panic or error even with no observation data.
		mgr.PostCompactionHook(ctx, sessionID)
	})

	t.Run("PostTurnHook", func(t *testing.T) {
		t.Parallel()
		// Should be a no-op without operational memory.
		mgr.PostTurnHook(ctx, sessionID)
	})

	t.Run("GetPressureTier", func(t *testing.T) {
		t.Parallel()
		tier, err := cm.GetPressureTier(ctx, sessionID)
		require.NoError(t, err)
		require.Equal(t, PressureLow, tier)
	})

	t.Run("SetProviderType", func(t *testing.T) {
		t.Parallel()
		mgr.SetProviderType("anthropic")
		// Verify it propagates to layer construction.
		result, err := mgr.RunLayeredCompaction(ctx, sessionID)
		require.NoError(t, err)
		require.NotNil(t, result)
	})
}

func TestIntegration_LayeredCompaction_WithPreservedContext(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	cm := mgr.(*compactionManager)
	ctx := context.Background()

	sessionID := "sess-preserved-ctx"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// Save preserved context.
	pc := &PreservedContext{
		SystemPromptContext: "important system context",
		ActiveFiles:         []string{"main.go", "utils.go"},
		RepoMapContent:      "repo map data",
		ToolState:           "tool state data",
	}
	cm.PreserveContext(sessionID, pc)

	// PostCompactionHook should restore and clear the preserved context.
	mgr.PostCompactionHook(ctx, sessionID)

	// Verify preserved context was cleared.
	loaded := cm.preservedContextStore.Load(sessionID)
	require.Nil(t, loaded)
}

func TestIntegration_LayeredCompaction_ObservationCoordination(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	llm := &mockLLMClient{
		response: `[{"event":"decision","context":"chose SQLite","implication":"migration needed"}]`,
	}
	mgr := NewManagerWithLLM(queries, sqlDB, llm)
	cm := mgr.(*compactionManager)
	ctx := context.Background()

	sessionID := "sess-obs-coord"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// Insert enough messages to exceed observation threshold.
	for i := range 10 {
		msgID := fmt.Sprintf("msg-obs-%d", i)
		text := fmt.Sprintf("Observation test message %d with sufficient content for token estimation purposes", i)
		createTestMessage(t, queries, sessionID, msgID, "user", text)
		require.NoError(t, queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 4000,
		}))
	}

	// Observation should trigger when over threshold.
	shouldObserve, err := cm.observer.ShouldObserve(ctx, sessionID)
	require.NoError(t, err)
	require.True(t, shouldObserve)

	// Run observation and verify it produces results.
	resultCh := cm.observer.Observe(ctx, sessionID)
	require.NotNil(t, resultCh)
	result := <-resultCh
	require.NoError(t, result.Error)
	require.Len(t, result.Observations, 1)
	require.Equal(t, "decision", result.Observations[0].Event)

	// Verify observations are retrievable.
	observations, err := cm.Observations(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, observations, 1)
}

func TestIntegration_LayeredCompaction_GhostCueInjection(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)

	cueInjector := NewCueInjector()
	cues := []GhostCue{
		cueInjector.NewCue(CueTypeSummaryID, 10, map[string]string{
			"SummaryID": "sum_abc123",
			"Snippet":   "some summary snippet",
		}),
		cueInjector.NewCue(CueTypeArchiveStub, 5, map[string]string{
			"SummaryID":  "sum_def456",
			"TokenCount": "5000",
		}),
	}

	// Inject into prompt.
	prompt := mgr.InjectCuesIntoPrompt("system instructions here", cues, 500)
	require.Contains(t, prompt, "system instructions here")
	require.Contains(t, prompt, "sum_abc123")
	require.Contains(t, prompt, "sum_def456")

	// Budget-constrained injection should drop lower-priority cues.
	smallBudget := mgr.InjectCuesIntoPrompt("base", cues, 5)
	require.Contains(t, smallBudget, "base")
}

func TestIntegration_LayeredCompaction_CompactPromptBuilder(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-prompt-build"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// Insert a summary and a message.
	summaryID := "sum_buildtest123456"
	require.NoError(t, queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  summaryID,
		SessionID:  sessionID,
		Kind:       KindLeaf,
		Content:    "Test summary content for prompt building",
		TokenCount: 50,
		FileIds:    "[]",
	}))
	require.NoError(t, queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "summary",
		SummaryID:  sql.NullString{String: summaryID, Valid: true},
		TokenCount: 50,
	}))

	msgID := "msg-prompt-1"
	createTestMessage(t, queries, sessionID, msgID, "user", "prompt build test")
	require.NoError(t, queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   1,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 100,
	}))

	prompt, err := mgr.BuildCompactPrompt(ctx, sessionID)
	require.NoError(t, err)
	require.Contains(t, prompt, "system-instructions")
}

func TestIntegration_LayeredCompaction_PressureTiers(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	cm := mgr.(*compactionManager)
	ctx := context.Background()

	sessionID := "sess-pressure"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// No messages — should be PressureLow.
	tier, err := cm.GetPressureTier(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, PressureLow, tier)

	// Insert enough tokens to trigger medium pressure.
	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)
	cfg := DefaultPressureConfig()
	compactThreshold := budget.ContextWindow - cfg.CompactOffset
	msgID := "msg-pressure-big"
	createTestMessage(t, queries, sessionID, msgID, "user", "pressure test")
	require.NoError(t, queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: compactThreshold + 1000,
	}))

	// Set provider-reported tokens to simulate being over medium threshold.
	mgr.SetActualPromptTokens(sessionID, compactThreshold+1000)
	tier, err = cm.GetPressureTier(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, PressureMedium, tier)
}

func TestIntegration_LayeredCompaction_OperationalMemory(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)

	// Use a mock operational memory store.
	om := &mockOperationalMemory{data: make(map[string]map[string]string)}
	mgr.SetOperationalMemory(om)

	ctx := context.Background()
	sessionID := "sess-opmem"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// PostTurnHook should be a no-op without auto-memory extractor.
	mgr.PostTurnHook(ctx, sessionID)

	// Operational memory should remain empty (auto-memory not yet wired).
	kv, err := om.List(ctx, sessionID)
	require.NoError(t, err)
	require.Empty(t, kv)
}

func TestIntegration_LayeredCompaction_ConcurrentSafety(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	cm := mgr.(*compactionManager)
	ctx := context.Background()

	sessionID := "sess-concurrent"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// Insert initial messages.
	for i := range 5 {
		msgID := fmt.Sprintf("msg-conc-%d", i)
		createTestMessage(t, queries, sessionID, msgID, "user", "concurrent test")
		require.NoError(t, queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 100,
		}))
	}

	// Run multiple operations concurrently to verify thread safety.
	const workers = 10
	var wg sync.WaitGroup
	errCh := make(chan error, workers*4)

	for i := range workers {
		wg.Add(4)
		go func(idx int) {
			defer wg.Done()
			_, err := mgr.RunLayeredCompaction(ctx, sessionID)
			if err != nil {
				errCh <- err
			}
		}(i)
		go func(idx int) {
			defer wg.Done()
			_, err := mgr.BuildCompactPrompt(ctx, sessionID)
			if err != nil {
				errCh <- err
			}
		}(i)
		go func(idx int) {
			defer wg.Done()
			_, err := cm.GetPressureTier(ctx, sessionID)
			if err != nil {
				errCh <- err
			}
		}(i)
		go func(idx int) {
			defer wg.Done()
			mgr.PostCompactionHook(ctx, sessionID)
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

// mockOperationalMemory is a test double for OperationalMemoryStore.
type mockOperationalMemory struct {
	mu   sync.Mutex
	data map[string]map[string]string
}

func (m *mockOperationalMemory) Get(_ context.Context, sessionID, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	kv, ok := m.data[sessionID]
	if !ok {
		return "", false, nil
	}
	v, ok := kv[key]
	return v, ok, nil
}

func (m *mockOperationalMemory) Set(_ context.Context, sessionID, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[sessionID] == nil {
		m.data[sessionID] = make(map[string]string)
	}
	m.data[sessionID][key] = value
	return nil
}

func (m *mockOperationalMemory) Delete(_ context.Context, sessionID, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if kv, ok := m.data[sessionID]; ok {
		delete(kv, key)
	}
	return nil
}

func (m *mockOperationalMemory) List(_ context.Context, sessionID string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	kv, ok := m.data[sessionID]
	if !ok {
		return map[string]string{}, nil
	}
	result := make(map[string]string, len(kv))
	maps.Copy(result, kv)
	return result, nil
}

func (m *mockOperationalMemory) ListByPriority(_ context.Context, _ string) ([]session.OMEntry, error) {
	return nil, nil
}

func TestTierCompactorWiring(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	llm := &mockLLMClient{response: "summary response"}
	mgr := NewManagerWithLLM(queries, sqlDB, llm)
	cm := mgr.(*compactionManager)

	sessionID := "sess-tier-wiring"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(context.Background(), sessionID))

	// Build the session layer manager which should wire compactors.
	lm := cm.newSessionLayerManager(sessionID)
	require.NotNil(t, lm)

	// Find the PressureCompactionSelector among the layers.
	layers := lm.Layers()
	var selector *PressureCompactionSelector
	for _, l := range layers {
		if s, ok := l.(*PressureCompactionSelector); ok {
			selector = s
			break
		}
	}
	require.NotNil(t, selector, "PressureCompactionSelector must be present in layer manager")

	t.Run("Low tier has only micro-compactor", func(t *testing.T) {
		t.Parallel()
		lowLayers := selector.SelectLayers(PressureLow)
		require.Len(t, lowLayers, 1)
		require.Equal(t, "micro-compactor", lowLayers[0].Name())
	})

	t.Run("Medium tier has session-compactor first then micro-compactor", func(t *testing.T) {
		t.Parallel()
		medLayers := selector.SelectLayers(PressureMedium)
		require.Len(t, medLayers, 2)
		require.Equal(t, "session-compactor", medLayers[0].Name())
		require.Equal(t, "micro-compactor", medLayers[1].Name())
	})

	t.Run("High tier has full-compactor first then micro-compactor", func(t *testing.T) {
		t.Parallel()
		highLayers := selector.SelectLayers(PressureHigh)
		require.Len(t, highLayers, 2)
		require.Equal(t, "full-compactor", highLayers[0].Name())
		require.Equal(t, "micro-compactor", highLayers[1].Name())
	})
}

func TestTurnCounterNoRace(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	cm := mgr.(*compactionManager)
	cm.autoMemoryExtractor = NewAutoMemoryExtractor(nil, nil, 0)

	ctx := context.Background()
	sessionID := "sess-turn-race"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			mgr.PostTurnHook(ctx, sessionID)
		}()
	}

	wg.Wait()

	val, ok := cm.turnCounter.Load(sessionID)
	require.True(t, ok)
	counter := val.(*atomic.Int64)
	require.Equal(t, int64(goroutines), counter.Load())
}
