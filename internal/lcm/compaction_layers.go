package lcm

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// CompactionLayerResult holds the result of a single layer's compaction pass.
// It is distinct from CompactionResult (which is the aggregate result returned
// by the compaction pipeline) so that the layer system is additive and does
// not alter the existing compaction API.
type CompactionLayerResult struct {
	// LayerName is the human-readable name of the layer that produced this
	// result.
	LayerName string

	// TokensFreed is the estimated number of tokens freed by this layer.
	TokensFreed int64

	// ItemsAffected counts the context items that were compacted or replaced.
	ItemsAffected int

	// ActionTaken indicates whether this layer made any changes.
	ActionTaken bool
}

// CompactionLayer is a single compaction strategy within the layered framework.
// Layers are executed in Priority order (lowest first). Each layer decides
// whether it should run given the current budget, and if so, performs its
// compaction pass and returns a result.
//
// Layers 1–5b are implemented; 6–7 are provided by CacheOptimizer:
//
//	1  — MicroCompactor:            inline truncation of large tool outputs
//	1b — TimeGapCompactor:          tool-output compaction across time gaps (time_gap_compactor.go)
//	2  — DedupCompactionLayer:      duplicate/near-duplicate message deduplication
//	3  — StaleEvictionLayer:        stale tool-output eviction
//	4  — PostCompactCleaner:        post-compaction context restore (post_compact.go)
//	5  — AdjacentCondensationLayer: condensation of adjacent summaries
//	5b — PressureCompactionSelector: pressure-driven layer selection
//	6  — CacheOptimizer:            cross-session memory pruning
//	7  — CacheOptimizer:            emergency context truncation
type CompactionLayer interface {
	// Name returns a human-readable identifier for this layer (e.g.
	// "micro-compactor").
	Name() string

	// Priority returns the execution order; lower values run first.
	Priority() int

	// ShouldCompact reports whether this layer has work to do given the
	// current budget. The budget is passed so layers can make threshold-
	// dependent decisions without reaching back into the manager.
	ShouldCompact(ctx context.Context, budget Budget) bool

	// Compact executes one compaction pass for this layer. The ctx carries
	// the session ID via the context key established by the layer manager.
	// Implementations must be safe for concurrent use across different
	// sessions.
	Compact(ctx context.Context, budget Budget) (*CompactionLayerResult, error)
}

// CompactionLayerManager orchestrates ordered execution of CompactionLayer
// instances. It does not replace the existing compaction pipeline
// (compactLocked/runLLMSummarization in manager.go); instead it provides a
// framework that future compaction strategies can plug into. The first
// consumer is the MicroCompactor (Layer 1).
type CompactionLayerManager struct {
	layers []CompactionLayer
}

// NewCompactionLayerManager creates a manager with the given layers, sorted by
// priority (ascending). Duplicate priority values are allowed; their relative
// order is preserved from the input slice.
func NewCompactionLayerManager(layers ...CompactionLayer) *CompactionLayerManager {
	sorted := make([]CompactionLayer, len(layers))
	copy(sorted, layers)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority() < sorted[j].Priority()
	})
	return &CompactionLayerManager{layers: sorted}
}

// Layers returns the ordered list of registered layers (read-only snapshot).
func (m *CompactionLayerManager) Layers() []CompactionLayer {
	out := make([]CompactionLayer, len(m.layers))
	copy(out, m.layers)
	return out
}

// RunAll executes every eligible layer in priority order and returns the
// aggregate result. If a layer returns an error, execution stops and the
// error is propagated. Layers where ShouldCompact returns false are skipped.
func (m *CompactionLayerManager) RunAll(ctx context.Context, budget Budget) (*CompactionLayerResult, error) {
	var aggregate CompactionLayerResult
	for _, layer := range m.layers {
		if !layer.ShouldCompact(ctx, budget) {
			continue
		}
		result, err := layer.Compact(ctx, budget)
		if err != nil {
			return nil, fmt.Errorf("layer %s: %w", layer.Name(), err)
		}
		if result != nil && result.ActionTaken {
			aggregate.TokensFreed += result.TokensFreed
			aggregate.ItemsAffected += result.ItemsAffected
			aggregate.ActionTaken = true
		}
	}
	if aggregate.ActionTaken {
		aggregate.LayerName = "layer-manager"
	}
	return &aggregate, nil
}

// ---------------------------------------------------------------------------
// Layer 1: MicroCompactor
// ---------------------------------------------------------------------------

// MicroCompactorConfig controls the behaviour of the MicroCompactor layer.
type MicroCompactorConfig struct {
	// TokenThreshold is the per-message token count above which the
	// compactor will attempt to store the content in LCM and replace it
	// with a reference. If zero, defaults to LargeOutputThreshold.
	TokenThreshold int64

	// PreviewChars is the number of leading characters included in the
	// inline preview when a large output is replaced. If zero, defaults
	// to previewChars (2000).
	PreviewChars int

	// Store is the LCM store used for persisting large text content.
	// Required.
	Store *Store

	// SessionID is the session this compactor operates on. Set via
	// WithSessionID before calling Compact.
	SessionID string

	// ReplacementStore is an optional ContentReplacementStore for
	// recording content replacements during compaction. When nil,
	// replacement recording is skipped (nil-safe).
	ReplacementStore ContentReplacementStore

	// Round is the current compaction round number, used when recording
	// replacements.
	Round int

	// CacheAware enables Anthropic cache re-ordering after large output
	// replacement. When true AND ProviderType is "anthropic", the compactor
	// triggers cache section re-ordering via CacheOptimizer after processing
	// oversized messages.
	CacheAware bool

	// ProviderType identifies the LLM provider (e.g. "anthropic", "openai").
	// Used in conjunction with CacheAware to determine whether cache
	// re-ordering should be applied.
	ProviderType string

	// CacheOptimizer is an optional CacheOptimizer instance used for cache
	// re-ordering when CacheAware is true. When nil, cache re-ordering is
	// skipped even if CacheAware is true.
	CacheOptimizer *CacheOptimizer
}

func (c MicroCompactorConfig) threshold() int64 {
	if c.TokenThreshold > 0 {
		return c.TokenThreshold
	}
	return LargeOutputThreshold
}

func (c MicroCompactorConfig) previewLimit() int {
	if c.PreviewChars > 0 {
		return c.PreviewChars
	}
	return previewChars
}

// MicroCompactor is Layer 1 of the 9-layer compaction framework. It scans
// context entries for messages whose estimated token count exceeds a
// threshold, stores their content in the LCM large-files table, and replaces
// the inline text with a compact reference + preview.
//
// This formalizes the ad-hoc interception logic in message_decorator.go's
// Create method into a reusable, testable compaction layer.
type MicroCompactor struct {
	cfg MicroCompactorConfig
}

// NewMicroCompactor creates a Layer 1 MicroCompactor with the given config.
func NewMicroCompactor(cfg MicroCompactorConfig) *MicroCompactor {
	return &MicroCompactor{cfg: cfg}
}

// Name returns "micro-compactor".
func (m *MicroCompactor) Name() string { return "micro-compactor" }

// Priority returns 1 (Layer 1).
func (m *MicroCompactor) Priority() int { return 1 }

// ShouldCompact reports whether there are context entries whose token count
// exceeds the configured threshold. It checks the store for oversized
// messages that have not yet been micro-compacted.
func (m *MicroCompactor) ShouldCompact(ctx context.Context, budget Budget) bool {
	if m.cfg.Store == nil || m.cfg.SessionID == "" {
		return false
	}

	entries, err := m.cfg.Store.GetContextEntries(ctx, m.cfg.SessionID)
	if err != nil {
		return false
	}

	threshold := m.cfg.threshold()
	for _, entry := range entries {
		if entry.ItemType == "message" && entry.TokenCount > threshold {
			return true
		}
	}
	return false
}

// Compact scans context entries for oversized messages, stores their content
// in the LCM large-files table, and replaces the inline content with a
// reference + preview. It returns the number of tokens freed and items
// affected.
func (m *MicroCompactor) Compact(ctx context.Context, budget Budget) (*CompactionLayerResult, error) {
	if m.cfg.Store == nil {
		return nil, fmt.Errorf("micro-compactor: %w", ErrStoreIsNil)
	}
	if m.cfg.SessionID == "" {
		return nil, fmt.Errorf("micro-compactor: %w", ErrSessionIDEmpty)
	}

	entries, err := m.cfg.Store.GetContextEntries(ctx, m.cfg.SessionID)
	if err != nil {
		return nil, fmt.Errorf("getting context entries: %w", err)
	}

	threshold := m.cfg.threshold()
	previewLimit := m.cfg.previewLimit()
	var totalFreed int64
	var affected int

	for _, entry := range entries {
		if entry.ItemType != "message" || entry.TokenCount <= threshold {
			continue
		}

		if m.isPinned(ctx, entry) {
			continue
		}

		// Fetch the message content.
		msgs, err := m.cfg.Store.GetMessages(ctx, m.cfg.SessionID)
		if err != nil {
			return nil, fmt.Errorf("getting messages: %w", err)
		}

		// Find the specific message.
		var msgText string
		for _, msg := range msgs {
			if msg.ID == entry.MessageID {
				msgText = msg.Content
				break
			}
		}

		if msgText == "" {
			continue
		}

		// Check if already stored (contains a reference marker).
		if isAlreadyReferenced(msgText) {
			continue
		}

		// Store the large content.
		fileID, err := m.cfg.Store.InsertLargeTextContent(ctx, m.cfg.SessionID, msgText, "")
		if err != nil {
			// If storage fails, skip this entry rather than failing the
			// entire compaction pass.
			continue
		}

		// Build the replacement reference.
		preview := truncateString(msgText, previewLimit)
		ref := fmt.Sprintf("[Large File Stored: %s]\nLCM File ID: %s\n\nPreview (first %d chars):\n%s",
			fileID, fileID, previewLimit, preview)

		newTokens := EstimateTokens(ref)
		freed := max(entry.TokenCount-newTokens, 0)

		m.recordReplacement(ctx, entry, fileID, int(entry.TokenCount), int(newTokens))

		totalFreed += freed
		affected++
	}

	result := &CompactionLayerResult{
		LayerName:     m.Name(),
		TokensFreed:   totalFreed,
		ItemsAffected: affected,
		ActionTaken:   affected > 0,
	}

	if m.cfg.CacheAware && strings.EqualFold(m.cfg.ProviderType, "anthropic") && m.cfg.CacheOptimizer != nil {
		cacheResult, err := m.cfg.CacheOptimizer.ReorderForCache(ctx)
		if err != nil {
			slog.Warn("Micro-compactor: cache re-ordering failed",
				slog.String("session_id", m.cfg.SessionID),
				slog.String("error", err.Error()),
			)
		} else if cacheResult != nil && cacheResult.ActionTaken {
			result.TokensFreed += cacheResult.TokensFreed
		}
	}

	return result, nil
}

// isAlreadyReferenced checks whether the text already contains an LCM large-
// output reference marker, indicating that micro-compaction has already been
// applied to this content.
func isAlreadyReferenced(text string) bool {
	return strings.Contains(text, "[Large File Stored:") ||
		strings.Contains(text, "[Large Tool Output Stored:") ||
		strings.Contains(text, "[Large User Text Stored:")
}

// isPinned checks whether the context entry has an active pinned replacement
// in the ReplacementStore, meaning it should be skipped during compaction.
func (m *MicroCompactor) isPinned(ctx context.Context, entry ContextEntry) bool {
	if m.cfg.ReplacementStore == nil {
		return false
	}
	replacements, err := m.cfg.ReplacementStore.GetBySessionPosition(ctx, m.cfg.SessionID, entry.Position)
	if err != nil {
		return false
	}
	for _, r := range replacements {
		if r.State == ReplacementPinned {
			return true
		}
	}
	return false
}

// recordReplacement records a content replacement in the ReplacementStore.
// Failures are logged but do not abort compaction.
func (m *MicroCompactor) recordReplacement(ctx context.Context, entry ContextEntry, fileID string, originalTokens, replacementTokens int) {
	if m.cfg.ReplacementStore == nil {
		return
	}
	_, err := m.cfg.ReplacementStore.RecordReplacement(ctx, ContentReplacement{
		SessionID:             m.cfg.SessionID,
		Position:              entry.Position,
		MessageID:             sql.NullString{String: entry.MessageID, Valid: entry.MessageID != ""},
		FileID:                sql.NullString{String: fileID, Valid: fileID != ""},
		State:                 ReplacementActive,
		Round:                 m.cfg.Round,
		OriginalTokenCount:    originalTokens,
		ReplacementTokenCount: replacementTokens,
	})
	if err != nil {
		slog.Warn("Micro-compactor: failed to record content replacement",
			slog.String("session_id", m.cfg.SessionID),
			slog.Int64("position", entry.Position),
			slog.String("error", err.Error()),
		)
	}
}

// ---------------------------------------------------------------------------
// Layer 2: DedupCompactionLayer
// ---------------------------------------------------------------------------

// DedupCompactionLayer collapses duplicate/near-duplicate content within a
// session. It detects messages with identical content hashes and keeps only
// the most recent occurrence, replacing earlier ones with archive stubs.
type DedupCompactionLayer struct {
	store     *Store
	sessionID string
}

// NewDedupCompactionLayer creates a Layer 2 dedup compaction layer.
func NewDedupCompactionLayer(store *Store, sessionID string) *DedupCompactionLayer {
	return &DedupCompactionLayer{store: store, sessionID: sessionID}
}

func (d *DedupCompactionLayer) Name() string  { return "dedup-compaction" }
func (d *DedupCompactionLayer) Priority() int { return 2 }

// ShouldCompact returns true when there are context entries with duplicate
// content hashes (after stripping whitespace).
func (d *DedupCompactionLayer) ShouldCompact(ctx context.Context, _ Budget) bool {
	if d.store == nil || d.sessionID == "" {
		return false
	}
	msgs, err := d.store.GetMessages(ctx, d.sessionID)
	if err != nil {
		return false
	}
	msgContent := make(map[string]string, len(msgs))
	for _, m := range msgs {
		msgContent[m.ID] = m.Content
	}
	entries, err := d.store.GetContextEntries(ctx, d.sessionID)
	if err != nil {
		return false
	}
	seen := make(map[string]struct{})
	for _, entry := range entries {
		if entry.ItemType == "message" && entry.MessageID != "" {
			h := contentHash(msgContent[entry.MessageID])
			if _, ok := seen[h]; ok {
				return true
			}
			seen[h] = struct{}{}
		}
	}
	return false
}

// Compact groups entries by content hash and archives all but the latest in
// each duplicate group.
func (d *DedupCompactionLayer) Compact(ctx context.Context, _ Budget) (*CompactionLayerResult, error) {
	if d.store == nil {
		return nil, fmt.Errorf("dedup-compaction: %w", ErrStoreIsNil)
	}
	if d.sessionID == "" {
		return nil, fmt.Errorf("dedup-compaction: %w", ErrSessionIDEmpty)
	}

	msgs, err := d.store.GetMessages(ctx, d.sessionID)
	if err != nil {
		return nil, fmt.Errorf("dedup-compaction: getting messages: %w", err)
	}
	msgContent := make(map[string]string, len(msgs))
	for _, m := range msgs {
		msgContent[m.ID] = m.Content
	}

	entries, err := d.store.GetContextEntries(ctx, d.sessionID)
	if err != nil {
		return nil, fmt.Errorf("dedup-compaction: getting context entries: %w", err)
	}

	type hashGroup struct {
		entries []ContextEntry
	}
	groups := make(map[string]*hashGroup)
	for _, entry := range entries {
		if entry.ItemType != "message" || entry.MessageID == "" {
			continue
		}
		text := msgContent[entry.MessageID]
		h := contentHash(text)
		if groups[h] == nil {
			groups[h] = &hashGroup{}
		}
		groups[h].entries = append(groups[h].entries, entry)
	}

	var totalFreed int64
	var affected int

	for _, group := range groups {
		if len(group.entries) < 2 {
			continue
		}

		// Keep the last entry (highest position), archive the rest.
		for i := 0; i < len(group.entries)-1; i++ {
			entry := group.entries[i]
			text := msgContent[entry.MessageID]
			if err := d.store.CreateArchiveStub(ctx, entry.MessageID, d.sessionID, text, entry.TokenCount); err != nil {
				continue
			}
			totalFreed += entry.TokenCount
			affected++
		}
	}

	return &CompactionLayerResult{
		LayerName:     d.Name(),
		TokensFreed:   totalFreed,
		ItemsAffected: affected,
		ActionTaken:   affected > 0,
	}, nil
}

// contentHash produces a deterministic SHA-256 hash of the stripped content.
func contentHash(text string) string {
	stripped := strings.TrimSpace(text)
	h := sha256.Sum256([]byte(stripped))
	return fmt.Sprintf("%x", h[:])
}

// ---------------------------------------------------------------------------
// Layer 3: StaleEvictionLayer
// ---------------------------------------------------------------------------

// StaleEvictionLayer evicts tool-output messages older than a configurable
// threshold. It removes stale context to free up budget for active work.
//
// Since context items do not carry a created_at timestamp, this layer resolves
// staleness by joining with the messages table created_at column.
type StaleEvictionLayer struct {
	store        *Store
	sessionID    string
	maxAge       time.Duration
	maxEvictions int
}

// NewStaleEvictionLayer creates a Layer 3 stale eviction layer. If maxAge is
// zero, it defaults to 30 minutes.
func NewStaleEvictionLayer(store *Store, sessionID string, maxAge time.Duration) *StaleEvictionLayer {
	if maxAge == 0 {
		maxAge = 30 * time.Minute
	}
	return &StaleEvictionLayer{
		store:        store,
		sessionID:    sessionID,
		maxAge:       maxAge,
		maxEvictions: 5,
	}
}

func (s *StaleEvictionLayer) Name() string  { return "stale-eviction" }
func (s *StaleEvictionLayer) Priority() int { return 3 }

// ShouldCompact returns true when there are message context entries whose
// corresponding message was created more than maxAge ago.
func (s *StaleEvictionLayer) ShouldCompact(ctx context.Context, _ Budget) bool {
	if s.store == nil || s.sessionID == "" {
		return false
	}
	stale, err := s.findStaleEntries(ctx)
	if err != nil {
		return false
	}
	return len(stale) > 0
}

// Compact evicts up to maxEvictions stale message entries, archiving them
// with CreateArchiveStub.
func (s *StaleEvictionLayer) Compact(ctx context.Context, _ Budget) (*CompactionLayerResult, error) {
	if s.store == nil {
		return nil, fmt.Errorf("stale-eviction: %w", ErrStoreIsNil)
	}
	if s.sessionID == "" {
		return nil, fmt.Errorf("stale-eviction: %w", ErrSessionIDEmpty)
	}

	stale, err := s.findStaleEntries(ctx)
	if err != nil {
		return nil, fmt.Errorf("stale-eviction: %w", err)
	}

	msgs, err := s.store.GetMessages(ctx, s.sessionID)
	if err != nil {
		return nil, fmt.Errorf("stale-eviction: getting messages: %w", err)
	}
	msgContent := make(map[string]string, len(msgs))
	for _, m := range msgs {
		msgContent[m.ID] = m.Content
	}

	limit := min(len(stale), s.maxEvictions)
	var totalFreed int64
	var affected int

	for i := range limit {
		entry := stale[i]
		text := msgContent[entry.MessageID]
		if err := s.store.CreateArchiveStub(ctx, entry.MessageID, s.sessionID, text, entry.TokenCount); err != nil {
			continue
		}
		totalFreed += entry.TokenCount
		affected++
	}

	return &CompactionLayerResult{
		LayerName:     s.Name(),
		TokensFreed:   totalFreed,
		ItemsAffected: affected,
		ActionTaken:   affected > 0,
	}, nil
}

// findStaleEntries returns message context entries whose corresponding message
// was created more than maxAge ago. The messages table stores created_at as a
// Unix timestamp in milliseconds.
func (s *StaleEvictionLayer) findStaleEntries(ctx context.Context) ([]ContextEntry, error) {
	entries, err := s.store.GetContextEntries(ctx, s.sessionID)
	if err != nil {
		return nil, err
	}

	msgCreatedAt, err := s.getMessageCreatedAt(ctx)
	if err != nil {
		return nil, err
	}

	threshold := time.Now().Add(-s.maxAge).Unix()

	var stale []ContextEntry
	for _, entry := range entries {
		if entry.ItemType != "message" || entry.MessageID == "" {
			continue
		}
		createdAt, ok := msgCreatedAt[entry.MessageID]
		if !ok {
			continue
		}
		if createdAt < threshold {
			stale = append(stale, entry)
		}
	}
	return stale, nil
}

func (s *StaleEvictionLayer) getMessageCreatedAt(ctx context.Context) (map[string]int64, error) {
	rows, err := s.store.rawDB.QueryContext(ctx,
		"SELECT id, created_at FROM messages WHERE session_id = ?", s.sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying message timestamps: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var id string
		var createdAt int64
		if err := rows.Scan(&id, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning message timestamp: %w", err)
		}
		result[id] = createdAt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating message timestamps: %w", err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Layer 5: AdjacentCondensationLayer
// ---------------------------------------------------------------------------

// AdjacentCondensationLayer merges adjacent summary entries when their
// combined token count is under a threshold, reducing context fragmentation.
type AdjacentCondensationLayer struct {
	store         *Store
	sessionID     string
	maxMergedSize int64
}

// NewAdjacentCondensationLayer creates a Layer 5 adjacent condensation layer.
// If maxMergedSize is zero, it defaults to 4000 tokens.
func NewAdjacentCondensationLayer(store *Store, sessionID string, maxMergedSize int64) *AdjacentCondensationLayer {
	if maxMergedSize == 0 {
		maxMergedSize = 4000
	}
	return &AdjacentCondensationLayer{
		store:         store,
		sessionID:     sessionID,
		maxMergedSize: maxMergedSize,
	}
}

func (a *AdjacentCondensationLayer) Name() string  { return "adjacent-condensation" }
func (a *AdjacentCondensationLayer) Priority() int { return 5 }

// ShouldCompact returns true when there are adjacent summary entries whose
// combined token count is below maxMergedSize.
func (a *AdjacentCondensationLayer) ShouldCompact(ctx context.Context, _ Budget) bool {
	if a.store == nil || a.sessionID == "" {
		return false
	}
	entries, err := a.store.GetContextEntries(ctx, a.sessionID)
	if err != nil {
		return false
	}
	for i := 0; i < len(entries)-1; i++ {
		if entries[i].ItemType == "summary" && entries[i+1].ItemType == "summary" {
			if entries[i].TokenCount+entries[i+1].TokenCount < a.maxMergedSize {
				return true
			}
		}
	}
	return false
}

// Compact iterates pairs of adjacent summary entries, merging those whose
// combined tokens are under maxMergedSize. Original summaries are archived
// with CreateArchiveStub and a new merged summary is created.
func (a *AdjacentCondensationLayer) Compact(ctx context.Context, _ Budget) (*CompactionLayerResult, error) {
	if a.store == nil {
		return nil, fmt.Errorf("adjacent-condensation: %w", ErrStoreIsNil)
	}
	if a.sessionID == "" {
		return nil, fmt.Errorf("adjacent-condensation: %w", ErrSessionIDEmpty)
	}

	entries, err := a.store.GetContextEntries(ctx, a.sessionID)
	if err != nil {
		return nil, fmt.Errorf("adjacent-condensation: getting entries: %w", err)
	}

	var totalFreed int64
	var affected int
	merged := make(map[int]bool)

	for i := 0; i < len(entries)-1; i++ {
		if merged[i] {
			continue
		}
		if entries[i].ItemType != "summary" || entries[i+1].ItemType != "summary" {
			continue
		}
		combined := entries[i].TokenCount + entries[i+1].TokenCount
		if combined >= a.maxMergedSize {
			continue
		}

		aText := entries[i].SummaryContent
		bText := entries[i+1].SummaryContent

		mergedText := aText + "\n\n---\n\n" + bText
		mergedTokens := EstimateTokens(mergedText)

		mergedID := "sum_merged_" + contentHash(aText + bText)[:16]
		if err := a.store.InsertLeafSummary(ctx, a.store.q, a.sessionID, mergedID, mergedText, mergedTokens, []string{}, nil); err != nil {
			continue
		}

		if err := a.archiveSummary(ctx, entries[i].SummaryID, aText, entries[i].TokenCount); err != nil {
			continue
		}
		if err := a.archiveSummary(ctx, entries[i+1].SummaryID, bText, entries[i+1].TokenCount); err != nil {
			continue
		}

		freed := combined - mergedTokens
		totalFreed += max(freed, 0)
		affected += 2
		merged[i] = true
		merged[i+1] = true
	}

	return &CompactionLayerResult{
		LayerName:     a.Name(),
		TokensFreed:   totalFreed,
		ItemsAffected: affected,
		ActionTaken:   affected > 0,
	}, nil
}

func (a *AdjacentCondensationLayer) archiveSummary(ctx context.Context, summaryID, content string, tokenCount int64) error {
	truncated := truncateString(content, 200)
	stubContent := "[Archived from " + summaryID + "] " + truncated
	_, err := a.store.rawDB.ExecContext(ctx,
		"UPDATE lcm_summaries SET kind = ?, content = ?, token_count = ? WHERE summary_id = ?",
		KindArchiveStub, stubContent, tokenCount/10, summaryID,
	)
	return err
}
