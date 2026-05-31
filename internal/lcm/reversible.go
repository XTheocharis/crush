package lcm

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TargetDetail controls the level of detail returned by Decompress.
type TargetDetail string

const (
	// TargetFull returns complete original messages.
	TargetFull TargetDetail = "full"
	// TargetPartial returns messages with truncated content (first 200 chars).
	TargetPartial TargetDetail = "partial"
	// TargetMetadata returns only message metadata (role, seq, id) with no content.
	TargetMetadata TargetDetail = "metadata"
)

// partialContentLimit is the maximum character count for partial detail messages.
const partialContentLimit = 200

// DecompressCommand requests decompression of a reversible summary.
type DecompressCommand struct {
	SummaryID    string
	TargetDetail TargetDetail
}

// BlockIDTracker assigns sequential block IDs to content chunks as they enter
// the context window.
type BlockIDTracker struct {
	mu      sync.Mutex
	counter int64
	prefix  string
}

// NewBlockIDTracker creates a tracker with the given prefix.
func NewBlockIDTracker(prefix string) *BlockIDTracker {
	return &BlockIDTracker{prefix: prefix}
}

// NextBlockID returns the next sequential block ID.
func (t *BlockIDTracker) NextBlockID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counter++
	return fmt.Sprintf("%s-%d", t.prefix, t.counter)
}

// Current returns the current counter value.
func (t *BlockIDTracker) Current() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counter
}

// ReversibleCompactor saves original messages alongside compressed summaries
// so that selected ("anchor") summaries can be decompressed later. Only
// summaries flagged as reversible store originals — non-reversible summaries
// fall back to the existing ExpandSummary path.
type ReversibleCompactor struct {
	store      *Store
	summarizer *Summarizer
}

// NewReversibleCompactor creates a ReversibleCompactor backed by the given Store.
func NewReversibleCompactor(store *Store) *ReversibleCompactor {
	return &ReversibleCompactor{store: store}
}

// SetSummarizer sets the LLM summarizer used for partial detail compression.
// When nil, partial detail falls back to simple truncation.
func (rc *ReversibleCompactor) SetSummarizer(s *Summarizer) {
	rc.summarizer = s
}

// SaveReversibleState persists the original messages for a summary so it can
// be decompressed later. This should only be called for "anchor" summaries
// that have been flagged as reversible.
func (rc *ReversibleCompactor) SaveReversibleState(ctx context.Context, summaryID string, sessionID string, kind string, content string, tokenCount int64, messages []MessageForSummary, blockID string) error {
	msgsJSON, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("marshaling original messages: %w", err)
	}

	return rc.store.InsertLcmSummaryWithBlock(ctx, summaryID, sessionID, kind, content, tokenCount, nil, blockID, string(msgsJSON))
}

// DeleteReversibleState removes a reversible summary and its associated
// messages and parent links. This is used to clean up orphaned state when
// the enclosing transaction fails after SaveReversibleState succeeded.
func (rc *ReversibleCompactor) DeleteReversibleState(ctx context.Context, summaryID string) {
	// Order matters: child tables first, then the summary row.
	_ = rc.store.queries.DeleteLcmSummaryMessages(ctx, summaryID)
	_ = rc.store.queries.DeleteLcmSummaryParents(ctx, summaryID)
	_ = rc.store.queries.DeleteLcmSummary(ctx, summaryID)
}

// IsReversible checks whether a summary has stored reversible state.
func (rc *ReversibleCompactor) IsReversible(ctx context.Context, blockID string) (bool, error) {
	_, found, err := rc.store.ExpandLossless(ctx, blockID)
	if err != nil {
		return false, fmt.Errorf("checking reversible state: %w", err)
	}
	return found, nil
}

// Decompress retrieves original messages from a reversible summary. If the
// summary is not reversible (no stored state), it falls back to ExpandSummary.
// The returned messages are filtered according to the TargetDetail in the
// DecompressCommand.
func (rc *ReversibleCompactor) Decompress(ctx context.Context, cmd DecompressCommand) ([]MessageForSummary, error) {
	msgs, err := rc.loadOriginalMessages(ctx, cmd.SummaryID)
	if err != nil {
		return nil, fmt.Errorf("decompressing summary %s: %v: %w", cmd.SummaryID, ErrDecompressFailed, err)
	}
	return rc.applyDetailLevel(ctx, msgs, cmd.TargetDetail), nil
}

// loadOriginalMessages attempts to load original messages via block_id.
// If no block_id or no lossless content exists, it falls back to
// ExpandSummary for graceful degradation.
func (rc *ReversibleCompactor) loadOriginalMessages(ctx context.Context, summaryID string) ([]MessageForSummary, error) {
	var blockID string
	err := rc.store.rawDB.QueryRowContext(ctx,
		`SELECT block_id FROM lcm_summaries WHERE summary_id = ?`,
		summaryID,
	).Scan(&blockID)
	if err != nil {
		if err == sql.ErrNoRows {
			msgs, expandErr := rc.store.ExpandSummary(ctx, summaryID)
			if expandErr != nil {
				return nil, fmt.Errorf("no summary and ExpandSummary failed: %v: %w", ErrExpansionFailed, expandErr)
			}
			return msgs, nil
		}
		return nil, fmt.Errorf("querying block_id: %v: %w", ErrInvalidBlockID, err)
	}

	if blockID == "" {
		msgs, expandErr := rc.store.ExpandSummary(ctx, summaryID)
		if expandErr != nil {
			return nil, fmt.Errorf("no block_id and ExpandSummary failed: %v: %w", ErrExpansionFailed, expandErr)
		}
		return msgs, nil
	}

	rawJSON, found, err := rc.store.ExpandLossless(ctx, blockID)
	if err != nil {
		return nil, fmt.Errorf("expanding block %s: %v: %w", blockID, ErrExpansionFailed, err)
	}
	if !found || rawJSON == "" {
		msgs, expandErr := rc.store.ExpandSummary(ctx, summaryID)
		if expandErr != nil {
			return nil, fmt.Errorf("no lossless content and ExpandSummary failed: %v: %w", ErrExpansionFailed, expandErr)
		}
		return msgs, nil
	}

	var msgs []MessageForSummary
	if unmarshalErr := json.Unmarshal([]byte(rawJSON), &msgs); unmarshalErr != nil {
		return nil, fmt.Errorf("unmarshaling original messages: %w", unmarshalErr)
	}
	return msgs, nil
}

// RecompressCommand requests recompression of a previously decompressed
// summary. It clears the stored original content, restoring the summary
// to its compressed-only state.
type RecompressCommand struct {
	SummaryID string
}

// Recompress reverses a previous decompression by clearing the stored
// original content for the given summary. It verifies that the summary
// exists and currently has decompressed (reversible) state before
// clearing it. The ancestry chain in lcm_summary_parents is preserved.
func (rc *ReversibleCompactor) Recompress(ctx context.Context, cmd RecompressCommand) error {
	// Verify the summary exists and has reversible state.
	var blockID string
	var originalContent string
	err := rc.store.rawDB.QueryRowContext(ctx,
		`SELECT block_id, original_content FROM lcm_summaries WHERE summary_id = ?`,
		cmd.SummaryID,
	).Scan(&blockID, &originalContent)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("recompress: summary %s not found: %w", cmd.SummaryID, ErrSummaryNotFound)
		}
		return fmt.Errorf("recompress: querying summary %s: %w", cmd.SummaryID, err)
	}

	if blockID == "" || originalContent == "" {
		return fmt.Errorf("recompress: summary %s is not in a decompressed state", cmd.SummaryID)
	}

	// Clear the original content to restore compressed-only state.
	_, err = rc.store.rawDB.ExecContext(ctx,
		`UPDATE lcm_summaries SET original_content = '' WHERE summary_id = ?`,
		cmd.SummaryID,
	)
	if err != nil {
		return fmt.Errorf("recompress: clearing original content for %s: %w", cmd.SummaryID, err)
	}

	return nil
}

// IsDecompressed checks whether a summary is currently in a decompressed
// state (has non-empty original_content).
func (rc *ReversibleCompactor) IsDecompressed(ctx context.Context, summaryID string) (bool, error) {
	var originalContent string
	err := rc.store.rawDB.QueryRowContext(ctx,
		`SELECT original_content FROM lcm_summaries WHERE summary_id = ?`,
		summaryID,
	).Scan(&originalContent)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("checking decompressed state: %w", err)
	}
	return originalContent != "", nil
}

// applyDetailLevel filters messages based on the requested detail level.
// For TargetPartial, uses LLM summarization when available (with a 5s timeout),
// falling back to simple truncation when the LLM is unavailable or fails.
func (rc *ReversibleCompactor) applyDetailLevel(ctx context.Context, msgs []MessageForSummary, detail TargetDetail) []MessageForSummary {
	switch detail {
	case TargetFull:
		return msgs
	case TargetPartial:
		result := make([]MessageForSummary, len(msgs))
		for i, m := range msgs {
			content := m.Content
			if len(content) > partialContentLimit {
				content = rc.summarizePartialContent(ctx, content)
			}
			result[i] = MessageForSummary{
				ID:        m.ID,
				SessionID: m.SessionID,
				Seq:       m.Seq,
				Role:      m.Role,
				Content:   content,
			}
		}
		return result
	case TargetMetadata:
		result := make([]MessageForSummary, len(msgs))
		for i, m := range msgs {
			result[i] = MessageForSummary{
				ID:        m.ID,
				SessionID: m.SessionID,
				Seq:       m.Seq,
				Role:      m.Role,
			}
		}
		return result
	default:
		return msgs
	}
}

// summarizePartialContent attempts to generate an LLM summary of content
// limited to partialContentLimit characters. Falls back to truncation when
// the LLM is nil, returns an error, or exceeds the 5-second timeout.
func (rc *ReversibleCompactor) summarizePartialContent(ctx context.Context, content string) string {
	if rc.summarizer == nil {
		return truncatePartial(content)
	}

	llm := rc.summarizer.llmClient()
	if llm == nil {
		return truncatePartial(content)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	summary, err := llm.Complete(timeoutCtx, partialSummarySystemPrompt, content)
	if err != nil {
		if !strings.Contains(err.Error(), "context canceled") {
			slog.Debug("Partial content LLM summary failed, falling back to truncation",
				"error", err)
		}
		return truncatePartial(content)
	}

	summary = strings.TrimSpace(summary)
	if len(summary) > partialContentLimit {
		summary = summary[:partialContentLimit]
	}
	if summary == "" {
		return truncatePartial(content)
	}
	return summary
}

func truncatePartial(content string) string {
	if len(content) > partialContentLimit {
		return content[:partialContentLimit]
	}
	return content
}

// partialSummarySystemPrompt instructs the LLM to produce a compact summary
// that preserves key technical information within the partial content budget.
const partialSummarySystemPrompt = `Summarize the following content in under 200 characters. Preserve function signatures, key decisions, data flows, and error handling patterns. Be concise and technical. Output only the summary, no preamble.`

// ---------------------------------------------------------------------------
// Nested placeholder resolution
// ---------------------------------------------------------------------------

// blockPlaceholderPattern matches `(bN)` placeholders where N is one or more
// digits. These placeholders appear in compressed content to reference other
// compressed blocks.
var blockPlaceholderPattern = regexp.MustCompile(`\(b(\d+)\)`)

// MaxPlaceholderDepth is the maximum recursion depth for resolving nested
// placeholders. Prevents infinite loops when blocks reference each other.
const MaxPlaceholderDepth = 5

// BlockResolver fetches block content by block ID. The ReversibleCompactor
// provides one backed by the Store's ExpandLossless method.
type BlockResolver func(ctx context.Context, blockID string) (string, bool, error)

// ResolveNestedPlaceholders expands `(bN)` placeholders in content by looking
// up each referenced block via the resolver. It recurses up to MaxPlaceholderDepth
// levels deep. Cycle detection prevents infinite resolution: if a block is
// encountered more than once in the resolution chain, its placeholder is left
// unresolved.
func ResolveNestedPlaceholders(ctx context.Context, content string, resolver BlockResolver) (string, error) {
	depth := 0
	return resolveNested(ctx, content, resolver, nil, &depth)
}

// resolveNested is the recursive implementation of ResolveNestedPlaceholders.
// The visited set prevents cycles; *depth is a shared counter that tracks total
// blocks resolved across the entire call chain so the MaxPlaceholderDepth limit
// is enforced globally. Blocks are added to visited and never removed so that
// expanded content that reintroduces a placeholder for an already-seen block is
// left unresolved.
func resolveNested(ctx context.Context, content string, resolver BlockResolver, visited map[string]struct{}, depth *int) (string, error) {
	if *depth >= MaxPlaceholderDepth {
		return content, nil
	}
	if visited == nil {
		visited = make(map[string]struct{})
	}

	for *depth < MaxPlaceholderDepth {
		loc := blockPlaceholderPattern.FindStringSubmatchIndex(content)
		if loc == nil {
			return content, nil
		}

		placeholder := content[loc[0]:loc[1]]
		blockNum := content[loc[2]:loc[3]]
		blockID := "b" + blockNum

		if _, seen := visited[blockID]; seen {
			break
		}

		resolved, found, err := resolver(ctx, blockID)
		if err != nil {
			return "", fmt.Errorf("resolving block %s: %v: %w", blockID, ErrInvalidBlockID, err)
		}
		if !found {
			break
		}

		visited[blockID] = struct{}{}
		*depth++

		expanded, err := resolveNested(ctx, resolved, resolver, visited, depth)
		if err != nil {
			return "", fmt.Errorf("expanding nested block %s: %v: %w", blockID, ErrExpansionFailed, err)
		}

		content = stringReplace(content, placeholder, expanded)
	}

	return content, nil
}

// stringReplace replaces the first occurrence of old with new in s.
func stringReplace(s, old, new string) string {
	for i := 0; i <= len(s)-len(old); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}
