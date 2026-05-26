package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
)

const (
	TeamDeleteToolName = "team_delete"
)

//go:embed team_delete.md
var teamDeleteDescription []byte

type TeamDeleteParams struct {
	TeamName string `json:"team_name" description:"Name of the team to delete"`
}

type TeamDeleteResponseMetadata struct {
	TeamName string `json:"team_name"`
	Deleted  bool   `json:"deleted"`
}

func NewTeamDeleteTool(registry AgentRegistry) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TeamDeleteToolName,
		string(teamDeleteDescription),
		func(ctx context.Context, params TeamDeleteParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.TeamName == "" {
				return fantasy.NewTextErrorResponse("missing team_name"), nil
			}

			if registry == nil {
				return fantasy.NewTextErrorResponse("agent registry not configured"), nil
			}

			// In the current implementation, teams are logical groupings.
			// Deleting a team means unregistering its member agents.
			agents := registry.List()
			found := false
			for _, name := range agents {
				if a, ok := registry.Get(name); ok {
					if a.Name() == params.TeamName {
						a.Stop()
						a.Close()
						found = true
					}
				}
			}

			if !found {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("team %q not found", params.TeamName)), nil
			}

			result := fmt.Sprintf("Team %q deleted", params.TeamName)
			metadata := TeamDeleteResponseMetadata{
				TeamName: params.TeamName,
				Deleted:  true,
			}
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result), metadata), nil
		},
	)
}
