package tools

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"charm.land/fantasy"
)

const (
	LcmDescribeToolName       = "lcm_describe"
	maxDescribeContentPreview = 2000
)

type LcmDescribeParams struct {
	ID string `json:"id" description:"A file_xxx or sum_xxx identifier to describe"`
}

var lcmDescribeDescription = `Describe a file or summary by its ID.

This tool retrieves detailed information about a large file or summary referenced by its ID.

Parameters:
- id: A file_xxx or sum_xxx identifier

For files (file_xxx):
- Shows the original path, size in tokens, and content preview
- Shows exploration summary if the file was explored by an explorer tool

For summaries (sum_xxx):
- Shows the summary kind (leaf or condensed)
- Shows the full summary content and token count
- For condensed summaries, shows the parent summaries it was created from`

func NewLcmDescribeTool(sqlDB *sql.DB) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		LcmDescribeToolName,
		lcmDescribeDescription,
		func(ctx context.Context, params LcmDescribeParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.ID == "" {
				return fantasy.NewTextErrorResponse("id is required"), nil
			}

			// Dispatch based on prefix
			if strings.HasPrefix(params.ID, "file_") {
				return describeFile(ctx, sqlDB, params.ID)
			} else if strings.HasPrefix(params.ID, "sum_") {
				return describeSummary(ctx, sqlDB, params.ID)
			} else {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Invalid ID format: %s (must start with file_ or sum_)", params.ID)), nil
			}
		})
}

func describeFile(ctx context.Context, db *sql.DB, fileID string) (fantasy.ToolResponse, error) {
	query := `SELECT original_path, content, token_count, exploration_summary, explorer_used
	          FROM lcm_large_files WHERE file_id = ?`

	var originalPath string
	var content sql.NullString
	var tokenCount int64
	var explorationSummary sql.NullString
	var explorerUsed sql.NullString

	err := db.QueryRowContext(ctx, query, fileID).Scan(
		&originalPath, &content, &tokenCount, &explorationSummary, &explorerUsed,
	)

	if err == sql.ErrNoRows {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("File not found: %s", fileID)), nil
	}
	if err != nil {
		return fantasy.ToolResponse{}, fmt.Errorf("error querying file: %w", err)
	}

	// Format output
	var output strings.Builder
	fmt.Fprintf(&output, "File ID: %s\n", fileID)
	fmt.Fprintf(&output, "Path: %s\n", originalPath)
	fmt.Fprintf(&output, "Size: %d tokens\n", tokenCount)

	if explorerUsed.Valid && explorerUsed.String != "" {
		fmt.Fprintf(&output, "Explorer: %s\n", explorerUsed.String)
	}

	if explorationSummary.Valid && explorationSummary.String != "" {
		fmt.Fprintf(&output, "Exploration summary:\n%s\n", explorationSummary.String)
	}

	if content.Valid && content.String != "" {
		fmt.Fprintf(&output, "\nContent preview:\n")
		preview := content.String
		if len(preview) > maxDescribeContentPreview {
			preview = preview[:maxDescribeContentPreview] + "\n... (truncated)"
		}
		output.WriteString(preview)
		output.WriteString("\n")
	}

	return fantasy.NewTextResponse(output.String()), nil
}

func describeSummary(ctx context.Context, db *sql.DB, summaryID string) (fantasy.ToolResponse, error) {
	// Get summary info
	query := `SELECT kind, content, token_count, file_ids FROM lcm_summaries WHERE summary_id = ?`

	var kind string
	var content string
	var tokenCount int64
	var fileIDs string

	err := db.QueryRowContext(ctx, query, summaryID).Scan(&kind, &content, &tokenCount, &fileIDs)

	if err == sql.ErrNoRows {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("Summary not found: %s", summaryID)), nil
	}
	if err != nil {
		return fantasy.ToolResponse{}, fmt.Errorf("error querying summary: %w", err)
	}

	// Get parent summaries if condensed
	var parents []string
	if kind == "condensed" {
		parentQuery := `SELECT parent_summary_id FROM lcm_summary_parents
		                WHERE summary_id = ? ORDER BY ord ASC`
		rows, err := db.QueryContext(ctx, parentQuery, summaryID)
		if err != nil {
			return fantasy.ToolResponse{}, fmt.Errorf("error querying parents: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var parentID string
			if err := rows.Scan(&parentID); err != nil {
				return fantasy.ToolResponse{}, fmt.Errorf("error scanning parent: %w", err)
			}
			parents = append(parents, parentID)
		}
		if err := rows.Err(); err != nil {
			return fantasy.ToolResponse{}, fmt.Errorf("error iterating parents: %w", err)
		}
	}

	// Format output
	var output strings.Builder
	fmt.Fprintf(&output, "Summary ID: %s\n", summaryID)
	fmt.Fprintf(&output, "Kind: %s\n", kind)
	fmt.Fprintf(&output, "Tokens: %d\n", tokenCount)

	if len(parents) > 0 {
		fmt.Fprintf(&output, "Parents: %s\n", strings.Join(parents, ", "))
	}

	if fileIDs != "" && fileIDs != "[]" {
		fmt.Fprintf(&output, "File IDs: %s\n", fileIDs)
	}

	fmt.Fprintf(&output, "\nContent:\n%s\n", content)

	return fantasy.NewTextResponse(output.String()), nil
}
