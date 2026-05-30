package extensions

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/lcm"
)

type LCMExtension struct {
	mu      sync.RWMutex
	host    ext.HostContext
	tools   []fantasy.AgentTool
	names   []string
	manager lcm.Manager
	active  bool
}

// [XRUSH: begin: wire compaction event to pill]
// TheLCMExtension is the singleton LCM extension instance registered at init.
var TheLCMExtension = &LCMExtension{}

// [XRUSH: end]

func (e *LCMExtension) Name() string { return "lcm" }

func (e *LCMExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host
	if host.DB() == nil {
		e.active = false
		return nil
	}

	// Factory tools: lcm_grep, lcm_describe, lcm_expand.
	factoryTools := buildLCMTools(host.DB())

	// Manager tools: 9 store-based retrieval tools (bindle, ancestry, dolt,
	// archive, sprig, time_query, file_search, active_context, lineage).
	e.manager = lcm.NewManager(db.New(host.DB()), host.DB())
	managerTools := lcm.ExtraAgentTools(e.manager)

	e.tools = make([]fantasy.AgentTool, 0, len(factoryTools)+len(managerTools))
	e.tools = append(e.tools, factoryTools...)
	e.tools = append(e.tools, managerTools...)

	e.names = make([]string, len(e.tools))
	for i, t := range e.tools {
		e.names[i] = t.Info().Name
	}
	e.active = true
	return nil
}

func (e *LCMExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tools = nil
	e.names = nil
	e.manager = nil
	e.active = false
	return nil
}

func (e *LCMExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	return append([]fantasy.AgentTool{}, e.tools...), nil
}

func (e *LCMExtension) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return append([]string{}, e.names...)
}

// SetLLMClient injects an LLM client for summarization. The adapter must
// implement the Complete(ctx, systemPrompt, userPrompt) (string, error)
// signature. Pass nil to disable summarization.
func (e *LCMExtension) SetLLMClient(client lcm.LLMClient) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.manager != nil && client != nil {
		e.manager.SetLLMClient(client)
	}
}

// SetAgentConfigRestorer connects the agent's config restorer so that
// checkpointed session agent configuration (skills, tools, agents) is
// restored after compaction.
func (e *LCMExtension) SetAgentConfigRestorer(restorer lcm.AgentConfigRestorer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.manager != nil {
		e.manager.SetAgentConfigRestorer(restorer)
	}
}

// Manager returns the LCM Manager for external callers (e.g. coordinator).
func (e *LCMExtension) Manager() lcm.Manager {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.manager
}

func (e *LCMExtension) StepHooks() []ext.StepHook {
	if !e.active {
		return nil
	}
	return []ext.StepHook{
		{
			Name: "lcm-compaction-trigger",
			OnStepFinish: func(ctx context.Context, sessionID string, step fantasy.StepResult) error {
				e.mu.RLock()
				mgr := e.manager
				e.mu.RUnlock()
				if mgr == nil {
					return nil
				}
				promptTokens := step.Usage.InputTokens + step.Usage.CacheReadTokens
				if promptTokens > 0 {
					mgr.SetActualPromptTokens(sessionID, promptTokens)
				}
				mgr.CompactIfOverHardLimit(ctx, sessionID)
				return nil
			},
		},
		{
			Name: "lcm-overhead-tracking",
			OnPrepareStep: func(ctx context.Context, sessionID string, messages []fantasy.Message) ([]fantasy.Message, error) {
				e.mu.RLock()
				mgr := e.manager
				e.mu.RUnlock()
				if mgr == nil {
					return messages, nil
				}
				if err := mgr.InitSession(ctx, sessionID); err != nil {
					slog.Debug("LCM InitSession failed", "session_id", sessionID, "error", err)
				}
				return messages, nil
			},
		},
	}
}

func (e *LCMExtension) RunHooks() []ext.RunHook {
	if !e.active {
		return nil
	}
	return []ext.RunHook{
		{
			Name: "lcm-session-lifecycle",
			OnRunStart: func(ctx context.Context, sessionID string, _ string) error {
				e.mu.RLock()
				mgr := e.manager
				e.mu.RUnlock()
				if mgr == nil {
					return nil
				}
				if err := mgr.InitSession(ctx, sessionID); err != nil {
					slog.Debug("LCM InitSession on run start", "session_id", sessionID, "error", err)
				}
				return nil
			},
			OnRunEnd: func(ctx context.Context, sessionID string, _ *fantasy.AgentResult, _ error) error {
				e.mu.RLock()
				mgr := e.manager
				e.mu.RUnlock()
				if mgr == nil {
					return nil
				}
				mgr.PostTurnHook(ctx, sessionID)
				return nil
			},
		},
	}
}

func buildLCMTools(dbConn *sql.DB) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		tools.NewLcmGrepTool(dbConn),
		tools.NewLcmDescribeTool(dbConn),
		tools.NewLcmExpandTool(dbConn),
	}
}

var (
	_ ext.Extension        = (*LCMExtension)(nil)
	_ ext.ToolProvider     = (*LCMExtension)(nil)
	_ ext.StepHookProvider = (*LCMExtension)(nil)
	_ ext.RunHookProvider  = (*LCMExtension)(nil)
)
