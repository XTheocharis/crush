package lcm

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/crush/internal/db"
)

// CompactOnce runs a single compaction round for a session.
// If force is false, it checks the threshold first and returns early if not over.
func (m *compactionManager) CompactOnce(ctx context.Context, sessionID string, force bool) (CompactionResult, error) {
	budget, err := m.GetBudget(ctx, sessionID)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("getting budget: %w", err)
	}

	tokenCount, err := m.store.GetContextTokenCount(ctx, sessionID)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("getting token count: %w", err)
	}

	if !force && tokenCount <= budget.SoftThreshold {
		return CompactionResult{
			TokenCount: tokenCount,
		}, nil
	}

	// Try leaf summarization first.
	summarized, err := m.trySummarize(ctx, sessionID, budget)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("summarization: %w", err)
	}

	if summarized {
		tokenCount, err = m.store.GetContextTokenCount(ctx, sessionID)
		if err != nil {
			return CompactionResult{}, fmt.Errorf("getting token count after summarization: %w", err)
		}

		// If still over threshold, try condensation.
		if tokenCount > budget.SoftThreshold {
			condensed, err := m.tryCondense(ctx, sessionID, budget)
			if err != nil {
				return CompactionResult{}, fmt.Errorf("condensation: %w", err)
			}
			if condensed {
				tokenCount, err = m.store.GetContextTokenCount(ctx, sessionID)
				if err != nil {
					return CompactionResult{}, fmt.Errorf("getting token count after condensation: %w", err)
				}
			}
		}

		return CompactionResult{
			Rounds:      1,
			ActionTaken: true,
			TokenCount:  tokenCount,
		}, nil
	}

	// No messages to summarize, try condensation directly.
	condensed, err := m.tryCondense(ctx, sessionID, budget)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("condensation: %w", err)
	}

	tokenCount, err = m.store.GetContextTokenCount(ctx, sessionID)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("getting token count after condensation: %w", err)
	}

	return CompactionResult{
		Rounds:      1,
		ActionTaken: condensed,
		TokenCount:  tokenCount,
	}, nil
}

// compactUntilUnderLimitLocked runs compaction until the session is under the
// hard limit. Caller must hold the per-session mutex.
func (m *compactionManager) compactUntilUnderLimitLocked(ctx context.Context, sessionID string) error {
	var lastTokenCount int64
	for i := range MaxCompactionRounds {
		budget, err := m.GetBudget(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("getting budget (round %d): %w", i, err)
		}

		tokenCount, err := m.store.GetContextTokenCount(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("getting token count (round %d): %w", i, err)
		}

		if tokenCount <= budget.HardLimit {
			return nil
		}

		// Detect stalls where action is taken but tokens don't decrease.
		if i > 0 && tokenCount >= lastTokenCount {
			return fmt.Errorf("compaction stalled at round %d: tokens=%d did not decrease from %d, hard_limit=%d",
				i, tokenCount, lastTokenCount, budget.HardLimit)
		}
		lastTokenCount = tokenCount

		result, err := m.CompactOnce(ctx, sessionID, true)
		if err != nil {
			return fmt.Errorf("compaction round %d: %w", i, err)
		}

		if !result.ActionTaken {
			return fmt.Errorf("compaction stalled at round %d: no progress made, tokens=%d, hard_limit=%d", i, tokenCount, budget.HardLimit)
		}
	}

	return fmt.Errorf("compaction did not converge after %d rounds", MaxCompactionRounds)
}

// trySummarize selects the oldest messages and summarizes them.
func (m *compactionManager) trySummarize(ctx context.Context, sessionID string, budget Budget) (bool, error) {
	entries, err := m.store.GetContextEntries(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("getting context entries: %w", err)
	}

	// Select oldest messages (not summaries) up to 75% of context window.
	tokenBudget := int64(float64(budget.ContextWindow) * 0.75)
	var selectedEntries []ContextEntry
	var selectedTokens int64

	for _, entry := range entries {
		if entry.ItemType != "message" {
			continue
		}
		if selectedTokens+entry.TokenCount > tokenBudget && len(selectedEntries) >= MinMessagesToSummarize {
			break
		}
		selectedEntries = append(selectedEntries, entry)
		selectedTokens += entry.TokenCount
	}

	if len(selectedEntries) < MinMessagesToSummarize {
		return false, nil
	}

	// Fetch full message content.
	messageIDs := make([]string, len(selectedEntries))
	for i, e := range selectedEntries {
		messageIDs[i] = e.MessageID
	}

	allMsgs, err := m.store.GetMessages(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("getting messages: %w", err)
	}

	// Filter to selected messages.
	msgIDSet := make(map[string]struct{}, len(messageIDs))
	for _, id := range messageIDs {
		msgIDSet[id] = struct{}{}
	}

	var msgsForSummary []MessageForSummary
	for _, msg := range allMsgs {
		if _, ok := msgIDSet[msg.ID]; ok {
			msgsForSummary = append(msgsForSummary, msg)
		}
	}

	// Extract file IDs from messages.
	var fileIDs []string
	for _, msg := range msgsForSummary {
		fileIDs = append(fileIDs, ExtractFileIDs(msg.Content)...)
	}

	// Generate summary.
	input := SummaryInput{
		SessionID: sessionID,
		Messages:  msgsForSummary,
	}

	summaryText, summaryTokens, err := m.summarizer.Summarize(ctx, input)
	if err != nil {
		return false, fmt.Errorf("generating summary: %w", err)
	}
	if summaryTokens == 0 {
		summaryTokens = EstimateTokens(summaryText)
	}

	// Store summary in a transaction.
	summaryID, _ := GenerateSummaryID(sessionID)
	position := selectedEntries[0].Position

	tx, err := m.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txQ := m.queries.WithTx(tx)

	err = m.store.InsertLeafSummary(ctx, txQ, sessionID, summaryID, summaryText, summaryTokens, fileIDs, messageIDs)
	if err != nil {
		return false, fmt.Errorf("inserting leaf summary: %w", err)
	}

	err = m.store.ReplacePositionsWithSummary(ctx, txQ, sessionID, summaryID, position, summaryTokens, messageIDs)
	if err != nil {
		return false, fmt.Errorf("replacing positions: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("committing transaction: %w", err)
	}

	return true, nil
}

// tryCondense attempts to condense adjacent summaries.
func (m *compactionManager) tryCondense(ctx context.Context, sessionID string, budget Budget) (bool, error) {
	entries, err := m.store.GetContextEntries(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("getting context entries: %w", err)
	}

	// Find adjacent summaries to condense.
	var summaryEntries []ContextEntry
	for _, entry := range entries {
		if entry.ItemType == "summary" {
			summaryEntries = append(summaryEntries, entry)
		}
	}

	if len(summaryEntries) == 0 {
		return false, nil
	}

	// Condense up to the first two adjacent summaries.
	toCondense := summaryEntries[:min(2, len(summaryEntries))]

	condensedText, condensedTokens, err := m.summarizer.Condense(ctx, toCondense)
	if err != nil {
		return false, fmt.Errorf("condensing summaries: %w", err)
	}
	if condensedTokens == 0 {
		condensedTokens = EstimateTokens(condensedText)
	}

	// Collect file IDs from parent summaries.
	var allFileIDs []string
	for _, s := range toCondense {
		summary, err := m.store.q.GetLcmSummary(ctx, s.SummaryID)
		if err != nil {
			return false, fmt.Errorf("getting summary for file IDs: %w", err)
		}
		var fids []string
		if err := json.Unmarshal([]byte(summary.FileIds), &fids); err == nil {
			allFileIDs = append(allFileIDs, fids...)
		}
	}

	// Store condensed summary in a transaction.
	condensedID, _ := GenerateSummaryID(sessionID)
	position := toCondense[0].Position

	tx, err := m.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txQ := m.queries.WithTx(tx)

	fileIDsJSON, err := json.Marshal(allFileIDs)
	if err != nil {
		return false, fmt.Errorf("marshaling file IDs: %w", err)
	}

	// Insert condensed summary.
	err = txQ.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  condensedID,
		SessionID:  sessionID,
		Kind:       KindCondensed,
		Content:    condensedText,
		TokenCount: condensedTokens,
		FileIds:    string(fileIDsJSON),
	})
	if err != nil {
		return false, fmt.Errorf("inserting condensed summary: %w", err)
	}

	// Link parent summaries.
	for i, s := range toCondense {
		err = txQ.InsertLcmSummaryParent(ctx, db.InsertLcmSummaryParentParams{
			SummaryID:       condensedID,
			ParentSummaryID: s.SummaryID,
			Ord:             int64(i),
		})
		if err != nil {
			return false, fmt.Errorf("inserting summary parent link: %w", err)
		}
	}

	// Replace the two summary context items with the condensed one.
	removedSummaryIDs := make([]string, len(toCondense))
	for i, s := range toCondense {
		removedSummaryIDs[i] = s.SummaryID
	}

	// Delete and rebuild context items.
	err = txQ.DeleteAllLcmContextItems(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("deleting context items: %w", err)
	}

	// Rebuild: replace condensed summaries with the new one.
	removedSet := make(map[string]struct{}, len(removedSummaryIDs))
	for _, id := range removedSummaryIDs {
		removedSet[id] = struct{}{}
	}

	var pos int64
	condensedInserted := false

	for _, item := range entries {
		if item.ItemType == "summary" && item.SummaryID != "" {
			if _, removed := removedSet[item.SummaryID]; removed {
				if !condensedInserted {
					err = txQ.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
						SessionID:  sessionID,
						Position:   pos,
						ItemType:   "summary",
						SummaryID:  sql.NullString{String: condensedID, Valid: true},
						TokenCount: condensedTokens,
					})
					if err != nil {
						return false, fmt.Errorf("inserting condensed context item: %w", err)
					}
					pos++
					condensedInserted = true
				}
				continue
			}
		}

		err = txQ.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   pos,
			ItemType:   item.ItemType,
			MessageID:  sql.NullString{String: item.MessageID, Valid: item.MessageID != ""},
			SummaryID:  sql.NullString{String: item.SummaryID, Valid: item.SummaryID != ""},
			TokenCount: item.TokenCount,
		})
		if err != nil {
			return false, fmt.Errorf("reinserting context item: %w", err)
		}
		pos++
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("committing transaction: %w", err)
	}

	_ = budget
	_ = position

	return true, nil
}
