package lcm

import (
	"context"
	"fmt"

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
}

type sprigParams struct {
	SessionID string `json:"session_id" description:"Session ID to get the latest summary for"`
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
		"Search summary content using full-text search within a session. Returns matching summaries with their IDs and kinds.",
		func(ctx context.Context, params archiveParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.SessionID == "" {
				return fantasy.NewTextErrorResponse("session_id is required"), nil
			}
			if params.Pattern == "" {
				return fantasy.NewTextErrorResponse("pattern is required"), nil
			}
			result, err := store.Archive(ctx, params.SessionID, params.Pattern)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Error searching summaries: %v", err)), nil
			}
			return fantasy.NewTextResponse(result), nil
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
