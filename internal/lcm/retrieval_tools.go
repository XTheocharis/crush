package lcm

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
)

type bindleParams struct {
	SummaryID string `json:"summary_id" description:"ID of the summary to retrieve"`
}

type ancestryParams struct {
	SummaryID string `json:"summary_id" description:"ID of the summary whose ancestry chain to trace"`
}

type doltParams struct {
	SessionID string `json:"session_id" description:"Session ID to list summaries for"`
}

type archiveParams struct {
	SessionID string `json:"session_id" description:"Session ID to search within"`
	Pattern   string `json:"pattern"   description:"Full-text search pattern"`
	Ranked    bool   `json:"ranked"    description:"Use bm25() relevance ranking with snippet highlighting (default true)"`
	Limit     int    `json:"limit"     description:"Maximum number of results (default 10, only used with ranked=true)"`
}

type sprigParams struct {
	SessionID string `json:"session_id" description:"Session ID to get the latest summary for"`
}

type timeQueryParams struct {
	SessionID string `json:"session_id" description:"Session ID to query messages for (required)"`
	StartTime int64  `json:"start_time" description:"Start of time range as Unix seconds (inclusive)"`
	EndTime   int64  `json:"end_time"   description:"End of time range as Unix seconds (inclusive)"`
}

type lineageParams struct {
	SummaryID string `json:"summary_id" description:"ID of the summary to trace lineage for"`
	Direction string `json:"direction"  description:"Traversal direction: ancestors, descendants, or both (default both)"`
	MaxDepth  int    `json:"max_depth"  description:"Maximum traversal depth (default 10)"`
}

func newBindleTool(store *Store) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"lcm_bindle",
		"Retrieve a compressed summary by ID. Returns the summary content with metadata including session, kind, token count, and parent summaries.",
		func(ctx context.Context, params bindleParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SummaryID == "" {
				return fantasy.NewTextErrorResponse("summary_id is required"), nil
			}
			result, err := store.Bindle(ctx, params.SummaryID)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Error retrieving summary: %v", err)), nil
			}
			return fantasy.NewTextResponse(result), nil
		})
}

func newAncestryTool(store *Store) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"lcm_ancestry",
		"Trace the ancestry chain of a summary. Returns the full parent chain with kind and token information for each level.",
		func(ctx context.Context, params ancestryParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SummaryID == "" {
				return fantasy.NewTextErrorResponse("summary_id is required"), nil
			}
			result, err := store.Ancestry(ctx, params.SummaryID)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Error tracing ancestry: %v", err)), nil
			}
			return fantasy.NewTextResponse(result), nil
		})
}

func newDoltTool(store *Store) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"lcm_dolt",
		"List all summaries for a session. Returns each summary with ID, kind, token count, and a content preview.",
		func(ctx context.Context, params doltParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}
			result, err := store.Dolt(ctx, params.SessionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Error listing summaries: %v", err)), nil
			}
			return fantasy.NewTextResponse(result), nil
		})
}

func newArchiveTool(store *Store) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"lcm_archive",
		"Search summary content using full-text search. When ranked=true (default), results include bm25() relevance scores and highlighted snippets.",
		func(ctx context.Context, params archiveParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}
			if params.Pattern == "" {
				return fantasy.NewTextErrorResponse("pattern is required"), nil
			}

			if !params.Ranked {
				result, err := store.Archive(ctx, params.SessionID, params.Pattern)
				if err != nil {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("Error searching summaries: %v", err)), nil
				}
				return fantasy.NewTextResponse(result), nil
			}

			results, err := store.SearchSummariesRanked(ctx, params.Pattern, params.Limit)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Error searching summaries: %v", err)), nil
			}
			if len(results) == 0 {
				return fantasy.NewTextResponse(fmt.Sprintf("No summaries matching %q found.", params.Pattern)), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "Found %d summaries matching %q (ranked by relevance):\n\n", len(results), params.Pattern)
			for i, r := range results {
				fmt.Fprintf(&b, "  [%d] %s  (rank=%.4f)\n", i, r.SummaryID, r.Rank)
				fmt.Fprintf(&b, "      %s\n", r.Snippet)
			}
			return fantasy.NewTextResponse(b.String()), nil
		})
}

func newSprigTool(store *Store) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"lcm_sprig",
		"Retrieve the most recent summary for a session. Returns the full content with metadata.",
		func(ctx context.Context, params sprigParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}
			result, err := store.Sprig(ctx, params.SessionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Error retrieving latest summary: %v", err)), nil
			}
			return fantasy.NewTextResponse(result), nil
		})
}

func newTimeQueryTool(store *Store) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"lcm_time_query",
		"Query messages by time range. Returns messages within [start_time, end_time] inclusive, with timestamps and roles.",
		func(ctx context.Context, params timeQueryParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}
			if params.StartTime == 0 && params.EndTime == 0 {
				return fantasy.NewTextErrorResponse("start_time and end_time are required"), nil
			}
			msgs, err := store.QueryByTime(ctx, params.SessionID, params.StartTime, params.EndTime)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Error querying by time: %v", err)), nil
			}
			if len(msgs) == 0 {
				return fantasy.NewTextResponse("No messages found in the specified time range."), nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Found %d messages in range [%d, %d]:\n", len(msgs), params.StartTime, params.EndTime)
			for _, m := range msgs {
				fmt.Fprintf(&b, "\n[%d] %s (seq %d): %s", m.CreatedAt, m.Role, m.Seq, truncateString(m.Content, 200))
			}
			return fantasy.NewTextResponse(b.String()), nil
		})
}

type activeContextParams struct {
	SessionID  string `json:"session_id"  description:"Session ID to get active context for"`
	FilterType string `json:"filter_type" description:"Filter by item type: 'message' or 'summary'"`
	MinTokens  int    `json:"min_tokens"  description:"Minimum token count filter"`
}

func newActiveContextTool(store *Store) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"lcm_active_context",
		"Get the active context overview for a session. Returns entry count, total tokens, and per-entry details.",
		func(ctx context.Context, params activeContextParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}

			var filter ContextFilter
			if params.FilterType != "" {
				ft := params.FilterType
				filter.Type = &ft
			}
			if params.MinTokens > 0 {
				mt := params.MinTokens
				filter.MinTokens = &mt
			}

			var ac *ActiveContext
			var err error

			if filter.Type != nil || filter.MinTokens != nil {
				ac, err = store.GetActiveContextFiltered(ctx, params.SessionID, filter)
			} else {
				ac, err = store.GetActiveContext(ctx, params.SessionID)
			}

			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Error getting active context: %v", err)), nil
			}

			return fantasy.NewTextResponse(formatActiveContext(ac)), nil
		})
}

func formatActiveContext(ac *ActiveContext) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Active Context for session %s\n", ac.SessionID)
	fmt.Fprintf(&sb, "Entries: %d | Total tokens: %d\n\n", ac.EntryCount, ac.TotalTokens)

	for i, e := range ac.Entries {
		fmt.Fprintf(&sb, "[%d] type=%s tokens=%d", i+1, e.ItemType, e.TokenCount)
		if e.MessageID != "" {
			fmt.Fprintf(&sb, " message_id=%s", e.MessageID)
		}
		if e.SummaryID != "" {
			fmt.Fprintf(&sb, " summary_id=%s", e.SummaryID)
		}
		sb.WriteByte('\n')
	}

	return sb.String()
}

func newLineageTool(store *Store) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"lcm_lineage",
		"Trace the lineage of a summary in the summary DAG. Returns ancestor and/or descendant summaries with depth and metadata.",
		func(ctx context.Context, params lineageParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SummaryID == "" {
				return fantasy.NewTextErrorResponse("summary_id is required"), nil
			}
			direction := parseLineageDirection(params.Direction)
			maxDepth := params.MaxDepth
			if maxDepth <= 0 {
				maxDepth = 10
			}
			result, err := store.Lineage(ctx, sessionIDFromContext(ctx), params.SummaryID, direction, maxDepth)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Error tracing lineage: %v", err)), nil
			}
			return fantasy.NewTextResponse(result), nil
		})
}

type fileSearchParams struct {
	SessionID string `json:"session_id" description:"Session ID to search large files in"`
	Query     string `json:"query"      description:"Full-text search query for large file content"`
	Limit     int    `json:"limit"      description:"Maximum number of results (default 20)"`
}

func newFileSearchTool(store *Store) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"lcm_file_search",
		"Search large file content stored in LCM using full-text search. Returns ranked results with file paths and snippets.",
		func(ctx context.Context, params fileSearchParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}
			if params.Query == "" {
				return fantasy.NewTextErrorResponse("query is required"), nil
			}
			limit := params.Limit
			if limit <= 0 {
				limit = 20
			}
			results, err := store.SearchLargeFiles(ctx, params.SessionID, params.Query, limit)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Error searching large files: %v", err)), nil
			}
			if len(results) == 0 {
				return fantasy.NewTextResponse("No matching large files found."), nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Found %d matching files:\n\n", len(results))
			for i, r := range results {
				fmt.Fprintf(&b, "[%d] %s (file_id=%s, rank=%.2f)\n    %s\n\n", i+1, r.Path, r.FileID, r.Rank, r.Snippet)
			}
			return fantasy.NewTextResponse(b.String()), nil
		})
}

func parseLineageDirection(s string) LineageDirection {
	switch s {
	case "ancestors":
		return LineageAncestors
	case "descendants":
		return LineageDescendants
	default:
		return LineageBoth
	}
}

func sessionIDFromContext(ctx context.Context) string {
	if s, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return s
	}
	return ""
}

type sessionIDKey struct{}
