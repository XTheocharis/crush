package extensions

import (
	"context"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/ext"
)

// TheRepomapExtension is the singleton repomap extension instance registered
// at init.
var TheRepomapExtension = &RepomapExtension{}

// RepomapExtension wraps the repository map subsystem as a ToolProvider and
// RunHookProvider.
type RepomapExtension struct {
	mu              sync.RWMutex
	host            ext.HostContext
	tools           []fantasy.AgentTool
	names           []string
	active          bool
	asyncRefresh    func(ctx context.Context, sessionID string) error
	loadCachedMap   func(sessionID string) (string, int)
	shouldInjectMap func(ctx context.Context, sessionID string) bool
	fileScores      func(ctx context.Context, sessionID string) map[string]float64
	closeSvc        func()
}

func (e *RepomapExtension) Name() string { return "repomap" }

func (e *RepomapExtension) isActive() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.active
}

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
	if e.closeSvc != nil {
		e.closeSvc()
		e.closeSvc = nil
	}
	e.tools = nil
	e.names = nil
	e.active = false
	e.loadCachedMap = nil
	e.shouldInjectMap = nil
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

// LoadCachedMap returns the cached repo map for the given session. Returns
// empty string and 0 when the service is unavailable or no map is cached.
func (e *RepomapExtension) LoadCachedMap(sessionID string) (string, int) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.loadCachedMap == nil {
		return "", 0
	}
	return e.loadCachedMap(sessionID)
}

// ShouldInjectMap reports whether the repo map should be injected for this
// session. It uses the repomap.Service.ShouldInject mechanism when a
// RunInjectionKey is available in context. Returns false when the service is
// unavailable or no run key is present.
func (e *RepomapExtension) ShouldInjectMap(ctx context.Context, sessionID string) bool {
	e.mu.RLock()
	fn := e.shouldInjectMap
	e.mu.RUnlock()
	if fn == nil {
		return false
	}
	return fn(ctx, sessionID)
}

// FileScores returns PageRank-based file scores for the given session.
func (e *RepomapExtension) FileScores(ctx context.Context, sessionID string) map[string]float64 {
	e.mu.RLock()
	fn := e.fileScores
	e.mu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn(ctx, sessionID)
}

var (
	_ ext.Extension       = (*RepomapExtension)(nil)
	_ ext.ToolProvider    = (*RepomapExtension)(nil)
	_ ext.RunHookProvider = (*RepomapExtension)(nil)
)
