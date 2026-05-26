package ext

import (
	"context"
	"database/sql"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/session"
)

// HostContext is the narrow facade extensions use to access host services.
type HostContext interface {
	Config() *config.Config
	WorkingDir() string
	RegisterTools(provider ToolProvider)
	RegisterRunHooks(provider RunHookProvider)
	RegisterStepHooks(provider StepHookProvider)
	RegisterPromptHook(provider PromptHookProvider)
	PublishEvent(ctx context.Context, eventType string, payload any) error
	LSP() *lsp.Manager
	DB() *sql.DB
	Sessions() session.Service
	Messages() message.Service
}

// HostDeps carries concrete service references from app.New().
type HostDeps struct {
	Sessions   session.Service
	Messages   message.Service
	LSP        *lsp.Manager
	DB         *sql.DB
	Config     *config.ConfigStore
	Events     *pubsub.Broker[tea.Msg]
	WorkingDir string
}
