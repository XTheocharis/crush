package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/ext"
)

// SwarmExtension wraps the swarm parallel subagent pattern as a ToolProvider.
type SwarmExtension struct {
	mu     sync.RWMutex
	host   ext.HostContext
	active bool
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
	e.active = false
	return nil
}

func (e *SwarmExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	return nil, nil
}

func (e *SwarmExtension) ToolNames() []string {
	return nil
}

// NewSwarm creates a swarm pattern instance for parallel task decomposition.
func (e *SwarmExtension) NewSwarm(cfg agent.SwarmConfig) *agent.SwarmPattern {
	cache := agent.NewSharedCache()
	par := agent.NewParallelController(agent.ParallelControllerConfig{})
	return agent.NewSwarmPattern(cfg, cache, par, nil)
}

var _ ext.Extension = (*SwarmExtension)(nil)
