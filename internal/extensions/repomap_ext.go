package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/ext"
)

// RepomapExtension wraps the repository map subsystem as a ToolProvider and
// RunHookProvider.
type RepomapExtension struct {
	mu           sync.RWMutex
	host         ext.HostContext
	tools        []fantasy.AgentTool
	names        []string
	active       bool
	asyncRefresh func(ctx context.Context, sessionID string) error
}

func (e *RepomapExtension) Name() string { return "repomap" }

func (e *RepomapExtension) Init(ctx context.Context, host ext.HostContext) error {
	e.host = host

	e.tools = e.buildRepomapTools(ctx, host)
	e.names = make([]string, len(e.tools))
	for i, t := range e.tools {
		e.names[i] = t.Info().Name
	}
	e.active = true
	return nil
}

func (e *RepomapExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tools = nil
	e.names = nil
	e.active = false
	return nil
}

func (e *RepomapExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	return append([]fantasy.AgentTool{}, e.tools...), nil
}

func (e *RepomapExtension) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return append([]string{}, e.names...)
}

// RunHooks returns lifecycle hooks for the repomap extension. When the
// repomap service is available (treesitter build), OnRunStart triggers an
// asynchronous refresh so the map is up-to-date for each agent run. When the
// service is nil (non-treesitter build), hooks are no-ops.
func (e *RepomapExtension) RunHooks() []ext.RunHook {
	if !e.active {
		return nil
	}
	return []ext.RunHook{
		{
			Name: "repomap-refresh-trigger",
			OnRunStart: func(ctx context.Context, sessionID string, _ string) error {
				e.mu.RLock()
				defer e.mu.RUnlock()
				if !e.active {
					return nil
				}
				return e.triggerRefresh(ctx, sessionID)
			},
			OnRunEnd: func(_ context.Context, _ string, _ *fantasy.AgentResult, _ error) error {
				// No-op: refresh is triggered at run start so the map is
				// ready before the agent processes the prompt. There is no
				// post-run action needed.
				return nil
			},
		},
	}
}

var (
	_ ext.Extension       = (*RepomapExtension)(nil)
	_ ext.ToolProvider    = (*RepomapExtension)(nil)
	_ ext.RunHookProvider = (*RepomapExtension)(nil)
)
