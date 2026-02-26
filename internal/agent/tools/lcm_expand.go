package tools

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"charm.land/fantasy"
)

const (
	LcmExpandToolName          = "lcm_expand"
	lcmExpandMainSessionDenied = "This tool is only available to sub-agent (Task) sessions. To expand a summary, delegate this task to a Task sub-agent."
)

type LcmExpandParams struct {
	SummaryID string `json:"summary_id" description:"The sum_xxx identifier to expand"`
}

var lcmExpandDescription = `Expand a summary to see the original messages it represents.

This tool is only available to sub-agent (Task) sessions. It recursively expands a summary
by retrieving all the original messages that were summarized, including messages from
parent summaries in the case of condensed summaries.

Parameters:
- summary_id: The sum_xxx identifier to expand

Returns the original messages in chronological order with their sequence numbers and roles.

Note: This tool can only be used by sub-agent sessions. If called from the main agent,
you will be instructed to delegate the task to a Task sub-agent.`

func NewLcmExpandTool(sqlDB *sql.DB) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		LcmExpandToolName,
		lcmExpandDescription,
		func(ctx context.Context, params LcmExpandParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SummaryID == "" {
				return fantasy.NewTextErrorResponse("summary_id is required"), nil
			}

			// Check if this is a sub-agent session
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.NewTextErrorResponse("Session ID not found in context"), nil
			}

			isSubAgent, err := isSubAgentSession(ctx, sqlDB, sessionID)
			if err != nil {
				return fantasy.ToolResponse{}, fmt.Errorf("error checking session type: %w", err)
			}

			if !isSubAgent {
				return fantasy.NewTextErrorResponse(lcmExpandMainSessionDenied), nil
			}

			// Expand the summary
			messages, err := expandSummary(ctx, sqlDB, sessionID, params.SummaryID)
			if err != nil {
				if err == sql.ErrNoRows {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("Summary not found: %s", params.SummaryID)), nil
				}
				if err == errLCMAccessDenied {
					return fantasy.NewTextErrorResponse(
						fmt.Sprintf("Access denied: %s is outside this session lineage", params.SummaryID),
					), nil
				}
				return fantasy.ToolResponse{}, fmt.Errorf("error expanding summary: %w", err)
			}

			if len(messages) == 0 {
				return fantasy.NewTextResponse("Summary contains no messages.\n"), nil
			}

			// Format output
			var output strings.Builder
			fmt.Fprintf(&output, "Expanded %d messages from summary %s:\n\n", len(messages), params.SummaryID)

			for _, msg := range messages {
				fmt.Fprintf(&output, "--- Message %s (seq: %d, role: %s) ---\n", msg.id, msg.seq, msg.role)
				text := extractTextFromParts(msg.parts)
				output.WriteString(text)
				output.WriteString("\n\n")
			}

			return fantasy.NewTextResponse(output.String()), nil
		})
}

type expandedMessage struct {
	id    string
	seq   int64
	role  string
	parts string
}

// isSubAgentSession checks if a session has a parent session (indicating it's a sub-agent)
func isSubAgentSession(ctx context.Context, db *sql.DB, sessionID string) (bool, error) {
	var parentSessionID sql.NullString
	query := `SELECT parent_session_id FROM sessions WHERE id = ?`

	err := db.QueryRowContext(ctx, query, sessionID).Scan(&parentSessionID)
	if err != nil {
		return false, err
	}

	return parentSessionID.Valid && parentSessionID.String != "", nil
}

// expandSummary recursively expands a summary to retrieve all original messages
func expandSummary(ctx context.Context, db *sql.DB, callerSessionID, summaryID string) ([]expandedMessage, error) {
	// Use recursive CTEs to scope the summary lookup by caller session lineage,
	// then expand parent summaries.
	query := `
WITH RECURSIVE lineage(id) AS (
    SELECT ?
    UNION
    SELECT s.parent_session_id
    FROM sessions s
    JOIN lineage l ON s.id = l.id
    WHERE s.parent_session_id IS NOT NULL
), expanded(summary_id) AS (
    SELECT ls.summary_id
    FROM lcm_summaries ls
    WHERE ls.summary_id = ?
      AND ls.session_id IN (SELECT id FROM lineage)
    UNION
    SELECT sp.parent_summary_id
    FROM lcm_summary_parents sp
    JOIN expanded e ON sp.summary_id = e.summary_id
)
SELECT DISTINCT m.id, m.seq, m.role, m.parts
FROM lcm_summary_messages sm
JOIN messages m ON m.id = sm.message_id
JOIN expanded e ON e.summary_id = sm.summary_id
ORDER BY m.seq ASC`

	rows, err := db.QueryContext(ctx, query, callerSessionID, summaryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []expandedMessage
	for rows.Next() {
		var msg expandedMessage
		if err := rows.Scan(&msg.id, &msg.seq, &msg.role, &msg.parts); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// If no messages found, check if summary exists and whether access is denied.
	if len(messages) == 0 {
		exists, err := lcmSummaryExists(ctx, db, summaryID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, sql.ErrNoRows
		}

		inLineage, err := lcmSummaryInSessionLineage(ctx, db, callerSessionID, summaryID)
		if err != nil {
			return nil, err
		}
		if !inLineage {
			return nil, errLCMAccessDenied
		}
	}

	return messages, nil
}
