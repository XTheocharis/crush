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
// CompactionLayer interface compliance
// ---------------------------------------------------------------------------

func TestSessionCompactor_ImplementsCompactionLayer(t *testing.T) {
	t.Parallel()
	var _ CompactionLayer = (*SessionCompactor)(nil)
}

// ---------------------------------------------------------------------------
// Name and Priority
// ---------------------------------------------------------------------------

func TestSessionCompactor_Name(t *testing.T) {
	t.Parallel()
	s := NewSessionCompactor(SessionCompactorConfig{})
	require.Equal(t, "session-compactor", s.Name())
}

func TestSessionCompactor_Priority(t *testing.T) {
	t.Parallel()
	s := NewSessionCompactor(SessionCompactorConfig{})
	require.Equal(t, 20, s.Priority())
}

// ---------------------------------------------------------------------------
// ShouldCompact
// ---------------------------------------------------------------------------

func TestSessionCompactor_ShouldCompact_NilStore(t *testing.T) {
	t.Parallel()
	s := NewSessionCompactor(SessionCompactorConfig{
		LLM:       &mockLLMClient{},
		SessionID: "s1",
	})
	require.False(t, s.ShouldCompact(context.Background(), Budget{}))
}

func TestSessionCompactor_ShouldCompact_NilLLM(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	s := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		SessionID: "s1",
	})
	require.False(t, s.ShouldCompact(context.Background(), Budget{}))
}

func TestSessionCompactor_ShouldCompact_EmptySessionID(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	s := NewSessionCompactor(SessionCompactorConfig{
		Store: store,
		LLM:   &mockLLMClient{},
	})
	require.False(t, s.ShouldCompact(context.Background(), Budget{}))
}

func TestSessionCompactor_ShouldCompact_NotEnoughTokens(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-sc-low"
	createTestSession(t, queries, sessionID)

	msgID := "msg-low"
	createTestMessage(t, queries, sessionID, msgID, "assistant", "small content")
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 1000,
	})
	require.NoError(t, err)

	store := newStore(queries, sqlDB)
	s := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		LLM:       &mockLLMClient{},
		SessionID: sessionID,
	})

	// Token count is 1000, well below default minBudget (50000).
	require.False(t, s.ShouldCompact(ctx, Budget{}))
}

func TestSessionCompactor_ShouldCompact_BelowSoftThreshold(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-sc-below-soft"
	createTestSession(t, queries, sessionID)

	// Insert enough tokens to exceed minBudget.
	msgID := "msg-big"
	createTestMessage(t, queries, sessionID, msgID, "assistant", strings.Repeat("x ", 300000))
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 80000,
	})
	require.NoError(t, err)

	store := newStore(queries, sqlDB)
	s := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		LLM:       &mockLLMClient{},
		SessionID: sessionID,
	})

	// Soft threshold at 100K, tokens at 80K — no pressure yet.
	require.False(t, s.ShouldCompact(ctx, Budget{
		SoftThreshold: 100000,
		HardLimit:     120000,
		ContextWindow: 200000,
	}))
}

func TestSessionCompactor_ShouldCompact_OverSoftThreshold(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-sc-over-soft"
	createTestSession(t, queries, sessionID)

	msgID := "msg-big"
	createTestMessage(t, queries, sessionID, msgID, "assistant", strings.Repeat("x ", 300000))
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 80000,
	})
	require.NoError(t, err)

	store := newStore(queries, sqlDB)
	s := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		LLM:       &mockLLMClient{},
		SessionID: sessionID,
	})

	// Soft threshold at 60K, tokens at 80K — over threshold = pressure.
	require.True(t, s.ShouldCompact(ctx, Budget{
		SoftThreshold: 60000,
		HardLimit:     80000,
		ContextWindow: 200000,
	}))
}

func TestSessionCompactor_ShouldCompact_ExistingSessionMemory(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-sc-existing"
	createTestSession(t, queries, sessionID)

	msgID := "msg-big"
	createTestMessage(t, queries, sessionID, msgID, "assistant", strings.Repeat("x ", 300000))
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 80000,
	})
	require.NoError(t, err)

	// Insert an existing session-memory summary.
	summaryID := "sum_existing"
	err = queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  summaryID,
		SessionID:  sessionID,
		Kind:       KindSessionMemory,
		Content:    "existing memory",
		TokenCount: 10,
		FileIds:    "[]",
	})
	require.NoError(t, err)
	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   1,
		ItemType:   "summary",
		SummaryID:  sql.NullString{String: summaryID, Valid: true},
		TokenCount: 10,
	})
	require.NoError(t, err)

	store := newStore(queries, sqlDB)
	s := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		LLM:       &mockLLMClient{},
		SessionID: sessionID,
	})

	require.False(t, s.ShouldCompact(ctx, Budget{
		SoftThreshold: 60000,
		HardLimit:     80000,
		ContextWindow: 200000,
	}))
}

// ---------------------------------------------------------------------------
// Compact
// ---------------------------------------------------------------------------

func TestSessionCompactor_Compact_NilStore_ReturnsError(t *testing.T) {
	t.Parallel()
	s := NewSessionCompactor(SessionCompactorConfig{
		LLM:       &mockLLMClient{},
		SessionID: "s1",
	})
	_, err := s.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrStoreIsNil))
}

func TestSessionCompactor_Compact_NilLLM_ReturnsError(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	s := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		SessionID: "s1",
	})
	_, err := s.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrLLMClientNil))
}

func TestSessionCompactor_Compact_EmptySessionID_ReturnsError(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	s := NewSessionCompactor(SessionCompactorConfig{
		Store: store,
		LLM:   &mockLLMClient{},
	})
	_, err := s.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSessionIDEmpty))
}

func TestSessionCompactor_Compact_EmptyEntries_NoAction(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-sc-empty"
	createTestSession(t, queries, sessionID)
	store := newStore(queries, sqlDB)

	llm := &mockLLMClient{response: "## Decisions\nNone"}
	s := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		LLM:       llm,
		SessionID: sessionID,
	})

	result, err := s.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
	require.Equal(t, int64(0), result.TokensFreed)
	require.Equal(t, 0, llm.callCount)
}

func TestSessionCompactor_Compact_Success(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-sc-success"
	createTestSession(t, queries, sessionID)

	// Insert messages to create a substantial context.
	for i := range 5 {
		msgID := fmt.Sprintf("msg-sc-%d", i)
		content := fmt.Sprintf("Message %d content with details about decision %d", i, i)
		createTestMessage(t, queries, sessionID, msgID, "assistant", content)
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 10000,
		})
		require.NoError(t, err)
	}

	store := newStore(queries, sqlDB)
	llmResponse := `## Decisions
- Decided to use structured markdown for session memory compaction output
- Chose Layer 2 priority of 20, after MicroCompactor at priority 1
- Token budget target range: 10K-40K tokens, scaled by context pressure
- Output format uses four markdown sections for structured extraction

## Patterns
- Context entries use position-based ordering within the LCM framework
- Summaries have kind-based classification (leaf, condensed, session)
- CompactionLayer interface requires Name, Priority, ShouldCompact, Compact
- Token estimation uses CharsPerToken = 4 with ceiling division
- All config fields provide zero-value defaults via helper methods

## Errors
- None encountered during implementation
- Guard against tiny LLM outputs (under 100 tokens treated as failure)

## Current State
- Session memory compactor implementation complete
- Files modified: internal/lcm/session_compactor.go, internal/lcm/session_compactor_test.go
- Interface: CompactionLayer with Name="session-compactor", Priority=20
- Registered as Layer 2 in CompactionLayerManager alongside MicroCompactor (Layer 1)
- ShouldCompact checks: store/LLM/sessionID configured, token count above minBudget, context pressure, no duplicate
- Compact gathers all context entries, formats prompt, calls LLM, estimates tokens freed`

	llm := &mockLLMClient{response: llmResponse}
	s := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		LLM:       llm,
		SessionID: sessionID,
	})

	result, err := s.Compact(ctx, Budget{
		SoftThreshold: 60000,
		HardLimit:     80000,
		ContextWindow: 200000,
	})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 5, result.ItemsAffected)
	require.True(t, result.TokensFreed > 0)
	require.Equal(t, 1, llm.callCount)

	// Verify the LLM was called with the right system prompt.
	// The response should be non-trivial.
	resultTokens := EstimateTokens(llmResponse)
	expectedFreed := max(int64(50000)-resultTokens, 0)
	require.Equal(t, expectedFreed, result.TokensFreed)
}

func TestSessionCompactor_Compact_LLMError(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-sc-llm-err"
	createTestSession(t, queries, sessionID)

	msgID := "msg-sc-err"
	createTestMessage(t, queries, sessionID, msgID, "assistant", "content")
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 1000,
	})
	require.NoError(t, err)

	store := newStore(queries, sqlDB)
	llm := &mockLLMClient{err: fmt.Errorf("LLM unavailable")}
	s := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		LLM:       llm,
		SessionID: sessionID,
	})

	_, err = s.Compact(ctx, Budget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "generating session memory")
	require.Contains(t, err.Error(), "LLM unavailable")
}

func TestSessionCompactor_Compact_TinyLLMOutput_NoAction(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-sc-tiny"
	createTestSession(t, queries, sessionID)

	msgID := "msg-sc-tiny"
	createTestMessage(t, queries, sessionID, msgID, "assistant", "substantial content here")
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 5000,
	})
	require.NoError(t, err)

	store := newStore(queries, sqlDB)
	// LLM returns a tiny response (under 100 tokens).
	llm := &mockLLMClient{response: "ok"}
	s := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		LLM:       llm,
		SessionID: sessionID,
	})

	result, err := s.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
}

// ---------------------------------------------------------------------------
// Config defaults
// ---------------------------------------------------------------------------

func TestSessionCompactorConfig_Defaults(t *testing.T) {
	t.Parallel()

	cfg := SessionCompactorConfig{}
	require.Equal(t, int64(sessionMemoryMinBudget), cfg.minBudget())
	require.Equal(t, int64(sessionMemoryMaxTokens), cfg.maxTokens())
	require.Equal(t, int64(sessionMemoryMinTokens), cfg.minTokens())
}

func TestSessionCompactorConfig_CustomOverrides(t *testing.T) {
	t.Parallel()

	cfg := SessionCompactorConfig{
		MinTokenBudget:  100000,
		MaxOutputTokens: 30000,
		MinOutputTokens: 5000,
	}
	require.Equal(t, int64(100000), cfg.minBudget())
	require.Equal(t, int64(30000), cfg.maxTokens())
	require.Equal(t, int64(5000), cfg.minTokens())
}

// ---------------------------------------------------------------------------
// targetTokens
// ---------------------------------------------------------------------------

func TestSessionCompactor_TargetTokens_ZeroBudget(t *testing.T) {
	t.Parallel()
	s := NewSessionCompactor(SessionCompactorConfig{})
	require.Equal(t, int64(sessionMemoryMinTokens), s.targetTokens(Budget{}))
}

func TestSessionCompactor_TargetTokens_ScaledByPressure(t *testing.T) {
	t.Parallel()
	s := NewSessionCompactor(SessionCompactorConfig{})

	// SoftThreshold is 50% of ContextWindow → ratio = 0.5 → target = 20000.
	target := s.targetTokens(Budget{
		SoftThreshold: 100000,
		ContextWindow: 200000,
	})
	require.Equal(t, int64(20000), target)
}

func TestSessionCompactor_TargetTokens_HighPressure(t *testing.T) {
	t.Parallel()
	s := NewSessionCompactor(SessionCompactorConfig{})

	// SoftThreshold is 90% of ContextWindow → ratio = 0.9 → target = 36000.
	target := s.targetTokens(Budget{
		SoftThreshold: 180000,
		ContextWindow: 200000,
	})
	require.Equal(t, int64(36000), target)
}

func TestSessionCompactor_TargetTokens_LowPressureClampsToMin(t *testing.T) {
	t.Parallel()
	s := NewSessionCompactor(SessionCompactorConfig{})

	// Very low ratio → target clamped to minTokens.
	target := s.targetTokens(Budget{
		SoftThreshold: 1000,
		ContextWindow: 200000,
	})
	require.Equal(t, int64(sessionMemoryMinTokens), target)
}

// ---------------------------------------------------------------------------
// formatContextForSessionMemory
// ---------------------------------------------------------------------------

func TestFormatContextForSessionMemory_Messages(t *testing.T) {
	t.Parallel()

	entries := []ContextEntry{
		{ItemType: "message", MessageID: "m1", TokenCount: 100},
		{ItemType: "message", MessageID: "m2", TokenCount: 200},
	}

	result := formatContextForSessionMemory(entries)
	require.Contains(t, result, "<session-context>")
	require.Contains(t, result, "</session-context>")
	require.Contains(t, result, "m1")
	require.Contains(t, result, "m2")
}

func TestFormatContextForSessionMemory_Summaries(t *testing.T) {
	t.Parallel()

	entries := []ContextEntry{
		{ItemType: "summary", SummaryID: "s1", SummaryKind: "leaf", SummaryContent: "summary text", TokenCount: 50},
	}

	result := formatContextForSessionMemory(entries)
	require.Contains(t, result, "summary text")
	require.Contains(t, result, "leaf")
}

func TestFormatContextForSessionMemory_TruncatesLargeEntryCount(t *testing.T) {
	t.Parallel()

	entries := make([]ContextEntry, maxContextEntriesForPrompt+50)
	for i := range entries {
		entries[i] = ContextEntry{
			ItemType:   "message",
			MessageID:  fmt.Sprintf("msg-%d", i),
			TokenCount: 100,
		}
	}

	result := formatContextForSessionMemory(entries)
	require.Contains(t, result, "Remaining")
}

func TestFormatContextForSessionMemory_Empty(t *testing.T) {
	t.Parallel()

	result := formatContextForSessionMemory(nil)
	require.Contains(t, result, "<session-context>")
	require.Contains(t, result, "</session-context>")
}

// ---------------------------------------------------------------------------
// buildSessionMemorySystemPrompt
// ---------------------------------------------------------------------------

func TestBuildSessionMemorySystemPrompt_ContainsSections(t *testing.T) {
	t.Parallel()

	prompt := buildSessionMemorySystemPrompt(20000)
	require.Contains(t, prompt, "## Decisions")
	require.Contains(t, prompt, "## Patterns")
	require.Contains(t, prompt, "## Errors")
	require.Contains(t, prompt, "## Current State")
	require.Contains(t, prompt, "20000")
}

// ---------------------------------------------------------------------------
// Integration: LayerManager with SessionCompactor
// ---------------------------------------------------------------------------

func TestLayerManager_Integration_WithSessionCompactor(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-lm-integration"
	createTestSession(t, queries, sessionID)

	// Insert messages to exceed the budget.
	for i := range 3 {
		msgID := fmt.Sprintf("msg-int-%d", i)
		content := strings.Repeat(fmt.Sprintf("content %d ", i), 50000)
		createTestMessage(t, queries, sessionID, msgID, "assistant", content)
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 50000,
		})
		require.NoError(t, err)
	}

	store := newStore(queries, sqlDB)
	llmResponse := `## Decisions
- Decision 1: Use layered compaction framework for session memory
- Decision 2: Target 10K-40K token output range scaled by pressure
- Decision 3: Structured markdown with four standard sections

## Patterns
- Pattern 1: CompactionLayer interface with Name/Priority/ShouldCompact/Compact
- Pattern 2: Token estimation uses CharsPerToken ceiling division
- Pattern 3: Config structs provide zero-value defaults via helper methods

## Errors
- None encountered during session memory generation

## Current State
- Integration test verifying SessionCompactor with LayerManager
- Session has 3 messages at 50K tokens each (150K total)
- Compaction should free significant tokens by replacing with structured summary`
	llm := &mockLLMClient{response: llmResponse}

	session := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		LLM:       llm,
		SessionID: sessionID,
	})

	mgr := NewCompactionLayerManager(session)

	budget := Budget{SoftThreshold: 60000, HardLimit: 100000, ContextWindow: 200000}
	result, err := mgr.RunAll(ctx, budget)
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.True(t, result.TokensFreed > 0)
}

func TestLayerManager_Integration_BothLayers(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-both-layers"
	createTestSession(t, queries, sessionID)

	// Insert a large message (triggers micro-compactor) and enough total
	// tokens (triggers session-compactor).
	largeMsgID := "msg-large-both"
	largeContent := strings.Repeat("large output data ", 10000)
	createTestMessage(t, queries, sessionID, largeMsgID, "tool", largeContent)
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: largeMsgID, Valid: true},
		TokenCount: 50000,
	})
	require.NoError(t, err)

	store := newStore(queries, sqlDB)

	micro := NewMicroCompactor(MicroCompactorConfig{
		Store:     store,
		SessionID: sessionID,
	})

	llm := &mockLLMClient{response: `## Decisions
- Used both MicroCompactor (Layer 1) and SessionCompactor (Layer 2) together
- Layer ordering ensures micro-compaction runs before session memory

## Patterns
- CompactionLayerManager sorts layers by priority ascending
- Each layer independently decides eligibility via ShouldCompact

## Errors
- None encountered in dual-layer integration

## Current State
- Testing that both layers activate and aggregate results correctly
- Single large message at 50K tokens triggers both layers`}
	session := NewSessionCompactor(SessionCompactorConfig{
		Store:     store,
		LLM:       llm,
		SessionID: sessionID,
	})

	mgr := NewCompactionLayerManager(micro, session)
	layers := mgr.Layers()
	require.Len(t, layers, 2)
	require.Equal(t, "micro-compactor", layers[0].Name())
	require.Equal(t, "session-compactor", layers[1].Name())

	budget := Budget{SoftThreshold: 60000, HardLimit: 100000, ContextWindow: 200000}
	result, err := mgr.RunAll(ctx, budget)
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
}
