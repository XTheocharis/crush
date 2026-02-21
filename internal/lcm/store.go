package lcm

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/crush/internal/db"
)

// fileIDPatterns are the patterns used to extract file IDs from message content.
var fileIDPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\[Large File Stored:\s*(file_[0-9a-f]{16})\]`),
	regexp.MustCompile(`\[Large User Text Stored:\s*(file_[0-9a-f]{16})\]`),
	regexp.MustCompile(`LCM File ID:\s*(file_[0-9a-f]{16})`),
	regexp.MustCompile(`file_id\s+"(file_[0-9a-f]{16})"`),
	regexp.MustCompile(`\[Large Tool Output Stored:\s*(file_[0-9a-f]{16})\]`),
}

// Store wraps the db.Querier with LCM-specific operations.
type Store struct {
	q       db.Querier
	queries *db.Queries // for WithTx transaction support
	rawDB   *sql.DB
}

func newStore(queries *db.Queries, rawDB *sql.DB) *Store {
	return &Store{q: queries, queries: queries, rawDB: rawDB}
}

// InsertLargeTextContent stores large text content and returns a file ID.
func (s *Store) InsertLargeTextContent(ctx context.Context, sessionID, content, originalPath string) (string, error) {
	fileID := GenerateFileID(sessionID, content)
	chars := int64(len([]rune(content)))
	tokenCount := (chars + CharsPerToken - 1) / CharsPerToken

	err := s.q.InsertLcmLargeFile(ctx, db.InsertLcmLargeFileParams{
		FileID:       fileID,
		SessionID:    sessionID,
		OriginalPath: originalPath,
		Content:      sql.NullString{String: content, Valid: true},
		TokenCount:   tokenCount,
	})
	if err != nil {
		return "", fmt.Errorf("inserting large file: %w", err)
	}
	return fileID, nil
}

// GetAncestorSessionIDs returns all ancestor session IDs via a recursive CTE
// walking sessions.parent_session_id.
func (s *Store) GetAncestorSessionIDs(ctx context.Context, sessionID string) ([]string, error) {
	query := `
		WITH RECURSIVE ancestors(id) AS (
			SELECT ?
			UNION
			SELECT s.parent_session_id
			FROM sessions s
			JOIN ancestors a ON s.id = a.id
			WHERE s.parent_session_id IS NOT NULL
		)
		SELECT id FROM ancestors
	`
	rows, err := s.rawDB.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying ancestor sessions: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning ancestor session ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ancestor sessions: %w", err)
	}
	return ids, nil
}

// GetSummaryMessageIDs returns message IDs from lcm_summary_messages for a
// leaf summary.
func (s *Store) GetSummaryMessageIDs(ctx context.Context, summaryID string) ([]string, error) {
	msgs, err := s.q.ListLcmSummaryMessages(ctx, summaryID)
	if err != nil {
		return nil, fmt.Errorf("listing summary messages: %w", err)
	}
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.MessageID
	}
	return ids, nil
}

// GetLargeFileContent reads from lcm_large_files, checks session ancestry
// for cross-session access, and truncates to maxBytes if > 0.
func (s *Store) GetLargeFileContent(ctx context.Context, fileID, sessionID string, maxBytes int) (string, error) {
	file, err := s.q.GetLcmLargeFile(ctx, fileID)
	if err != nil {
		return "", fmt.Errorf("getting large file: %w", err)
	}

	// Verify session access.
	if file.SessionID != sessionID {
		ancestors, err := s.GetAncestorSessionIDs(ctx, sessionID)
		if err != nil {
			return "", fmt.Errorf("checking session ancestry: %w", err)
		}
		found := slices.Contains(ancestors, file.SessionID)
		if !found {
			return "", fmt.Errorf("file %s does not belong to session %s or its ancestors", fileID, sessionID)
		}
	}

	content := file.Content.String
	if maxBytes > 0 && len(content) > maxBytes {
		content = content[:maxBytes]
	}
	return content, nil
}

// LargeFileExists checks whether a large file exists and is accessible from
// the given session (including ancestry).
func (s *Store) LargeFileExists(ctx context.Context, fileID, sessionID string) (bool, error) {
	file, err := s.q.GetLcmLargeFile(ctx, fileID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("checking large file existence: %w", err)
	}

	if file.SessionID == sessionID {
		return true, nil
	}

	ancestors, err := s.GetAncestorSessionIDs(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("checking session ancestry: %w", err)
	}
	if slices.Contains(ancestors, file.SessionID) {
		return true, nil
	}
	return false, nil
}

// GetMessages fetches all messages for a session and extracts text content
// from the JSON parts column.
func (s *Store) GetMessages(ctx context.Context, sessionID string) ([]MessageForSummary, error) {
	dbMsgs, err := s.q.ListMessagesBySessionSeq(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("listing messages by session seq: %w", err)
	}

	msgs := make([]MessageForSummary, 0, len(dbMsgs))
	for _, m := range dbMsgs {
		content := extractTextFromParts(m.Parts)
		msgs = append(msgs, MessageForSummary{
			ID:        m.ID,
			SessionID: m.SessionID,
			Seq:       m.Seq,
			Role:      m.Role,
			Content:   content,
		})
	}
	return msgs, nil
}

// InsertLeafSummary inserts a leaf summary and its message links using the
// given querier (for transaction support).
func (s *Store) InsertLeafSummary(ctx context.Context, q db.Querier, sessionID, summaryID, content string, tokenCount int64, fileIDs, messageIDs []string) error {
	fileIDsJSON, err := json.Marshal(fileIDs)
	if err != nil {
		return fmt.Errorf("marshaling file IDs: %w", err)
	}

	err = q.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  summaryID,
		SessionID:  sessionID,
		Kind:       KindLeaf,
		Content:    content,
		TokenCount: tokenCount,
		FileIds:    string(fileIDsJSON),
	})
	if err != nil {
		return fmt.Errorf("inserting summary: %w", err)
	}

	for i, msgID := range messageIDs {
		err = q.InsertLcmSummaryMessage(ctx, db.InsertLcmSummaryMessageParams{
			SummaryID: summaryID,
			MessageID: msgID,
			Ord:       int64(i),
		})
		if err != nil {
			return fmt.Errorf("inserting summary message link: %w", err)
		}
	}
	return nil
}

// ReplacePositionsWithSummary replaces context items: deletes all, then
// rebuilds with the summary inserted at the given position. Messages not in
// removedMessageIDs are preserved.
func (s *Store) ReplacePositionsWithSummary(ctx context.Context, q db.Querier, sessionID, summaryID string, position int64, tokenCount int64, removedMessageIDs []string) error {
	// Get current context items before deleting.
	items, err := s.q.ListLcmContextItems(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("listing context items: %w", err)
	}

	// Build a set of removed message IDs.
	removedSet := make(map[string]struct{}, len(removedMessageIDs))
	for _, id := range removedMessageIDs {
		removedSet[id] = struct{}{}
	}

	// Delete all context items.
	err = q.DeleteAllLcmContextItems(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("deleting context items: %w", err)
	}

	// Rebuild: insert items that are not removed, and insert the summary at
	// the given position.
	var pos int64
	summaryInserted := false

	for _, item := range items {
		// Skip removed messages.
		if item.ItemType == "message" && item.MessageID.Valid {
			if _, removed := removedSet[item.MessageID.String]; removed {
				continue
			}
		}

		// Insert summary before items with position >= the target position.
		if !summaryInserted && item.Position >= position {
			err = q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
				SessionID:  sessionID,
				Position:   pos,
				ItemType:   "summary",
				SummaryID:  sql.NullString{String: summaryID, Valid: true},
				TokenCount: tokenCount,
			})
			if err != nil {
				return fmt.Errorf("inserting summary context item: %w", err)
			}
			pos++
			summaryInserted = true
		}

		err = q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   pos,
			ItemType:   item.ItemType,
			MessageID:  item.MessageID,
			SummaryID:  item.SummaryID,
			TokenCount: item.TokenCount,
		})
		if err != nil {
			return fmt.Errorf("reinserting context item: %w", err)
		}
		pos++
	}

	// If summary was not inserted yet (all removed items were at the end),
	// insert it at the end.
	if !summaryInserted {
		err = q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   pos,
			ItemType:   "summary",
			SummaryID:  sql.NullString{String: summaryID, Valid: true},
			TokenCount: tokenCount,
		})
		if err != nil {
			return fmt.Errorf("inserting summary context item at end: %w", err)
		}
	}

	return nil
}

// GetContextEntries returns all context items with summary content populated.
func (s *Store) GetContextEntries(ctx context.Context, sessionID string) ([]ContextEntry, error) {
	items, err := s.q.ListLcmContextItems(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("listing context items: %w", err)
	}

	entries := make([]ContextEntry, 0, len(items))
	for _, item := range items {
		entry := ContextEntry{
			Position:   item.Position,
			ItemType:   item.ItemType,
			TokenCount: item.TokenCount,
		}

		if item.MessageID.Valid {
			entry.MessageID = item.MessageID.String
		}
		if item.SummaryID.Valid {
			entry.SummaryID = item.SummaryID.String
		}

		// Populate summary fields.
		if item.ItemType == "summary" && item.SummaryID.Valid {
			summary, err := s.q.GetLcmSummary(ctx, item.SummaryID.String)
			if err != nil {
				return nil, fmt.Errorf("getting summary %s: %w", item.SummaryID.String, err)
			}
			entry.SummaryContent = summary.Content
			entry.SummaryKind = summary.Kind

			// Load parent IDs for condensed summaries.
			if summary.Kind == KindCondensed {
				parents, err := s.q.ListLcmSummaryParents(ctx, item.SummaryID.String)
				if err != nil {
					return nil, fmt.Errorf("listing summary parents: %w", err)
				}
				for _, p := range parents {
					entry.ParentIDs = append(entry.ParentIDs, p.ParentSummaryID)
				}
			}
		}

		entries = append(entries, entry)
	}
	return entries, nil
}

// GetContextTokenCount returns the sum of token_count from lcm_context_items.
func (s *Store) GetContextTokenCount(ctx context.Context, sessionID string) (int64, error) {
	result, err := s.q.GetLcmContextTokenCount(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("getting context token count: %w", err)
	}

	// The result is interface{} due to COALESCE; handle type assertion.
	switch v := result.(type) {
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("unexpected token count type: %T", result)
	}
}

// SearchSummaries performs FTS search using db.SearchLcmSummaries.
func (s *Store) SearchSummaries(ctx context.Context, sessionID, ftsQuery string) ([]SummarySearchResult, error) {
	rows, err := s.q.SearchLcmSummaries(ctx, db.SearchLcmSummariesParams{
		Content:   ftsQuery,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("searching summaries: %w", err)
	}

	results := make([]SummarySearchResult, len(rows))
	for i, row := range rows {
		results[i] = SummarySearchResult{
			SummaryID: row.SummaryID,
			Kind:      row.Kind,
		}
	}
	return results, nil
}

// ExpandSummary recursively expands a summary to its original messages using
// a recursive CTE.
func (s *Store) ExpandSummary(ctx context.Context, summaryID string) ([]MessageForSummary, error) {
	query := `
		WITH RECURSIVE expanded(summary_id, depth) AS (
			SELECT ?, 0
			UNION
			SELECT sp.parent_summary_id, expanded.depth + 1
			FROM lcm_summary_parents sp
			JOIN expanded ON sp.summary_id = expanded.summary_id
		)
		SELECT m.id, m.session_id, m.seq, m.role, m.parts
		FROM lcm_summary_messages sm
		JOIN messages m ON m.id = sm.message_id
		JOIN expanded e ON e.summary_id = sm.summary_id
		ORDER BY m.seq ASC
	`
	rows, err := s.rawDB.QueryContext(ctx, query, summaryID)
	if err != nil {
		return nil, fmt.Errorf("expanding summary: %w", err)
	}
	defer rows.Close()

	var msgs []MessageForSummary
	for rows.Next() {
		var m MessageForSummary
		var parts string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Seq, &m.Role, &parts); err != nil {
			return nil, fmt.Errorf("scanning expanded message: %w", err)
		}
		m.Content = extractTextFromParts(parts)
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating expanded messages: %w", err)
	}
	return msgs, nil
}

// ExtractFileIDs extracts file IDs from message content using known patterns.
func ExtractFileIDs(content string) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, pat := range fileIDPatterns {
		matches := pat.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if len(m) > 1 {
				id := m[1]
				if _, ok := seen[id]; !ok {
					seen[id] = struct{}{}
					ids = append(ids, id)
				}
			}
		}
	}
	return ids
}

// InsertLeafSummaryAtomically inserts a leaf summary and replaces context
// items in a single transaction, ensuring atomicity. A crash between the
// two operations cannot leave the DB in an inconsistent state.
func (s *Store) InsertLeafSummaryAtomically(
	ctx context.Context,
	sessionID, summaryID, content string,
	tokenCount int64,
	fileIDs, messageIDs []string,
	position int64,
	removedMessageIDs []string,
) error {
	tx, err := s.rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	if err := s.InsertLeafSummary(ctx, qtx, sessionID, summaryID, content, tokenCount, fileIDs, messageIDs); err != nil {
		return err
	}
	if err := s.ReplacePositionsWithSummary(ctx, qtx, sessionID, summaryID, position, tokenCount, removedMessageIDs); err != nil {
		return err
	}

	return tx.Commit()
}

// GetMessageCount returns the number of messages for a session.
// Intended for diagnostics and metrics.
func (s *Store) GetMessageCount(ctx context.Context, sessionID string) (int, error) {
	var count int
	err := s.rawDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE session_id = ?`, sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting messages: %w", err)
	}
	return count, nil
}

// GetLargeFilesBySession returns all large files for a session.
// No current consumer — added for API completeness.
func (s *Store) GetLargeFilesBySession(ctx context.Context, sessionID string) ([]db.LcmLargeFile, error) {
	files, err := s.q.ListLcmLargeFilesBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("listing large files: %w", err)
	}
	return files, nil
}

// GetMessagePartsBatch fetches the raw parts JSON for a batch of message IDs.
// Returns a map of messageID → parts JSON string.
// No current consumer — added for API completeness.
func (s *Store) GetMessagePartsBatch(ctx context.Context, messageIDs []string) (map[string]string, error) {
	if len(messageIDs) == 0 {
		return map[string]string{}, nil
	}

	// Build a parameterized query with the right number of placeholders.
	placeholders := make([]byte, 0, len(messageIDs)*2)
	for i := range messageIDs {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
	}
	query := "SELECT id, parts FROM messages WHERE id IN (" + string(placeholders) + ")"

	args := make([]any, len(messageIDs))
	for i, id := range messageIDs {
		args[i] = id
	}

	rows, err := s.rawDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetching message parts batch: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string, len(messageIDs))
	for rows.Next() {
		var id, parts string
		if err := rows.Scan(&id, &parts); err != nil {
			return nil, fmt.Errorf("scanning message parts: %w", err)
		}
		result[id] = parts
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating message parts: %w", err)
	}
	return result, nil
}

// partJSON is a minimal structure for parsing message parts JSON.
type partJSON struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// textData extracts content from text-type parts.
type textData struct {
	Text    string `json:"text"`
	Content string `json:"content"`
}

// extractTextFromParts extracts text content from the JSON parts column.
// Each part is {"type":"...","data":{"text":"..."|"content":"..."}}.
func extractTextFromParts(partsJSON string) string {
	var parts []partJSON
	if err := json.Unmarshal([]byte(partsJSON), &parts); err != nil {
		return ""
	}

	var sb strings.Builder
	for _, p := range parts {
		switch p.Type {
		case "text":
			var d textData
			if err := json.Unmarshal(p.Data, &d); err == nil && d.Text != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(d.Text)
			}
		case "tool_result":
			var d textData
			if err := json.Unmarshal(p.Data, &d); err == nil && d.Content != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(d.Content)
			}
		}
	}
	return sb.String()
}
