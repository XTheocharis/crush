package extensions

import (
	"context"
	"log/slog"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
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
	e.service = host.RewindService()
	e.synTool = tools.NewSyntheticOutputTool()
	e.active = true
	slog.Info("RewindExt: Init completed",
		"service_nil", e.service == nil,
		"host_messages_nil", host.Messages() == nil,
	)
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
		slog.Warn("RewindExt: StepHooks returning nil",
			"active", e.active,
			"service_nil", e.service == nil,
		)
		return nil
	}
	slog.Info("RewindExt: StepHooks returning rewind-snapshot hook")
	return []ext.StepHook{
		{
			Name: "rewind-snapshot",
			OnStepFinish: func(ctx context.Context, sessionID string, _ fantasy.StepResult) error {
				slog.Info("RewindExt: OnStepFinish hook fired",
					"session_id", sessionID,
				)
				seq := e.latestUserSeq(ctx, sessionID)
				slog.Info("RewindExt: latestUserSeq result",
					"session_id", sessionID,
					"seq", seq,
				)
				svc := e.service
				if svc == nil {
					slog.Warn("RewindExt: service is nil, skipping snapshot")
					return nil
				}
				// Use a detached context so the snapshot and cleanup persist even
				// when the parent request context is cancelled (e.g. user abort).
				detachedCtx := context.WithoutCancel(ctx)
				go func() {
					slog.Info("RewindExt: CaptureSnapshot goroutine started",
						"session_id", sessionID,
						"seq", seq,
					)
					if err := svc.CaptureSnapshot(detachedCtx, sessionID, seq); err != nil {
						slog.Error("RewindExt: CaptureSnapshot failed",
							"session_id", sessionID,
							"seq", seq,
							"error", err,
						)
						return
					}
					slog.Info("RewindExt: CaptureSnapshot succeeded",
						"session_id", sessionID,
						"seq", seq,
					)
					_ = svc.CleanupOldSnapshots(detachedCtx, sessionID)
				}()
				return nil
			},
		},
	}
}

func (e *RewindExtension) latestUserSeq(ctx context.Context, sessionID string) int {
	msgSvc := e.host.Messages()
	if msgSvc == nil {
		slog.Warn("RewindExt: latestUserSeq: Messages() returned nil")
		return 0
	}
	msgs, err := msgSvc.ListUserMessages(ctx, sessionID)
	if err != nil {
		slog.Warn("RewindExt: latestUserSeq: ListUserMessages failed",
			"session_id", sessionID,
			"error", err,
		)
		return 0
	}
	slog.Info("RewindExt: latestUserSeq",
		"session_id", sessionID,
		"user_message_count", len(msgs),
		"first_seq", func() int {
			if len(msgs) > 0 {
				return msgs[0].Seq
			}
			return 0
		}(),
	)
	if len(msgs) == 0 {
		return 0
	}
	return msgs[0].Seq
}

var (
	_ ext.Extension        = (*RewindExtension)(nil)
	_ ext.ToolProvider     = (*RewindExtension)(nil)
	_ ext.StepHookProvider = (*RewindExtension)(nil)
)
