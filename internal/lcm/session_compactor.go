package lcm

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/crush/internal/db"
)

// KindSessionMemory is the summary kind for session memory compaction.
const KindSessionMemory = "session"

// SessionCompactorConfig controls the behaviour of the SessionCompactor layer.
type SessionCompactorConfig struct {
	// Store is the LCM store used for reading context entries and persisting
	// summaries. Required.
	Store *Store

	// LLM is the language model client used to generate the session memory.
	// If nil, Compact will return an error.
	LLM LLMClient

	// SessionID is the session this compactor operates on. Required.
	SessionID string

	// MinTokenBudget is the minimum soft-threshold token count below which
	// session compaction should be considered. If zero, defaults to
	// sessionMemoryMinBudget.
	MinTokenBudget int64

	// MaxOutputTokens caps the target output token budget. If zero, defaults
	// to sessionMemoryMaxTokens (40K).
	MaxOutputTokens int64

	// MinOutputTokens is the lower bound for the output token budget. If
	// zero, defaults to sessionMemoryMinTokens (10K).
	MinOutputTokens int64
}

func (c SessionCompactorConfig) minBudget() int64 {
	if c.MinTokenBudget > 0 {
		return c.MinTokenBudget
	}
	return sessionMemoryMinBudget
}

func (c SessionCompactorConfig) maxTokens() int64 {
	if c.MaxOutputTokens > 0 {
		return c.MaxOutputTokens
	}
	return sessionMemoryMaxTokens
}

func (c SessionCompactorConfig) minTokens() int64 {
	if c.MinOutputTokens > 0 {
		return c.MinOutputTokens
	}
	return sessionMemoryMinTokens
}

const (
	// sessionMemoryMaxTokens is the upper target for session memory output
	// (40K tokens ≈ 160K characters).
	sessionMemoryMaxTokens = 40000

	// sessionMemoryMinTokens is the lower target for session memory output
	// (10K tokens ≈ 40K characters).
	sessionMemoryMinTokens = 10000

	// sessionMemoryMinBudget is the minimum context token count that triggers
	// session compaction. Below this, the context is small enough that
	// session memory is unnecessary.
	sessionMemoryMinBudget = 50000
)

// SessionCompactor implements a session-scoped compaction strategy.
type SessionCompactor struct {
	cfg SessionCompactorConfig
}

// NewSessionCompactor creates a Layer 2 SessionCompactor with the given
// config.
func NewSessionCompactor(cfg SessionCompactorConfig) *SessionCompactor {
	return &SessionCompactor{cfg: cfg}
}

// Name returns "session-compactor".
func (s *SessionCompactor) Name() string { return "session-compactor" }

// Priority returns 20 (Layer 2, after MicroCompactor at 1).
func (s *SessionCompactor) Priority() int { return 20 }

// ShouldCompact reports whether session memory compaction is warranted. It
// returns true when:
//  1. The store, LLM client, and session ID are all configured.
//  2. The session has enough context tokens to benefit from compaction.
//  3. The budget indicates context pressure (current tokens approaching
//     soft threshold).
//  4. There is not already a session-memory summary for this session.
func (s *SessionCompactor) ShouldCompact(ctx context.Context, budget Budget) bool {
	if s.cfg.Store == nil || s.cfg.LLM == nil || s.cfg.SessionID == "" {
		return false
	}

	// Only compact when there is meaningful context to compact.
	tokenCount, err := s.cfg.Store.GetContextTokenCount(ctx, s.cfg.SessionID)
	if err != nil || tokenCount < s.cfg.minBudget() {
		return false
	}

	// Only compact under context pressure.
	if budget.SoftThreshold > 0 && tokenCount < budget.SoftThreshold {
		return false
	}

	// Don't create a duplicate session memory.
	if s.hasExistingSessionMemory(ctx) {
		return false
	}

	return true
}

// Compact compiles session history into a structured memory document via the
// LLM. It reads all context entries (messages and summaries), formats them
// for the LLM, and stores the result as a session-memory summary.
func (s *SessionCompactor) Compact(ctx context.Context, budget Budget) (*CompactionLayerResult, error) {
	if s.cfg.Store == nil {
		return nil, fmt.Errorf("session-compactor: %w", ErrStoreIsNil)
	}
	if s.cfg.LLM == nil {
		return nil, fmt.Errorf("session-compactor: %w", ErrLLMClientNil)
	}
	if s.cfg.SessionID == "" {
		return nil, fmt.Errorf("session-compactor: %w", ErrSessionIDEmpty)
	}

	// Gather all context content.
	entries, err := s.cfg.Store.GetContextEntries(ctx, s.cfg.SessionID)
	if err != nil {
		return nil, fmt.Errorf("getting context entries: %w", err)
	}

	if len(entries) == 0 {
		return &CompactionLayerResult{LayerName: s.Name()}, nil
	}

	// Build the prompt input from context entries.
	userPrompt := formatContextForSessionMemory(entries)

	// Compute the target token budget for the output.
	targetTokens := s.targetTokens(budget)

	// Call the LLM to generate structured session memory.
	systemPrompt := buildSessionMemorySystemPrompt(targetTokens)
	result, err := s.cfg.LLM.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("generating session memory: %w", err)
	}

	// If the LLM produced something too small, it likely failed. Return a
	// no-op result rather than storing garbage.
	resultTokens := EstimateTokens(result)
	if resultTokens < 100 {
		return &CompactionLayerResult{LayerName: s.Name()}, nil
	}

	// Compute tokens freed before we modify the store.
	originalTokens, err := s.cfg.Store.GetContextTokenCount(ctx, s.cfg.SessionID)
	if err != nil {
		return nil, fmt.Errorf("computing original token count: %w", err)
	}

	messageIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.ItemType == "message" && entry.MessageID != "" {
			messageIDs = append(messageIDs, entry.MessageID)
		}
	}

	summaryID, _ := GenerateSummaryID(s.cfg.SessionID)

	tx, err := s.cfg.Store.rawDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txQ := s.cfg.Store.queries.WithTx(tx)

	fileIDsJSON, _ := json.Marshal([]string{})
	if err := txQ.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  summaryID,
		SessionID:  s.cfg.SessionID,
		Kind:       KindSessionMemory,
		Content:    result,
		TokenCount: resultTokens,
		FileIds:    string(fileIDsJSON),
	}); err != nil {
		return nil, fmt.Errorf("inserting session summary: %w", err)
	}

	for i, msgID := range messageIDs {
		if err := txQ.InsertLcmSummaryMessage(ctx, db.InsertLcmSummaryMessageParams{
			SummaryID: summaryID,
			MessageID: msgID,
			Ord:       int64(i),
		}); err != nil {
			return nil, fmt.Errorf("linking summary message: %w", err)
		}
	}

	if err := txQ.DeleteAllLcmContextItems(ctx, s.cfg.SessionID); err != nil {
		return nil, fmt.Errorf("deleting context items: %w", err)
	}

	if err := txQ.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  s.cfg.SessionID,
		Position:   0,
		ItemType:   "summary",
		SummaryID:  sql.NullString{String: summaryID, Valid: true},
		TokenCount: resultTokens,
	}); err != nil {
		return nil, fmt.Errorf("inserting summary context item: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	freed := max(originalTokens-resultTokens, 0)

	return &CompactionLayerResult{
		LayerName:     s.Name(),
		TokensFreed:   freed,
		ItemsAffected: len(entries),
		ActionTaken:   true,
	}, nil
}

// hasExistingSessionMemory checks whether a session-memory summary already
// exists for this session.
func (s *SessionCompactor) hasExistingSessionMemory(ctx context.Context) bool {
	entries, err := s.cfg.Store.GetContextEntries(ctx, s.cfg.SessionID)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.ItemType == "summary" && entry.SummaryKind == KindSessionMemory {
			return true
		}
	}
	return false
}

// targetTokens computes the output token target based on the budget. It
// targets a range between minTokens and maxTokens, scaled by how much
// context pressure exists.
func (s *SessionCompactor) targetTokens(budget Budget) int64 {
	minT := s.cfg.minTokens()
	maxT := s.cfg.maxTokens()

	// Without budget info, target the minimum.
	if budget.SoftThreshold <= 0 || budget.ContextWindow <= 0 {
		return minT
	}

	// Scale output size based on context window pressure. More pressure
	// → smaller target.
	ratio := float64(budget.SoftThreshold) / float64(budget.ContextWindow)
	target := int64(float64(maxT) * ratio)
	return max(min(target, maxT), minT)
}

// buildSessionMemorySystemPrompt constructs the system prompt that instructs
// the LLM to produce structured session memory.
func buildSessionMemorySystemPrompt(targetTokens int64) string {
	return fmt.Sprintf(sessionMemorySystemPrompt, targetTokens, targetTokens)
}

const sessionMemorySystemPrompt = `You are a session memory compactor. Analyze the provided session history (messages and summaries) and produce a structured memory document that captures all essential context needed to continue the session.

Output structured markdown with exactly these sections:

## Decisions
List all architectural, design, and implementation decisions made during the session. Include rationale where available.

## Patterns
Document code patterns, conventions, and recurring structures discovered or used. Include file paths and function names.

## Errors
List errors encountered, their root causes, and how they were resolved. Include relevant stack traces or error messages.

## Current State
Describe the current task state: what is done, what is in progress, what is blocked, and what comes next. Be specific about file paths and functions.

Target token count: %d tokens (approximately %d characters). Be thorough but concise. Preserve all file paths, function names, and specific technical details. Do not include conversational filler.`

// formatContextForSessionMemory formats context entries into a prompt for
// the LLM.
func formatContextForSessionMemory(entries []ContextEntry) string {
	var sb strings.Builder
	sb.WriteString("<session-context>\n")
	for i, entry := range entries {
		switch entry.ItemType {
		case "message":
			fmt.Fprintf(&sb, "--- Entry %d (type: message, id: %s, tokens: %d) ---\n",
				i, entry.MessageID, entry.TokenCount)
		case "summary":
			fmt.Fprintf(&sb, "--- Entry %d (type: summary, kind: %s, id: %s, tokens: %d) ---\n",
				i, entry.SummaryKind, entry.SummaryID, entry.TokenCount)
			if entry.SummaryContent != "" {
				fmt.Fprintf(&sb, "%s\n", entry.SummaryContent)
			}
		}
		if i >= maxContextEntriesForPrompt {
			fmt.Fprintf(&sb, "--- Remaining %d entries omitted for brevity ---\n",
				len(entries)-i-1)
			break
		}
	}
	sb.WriteString("</session-context>")
	return sb.String()
}

// maxContextEntriesForPrompt caps the number of context entries sent to the
// LLM to avoid exceeding token limits on very large sessions.
const maxContextEntriesForPrompt = 200
