package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
)

// TheSwarmExtension is the singleton swarm extension instance registered at
// init.
var TheSwarmExtension = &SwarmExtension{}

// SwarmExtension wraps the swarm parallel subagent pattern as a ToolProvider.
type SwarmExtension struct {
	mu       sync.RWMutex
	host     ext.HostContext
	factory  agent.StructuredSubagentFactory
	registry tools.AgentRegistry
	mailbox  tools.Mailbox
	tools    []fantasy.AgentTool
	names    []string
	active   bool
}

func (e *SwarmExtension) Name() string { return "swarm" }

func (e *SwarmExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.active = true
	return nil
}

func (e *SwarmExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.factory = nil
	e.registry = nil
	e.mailbox = nil
	e.tools = nil
	e.names = nil
	e.active = false
	return nil
}

func (e *SwarmExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	return append([]fantasy.AgentTool{}, e.tools...), nil
}

func (e *SwarmExtension) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return append([]string{}, e.names...)
}

func (e *SwarmExtension) SetFactory(factory agent.StructuredSubagentFactory) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.factory = factory
}

func (e *SwarmExtension) SetRegistry(registry tools.AgentRegistry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registry = registry
}

func (e *SwarmExtension) SetMailbox(mailbox tools.Mailbox) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mailbox = mailbox
}

func (e *SwarmExtension) RebuildTools() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tools = buildSwarmTools(e.factory)
	e.names = make([]string, len(e.tools))
	for i, t := range e.tools {
		e.names[i] = t.Info().Name
	}
}

func buildSwarmTools(factory agent.StructuredSubagentFactory) []fantasy.AgentTool {
	if factory == nil {
		return nil
	}
	return []fantasy.AgentTool{
		newSwarmExecuteTool(factory),
	}
}

const swarmExecuteToolName = "swarm_execute"

type swarmExecuteParams struct {
	Goal         string `json:"goal" description:"High-level goal to decompose and execute in parallel"`
	MaxSubagents int    `json:"max_subagents,omitempty" description:"Maximum parallel subagents (default 5)"`
}

func newSwarmExecuteTool(factory agent.StructuredSubagentFactory) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		swarmExecuteToolName,
		swarmExecuteDescription,
		func(ctx context.Context, params swarmExecuteParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Goal == "" {
				return fantasy.NewTextErrorResponse("missing goal"), nil
			}
			if factory == nil {
				return fantasy.NewTextErrorResponse("swarm not configured: no structured subagent factory"), nil
			}

			sessionID := tools.GetSessionFromContext(ctx)

			cfg := agent.SwarmConfig{
				DecomposeFn:  agent.DefaultDecompose,
				SynthesizeFn: agent.DefaultSynthesize,
				MaxSubagents: params.MaxSubagents,
			}

			cache := agent.NewSharedCache()
			par := agent.NewParallelController(agent.ParallelControllerConfig{})
			swarm := agent.NewSwarmPattern(cfg, cache, par, factory)

			resp, err := swarm.Execute(ctx, sessionID, params.Goal)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					"swarm execution failed: " + err.Error(),
				), nil
			}

			if !resp.Success {
				return fantasy.NewTextErrorResponse(
					"swarm execution unsuccessful: " + resp.Error,
				), nil
			}

			return fantasy.NewTextResponse(resp.Result), nil
		},
	)
}

var swarmExecuteDescription = `Decompose a task into subtasks, run them in parallel via subagents, and synthesize the results.

<usage>
- Provide a high-level goal that can be broken into independent subtasks
- Each line of the goal is treated as a separate subtask for parallel execution
- Results from all subtasks are merged into a single synthesized response
- Use max_subagents to cap parallelism (default 5)
</usage>

<features>
- Parallel execution of independent subtasks
- Shared caching avoids redundant work across subagents
- Configurable concurrency limits
- Automatic result synthesis from all subagent outputs
</features>

<tips>
- Break complex tasks into independent, line-separated subtasks for best results
- Subtasks should be self-contained and not depend on each other's output
- Use for research, analysis, or exploration tasks that benefit from parallelism
- Not suitable for tasks with sequential dependencies between subtasks
</tips>`

func (e *SwarmExtension) NewSwarm(cfg agent.SwarmConfig) *agent.SwarmPattern {
	e.mu.RLock()
	factory := e.factory
	e.mu.RUnlock()
	cache := agent.NewSharedCache()
	par := agent.NewParallelController(agent.ParallelControllerConfig{})
	return agent.NewSwarmPattern(cfg, cache, par, factory)
}

var (
	_ ext.Extension    = (*SwarmExtension)(nil)
	_ ext.ToolProvider = (*SwarmExtension)(nil)
)
