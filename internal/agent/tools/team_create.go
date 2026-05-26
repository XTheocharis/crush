package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
)

const (
	TeamCreateToolName = "team_create"
)

//go:embed team_create.md
var teamCreateDescription []byte

type TeamCreateParams struct {
	TeamName string   `json:"team_name" description:"Name for the new team"`
	Agents   []string `json:"agents" description:"List of agent names to include in the team"`
}

type TeamCreateResponseMetadata struct {
	TeamName    string   `json:"team_name"`
	Agents      []string `json:"agents"`
	AgentsFound int      `json:"agents_found"`
}

func NewTeamCreateTool(registry AgentRegistry, mailbox Mailbox) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TeamCreateToolName,
		string(teamCreateDescription),
		func(ctx context.Context, params TeamCreateParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.TeamName == "" {
				return fantasy.NewTextErrorResponse("missing team_name"), nil
			}
			if len(params.Agents) == 0 {
				return fantasy.NewTextErrorResponse("missing agents list"), nil
			}

			if registry == nil {
				return fantasy.NewTextErrorResponse("agent registry not configured"), nil
			}

			var found []string
			var missing []string
			for _, name := range params.Agents {
				if registry.HasAgent(name) {
					found = append(found, name)
				} else {
					missing = append(missing, name)
				}
			}

			if len(found) == 0 {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("none of the specified agents found: %v", params.Agents)), nil
			}

			result := fmt.Sprintf("Team %q created with %d agent(s)", params.TeamName, len(found))
			if len(missing) > 0 {
				result += fmt.Sprintf(" (skipped missing: %v)", missing)
			}

			metadata := TeamCreateResponseMetadata{
				TeamName:    params.TeamName,
				Agents:      found,
				AgentsFound: len(found),
			}
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result), metadata), nil
		},
	)
}
