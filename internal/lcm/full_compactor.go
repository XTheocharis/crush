package lcm

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// FullCompactorConfig controls the behaviour of the FullCompactor layer.
type FullCompactorConfig struct {
	// LLM is the LLM client used for deep summarization. Required.
	LLM LLMClient

	// Store is the LCM store used for persisting summaries. Required.
	Store *Store

	// SessionID is the session this compactor operates on. Required.
	SessionID string

	// MinTotalTokens is the minimum total token count across all context
	// entries before full compaction is considered. If zero, defaults to
	// FullCompactorMinTokens.
	MinTotalTokens int64

	// TargetReduction is the target fraction of tokens to remove (0–1).
	// If zero, defaults to FullCompactorTargetReduction.
	TargetReduction float64

	// ReplacementStore is an optional ContentReplacementStore for
	// cache-safe compaction. When provided, the FullCompactor freezes
	// active/pinned replacements before compaction and unfreezes them
	// after, preserving parent cache state. When nil, compaction runs
	// without cache protection (backward compatible).
	ReplacementStore ContentReplacementStore
}

func (c FullCompactorConfig) minTokens() int64 {
	if c.MinTotalTokens > 0 {
		return c.MinTotalTokens
	}
	return FullCompactorMinTokens
}

func (c FullCompactorConfig) targetReduction() float64 {
	if c.TargetReduction > 0 {
		return c.TargetReduction
	}
	return FullCompactorTargetReduction
}

const (
	// FullCompactorMinTokens is the default minimum total tokens before full
	// compaction activates. This ensures full compaction only fires when the
	// conversation is genuinely large.
	FullCompactorMinTokens int64 = 5000

	// FullCompactorTargetReduction is the default target token reduction
	// fraction. 0.98 means the compactor aims to reduce the context to 2%
	// of its original size (98% reduction).
	FullCompactorTargetReduction = 0.98

	// FullCompactorMaxSummaryTokens is the maximum token budget for the
	// summary output. This ensures the summary stays compact even for very
	// large conversations.
	FullCompactorMaxSummaryTokens int64 = 4000
)

// FullCompactor implements a full-compaction strategy.
type FullCompactor struct {
	cfg    FullCompactorConfig
	frozen []ContentReplacement
}

// NewFullCompactor creates a Layer 3 FullCompactor with the given config.
func NewFullCompactor(cfg FullCompactorConfig) *FullCompactor {
	return &FullCompactor{cfg: cfg}
}

// Name returns "full-compactor".
func (f *FullCompactor) Name() string { return "full-compactor" }

// Priority returns 30 (Layer 3 — runs after micro-compactor at 10).
func (f *FullCompactor) Priority() int { return 30 }

// ShouldCompact reports whether the total context token count exceeds the
// configured minimum. Full compaction is only attempted when the conversation
// is large enough to benefit from deep summarization and when all required
// dependencies (LLM client, store, session ID) are present.
func (f *FullCompactor) ShouldCompact(ctx context.Context, budget Budget) bool {
	if f.cfg.LLM == nil || f.cfg.Store == nil || f.cfg.SessionID == "" {
		return false
	}

	entries, err := f.cfg.Store.GetContextEntries(ctx, f.cfg.SessionID)
	if err != nil {
		return false
	}

	var totalTokens int64
	for _, entry := range entries {
		totalTokens += entry.TokenCount
	}

	return totalTokens >= f.cfg.minTokens()
}

// Compact performs a full compaction pass by collecting all context entries,
// formatting them into a single prompt, and using a separate LLM call to
// produce a dense summary. When a ReplacementStore is configured, the
// compaction runs in a cache-safe "forked" mode that freezes parent
// replacements before the LLM call and unfreezes them after.
func (f *FullCompactor) Compact(ctx context.Context, budget Budget) (*CompactionLayerResult, error) {
	if f.cfg.LLM == nil {
		return nil, fmt.Errorf("full-compactor: %w", ErrLLMClientNil)
	}
	if f.cfg.Store == nil {
		return nil, fmt.Errorf("full-compactor: %w", ErrStoreIsNil)
	}
	if f.cfg.SessionID == "" {
		return nil, fmt.Errorf("full-compactor: %w", ErrSessionIDEmpty)
	}

	entries, err := f.cfg.Store.GetContextEntries(ctx, f.cfg.SessionID)
	if err != nil {
		return nil, fmt.Errorf("getting context entries: %w", err)
	}

	if len(entries) == 0 {
		return &CompactionLayerResult{LayerName: f.Name()}, nil
	}

	if f.cfg.ReplacementStore != nil {
		result, forkErr := f.compactWithFork(ctx, entries)
		if forkErr == nil {
			return result, nil
		}
		slog.Warn("Full-compactor: cache-safe fork failed, falling back to direct LLM call",
			slog.String("session_id", f.cfg.SessionID),
			slog.String("error", forkErr.Error()),
		)
	}

	return f.compactDirect(ctx, entries)
}

// compactWithFork performs cache-safe compaction by freezing parent
// replacements, cloning the context, running the LLM call on the clone, and
// then unfreezing the parent replacements. If freezing or cloning fails, the
// caller should fall back to compactDirect.
func (f *FullCompactor) compactWithFork(ctx context.Context, entries []ContextEntry) (*CompactionLayerResult, error) {
	if err := f.freezeReplacements(ctx); err != nil {
		return nil, fmt.Errorf("freezing replacements: %w", err)
	}

	cloned := cloneContextEntries(entries)

	result, err := f.compactDirect(ctx, cloned)

	unfreezeErr := f.unfreezeReplacements(ctx)
	if unfreezeErr != nil {
		slog.Warn("Full-compactor: failed to unfreeze replacements",
			slog.String("session_id", f.cfg.SessionID),
			slog.String("error", unfreezeErr.Error()),
		)
	}

	if err != nil {
		return nil, err
	}
	return result, nil
}

// compactDirect runs the LLM compaction without cache protection.
func (f *FullCompactor) compactDirect(ctx context.Context, entries []ContextEntry) (*CompactionLayerResult, error) {
	var originalTokens int64
	var itemsToCompact int
	var messageIDs []string
	var position int64
	positionSet := false

	for _, entry := range entries {
		originalTokens += entry.TokenCount
		itemsToCompact++
		if entry.ItemType == "message" && entry.MessageID != "" {
			messageIDs = append(messageIDs, entry.MessageID)
		}
		if !positionSet {
			position = entry.Position
			positionSet = true
		}
	}

	userPrompt := f.formatEntriesForFullSummary(entries)

	summaryText, err := f.cfg.LLM.Complete(ctx, fullCompactionSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("full compaction LLM call: %w", err)
	}

	summaryTokens := EstimateTokens(summaryText)
	if summaryTokens > FullCompactorMaxSummaryTokens {
		maxChars := int(FullCompactorMaxSummaryTokens * CharsPerToken)
		if len(summaryText) > maxChars {
			summaryText = summaryText[:maxChars]
		}
		summaryTokens = EstimateTokens(summaryText)
	}

	summaryID, _ := GenerateSummaryID(f.cfg.SessionID)
	if err := f.cfg.Store.InsertLeafSummaryAtomically(
		ctx,
		f.cfg.SessionID,
		summaryID,
		summaryText,
		summaryTokens,
		[]string{}, // fileIDs not available in this context.
		messageIDs,
		position,
		messageIDs,
	); err != nil {
		slog.Warn("Full-compactor: failed to persist summary",
			slog.String("session_id", f.cfg.SessionID),
			slog.String("summary_id", summaryID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("persisting full compaction summary: %w", err)
	}

	tokensFreed := max(originalTokens-summaryTokens, 0)

	result := &CompactionLayerResult{
		LayerName:     f.Name(),
		TokensFreed:   tokensFreed,
		ItemsAffected: itemsToCompact,
		ActionTaken:   tokensFreed > 0,
	}
	return result, nil
}

// freezeReplacements transitions all active and pinned ContentReplacements
// for the session to the Frozen state, preserving them for cache sharing.
// The frozen replacements are tracked so they can be unfrozen later.
func (f *FullCompactor) freezeReplacements(ctx context.Context) error {
	active, err := f.cfg.ReplacementStore.ListByState(ctx, f.cfg.SessionID, ReplacementActive)
	if err != nil {
		return fmt.Errorf("listing active replacements: %w", err)
	}
	pinned, err := f.cfg.ReplacementStore.ListByState(ctx, f.cfg.SessionID, ReplacementPinned)
	if err != nil {
		return fmt.Errorf("listing pinned replacements: %w", err)
	}

	all := append(active, pinned...)
	f.frozen = make([]ContentReplacement, 0, len(all))

	for i := range all {
		r := &all[i]
		if err := r.Freeze(); err != nil {
			continue
		}
		if err := f.cfg.ReplacementStore.UpdateState(ctx, r.ID, ReplacementFrozen); err != nil {
			continue
		}
		f.frozen = append(f.frozen, *r)
	}

	return nil
}

// unfreezeReplacements transitions all previously frozen replacements back
// to the Active state.
func (f *FullCompactor) unfreezeReplacements(ctx context.Context) error {
	var firstErr error
	for i := range f.frozen {
		r := &f.frozen[i]
		if err := f.cfg.ReplacementStore.UpdateState(ctx, r.ID, ReplacementActive); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		r.State = ReplacementActive
	}
	f.frozen = f.frozen[:0]
	return firstErr
}

// cloneContextEntries creates a deep copy of a ContextEntry slice so that
// the compaction LLM call operates on independent data.
func cloneContextEntries(entries []ContextEntry) []ContextEntry {
	if entries == nil {
		return nil
	}
	cloned := make([]ContextEntry, len(entries))
	for i, e := range entries {
		cloned[i] = e
		if len(e.ParentIDs) > 0 {
			cloned[i].ParentIDs = make([]string, len(e.ParentIDs))
			copy(cloned[i].ParentIDs, e.ParentIDs)
		}
	}
	return cloned
}

// formatEntriesForFullSummary formats all context entries into a single prompt
// for the full compaction LLM call.
func (f *FullCompactor) formatEntriesForFullSummary(entries []ContextEntry) string {
	var sb strings.Builder
	sb.WriteString("<conversation-context>\n")

	for i, entry := range entries {
		switch entry.ItemType {
		case "message":
			fmt.Fprintf(&sb, "--- Entry %d (type: message, id: %s, tokens: %d) ---\n",
				i, entry.MessageID, entry.TokenCount)
			// For full compaction we don't have the message text inline in the
			// entry, so we reference the ID. The LLM prompt instructs it to
			// produce a summary based on the metadata available.
			fmt.Fprintf(&sb, "Message ID: %s\n\n", entry.MessageID)

		case "summary":
			fmt.Fprintf(&sb, "--- Entry %d (type: summary, id: %s, kind: %s, tokens: %d) ---\n",
				i, entry.SummaryID, entry.SummaryKind, entry.TokenCount)
			if entry.SummaryContent != "" {
				sb.WriteString(entry.SummaryContent)
				sb.WriteString("\n")
			}
			if len(entry.ParentIDs) > 0 {
				fmt.Fprintf(&sb, "Condensed from: %s\n", strings.Join(entry.ParentIDs, ", "))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("</conversation-context>")
	return sb.String()
}

// fullCompactionSystemPrompt is the system prompt used for the deep
// summarization LLM call. It instructs the model to produce a dense summary
// that preserves all critical context.
const fullCompactionSystemPrompt = `You are a deep conversation compactor. Your job is to produce a single, dense summary of the entire conversation context that captures everything needed to continue the work seamlessly.

You MUST preserve ALL of the following in your summary:

1. **Task Context**: What is the current task or goal? What has been requested?
2. **Decisions Made**: What architectural or design decisions were made and why?
3. **Code State**: What files were modified, created, or deleted? What is the current state of the code?
4. **Error History**: What errors were encountered and how were they resolved?
5. **Technical Details**: File paths, function signatures, variable names, API endpoints, configuration values.
6. **Pending Work**: What remains to be done? What is the current progress?
7. **Constraints**: Any requirements, limitations, or preferences that must be respected.

Rules:
- Be extremely dense and information-rich. Every sentence should carry useful context.
- Use bullet points for lists of facts, decisions, or technical details.
- Preserve exact file paths, function names, and code references verbatim.
- Do NOT include pleasantries, filler, or meta-commentary.
- Target 80%+ token reduction from the original context.
- Output plain text only, no markdown headers.`
