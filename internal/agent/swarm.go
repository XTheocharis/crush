package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	swarmCachePrefix    = "swarm"
	teammateCachePrefix = "teammate"
)

// SwarmDecomposeFunc splits a task description into N subtasks for parallel
// execution. Return an empty slice to indicate decomposition is not possible.
type SwarmDecomposeFunc func(ctx context.Context, task string) ([]string, error)

// SwarmSynthesizeFunc merges N subagent results into a single response.
type SwarmSynthesizeFunc func(results []StructuredResponse) (string, error)

// SwarmConfig configures a SwarmPattern instance.
type SwarmConfig struct {
	DecomposeFn     SwarmDecomposeFunc
	SynthesizeFn    SwarmSynthesizeFunc
	MaxSubagents    int
	CacheTTL        time.Duration
	SubagentTools   []string
	SubagentSteps   int
	SubagentTimeout time.Duration
}

func (c SwarmConfig) maxSubagents() int {
	if c.MaxSubagents <= 0 {
		return 5
	}
	return c.MaxSubagents
}

func (c SwarmConfig) cacheTTL() time.Duration {
	if c.CacheTTL <= 0 {
		return DefaultRepoMapTTL
	}
	return c.CacheTTL
}

// SwarmPattern decomposes a task into subtasks, runs them in parallel via
// StructuredSubagent, collects results, and synthesizes a single answer.
// It uses SharedCache to avoid redundant work and ParallelController to
// respect concurrency limits.
type SwarmPattern struct {
	cfg     SwarmConfig
	cache   *SharedCache
	par     *ParallelController
	factory StructuredSubagentFactory
}

func NewSwarmPattern(cfg SwarmConfig, cache *SharedCache, par *ParallelController, factory StructuredSubagentFactory) *SwarmPattern {
	return &SwarmPattern{
		cfg:     cfg,
		cache:   cache,
		par:     par,
		factory: factory,
	}
}

// Execute decomposes the task, spawns subagents in parallel, and returns a
// synthesized result. If decomposition returns zero subtasks, Execute returns
// a failure response.
func (s *SwarmPattern) Execute(ctx context.Context, parentSessionID, task string) (StructuredResponse, error) {
	if s.factory == nil {
		return StructuredResponse{}, fmt.Errorf("swarm: no structured subagent factory")
	}
	if task == "" {
		return StructuredResponse{Success: false, Error: "swarm: empty task"}, nil
	}

	cacheKey := CacheKey(swarmCachePrefix, parentSessionID, task)
	if cached, ok := s.cache.Get(cacheKey); ok {
		if resp, ok := cached.(StructuredResponse); ok {
			return resp, nil
		}
	}

	subtasks, err := s.cfg.DecomposeFn(ctx, task)
	if err != nil {
		return StructuredResponse{}, fmt.Errorf("swarm decompose: %w", err)
	}
	if len(subtasks) == 0 {
		return StructuredResponse{
			Success: false,
			Error:   "swarm: decomposition produced no subtasks",
		}, nil
	}
	if len(subtasks) > s.cfg.maxSubagents() {
		subtasks = subtasks[:s.cfg.maxSubagents()]
	}

	var mu sync.Mutex
	results := make([]StructuredResponse, len(subtasks))

	for i, sub := range subtasks {
		i, sub := i, sub
		focusArea := fmt.Sprintf("swarm:%s:%d", parentSessionID, i)
		_, err := s.par.Submit(ctx, func(ctx context.Context) (any, error) {
			subagent, err := s.factory.NewStructuredSubagent(ctx, parentSessionID)
			if err != nil {
				return nil, fmt.Errorf("create subagent %d: %w", i, err)
			}

			resp, err := subagent.Execute(ctx, StructuredRequest{
				Task:     sub,
				Tools:    s.cfg.SubagentTools,
				MaxSteps: s.cfg.SubagentSteps,
				Timeout:  s.cfg.SubagentTimeout,
			})
			if err != nil {
				resp = StructuredResponse{Success: false, Error: err.Error()}
			}

			mu.Lock()
			results[i] = resp
			mu.Unlock()
			return resp, nil
		}, focusArea)
		if err != nil {
			mu.Lock()
			results[i] = StructuredResponse{Success: false, Error: err.Error()}
			mu.Unlock()
		}
	}

	// XRUSH: log error before discarding
	if _, err := s.par.WaitAll(ctx); err != nil {
		slog.Warn("Swarm: WaitAll returned errors", "error", err)
	}

	synthesized, err := s.cfg.SynthesizeFn(results)
	if err != nil {
		return StructuredResponse{}, fmt.Errorf("swarm synthesize: %w", err)
	}

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	resp := StructuredResponse{
		Result:     synthesized,
		Success:    successCount > 0,
		StepsTaken: len(results),
	}

	s.cache.Set(cacheKey, resp, s.cfg.cacheTTL())
	return resp, nil
}

// TeammateRole defines the role and capabilities of a persistent teammate.
type TeammateRole string

const (
	RoleResearcher TeammateRole = "researcher"
	RoleTester     TeammateRole = "tester"
	RoleReviewer   TeammateRole = "reviewer"
)

// TeammateConfig configures a TeammatePattern instance.
type TeammateConfig struct {
	Role         TeammateRole
	Tools        []string
	MaxSteps     int
	Timeout      time.Duration
	CacheTTL     time.Duration
	SystemPrompt string
}

func (c TeammateConfig) cacheTTL() time.Duration {
	if c.CacheTTL <= 0 {
		return DefaultDiagnosticsTTL
	}
	return c.CacheTTL
}

func (c TeammateConfig) systemPrompt() string {
	if c.SystemPrompt != "" {
		return c.SystemPrompt
	}
	switch c.Role {
	case RoleResearcher:
		return "You are a research specialist. Investigate thoroughly and report findings."
	case RoleTester:
		return "You are a testing specialist. Write and validate tests for the given code."
	case RoleReviewer:
		return "You are a code reviewer. Analyze code quality, correctness, and suggest improvements."
	default:
		return fmt.Sprintf("You are a %s specialist.", string(c.Role))
	}
}

// TeammatePattern maintains a persistent subagent with an assigned role that
// retains context across multiple interactions. It uses SharedCache to avoid
// re-computing results for identical queries.
type TeammatePattern struct {
	cfg            TeammateConfig
	cache          *SharedCache
	factory        StructuredSubagentFactory
	interactionMu  sync.Mutex
	interactionSeq int
}

func NewTeammatePattern(cfg TeammateConfig, cache *SharedCache, factory StructuredSubagentFactory) *TeammatePattern {
	return &TeammatePattern{
		cfg:     cfg,
		cache:   cache,
		factory: factory,
	}
}

// Execute sends a task to the teammate and returns its response. Results are
// cached so that identical queries within the TTL return the cached response.
// Sequential interactions for the same teammate are serialized.
func (t *TeammatePattern) Execute(ctx context.Context, parentSessionID, task string) (StructuredResponse, error) {
	if t.factory == nil {
		return StructuredResponse{}, fmt.Errorf("teammate: no structured subagent factory")
	}
	if task == "" {
		return StructuredResponse{Success: false, Error: "teammate: empty task"}, nil
	}

	cacheKey := CacheKey(teammateCachePrefix, string(t.cfg.Role), parentSessionID, task)
	if cached, ok := t.cache.Get(cacheKey); ok {
		if resp, ok := cached.(StructuredResponse); ok {
			return resp, nil
		}
	}

	t.interactionMu.Lock()
	defer t.interactionMu.Unlock()

	subagent, err := t.factory.NewStructuredSubagent(ctx, parentSessionID)
	if err != nil {
		return StructuredResponse{}, fmt.Errorf("teammate create: %w", err)
	}

	prompt := t.cfg.systemPrompt() + "\n\n" + task
	resp, err := subagent.Execute(ctx, StructuredRequest{
		Task:     prompt,
		Tools:    t.cfg.Tools,
		MaxSteps: t.cfg.MaxSteps,
		Timeout:  t.cfg.Timeout,
		Context:  map[string]string{"role": string(t.cfg.Role)},
	})
	if err != nil {
		return StructuredResponse{}, fmt.Errorf("teammate execute: %w", err)
	}

	t.interactionSeq++
	t.cache.Set(cacheKey, resp, t.cfg.cacheTTL())
	return resp, nil
}

// Role returns the teammate's assigned role.
func (t *TeammatePattern) Role() TeammateRole {
	return t.cfg.Role
}

// InteractionCount returns the number of interactions this teammate has
// performed in its lifetime.
func (t *TeammatePattern) InteractionCount() int {
	return t.interactionSeq
}

// DefaultDecompose splits a task by newlines, treating each non-empty line as
// a subtask. This is a simple default; callers should provide their own
// decomposition function for complex tasks.
func DefaultDecompose(_ context.Context, task string) ([]string, error) {
	lines := strings.Split(task, "\n")
	var subtasks []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			subtasks = append(subtasks, trimmed)
		}
	}
	return subtasks, nil
}

// DefaultSynthesize concatenates successful results with a separator.
func DefaultSynthesize(results []StructuredResponse) (string, error) {
	var parts []string
	for _, r := range results {
		if r.Success && r.Result != "" {
			parts = append(parts, r.Result)
		}
	}
	if len(parts) == 0 {
		return "no successful results", nil
	}
	return strings.Join(parts, "\n---\n"), nil
}
