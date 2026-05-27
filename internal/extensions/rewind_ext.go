package extensions

import (
	"context"
	"database/sql"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/rewind"
)

// RewindExtension wraps the rewind service as a ToolProvider and StepHookProvider.
// As a ToolProvider it provides the synthetic output tool. As a StepHookProvider
// it captures file snapshots after each agent step for undo/rewind support.
type RewindExtension struct {
	mu      sync.RWMutex
	host    ext.HostContext
	service rewind.Service
	synTool fantasy.AgentTool
	active  bool
}

func (e *RewindExtension) Name() string { return "rewind" }

func (e *RewindExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	var sqlDB *sql.DB
	if dbFn := host.DB(); dbFn != nil {
		sqlDB = dbFn
	}

	if sqlDB != nil && host.Sessions() != nil {
		q := db.New(sqlDB)
		e.service = rewind.NewService(q, host.Sessions(), host.WorkingDir())
	}

	e.synTool = tools.NewSyntheticOutputTool()
	e.active = true
	return nil
}

func (e *RewindExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.service = nil
	e.synTool = nil
	e.active = false
	return nil
}

func (e *RewindExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active || e.synTool == nil {
		return nil, nil
	}
	return []fantasy.AgentTool{e.synTool}, nil
}

func (e *RewindExtension) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return []string{tools.SyntheticOutputToolName}
}

func (e *RewindExtension) StepHooks() []ext.StepHook {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active || e.service == nil {
		return nil
	}
	return []ext.StepHook{
		{
			Name: "rewind-snapshot",
			OnStepFinish: func(ctx context.Context, sessionID string, _ fantasy.StepResult) error {
				return e.service.CaptureSnapshot(ctx, sessionID, 0)
			},
		},
	}
}

var (
	_ ext.Extension        = (*RewindExtension)(nil)
	_ ext.ToolProvider     = (*RewindExtension)(nil)
	_ ext.StepHookProvider = (*RewindExtension)(nil)
)
