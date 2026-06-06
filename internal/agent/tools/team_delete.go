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

func NewTeamDeleteTool(registry AgentRegistry, teamManager TeamManager) fantasy.AgentTool {
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

			if teamManager == nil {
				return fantasy.NewTextErrorResponse("team manager not configured"), nil
			}

			agentIDs, ok := teamManager.GetTeamAgents(params.TeamName)
			if !ok {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("team %q not found", params.TeamName)), nil
			}

			for _, id := range agentIDs {
				if a, ok := registry.Get(id); ok {
					a.Stop()
					a.Close()
				}
			}

			teamManager.DeleteTeam(params.TeamName)

			result := fmt.Sprintf("Team %q deleted (%d agent(s) stopped)", params.TeamName, len(agentIDs))
			metadata := TeamDeleteResponseMetadata{
				TeamName: params.TeamName,
				Deleted:  true,
			}
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result), metadata), nil
		},
	)
}
