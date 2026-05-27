package tools

import (
	"context"

	"charm.land/fantasy"
)

const MapRefreshToolName = "map_refresh"

type MapRefreshParams struct {
	Sync bool `json:"sync,omitempty" description:"When true, run refresh synchronously and return after completion"`
}

// MapRefreshFn refreshes repo-map state for a session.
type MapRefreshFn func(ctx context.Context, sessionID string) error

func NewMapRefreshTool(refreshSync MapRefreshFn, refreshAsync MapRefreshFn) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		MapRefreshToolName,
		`Refresh the repository map for the current session.

Use this tool when the repository map appears stale and you need it rebuilt.

By default this schedules an asynchronous refresh. Set sync=true to run synchronously.

This tool is safe to call repeatedly.`,
		func(ctx context.Context, params MapRefreshParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.NewTextErrorResponse("session ID is required for map refresh"), nil
			}

			if params.Sync {
				if refreshSync == nil {
					return fantasy.NewTextErrorResponse("repo map refresh is not available in this session"), nil
				}
				if err := refreshSync(ctx, sessionID); err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				return fantasy.NewTextResponse("Repository map refreshed."), nil
			}

			if refreshAsync == nil {
				return fantasy.NewTextErrorResponse("repo map refresh is not available in this session"), nil
			}
			if err := refreshAsync(ctx, sessionID); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.NewTextResponse("Repository map refresh scheduled."), nil
		},
	)
}
