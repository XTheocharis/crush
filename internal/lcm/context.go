package lcm

import (
	"context"
	"fmt"
	"strings"
)

// FormattedContextEntry is a context entry formatted for LLM consumption.
type FormattedContextEntry struct {
	ID      string // message ID or summary ID
	Role    string
	Content string
}

// GetFormattedContext returns context entries formatted for LLM consumption.
// Summaries are returned with [Summary ID: sum_xxx] markers.
func (m *compactionManager) GetFormattedContext(ctx context.Context, sessionID string) ([]FormattedContextEntry, error) {
	entries, err := m.store.GetContextEntries(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("getting context entries: %w", err)
	}

	var result []FormattedContextEntry
	for _, entry := range entries {
		switch entry.ItemType {
		case "message":
			result = append(result, FormattedContextEntry{
				ID:   entry.MessageID,
				Role: "user", // role will be replaced with actual role by caller
			})
		case "summary":
			content := formatSummaryContent(entry)
			result = append(result, FormattedContextEntry{
				ID:      entry.SummaryID,
				Role:    "user",
				Content: content,
			})
		}
	}
	return result, nil
}

// formatSummaryContent formats a summary with its markers for LLM consumption.
func formatSummaryContent(entry ContextEntry) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[Summary ID: %s]\n", entry.SummaryID)
	if len(entry.ParentIDs) > 0 {
		fmt.Fprintf(&sb, "[Condensed from: %s]\n\n", strings.Join(entry.ParentIDs, ", "))
	}
	sb.WriteString(entry.SummaryContent)
	return sb.String()
}
