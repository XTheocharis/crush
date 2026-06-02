package extensions

import (
	"context"
	"strconv"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
)

var TheProductiveExtension = &ProductiveExtension{}

type ProductiveExtension struct {
	mu      sync.RWMutex
	host    ext.HostContext
	factory agent.StructuredSubagentFactory
	tools   []fantasy.AgentTool
	names   []string
	active  bool
}

func (e *ProductiveExtension) Name() string { return "productive" }

func (e *ProductiveExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	e.active = true
	return nil
}

func (e *ProductiveExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.factory = nil
	e.tools = nil
	e.names = nil
	e.active = false
	return nil
}

func (e *ProductiveExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	return append([]fantasy.AgentTool{}, e.tools...), nil
}

func (e *ProductiveExtension) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return append([]string{}, e.names...)
}

func (e *ProductiveExtension) SetFactory(factory agent.StructuredSubagentFactory) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.factory = factory
}

func (e *ProductiveExtension) RebuildTools() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tools = buildProductiveTools(e.factory)
	e.names = make([]string, len(e.tools))
	for i, t := range e.tools {
		e.names[i] = t.Info().Name
	}
}

func buildProductiveTools(factory agent.StructuredSubagentFactory) []fantasy.AgentTool {
	if factory == nil {
		return nil
	}
	return []fantasy.AgentTool{
		newProductiveExecuteTool(factory),
	}
}

const productiveExecuteToolName = "productive_execute"

type productiveExecuteParams struct {
	Task          string `json:"task" description:"Task description for iterative refinement"`
	MaxIterations int    `json:"max_iterations,omitempty" description:"Maximum iterations (default 5)"`
}

func newProductiveExecuteTool(factory agent.StructuredSubagentFactory) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		productiveExecuteToolName,
		productiveExecuteDescription,
		func(ctx context.Context, params productiveExecuteParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Task == "" {
				return fantasy.NewTextErrorResponse("missing task"), nil
			}
			if factory == nil {
				return fantasy.NewTextErrorResponse("productive not configured: no structured subagent factory"), nil
			}

			sessionID := tools.GetSessionFromContext(ctx)

			maxIter := params.MaxIterations
			if maxIter <= 0 {
				maxIter = 5
			}

			cfg := agent.ProductiveConfig{
				MaxIterations: maxIter,
			}

			cache := agent.NewSharedCache()
			prod := agent.NewProductive(cfg, cache, factory, nil)

			result := prod.Run(ctx, sessionID, params.Task)

			if result.Error != "" {
				return fantasy.NewTextErrorResponse("productive execution failed: " + result.Error), nil
			}

			if result.Stalled {
				return fantasy.NewTextResponse(
					"Productive loop stalled after " + strconv.Itoa(result.Iterations) + " iterations.\n\n" + result.Result,
				), nil
			}

			return fantasy.NewTextResponse(result.Result), nil
		},
	)
}

var productiveExecuteDescription = `Run an iterative refinement loop on a task using subagents.

<usage>
- Provide a task description that benefits from iterative refinement
- Each iteration spawns a subagent with accumulated context from prior iterations
- The loop continues until success, max iterations, or a progress stall is detected
- Use max_iterations to cap the loop (default 5)
</usage>

<features>
- Iterative refinement with accumulated context across iterations
- Automatic stall detection when outputs stop changing
- Shared caching avoids redundant work
- Doom-loop awareness via ProductiveLoopDetector
</features>

<tips>
- Best for tasks that benefit from incremental improvement: code review, refactoring, research
- Each iteration receives context from all prior iterations
- Stall detection exits early when outputs stop changing (same fingerprint)
- Not suitable for one-shot tasks that don't benefit from iteration
</tips>`

var (
	_ ext.Extension    = (*ProductiveExtension)(nil)
	_ ext.ToolProvider = (*ProductiveExtension)(nil)
)
