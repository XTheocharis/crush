package lcm

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/crush/internal/lcm/nudge"
)

// ---------------------------------------------------------------------------
// Prompt Section types
// ---------------------------------------------------------------------------

// PromptSection represents a named section within the compact prompt
// structure. Sections are ordered by cache stability: stable sections that
// rarely change across turns (e.g. system instructions) should have lower
// indices, while volatile sections (e.g. user request) should have higher
// indices.
type PromptSection struct {
	// Name is the human-readable identifier for this section.
	Name string

	// Content is the rendered text for this section. May be empty if the
	// section is not applicable for the current turn.
	Content string

	// StabilityScore ranks how rarely this section changes between turns.
	// Lower values are more stable. Used to order sections for Anthropic
	// prefix caching — stable sections placed first maximise cache hits.
	StabilityScore int
}

// EstimatedTokens returns an approximate token count for this section.
func (s PromptSection) EstimatedTokens() int64 {
	if s.Content == "" {
		return 0
	}
	return EstimateTokens(s.Content)
}

// SectionName constants identify the ten prompt sections in the compact
// prompt structure (Layer 6). Nine sections are defined by DREAM spec §B.1:
//
//  1. system-instructions (stability 10)
//  2. repo-map            (stability 20)
//  3. active-files        (stability 30)
//  4. nudge               (stability 35)
//  5. recent-edits        (stability 40)
//  6. test-results        (stability 50)
//  7. lcm-context         (stability 60)
//  8. session-memory      (stability 70)
//  9. user-request        (stability 90)
//
// One additional section comes from DREAM spec §B.4 (LCM context injection):
//
//  10. ghost-cues           (stability 80)
const (
	SectionSystemInstructions = "system-instructions"
	SectionRepoMap            = "repo-map"
	SectionActiveFiles        = "active-files"
	SectionNudge              = "nudge"
	SectionRecentEdits        = "recent-edits"
	SectionTestResults        = "test-results"
	SectionLCMContext         = "lcm-context"
	SectionSessionMemory      = "session-memory"
	SectionGhostCues          = "ghost-cues"
	SectionUserRequest        = "user-request"
)

// DefaultStabilityScores maps each canonical section to its default stability
// score. Lower values indicate more stable (cache-friendly) sections.
var DefaultStabilityScores = map[string]int{
	SectionSystemInstructions: 10,
	SectionRepoMap:            20,
	SectionActiveFiles:        30,
	SectionNudge:              35,
	SectionRecentEdits:        40,
	SectionTestResults:        50,
	SectionLCMContext:         60,
	SectionSessionMemory:      70,
	SectionGhostCues:          80,
	SectionUserRequest:        90,
}

// ---------------------------------------------------------------------------
// CompactPromptBuilder (Layer 6)
// ---------------------------------------------------------------------------

// CompactPromptBuilder assembles the 10-section compact prompt structure
// used by Layer 6. Sections are collected dynamically from available context
// and ordered by cache stability score.
type CompactPromptBuilder struct {
	sections []PromptSection
}

// NewCompactPromptBuilder creates a builder with the default section ordering.
func NewCompactPromptBuilder() *CompactPromptBuilder {
	return &CompactPromptBuilder{}
}

// SetSection adds or updates a prompt section. If a section with the same name
// already exists, its content is replaced. The stability score is set from
// DefaultStabilityScores unless the section already has a custom score.
func (b *CompactPromptBuilder) SetSection(name, content string) {
	score, ok := DefaultStabilityScores[name]
	if !ok {
		score = 100 // Default: volatile.
	}

	for i := range b.sections {
		if b.sections[i].Name == name {
			b.sections[i].Content = content
			if DefaultStabilityScores[name] != 0 {
				b.sections[i].StabilityScore = score
			}
			return
		}
	}

	b.sections = append(b.sections, PromptSection{
		Name:           name,
		Content:        content,
		StabilityScore: score,
	})
}

// SetSectionWithScore adds or updates a prompt section with an explicit
// stability score.
func (b *CompactPromptBuilder) SetSectionWithScore(name, content string, stabilityScore int) {
	for i := range b.sections {
		if b.sections[i].Name == name {
			b.sections[i].Content = content
			b.sections[i].StabilityScore = stabilityScore
			return
		}
	}

	b.sections = append(b.sections, PromptSection{
		Name:           name,
		Content:        content,
		StabilityScore: stabilityScore,
	})
}

// Sections returns the current sections in their insertion order (not yet
// sorted by stability).
func (b *CompactPromptBuilder) Sections() []PromptSection {
	out := make([]PromptSection, len(b.sections))
	copy(out, b.sections)
	return out
}

// Build assembles all non-empty sections into a single prompt string. Sections
// are sorted by stability score (ascending) so that stable content appears
// first, which maximises Anthropic prefix caching efficiency.
func (b *CompactPromptBuilder) Build() string {
	sections := b.sortedSections()
	if len(sections) == 0 {
		return ""
	}

	var buf strings.Builder
	for i, s := range sections {
		if i > 0 {
			buf.WriteString("\n\n")
		}
		fmt.Fprintf(&buf, "--- %s ---\n%s", s.Name, s.Content)
	}
	return buf.String()
}

// sortedSections returns non-empty sections ordered by stability score
// (ascending).
func (b *CompactPromptBuilder) sortedSections() []PromptSection {
	var result []PromptSection
	for _, s := range b.sections {
		if s.Content != "" {
			result = append(result, s)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].StabilityScore < result[j].StabilityScore
	})
	return result
}

// TotalTokens returns the estimated total token count across all non-empty
// sections.
func (b *CompactPromptBuilder) TotalTokens() int64 {
	var total int64
	for _, s := range b.sections {
		total += s.EstimatedTokens()
	}
	return total
}

// SectionCount returns the number of non-empty sections.
func (b *CompactPromptBuilder) SectionCount() int {
	var count int
	for _, s := range b.sections {
		if s.Content != "" {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// CacheOptimizer (Layer 6 — Compact Prompt Structure)
// ---------------------------------------------------------------------------

// CacheOptimizerConfig controls the behaviour of the CacheOptimizer layer.
type CacheOptimizerConfig struct {
	// ProviderType identifies the active LLM provider (e.g. "anthropic",
	// "openai"). Used by Layer 7 to decide whether cache-specific ordering
	// should be applied.
	ProviderType string

	// Store is the LCM store used for persisting context. Optional — the
	// cache optimizer can operate without a store by assembling sections
	// purely from provided context.
	Store *Store

	// SessionID is the session this optimizer operates on.
	SessionID string

	// NudgeInjector optionally injects context-limit nudges. When nil, no
	// nudge section is added.
	NudgeInjector *nudge.NudgeInjector

	// TurnCountFunc returns the current turn count for the session. When
	// nil, TurnCount defaults to 0 (no turn-based nudges).
	TurnCountFunc func() int64

	// IterationCountFunc returns the current iteration count for the
	// session. When nil, IterationCount defaults to 0 (no iteration-based
	// nudges).
	IterationCountFunc func() int64
}

// CacheOptimizer implements Layers 6 and 7 of the compaction framework.
//
// Layer 6 (priority 60): Assembles the 10-section compact prompt structure,
// ordering sections by cache stability (stable sections first).
//
// Layer 7 (priority 70): Detects Anthropic providers and applies additional
// cache-optimization heuristics. For non-Anthropic providers, Layer 7 is a
// no-op.
type CacheOptimizer struct {
	cfg CacheOptimizerConfig
}

// NewCacheOptimizer creates a CacheOptimizer with the given configuration.
func NewCacheOptimizer(cfg CacheOptimizerConfig) *CacheOptimizer {
	return &CacheOptimizer{cfg: cfg}
}

// ---------------------------------------------------------------------------
// Layer 6: CompactPromptStructure
// ---------------------------------------------------------------------------

// compactPromptLayer wraps CacheOptimizer to expose Layer 6 independently.
type compactPromptLayer struct {
	optimizer *CacheOptimizer
}

// Name returns "compact-prompt-structure".
func (l *compactPromptLayer) Name() string { return "compact-prompt-structure" }

// Priority returns 60 (Layer 6).
func (l *compactPromptLayer) Priority() int { return 60 }

// ShouldCompact reports whether there are enough context entries to benefit
// from prompt reassembly.
func (l *compactPromptLayer) ShouldCompact(ctx context.Context, budget Budget) bool {
	return l.optimizer.hasEnoughContext(ctx)
}

// Compact assembles the compact prompt structure from available context entries
// and returns a result indicating the sections that were assembled.
func (l *compactPromptLayer) Compact(ctx context.Context, budget Budget) (*CompactionLayerResult, error) {
	return l.optimizer.buildCompactPrompt(ctx, budget)
}

// ---------------------------------------------------------------------------
// Layer 7: AnthropicCacheManagement
// ---------------------------------------------------------------------------

// anthropicCacheLayer wraps CacheOptimizer to expose Layer 7 independently.
type anthropicCacheLayer struct {
	optimizer *CacheOptimizer
}

// Name returns "anthropic-cache-management".
func (l *anthropicCacheLayer) Name() string { return "anthropic-cache-management" }

// Priority returns 70 (Layer 7).
func (l *anthropicCacheLayer) Priority() int { return 70 }

// ShouldCompact reports true only when the provider is Anthropic and there is
// enough context to benefit from cache-optimised ordering.
func (l *anthropicCacheLayer) ShouldCompact(ctx context.Context, budget Budget) bool {
	if !l.optimizer.isAnthropic() {
		return false
	}
	return l.optimizer.hasEnoughContext(ctx)
}

// Compact re-orders prompt sections for optimal Anthropic prefix caching and
// returns the estimated token savings from improved cache hits.
func (l *anthropicCacheLayer) Compact(ctx context.Context, budget Budget) (*CompactionLayerResult, error) {
	return l.optimizer.optimizeForAnthropic(ctx, budget)
}

// ---------------------------------------------------------------------------
// CacheOptimizer public API
// ---------------------------------------------------------------------------

// Layer6 returns the Layer 6 (Compact Prompt Structure) CompactionLayer.
func (o *CacheOptimizer) Layer6() CompactionLayer {
	return &compactPromptLayer{optimizer: o}
}

// Layer7 returns the Layer 7 (Anthropic Cache Management) CompactionLayer.
func (o *CacheOptimizer) Layer7() CompactionLayer {
	return &anthropicCacheLayer{optimizer: o}
}

// Layers returns both Layer 6 and Layer 7 as CompactionLayer instances.
func (o *CacheOptimizer) Layers() []CompactionLayer {
	return []CompactionLayer{o.Layer6(), o.Layer7()}
}

// BuildPrompt assembles the compact prompt from the given context entries.
// This is the primary API for callers that want to use the compact prompt
// structure without going through the compaction framework.
func (o *CacheOptimizer) BuildPrompt(ctx context.Context, entries []ContextEntry) (string, error) {
	builder := NewCompactPromptBuilder()

	// Assemble sections dynamically from available context.
	o.assembleSections(builder, entries)

	return builder.Build(), nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// isAnthropic reports whether the configured provider is Anthropic.
func (o *CacheOptimizer) isAnthropic() bool {
	return strings.EqualFold(o.cfg.ProviderType, "anthropic")
}

// hasEnoughContext reports whether there are enough context entries to benefit
// from prompt reassembly. We require at least 2 entries so that section
// ordering actually matters.
func (o *CacheOptimizer) hasEnoughContext(ctx context.Context) bool {
	if o.cfg.Store == nil || o.cfg.SessionID == "" {
		return false
	}

	entries, err := o.cfg.Store.GetContextEntries(ctx, o.cfg.SessionID)
	if err != nil {
		return false
	}
	return len(entries) >= 2
}

// buildCompactPrompt is the Compact implementation for Layer 6.
func (o *CacheOptimizer) buildCompactPrompt(ctx context.Context, _ Budget) (*CompactionLayerResult, error) {
	if o.cfg.Store == nil {
		return nil, fmt.Errorf("cache-optimizer: %w", ErrStoreIsNil)
	}
	if o.cfg.SessionID == "" {
		return nil, fmt.Errorf("cache-optimizer: %w", ErrSessionIDEmpty)
	}

	entries, err := o.cfg.Store.GetContextEntries(ctx, o.cfg.SessionID)
	if err != nil {
		return nil, fmt.Errorf("getting context entries: %w", err)
	}

	builder := NewCompactPromptBuilder()
	o.assembleSections(builder, entries)

	sectionCount := builder.SectionCount()
	if sectionCount == 0 {
		return &CompactionLayerResult{
			LayerName:   "compact-prompt-structure",
			ActionTaken: false,
		}, nil
	}

	return &CompactionLayerResult{
		LayerName:     "compact-prompt-structure",
		TokensFreed:   0, // Structure reassembly does not free tokens directly.
		ItemsAffected: sectionCount,
		ActionTaken:   true,
	}, nil
}

// optimizeForAnthropic is the Compact implementation for Layer 7. It estimates
// the token savings from improved cache hits when sections are optimally
// ordered for Anthropic prefix caching.
func (o *CacheOptimizer) optimizeForAnthropic(ctx context.Context, _ Budget) (*CompactionLayerResult, error) {
	if o.cfg.Store == nil {
		return nil, fmt.Errorf("cache-optimizer: %w", ErrStoreIsNil)
	}
	if o.cfg.SessionID == "" {
		return nil, fmt.Errorf("cache-optimizer: %w", ErrSessionIDEmpty)
	}

	if !o.isAnthropic() {
		// No-op for non-Anthropic providers.
		return &CompactionLayerResult{
			LayerName:   "anthropic-cache-management",
			ActionTaken: false,
		}, nil
	}

	entries, err := o.cfg.Store.GetContextEntries(ctx, o.cfg.SessionID)
	if err != nil {
		return nil, fmt.Errorf("getting context entries: %w", err)
	}

	builder := NewCompactPromptBuilder()
	o.assembleSections(builder, entries)

	sections := builder.Sections()
	if len(sections) == 0 {
		return &CompactionLayerResult{
			LayerName:   "anthropic-cache-management",
			ActionTaken: false,
		}, nil
	}

	// Estimate cache savings: stable sections at the front that haven't
	// changed since the last turn can be served from cache. The savings
	// are a fraction of the total stable-section tokens.
	var stableTokens int64
	for _, s := range sections {
		if s.StabilityScore <= 30 && s.Content != "" {
			stableTokens += s.EstimatedTokens()
		}
	}

	// Assume ~50% cache hit rate on stable sections as a conservative
	// estimate.
	cacheSavings := stableTokens / 2

	return &CompactionLayerResult{
		LayerName:     "anthropic-cache-management",
		TokensFreed:   cacheSavings,
		ItemsAffected: builder.SectionCount(),
		ActionTaken:   cacheSavings > 0,
	}, nil
}

// assembleSections populates the builder with sections derived from the given
// context entries. Sections are assembled dynamically based on what is actually
// available.
func (o *CacheOptimizer) assembleSections(builder *CompactPromptBuilder, entries []ContextEntry) {
	// System instructions: always present when LCM is active.
	builder.SetSection(SectionSystemInstructions, LCMSystemPrompt)

	// Categorise entries by type to fill sections.
	var summaries []ContextEntry
	var messages []ContextEntry
	for _, entry := range entries {
		switch entry.ItemType {
		case "summary":
			summaries = append(summaries, entry)
		case "message":
			messages = append(messages, entry)
		}
	}

	// LCM Context: summary entries.
	if len(summaries) > 0 {
		var parts []string
		for _, s := range summaries {
			if s.SummaryContent != "" {
				parts = append(parts, s.SummaryContent)
			}
		}
		if len(parts) > 0 {
			builder.SetSection(SectionLCMContext, strings.Join(parts, "\n\n"))
		}
	}

	// Active files: tool-output messages (grep, view, read results).
	var activeFileContent []string
	for _, m := range messages {
		if m.TokenCount > 0 && m.TokenCount < LargeOutputThreshold {
			activeFileContent = append(activeFileContent,
				fmt.Sprintf("[message:%s tokens:%d]", m.MessageID, m.TokenCount))
		}
	}
	if len(activeFileContent) > 0 {
		builder.SetSection(SectionActiveFiles, strings.Join(activeFileContent, "\n"))
	}

	// Repo Map: derive from summaries with "repo-map" kind or provide an
	// overview of the active context scope.
	var repoMapParts []string
	for _, s := range summaries {
		if s.SummaryKind == "repo-map" && s.SummaryContent != "" {
			repoMapParts = append(repoMapParts, s.SummaryContent)
		}
	}
	if len(repoMapParts) > 0 {
		builder.SetSection(SectionRepoMap, strings.Join(repoMapParts, "\n"))
	} else if len(entries) > 0 {
		builder.SetSection(SectionRepoMap, fmt.Sprintf(
			"Context covers %d entries (%d summaries, %d messages)",
			len(entries), len(summaries), len(messages)))
	}

	// Recent Edits: small messages likely represent edit/tool invocations
	// rather than large tool output.
	var recentEditParts []string
	for _, m := range messages {
		if m.TokenCount > 0 && m.TokenCount < 500 {
			recentEditParts = append(recentEditParts,
				fmt.Sprintf("edit:%s (tokens:%d)", m.MessageID, m.TokenCount))
		}
	}
	if len(recentEditParts) > 0 {
		builder.SetSection(SectionRecentEdits, strings.Join(recentEditParts, "\n"))
	}

	// Test Results: medium-to-large messages likely contain test or build
	// output.
	var testResultParts []string
	for _, m := range messages {
		if m.TokenCount >= 500 && m.TokenCount < LargeOutputThreshold {
			testResultParts = append(testResultParts,
				fmt.Sprintf("output:%s (tokens:%d)", m.MessageID, m.TokenCount))
		}
	}
	if len(testResultParts) > 0 {
		builder.SetSection(SectionTestResults, strings.Join(testResultParts, "\n"))
	}

	// Session Memory: aggregate statistics across all entries.
	var totalTokens int64
	var condensedCount int
	for _, e := range entries {
		totalTokens += e.TokenCount
		if len(e.ParentIDs) > 0 {
			condensedCount++
		}
	}
	builder.SetSection(SectionSessionMemory, fmt.Sprintf(
		"Session has %d entries (%d messages, %d summaries, %d condensed).\nTotal tracked tokens: %d.",
		len(entries), len(messages), len(summaries), condensedCount, totalTokens))

	// Ghost Cues: condensed summary references act as cues about previously
	// discussed topics that have been compacted away.
	var ghostCueParts []string
	for _, s := range summaries {
		// SummaryID cue: condensed summaries with content.
		if len(s.ParentIDs) > 0 && s.SummaryContent != "" {
			snippet := s.SummaryContent
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			ghostCueParts = append(ghostCueParts,
				fmt.Sprintf("[%s] %s", s.SummaryID, snippet))
		}

		// LineagePointer cue: entries with parent IDs carry lineage
		// information for retrieval chaining.
		if len(s.ParentIDs) > 0 {
			ghostCueParts = append(ghostCueParts,
				fmt.Sprintf("[Lineage: %s, depth=%d]",
					strings.Join(s.ParentIDs, ","),
					len(s.ParentIDs)))
		}

		// ArchiveStub cue: archived summaries preserve a lightweight
		// reference with token metadata.
		if s.SummaryKind == KindArchiveStub {
			ghostCueParts = append(ghostCueParts,
				fmt.Sprintf("[Archived: %s, tokens=%d]",
					s.SummaryID, s.TokenCount))
		}
	}
	if len(ghostCueParts) > 0 {
		builder.SetSection(SectionGhostCues, strings.Join(ghostCueParts, "\n"))
	}

	// Nudge: inject context-limit nudge when pressure is high and token count
	// exceeds the configured limit.
	if o.cfg.NudgeInjector != nil {
		var totalTokens int64
		for _, e := range entries {
			totalTokens += e.TokenCount
		}
		if totalTokens > 0 {
			const defaultContextWindow = 200000
			var turnCount, iterCount int64
			if o.cfg.TurnCountFunc != nil {
				turnCount = o.cfg.TurnCountFunc()
			}
			if o.cfg.IterationCountFunc != nil {
				iterCount = o.cfg.IterationCountFunc()
			}
			nudgeResult, err := o.cfg.NudgeInjector.InjectFull(
				context.Background(), nudge.InjectParams{
					Prompt:         "",
					CurrentTokens:  totalTokens,
					ContextWindow:  defaultContextWindow,
				TurnCount:      int(turnCount),
				IterationCount: int(iterCount),
				})
			if err == nil && nudgeResult != "" {
				builder.SetSection(SectionNudge, nudgeResult)
			}
		}
	}

	// User Request: the most recent message represents the active user
	// request context.
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		builder.SetSection(SectionUserRequest,
			fmt.Sprintf("Most recent activity: %s (tokens: %d)", last.MessageID, last.TokenCount))
	}
}

// ReorderForCache performs cache-aware section re-ordering for Anthropic
// providers. It fetches context entries from the store, assembles prompt
// sections, sorts them by stability, and returns the estimated cache savings.
// When the provider is not Anthropic or the store/session is not configured, it
// returns a no-op result without error. This method is designed for integration
// with the MicroCompactor layer — the standalone Layer 7 (anthropicCacheLayer)
// remains unchanged.
func (o *CacheOptimizer) ReorderForCache(ctx context.Context) (*CompactionLayerResult, error) {
	if o.cfg.Store == nil {
		return nil, fmt.Errorf("cache-optimizer: %w", ErrStoreIsNil)
	}
	if o.cfg.SessionID == "" {
		return nil, fmt.Errorf("cache-optimizer: %w", ErrSessionIDEmpty)
	}

	if !o.isAnthropic() {
		return &CompactionLayerResult{
			LayerName:   "anthropic-cache-management",
			ActionTaken: false,
		}, nil
	}

	entries, err := o.cfg.Store.GetContextEntries(ctx, o.cfg.SessionID)
	if err != nil {
		return nil, fmt.Errorf("getting context entries: %w", err)
	}

	builder := NewCompactPromptBuilder()
	o.assembleSections(builder, entries)

	sections := builder.Sections()
	if len(sections) == 0 {
		return &CompactionLayerResult{
			LayerName:   "anthropic-cache-management",
			ActionTaken: false,
		}, nil
	}

	SortSectionsByStability(sections)

	var stableTokens int64
	for _, s := range sections {
		if s.StabilityScore <= 30 && s.Content != "" {
			stableTokens += s.EstimatedTokens()
		}
	}

	cacheSavings := stableTokens / 2

	return &CompactionLayerResult{
		LayerName:     "anthropic-cache-management",
		TokensFreed:   cacheSavings,
		ItemsAffected: builder.SectionCount(),
		ActionTaken:   cacheSavings > 0,
	}, nil
}

// SortSectionsByStability sorts a slice of PromptSection by stability score
// (ascending). This is the canonical ordering function used for cache
// optimisation.
func SortSectionsByStability(sections []PromptSection) {
	sort.SliceStable(sections, func(i, j int) bool {
		return sections[i].StabilityScore < sections[j].StabilityScore
	})
}

// FilterStableSections returns only sections with stability scores at or below
// the given threshold.
func FilterStableSections(sections []PromptSection, maxScore int) []PromptSection {
	var result []PromptSection
	for _, s := range sections {
		if s.StabilityScore <= maxScore {
			result = append(result, s)
		}
	}
	return result
}
