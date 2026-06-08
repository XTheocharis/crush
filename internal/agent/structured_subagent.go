package agent

import (
	"context"
	"fmt"
	"slices"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/config"
)

const (
	// DefaultMaxRecursionDepth is the default maximum recursion depth for
	// subagents.
	DefaultMaxRecursionDepth = 3
)

// CoordinatorOption mutates coordinator construction.
type CoordinatorOption func(*coordinator)

// WithStructuredSubagentFactory wires a StructuredSubagentFactory into the
// coordinator for typed subagent request/response execution.
func WithStructuredSubagentFactory(factory StructuredSubagentFactory) CoordinatorOption {
	return func(c *coordinator) {
		c.structuredSubagentFactory = factory
	}
}

// WithCostTracker overrides the default CostTracker.
func WithCostTracker(ct *CostTracker) CoordinatorOption {
	return func(c *coordinator) {
		c.costTracker = ct
	}
}

// WithMetricsStore overrides the default MetricsStore.
func WithMetricsStore(store *MetricsStore) CoordinatorOption {
	return func(c *coordinator) {
		c.metricsStore = store
	}
}

// WithTierRouter wires a TierRouter for fallback-chain resolution during
// LLM retries. When set, Run and runSubAgent wrap their calls with
// ExecuteWithFallback.
func WithTierRouter(r *TierRouter) CoordinatorOption {
	return func(c *coordinator) {
		c.tierRouter = r
	}
}

// WithTieredModelProvider wires a TieredModelProvider for per-tier model
// selection. When set, the coordinator resolves different models per step
// based on task complexity. When nil, the single-model path is used.
func WithTieredModelProvider(p *TieredModelProvider) CoordinatorOption {
	return func(c *coordinator) {
		c.tieredProvider = p
	}
}

type structuredSubagent struct {
	coordinator     *coordinator
	parentSessionID string
	agent           SessionAgent
	allTools        []fantasy.AgentTool
	depth           int
	maxDepth        int
}

// NewStructuredSubagentFactory returns a factory backed by the given
// coordinator.
func NewStructuredSubagentFactory(c *coordinator) StructuredSubagentFactory {
	return &coordinatorFactory{coordinator: c}
}

// coordinatorFactory creates StructuredSubagent instances backed by a single
// coordinator. It implements StructuredSubagentFactory.
type coordinatorFactory struct {
	coordinator *coordinator
}

func (f *coordinatorFactory) NewStructuredSubagent(ctx context.Context, parentSessionID string) (StructuredSubagent, error) {
	return f.newSubagent(ctx, parentSessionID, 0)
}

func (f *coordinatorFactory) newSubagent(ctx context.Context, parentSessionID string, currentDepth int) (StructuredSubagent, error) {
	if f.coordinator == nil {
		return nil, fmt.Errorf("structured subagent: coordinator is nil")
	}
	if parentSessionID == "" {
		return nil, fmt.Errorf("structured subagent: parent session ID is required")
	}

	agent, allTools, err := f.coordinator.buildStructuredAgent(ctx)
	if err != nil {
		return nil, fmt.Errorf("structured subagent: build agent: %w", err)
	}

	return &structuredSubagent{
		coordinator:     f.coordinator,
		parentSessionID: parentSessionID,
		agent:           agent,
		allTools:        allTools,
		depth:           currentDepth,
		maxDepth:        DefaultMaxRecursionDepth,
	}, nil
}

// Capabilities returns the names of all tools available to this subagent.
func (s *structuredSubagent) Capabilities() []string {
	var names []string
	for _, t := range s.allTools {
		names = append(names, t.Info().Name)
	}
	return names
}

// Depth returns the current recursion depth of this subagent.
func (s *structuredSubagent) Depth() int {
	return s.depth
}

// MaxDepth returns the maximum allowed recursion depth for nested subagents.
func (s *structuredSubagent) MaxDepth() int {
	return s.maxDepth
}

// Execute runs the subagent with the given structured request. It enforces
// recursion depth limits, applies optional timeouts, filters the tool set
// when req.Tools is non-empty, and returns a typed response.
func (s *structuredSubagent) Execute(ctx context.Context, req StructuredRequest) (StructuredResponse, error) {
	if req.Task == "" {
		return StructuredResponse{Success: false, Error: "task is required"}, nil
	}

	if s.depth >= s.maxDepth {
		return StructuredResponse{
			Success: false,
			Error:   fmt.Sprintf("max recursion depth %d reached", s.maxDepth),
		}, nil
	}

	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	if len(req.Tools) > 0 {
		s.agent.SetTools(filterTools(s.allTools, req.Tools))
	}

	toolCallID := fmt.Sprintf("structured-%d", time.Now().UnixNano())
	agentMessageID := fmt.Sprintf("structured-msg-%d", time.Now().UnixNano())

	result, err := s.coordinator.runSubAgent(ctx, subAgentParams{
		Agent:          s.agent,
		SessionID:      s.parentSessionID,
		AgentMessageID: agentMessageID,
		ToolCallID:     toolCallID,
		Prompt:         buildStructuredPrompt(req),
		SessionTitle:   "Structured Subagent",
	})
	if err != nil {
		return StructuredResponse{
			Success: false,
			Error:   fmt.Sprintf("execution failed: %v", err),
		}, nil
	}

	return StructuredResponse{
		Result:     result.Content,
		Success:    !result.IsError,
		StepsTaken: 1,
	}, nil
}

// buildStructuredAgent creates a sub-agent using the task agent configuration
// and returns both the agent and its full tool set. This mirrors how
// agentTool builds its agent.
func (c *coordinator) buildStructuredAgent(ctx context.Context) (SessionAgent, []fantasy.AgentTool, error) {
	agentCfg, ok := c.cfg.Config().Agents[config.AgentTask]
	if !ok {
		return nil, nil, fmt.Errorf("task agent not configured")
	}

	p, err := taskPrompt(prompt.WithWorkingDir(c.cfg.WorkingDir()))
	if err != nil {
		return nil, nil, fmt.Errorf("task prompt: %w", err)
	}

	agent, err := c.buildAgent(ctx, p, agentCfg, true)
	if err != nil {
		return nil, nil, fmt.Errorf("build agent: %w", err)
	}

	tools, err := c.buildTools(ctx, agentCfg, true)
	if err != nil {
		return nil, nil, fmt.Errorf("build tools: %w", err)
	}

	return agent, tools, nil
}

func buildStructuredPrompt(req StructuredRequest) string {
	p := req.Task
	if len(req.Context) > 0 {
		p = "Context:\n"
		for k, v := range req.Context {
			p += fmt.Sprintf("- %s: %s\n", k, v)
		}
		p += "\nTask:\n" + req.Task
	}
	if req.MaxSteps > 0 {
		p += fmt.Sprintf("\n\n(Limit your approach to at most %d steps.)", req.MaxSteps)
	}
	return p
}

func filterTools(all []fantasy.AgentTool, allow []string) []fantasy.AgentTool {
	if len(allow) == 0 {
		return all
	}
	filtered := make([]fantasy.AgentTool, 0, len(allow))
	for _, t := range all {
		if slices.Contains(allow, t.Info().Name) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
