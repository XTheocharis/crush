package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
)

const (
	TaskStopToolName = "task_stop"
)

//go:embed task_stop.md
var taskStopDescription []byte

type TaskStopParams struct {
	AgentName string `json:"agent_name" description:"Name of the forked agent to stop"`
}

type TaskStopResponseMetadata struct {
	AgentName string `json:"agent_name"`
	Stopped   bool   `json:"stopped"`
}

func NewTaskStopTool(registry AgentRegistry) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TaskStopToolName,
		string(taskStopDescription),
		func(ctx context.Context, params TaskStopParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.AgentName == "" {
				return fantasy.NewTextErrorResponse("missing agent_name"), nil
			}

			if registry == nil {
				return fantasy.NewTextErrorResponse("agent registry not configured"), nil
			}

			forkedAgent, ok := registry.Get(params.AgentName)
			if !ok {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("agent %q not found", params.AgentName)), nil
			}

			wasRunning := forkedAgent.IsRunning()
			forkedAgent.Stop()

			result := fmt.Sprintf("Agent %q stop signal sent", params.AgentName)
			if !wasRunning {
				result = fmt.Sprintf("Agent %q was not running", params.AgentName)
			}

			metadata := TaskStopResponseMetadata{
				AgentName: params.AgentName,
				Stopped:   wasRunning,
			}
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result), metadata), nil
		},
	)
}
