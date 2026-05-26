package lcm

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// CompactionLayer interface compliance
// ---------------------------------------------------------------------------

func TestCompactPromptLayer_ImplementsCompactionLayer(t *testing.T) {
	t.Parallel()
	var _ CompactionLayer = (*compactPromptLayer)(nil)
}

func TestAnthropicCacheLayer_ImplementsCompactionLayer(t *testing.T) {
	t.Parallel()
	var _ CompactionLayer = (*anthropicCacheLayer)(nil)
}

// ---------------------------------------------------------------------------
// CacheOptimizer construction
// ---------------------------------------------------------------------------

func TestNewCacheOptimizer(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{ProviderType: "anthropic"})
	require.NotNil(t, o)
}

func TestCacheOptimizer_Layers_ReturnsTwoLayers(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{ProviderType: "openai"})
	layers := o.Layers()
	require.Len(t, layers, 2)
	require.Equal(t, "compact-prompt-structure", layers[0].Name())
	require.Equal(t, "anthropic-cache-management", layers[1].Name())
	require.Equal(t, 60, layers[0].Priority())
	require.Equal(t, 70, layers[1].Priority())
}

// ---------------------------------------------------------------------------
// Layer 6: CompactPromptStructure — ShouldCompact / Compact
// ---------------------------------------------------------------------------

func TestCompactPromptLayer_ShouldCompact_NoStore(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{SessionID: "s1"})
	require.False(t, o.Layer6().ShouldCompact(context.Background(), Budget{}))
}

func TestCompactPromptLayer_ShouldCompact_EmptySessionID(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	o := NewCacheOptimizer(CacheOptimizerConfig{Store: store})
	require.False(t, o.Layer6().ShouldCompact(context.Background(), Budget{}))
}

func TestCompactPromptLayer_ShouldCompact_EnoughEntries(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-cache-enough"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	for i := range 3 {
		msgID := "msg-" + strings.Repeat("a", i)
		createTestMessage(t, queries, sessionID, msgID, "assistant", "hello")
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 100,
		})
		require.NoError(t, err)
	}

	o := NewCacheOptimizer(CacheOptimizerConfig{Store: store, SessionID: sessionID})
	require.True(t, o.Layer6().ShouldCompact(ctx, Budget{}))
}

func TestCompactPromptLayer_ShouldCompact_TooFewEntries(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-cache-few"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	msgID := "msg-single"
	createTestMessage(t, queries, sessionID, msgID, "assistant", "hello")
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 100,
	})
	require.NoError(t, err)

	o := NewCacheOptimizer(CacheOptimizerConfig{Store: store, SessionID: sessionID})
	require.False(t, o.Layer6().ShouldCompact(ctx, Budget{}))
}

func TestCompactPromptLayer_Compact_NilStore_ReturnsError(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{SessionID: "s1"})
	_, err := o.Layer6().Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrStoreIsNil))
}

func TestCompactPromptLayer_Compact_EmptySessionID_ReturnsError(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	o := NewCacheOptimizer(CacheOptimizerConfig{Store: store})
	_, err := o.Layer6().Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSessionIDEmpty))
}

func TestCompactPromptLayer_Compact_AssemblesSections(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-cache-compact"
	createTestSession(t, queries, sessionID)

	store := newStore(queries, sqlDB)

	for i := range 3 {
		msgID := "msg-" + strings.Repeat("b", i)
		content := strings.Repeat("test content ", 100)
		createTestMessage(t, queries, sessionID, msgID, "assistant", content)
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: int64(50 + i*10),
		})
		require.NoError(t, err)
	}

	o := NewCacheOptimizer(CacheOptimizerConfig{Store: store, SessionID: sessionID})
	result, err := o.Layer6().Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.True(t, result.ItemsAffected > 0)
	require.Equal(t, "compact-prompt-structure", result.LayerName)
}

// ---------------------------------------------------------------------------
// Layer 7: AnthropicCacheManagement
// ---------------------------------------------------------------------------

func TestAnthropicCacheLayer_ShouldCompact_NonAnthropic(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-cache-nonanthro"
	createTestSession(t, queries, sessionID)
	store := newStore(queries, sqlDB)

	// Add entries to make hasEnoughContext true.
	for i := range 3 {
		msgID := "msg-na-" + strings.Repeat("c", i)
		createTestMessage(t, queries, sessionID, msgID, "assistant", "hello")
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 100,
		})
		require.NoError(t, err)
	}

	o := NewCacheOptimizer(CacheOptimizerConfig{
		ProviderType: "openai",
		Store:        store,
		SessionID:    sessionID,
	})
	require.False(t, o.Layer7().ShouldCompact(ctx, Budget{}))
}

func TestAnthropicCacheLayer_ShouldCompact_Anthropic(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-cache-anthro"
	createTestSession(t, queries, sessionID)
	store := newStore(queries, sqlDB)

	for i := range 3 {
		msgID := "msg-anthro-" + strings.Repeat("d", i)
		createTestMessage(t, queries, sessionID, msgID, "assistant", "hello")
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 100,
		})
		require.NoError(t, err)
	}

	o := NewCacheOptimizer(CacheOptimizerConfig{
		ProviderType: "anthropic",
		Store:        store,
		SessionID:    sessionID,
	})
	require.True(t, o.Layer7().ShouldCompact(ctx, Budget{}))
}

func TestAnthropicCacheLayer_Compact_NonAnthropic_NoOp(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-cache-noop"
	createTestSession(t, queries, sessionID)
	store := newStore(queries, sqlDB)

	for i := range 3 {
		msgID := "msg-noop-" + strings.Repeat("e", i)
		createTestMessage(t, queries, sessionID, msgID, "assistant", "hello")
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 100,
		})
		require.NoError(t, err)
	}

	o := NewCacheOptimizer(CacheOptimizerConfig{
		ProviderType: "openai",
		Store:        store,
		SessionID:    sessionID,
	})
	result, err := o.Layer7().Compact(ctx, Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
}

func TestAnthropicCacheLayer_Compact_Anthropic_EstimatesSavings(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-cache-savings"
	createTestSession(t, queries, sessionID)
	store := newStore(queries, sqlDB)

	for i := range 3 {
		msgID := "msg-save-" + strings.Repeat("f", i)
		content := strings.Repeat("stable content for caching ", 100)
		createTestMessage(t, queries, sessionID, msgID, "assistant", content)
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: int64(100 + i*50),
		})
		require.NoError(t, err)
	}

	o := NewCacheOptimizer(CacheOptimizerConfig{
		ProviderType: "anthropic",
		Store:        store,
		SessionID:    sessionID,
	})
	result, err := o.Layer7().Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.True(t, result.TokensFreed > 0, "should estimate cache savings for Anthropic")
	require.Equal(t, "anthropic-cache-management", result.LayerName)
}

func TestAnthropicCacheLayer_Compact_NilStore_ReturnsError(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{
		ProviderType: "anthropic",
		SessionID:    "s1",
	})
	_, err := o.Layer7().Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrStoreIsNil))
}

func TestAnthropicCacheLayer_Compact_EmptySessionID_ReturnsError(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	o := NewCacheOptimizer(CacheOptimizerConfig{
		ProviderType: "anthropic",
		Store:        store,
	})
	_, err := o.Layer7().Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSessionIDEmpty))
}

// ---------------------------------------------------------------------------
// CompactPromptBuilder
// ---------------------------------------------------------------------------

func TestCompactPromptBuilder_SetSection(t *testing.T) {
	t.Parallel()
	b := NewCompactPromptBuilder()
	b.SetSection(SectionSystemInstructions, "system content")
	b.SetSection(SectionUserRequest, "user query")

	sections := b.Sections()
	require.Len(t, sections, 2)
	require.Equal(t, SectionSystemInstructions, sections[0].Name)
	require.Equal(t, SectionUserRequest, sections[1].Name)
}

func TestCompactPromptBuilder_SetSection_UpdatesExisting(t *testing.T) {
	t.Parallel()
	b := NewCompactPromptBuilder()
	b.SetSection(SectionSystemInstructions, "v1")
	b.SetSection(SectionSystemInstructions, "v2")

	sections := b.Sections()
	require.Len(t, sections, 1)
	require.Equal(t, "v2", sections[0].Content)
}

func TestCompactPromptBuilder_SetSectionWithScore(t *testing.T) {
	t.Parallel()
	b := NewCompactPromptBuilder()
	b.SetSectionWithScore("custom-section", "content", 42)

	sections := b.Sections()
	require.Len(t, sections, 1)
	require.Equal(t, 42, sections[0].StabilityScore)
	require.Equal(t, "custom-section", sections[0].Name)
}

func TestCompactPromptBuilder_Build_SortsByStability(t *testing.T) {
	t.Parallel()
	b := NewCompactPromptBuilder()

	// Add sections in non-stability order.
	b.SetSection(SectionUserRequest, "volatile request")
	b.SetSection(SectionSystemInstructions, "stable system")
	b.SetSection(SectionRepoMap, "semi-stable map")

	result := b.Build()

	// System instructions (score 10) should come before repo map (20),
	// which should come before user request (90).
	sysIdx := strings.Index(result, "--- system-instructions ---")
	mapIdx := strings.Index(result, "--- repo-map ---")
	userIdx := strings.Index(result, "--- user-request ---")

	require.True(t, sysIdx < mapIdx, "system instructions should come before repo map")
	require.True(t, mapIdx < userIdx, "repo map should come before user request")
}

func TestCompactPromptBuilder_Build_EmptyBuilder(t *testing.T) {
	t.Parallel()
	b := NewCompactPromptBuilder()
	require.Equal(t, "", b.Build())
}

func TestCompactPromptBuilder_Build_SkipsEmptySections(t *testing.T) {
	t.Parallel()
	b := NewCompactPromptBuilder()
	b.SetSection(SectionSystemInstructions, "content")
	b.SetSection(SectionUserRequest, "") // Empty section.

	result := b.Build()
	require.Contains(t, result, "system-instructions")
	require.NotContains(t, result, "user-request")
}

func TestCompactPromptBuilder_TotalTokens(t *testing.T) {
	t.Parallel()
	b := NewCompactPromptBuilder()
	b.SetSection(SectionSystemInstructions, "short")
	b.SetSection(SectionRepoMap, strings.Repeat("x", 100))

	total := b.TotalTokens()
	require.True(t, total > 0)
}

func TestCompactPromptBuilder_SectionCount(t *testing.T) {
	t.Parallel()
	b := NewCompactPromptBuilder()
	require.Equal(t, 0, b.SectionCount())

	b.SetSection(SectionSystemInstructions, "content")
	require.Equal(t, 1, b.SectionCount())

	b.SetSection(SectionRepoMap, "")
	require.Equal(t, 1, b.SectionCount()) // Empty sections not counted.
}

func TestCompactPromptBuilder_SectionCount_AllNine(t *testing.T) {
	t.Parallel()
	b := NewCompactPromptBuilder()
	for _, name := range []string{
		SectionSystemInstructions, SectionRepoMap, SectionActiveFiles,
		SectionRecentEdits, SectionTestResults, SectionLCMContext,
		SectionSessionMemory, SectionGhostCues, SectionUserRequest,
	} {
		b.SetSection(name, "some content")
	}
	require.Equal(t, 9, b.SectionCount())
}

// ---------------------------------------------------------------------------
// PromptSection.EstimatedTokens
// ---------------------------------------------------------------------------

func TestPromptSection_EstimatedTokens(t *testing.T) {
	t.Parallel()
	s := PromptSection{Content: ""}
	require.Equal(t, int64(0), s.EstimatedTokens())

	s = PromptSection{Content: "hello world"}
	require.True(t, s.EstimatedTokens() > 0)
}

// ---------------------------------------------------------------------------
// DefaultStabilityScores
// ---------------------------------------------------------------------------

func TestDefaultStabilityScores_AllSectionsPresent(t *testing.T) {
	t.Parallel()
	expected := []string{
		SectionSystemInstructions, SectionRepoMap, SectionActiveFiles,
		SectionRecentEdits, SectionTestResults, SectionLCMContext,
		SectionSessionMemory, SectionGhostCues, SectionUserRequest,
	}
	for _, name := range expected {
		_, ok := DefaultStabilityScores[name]
		require.True(t, ok, "missing stability score for section %q", name)
	}
}

func TestDefaultStabilityScores_Ordering(t *testing.T) {
	t.Parallel()
	// System instructions should be most stable.
	require.Less(t,
		DefaultStabilityScores[SectionSystemInstructions],
		DefaultStabilityScores[SectionUserRequest],
	)
	// Repo map should be more stable than recent edits.
	require.Less(t,
		DefaultStabilityScores[SectionRepoMap],
		DefaultStabilityScores[SectionRecentEdits],
	)
}

// ---------------------------------------------------------------------------
// SortSectionsByStability / FilterStableSections
// ---------------------------------------------------------------------------

func TestSortSectionsByStability(t *testing.T) {
	t.Parallel()
	sections := []PromptSection{
		{Name: "c", StabilityScore: 50},
		{Name: "a", StabilityScore: 10},
		{Name: "b", StabilityScore: 30},
	}
	SortSectionsByStability(sections)

	require.Equal(t, "a", sections[0].Name)
	require.Equal(t, "b", sections[1].Name)
	require.Equal(t, "c", sections[2].Name)
}

func TestSortSectionsByStability_PreservesEqualScores(t *testing.T) {
	t.Parallel()
	sections := []PromptSection{
		{Name: "first", StabilityScore: 20},
		{Name: "second", StabilityScore: 20},
	}
	SortSectionsByStability(sections)
	require.Equal(t, "first", sections[0].Name)
	require.Equal(t, "second", sections[1].Name)
}

func TestFilterStableSections(t *testing.T) {
	t.Parallel()
	sections := []PromptSection{
		{Name: "stable", StabilityScore: 10, Content: "a"},
		{Name: "medium", StabilityScore: 30, Content: "b"},
		{Name: "volatile", StabilityScore: 90, Content: "c"},
	}
	filtered := FilterStableSections(sections, 30)
	require.Len(t, filtered, 2)
	require.Equal(t, "stable", filtered[0].Name)
	require.Equal(t, "medium", filtered[1].Name)
}

func TestFilterStableSections_AllStable(t *testing.T) {
	t.Parallel()
	sections := []PromptSection{
		{Name: "a", StabilityScore: 10},
		{Name: "b", StabilityScore: 20},
	}
	filtered := FilterStableSections(sections, 100)
	require.Len(t, filtered, 2)
}

func TestFilterStableSections_NoneStable(t *testing.T) {
	t.Parallel()
	sections := []PromptSection{
		{Name: "a", StabilityScore: 90},
	}
	filtered := FilterStableSections(sections, 10)
	require.Empty(t, filtered)
}

// ---------------------------------------------------------------------------
// CacheOptimizer.isAnthropic
// ---------------------------------------------------------------------------

func TestCacheOptimizer_IsAnthropic(t *testing.T) {
	t.Parallel()
	require.True(t, NewCacheOptimizer(CacheOptimizerConfig{ProviderType: "anthropic"}).isAnthropic())
	require.True(t, NewCacheOptimizer(CacheOptimizerConfig{ProviderType: "Anthropic"}).isAnthropic())
	require.True(t, NewCacheOptimizer(CacheOptimizerConfig{ProviderType: "ANTHROPIC"}).isAnthropic())
	require.False(t, NewCacheOptimizer(CacheOptimizerConfig{ProviderType: "openai"}).isAnthropic())
	require.False(t, NewCacheOptimizer(CacheOptimizerConfig{ProviderType: ""}).isAnthropic())
}

// ---------------------------------------------------------------------------
// CacheOptimizer.BuildPrompt
// ---------------------------------------------------------------------------

func TestCacheOptimizer_BuildPrompt(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	entries := []ContextEntry{
		{ItemType: "summary", SummaryContent: "summarised discussion about auth"},
		{ItemType: "message", MessageID: "msg-1", TokenCount: 500},
	}

	prompt, err := o.BuildPrompt(context.Background(), entries)
	require.NoError(t, err)
	require.Contains(t, prompt, "system-instructions")
}

func TestCacheOptimizer_BuildPrompt_EmptyEntries(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	prompt, err := o.BuildPrompt(context.Background(), nil)
	require.NoError(t, err)
	require.Contains(t, prompt, "system-instructions")
}

// ---------------------------------------------------------------------------
// Integration: LayerManager with CacheOptimizer
// ---------------------------------------------------------------------------

func TestLayerManager_Integration_WithCacheOptimizer(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "sess-cache-integration"
	createTestSession(t, queries, sessionID)
	store := newStore(queries, sqlDB)

	for i := range 3 {
		msgID := "msg-int-" + strings.Repeat("g", i)
		createTestMessage(t, queries, sessionID, msgID, "assistant", "integration content")
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 100,
		})
		require.NoError(t, err)
	}

	o := NewCacheOptimizer(CacheOptimizerConfig{
		ProviderType: "anthropic",
		Store:        store,
		SessionID:    sessionID,
	})

	mgr := NewCompactionLayerManager(o.Layer6(), o.Layer7())
	budget := Budget{SoftThreshold: 50000, HardLimit: 60000, ContextWindow: 128000}

	result, err := mgr.RunAll(ctx, budget)
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
}

// ---------------------------------------------------------------------------
// assembleSections — 6 previously-stubbed sections
// ---------------------------------------------------------------------------

func TestAssembleSections_AllNinePopulated(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	entries := []ContextEntry{
		{ItemType: "summary", SummaryID: "sum_1", SummaryContent: "auth discussion", SummaryKind: "general", TokenCount: 200},
		{ItemType: "summary", SummaryID: "sum_2", SummaryContent: "file structure overview", SummaryKind: "repo-map", TokenCount: 300},
		{ItemType: "summary", SummaryID: "sum_3", SummaryContent: "condensed prior context about database migration", SummaryKind: "condensed", ParentIDs: []string{"msg_a", "msg_b"}, TokenCount: 500},
		{ItemType: "message", MessageID: "msg-edit-1", TokenCount: 100},
		{ItemType: "message", MessageID: "msg-test-1", TokenCount: 800},
		{ItemType: "message", MessageID: "msg-recent", TokenCount: 50},
	}

	o.assembleSections(builder, entries)
	sections := builder.Sections()

	require.Equal(t, 9, builder.SectionCount(), "all 9 sections should be populated")

	names := make(map[string]bool)
	for _, s := range sections {
		names[s.Name] = true
	}
	for _, expected := range []string{
		SectionSystemInstructions, SectionRepoMap, SectionActiveFiles,
		SectionRecentEdits, SectionTestResults, SectionLCMContext,
		SectionSessionMemory, SectionGhostCues, SectionUserRequest,
	} {
		require.True(t, names[expected], "section %q should be present", expected)
	}
}

func TestAssembleSections_RepoMap_SummaryKind(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	entries := []ContextEntry{
		{ItemType: "summary", SummaryID: "sum_rm", SummaryContent: "src/main.go\nsrc/util.go", SummaryKind: "repo-map", TokenCount: 100},
	}

	o.assembleSections(builder, entries)

	prompt := builder.Build()
	require.Contains(t, prompt, "--- repo-map ---")
	require.Contains(t, prompt, "src/main.go")
}

func TestAssembleSections_RepoMap_FallbackScope(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	entries := []ContextEntry{
		{ItemType: "message", MessageID: "msg-1", TokenCount: 100},
	}

	o.assembleSections(builder, entries)

	prompt := builder.Build()
	require.Contains(t, prompt, "--- repo-map ---")
	require.Contains(t, prompt, "Context covers 1 entries")
}

func TestAssembleSections_RecentEdits(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	entries := []ContextEntry{
		{ItemType: "message", MessageID: "msg-edit", TokenCount: 120},
		{ItemType: "message", MessageID: "msg-large", TokenCount: 600},
	}

	o.assembleSections(builder, entries)

	prompt := builder.Build()
	require.Contains(t, prompt, "--- recent-edits ---")
	require.Contains(t, prompt, "msg-edit")

	sections := builder.Sections()
	for _, s := range sections {
		if s.Name == SectionRecentEdits {
			require.NotContains(t, s.Content, "msg-large", "large messages should not appear in recent-edits")
			break
		}
	}
}

func TestAssembleSections_TestResults(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	entries := []ContextEntry{
		{ItemType: "message", MessageID: "msg-testout", TokenCount: 900},
	}

	o.assembleSections(builder, entries)

	prompt := builder.Build()
	require.Contains(t, prompt, "--- test-results ---")
	require.Contains(t, prompt, "msg-testout")
}

func TestAssembleSections_SessionMemory(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	entries := []ContextEntry{
		{ItemType: "summary", SummaryID: "sum_1", SummaryContent: "summary text", TokenCount: 200},
		{ItemType: "message", MessageID: "msg-1", TokenCount: 300},
		{ItemType: "message", MessageID: "msg-2", SummaryID: "sum_cond", ParentIDs: []string{"msg_old"}, TokenCount: 100},
	}

	o.assembleSections(builder, entries)

	prompt := builder.Build()
	require.Contains(t, prompt, "--- session-memory ---")
	require.Contains(t, prompt, "3 entries")
	require.Contains(t, prompt, "2 messages")
	require.Contains(t, prompt, "1 summaries")
	require.Contains(t, prompt, "Total tracked tokens: 600")
}

func TestAssembleSections_GhostCues(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	entries := []ContextEntry{
		{
			ItemType:       "summary",
			SummaryID:      "sum_ghost",
			SummaryContent: "Discussion about refactoring the auth module",
			SummaryKind:    "condensed",
			ParentIDs:      []string{"msg_a", "msg_b"},
			TokenCount:     150,
		},
	}

	o.assembleSections(builder, entries)

	prompt := builder.Build()
	require.Contains(t, prompt, "--- ghost-cues ---")
	require.Contains(t, prompt, "sum_ghost")
	require.Contains(t, prompt, "refactoring the auth module")
}

func TestAssembleSections_GhostCues_SnippetTruncation(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	longContent := strings.Repeat("x", 300)
	entries := []ContextEntry{
		{
			ItemType:       "summary",
			SummaryID:      "sum_long",
			SummaryContent: longContent,
			ParentIDs:      []string{"msg_a"},
			TokenCount:     100,
		},
	}

	o.assembleSections(builder, entries)

	prompt := builder.Build()
	require.Contains(t, prompt, "--- ghost-cues ---")
	require.Contains(t, prompt, "sum_long")
	// Snippet should be truncated with ellipsis.
	sections := builder.Sections()
	for _, s := range sections {
		if s.Name == SectionGhostCues {
			require.Contains(t, s.Content, "...", "long ghost cues should be truncated")
			break
		}
	}
}

func TestAssembleSections_GhostCues_LineagePointer(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	entries := []ContextEntry{
		{
			ItemType:    "summary",
			SummaryID:   "sum_lineage",
			SummaryKind: "condensed",
			ParentIDs:   []string{"msg_a", "msg_b", "msg_c"},
			TokenCount:  200,
		},
	}

	o.assembleSections(builder, entries)

	prompt := builder.Build()
	require.Contains(t, prompt, "--- ghost-cues ---")
	require.Contains(t, prompt, "[Lineage: msg_a,msg_b,msg_c, depth=3]")
}

func TestAssembleSections_GhostCues_ArchiveStub(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	entries := []ContextEntry{
		{
			ItemType:       "summary",
			SummaryID:      "sum_archived",
			SummaryKind:    KindArchiveStub,
			SummaryContent: "[Archived from sum_original] some content",
			TokenCount:     42,
		},
	}

	o.assembleSections(builder, entries)

	prompt := builder.Build()
	require.Contains(t, prompt, "--- ghost-cues ---")
	require.Contains(t, prompt, "[Archived: sum_archived, tokens=42]")
}

func TestAssembleSections_UserRequest(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	entries := []ContextEntry{
		{ItemType: "message", MessageID: "msg-first", TokenCount: 100},
		{ItemType: "message", MessageID: "msg-last", TokenCount: 200},
	}

	o.assembleSections(builder, entries)

	prompt := builder.Build()
	require.Contains(t, prompt, "--- user-request ---")
	require.Contains(t, prompt, "msg-last", "most recent message should be in user-request")

	sections := builder.Sections()
	for _, s := range sections {
		if s.Name == SectionUserRequest {
			require.NotContains(t, s.Content, "msg-first", "only the last message should appear in user-request")
			break
		}
	}
}

func TestAssembleSections_EmptyEntries_OnlySystemAndMemory(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})
	builder := NewCompactPromptBuilder()

	o.assembleSections(builder, nil)

	prompt := builder.Build()
	// System instructions always present.
	require.Contains(t, prompt, "--- system-instructions ---")
	// Session memory should report 0 entries.
	require.Contains(t, prompt, "--- session-memory ---")
	require.Contains(t, prompt, "0 entries")
	// Sections that require entries should be absent.
	require.NotContains(t, prompt, "--- repo-map ---")
	require.NotContains(t, prompt, "--- recent-edits ---")
	require.NotContains(t, prompt, "--- test-results ---")
	require.NotContains(t, prompt, "--- ghost-cues ---")
	require.NotContains(t, prompt, "--- user-request ---")
}

func TestAssembleSections_BuildPrompt_AllNineInOutput(t *testing.T) {
	t.Parallel()
	o := NewCacheOptimizer(CacheOptimizerConfig{})

	entries := []ContextEntry{
		{ItemType: "summary", SummaryID: "sum_1", SummaryContent: "discussed caching", SummaryKind: "general", TokenCount: 200},
		{ItemType: "summary", SummaryID: "sum_2", SummaryContent: "file tree overview", SummaryKind: "repo-map", TokenCount: 150},
		{ItemType: "summary", SummaryID: "sum_3", SummaryContent: "condensed prior context about auth", SummaryKind: "condensed", ParentIDs: []string{"m1", "m2"}, TokenCount: 300},
		{ItemType: "message", MessageID: "msg-edit", TokenCount: 100},
		{ItemType: "message", MessageID: "msg-test", TokenCount: 800},
		{ItemType: "message", MessageID: "msg-active", TokenCount: 300},
		{ItemType: "message", MessageID: "msg-recent", TokenCount: 50},
	}

	prompt, err := o.BuildPrompt(context.Background(), entries)
	require.NoError(t, err)

	for _, section := range []string{
		"--- system-instructions ---",
		"--- repo-map ---",
		"--- active-files ---",
		"--- recent-edits ---",
		"--- test-results ---",
		"--- lcm-context ---",
		"--- session-memory ---",
		"--- ghost-cues ---",
		"--- user-request ---",
	} {
		require.Contains(t, prompt, section, "prompt should contain section %q", section)
	}
}
