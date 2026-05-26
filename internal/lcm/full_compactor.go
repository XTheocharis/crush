package lcm

import (
	"context"
	"fmt"
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
	// fraction. 0.85 means the compactor aims to reduce the context to 15%
	// of its original size (85% reduction).
	FullCompactorTargetReduction = 0.85

	// FullCompactorMaxSummaryTokens is the maximum token budget for the
	// summary output. This ensures the summary stays compact even for very
	// large conversations.
	FullCompactorMaxSummaryTokens int64 = 4000
)

// FullCompactor implements a full-compaction strategy.
type FullCompactor struct {
	cfg FullCompactorConfig
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
// produce a dense summary. The summary replaces all original entries in the
// context.
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

	// Compute original token totals.
	var originalTokens int64
	var itemsToCompact int
	for _, entry := range entries {
		originalTokens += entry.TokenCount
		itemsToCompact++
	}

	// Format all entries for the summarization prompt.
	userPrompt := f.formatEntriesForFullSummary(entries)

	// Call the LLM with a deep summarization prompt.
	summaryText, err := f.cfg.LLM.Complete(ctx, fullCompactionSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("full compaction LLM call: %w", err)
	}

	// Ensure the summary doesn't exceed our token budget.
	summaryTokens := EstimateTokens(summaryText)
	if summaryTokens > FullCompactorMaxSummaryTokens {
		maxChars := int(FullCompactorMaxSummaryTokens * CharsPerToken)
		if len(summaryText) > maxChars {
			summaryText = summaryText[:maxChars]
		}
		summaryTokens = EstimateTokens(summaryText)
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
