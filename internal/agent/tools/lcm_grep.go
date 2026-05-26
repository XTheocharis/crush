package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"charm.land/fantasy"
)

const (
	LcmGrepToolName    = "lcm_grep"
	maxLcmGrepMatches  = 50
	maxLcmGrepOutput   = 40000
	maxMatchTextLength = 200
)

type LcmGrepParams struct {
	Pattern        string `json:"pattern" description:"Search pattern (plain text for FTS, or /regex/ for regex search)"`
	ConversationID string `json:"conversation_id" description:"Session ID to search in"`
	SummaryID      string `json:"summary_id,omitempty" description:"Optional: Limit search to a specific summary scope"`
	Page           int    `json:"page,omitempty" description:"Page number for pagination (default: 1)"`
}

var lcmGrepDescription = `Search conversation history with full-text search or regex pattern.

This tool allows you to search through past conversation messages to find specific information.

Parameters:
- pattern: The search pattern. Use plain text for full-text search, or wrap in slashes like /pattern/ for regex search
- conversation_id: The session ID to search within
- summary_id: (optional) Limit search to messages within a specific summary
- page: (optional) Page number for pagination, defaults to 1

Returns up to 50 matches per page, with each match showing the message sequence number, role, and truncated content.`

func NewLcmGrepTool(sqlDB *sql.DB) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		LcmGrepToolName,
		lcmGrepDescription,
		func(ctx context.Context, params LcmGrepParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Pattern == "" {
				return fantasy.NewTextErrorResponse("pattern is required"), nil
			}
			if params.ConversationID == "" {
				return fantasy.NewTextErrorResponse("conversation_id is required"), nil
			}

			// Default to page 1
			page := max(params.Page, 1)
			offset := (page - 1) * maxLcmGrepMatches

			// Check if pattern is regex (starts and ends with /)
			isRegex := strings.HasPrefix(params.Pattern, "/") && strings.HasSuffix(params.Pattern, "/") && len(params.Pattern) > 2

			var matches []messageMatch
			var err error

			if isRegex {
				// Extract regex pattern (remove surrounding slashes)
				regexPattern := params.Pattern[1 : len(params.Pattern)-1]
				matches, err = searchMessagesRegex(ctx, sqlDB, params.ConversationID, regexPattern, offset, maxLcmGrepMatches+1)
			} else {
				// FTS search - preprocess pattern
				ftsPattern := preprocessFTSPattern(params.Pattern)
				matches, err = searchMessagesFTS(ctx, sqlDB, params.ConversationID, ftsPattern, offset, maxLcmGrepMatches+1)
			}

			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Error searching messages: %v", err)), nil
			}

			// Check if truncated
			truncated := len(matches) > maxLcmGrepMatches
			if truncated {
				matches = matches[:maxLcmGrepMatches]
			}

			// Format output
			var output strings.Builder
			totalBytes := 0

			if len(matches) == 0 {
				output.WriteString("No matches found.\n")
			} else {
				fmt.Fprintf(&output, "Found %d matches", len(matches))
				if truncated {
					fmt.Fprintf(&output, " (showing first %d, use page=%d for more)", maxLcmGrepMatches, page+1)
				}
				output.WriteString("\n\n")

				for _, match := range matches {
					text := extractTextFromParts(match.parts)
					if len(text) > maxMatchTextLength {
						text = text[:maxMatchTextLength] + "..."
					}
					line := fmt.Sprintf("[seq=%d, role=%s]: %s\n", match.seq, match.role, text)

					// Check output size limit
					if totalBytes+len(line) > maxLcmGrepOutput {
						output.WriteString("\n(Output truncated. Use more specific pattern or page parameter.)\n")
						break
					}

					output.WriteString(line)
					totalBytes += len(line)
				}
			}

			return fantasy.NewTextResponse(output.String()), nil
		})
}

type messageMatch struct {
	id    string
	seq   int64
	role  string
	parts string
}

// preprocessFTSPattern converts a plain text search into FTS5 query syntax
// by splitting on whitespace and joining with AND
func preprocessFTSPattern(pattern string) string {
	fields := strings.Fields(pattern)
	if len(fields) == 0 {
		return pattern
	}
	// Join with AND for FTS5
	return strings.Join(fields, " AND ")
}

// searchMessagesFTS searches messages using FTS5 full-text search
func searchMessagesFTS(ctx context.Context, db *sql.DB, sessionID, ftsPattern string, offset, limit int) ([]messageMatch, error) {
	query := `
SELECT m.id, m.seq, m.role, m.parts
FROM messages m
WHERE m.session_id = ? AND m.rowid IN (
    SELECT rowid FROM messages_fts WHERE content MATCH ?
)
ORDER BY m.seq ASC
LIMIT ? OFFSET ?`

	rows, err := db.QueryContext(ctx, query, sessionID, ftsPattern, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []messageMatch
	for rows.Next() {
		var m messageMatch
		if err := rows.Scan(&m.id, &m.seq, &m.role, &m.parts); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}

	return matches, rows.Err()
}

// searchMessagesRegex searches messages using Go regex (fallback for complex patterns)
func searchMessagesRegex(ctx context.Context, db *sql.DB, sessionID, regexPattern string, offset, limit int) ([]messageMatch, error) {
	// Compile regex
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	// Fetch all messages for the session (we'll filter in Go)
	query := `SELECT id, seq, role, parts FROM messages WHERE session_id = ? ORDER BY seq ASC`
	rows, err := db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []messageMatch
	matchCount := 0

	for rows.Next() {
		var m messageMatch
		if err := rows.Scan(&m.id, &m.seq, &m.role, &m.parts); err != nil {
			return nil, err
		}

		// Extract text and check regex match
		text := extractTextFromParts(m.parts)
		if re.MatchString(text) {
			// Apply offset/limit
			if matchCount >= offset {
				matches = append(matches, m)
				if len(matches) >= limit {
					break
				}
			}
			matchCount++
		}
	}

	return matches, rows.Err()
}

// extractTextFromParts extracts text content from the JSON parts column
func extractTextFromParts(partsJSON string) string {
	var parts []struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal([]byte(partsJSON), &parts); err != nil {
		return ""
	}

	var texts []string
	for _, p := range parts {
		if p.Type == "text" {
			var textData struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(p.Data, &textData); err == nil {
				texts = append(texts, textData.Text)
			}
		}
	}

	return strings.Join(texts, " ")
}
