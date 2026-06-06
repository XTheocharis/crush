package tools

import (
	"context"
	_ "embed"
	"fmt"
	"sort"
	"strings"

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

var knownRoles = map[string]bool{
	"researcher": true,
	"tester":     true,
	"reviewer":   true,
}

func isKnownRole(name string) bool {
	return knownRoles[name]
}

func sortedRoleNames() []string {
	names := make([]string, 0, len(knownRoles))
	for name := range knownRoles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func NewTeamCreateTool(registry AgentRegistry, mailbox Mailbox, teamManager TeamManager) fantasy.AgentTool {
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
			var invalidRoles []string
			for _, name := range params.Agents {
				if registry.HasAgent(name) {
					found = append(found, name)
					continue
				}
				if !isKnownRole(name) {
					invalidRoles = append(invalidRoles, name)
					continue
				}
				found = append(found, name)
			}

			if len(found) == 0 {
				return fantasy.NewTextErrorResponse(fmt.Sprintf(
					"no valid agents or roles found: %v (valid roles: %s)",
					params.Agents, strings.Join(sortedRoleNames(), ", "),
				)), nil
			}

			if teamManager != nil {
				teamManager.CreateTeam(params.TeamName, found)
			}

			result := fmt.Sprintf("Team %q created with %d agent(s)", params.TeamName, len(found))
			if len(invalidRoles) > 0 {
				result += fmt.Sprintf(" (invalid roles: %v)", invalidRoles)
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
