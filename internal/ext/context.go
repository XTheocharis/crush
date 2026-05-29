package ext

import (
	"context"
	"database/sql"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/processor"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/session"
)

// TextCompleter is a function that sends a prompt+input pair to an LLM and
// returns the text response. Extensions use this for lightweight LLM calls
// (e.g., system prompt leak detection).
type TextCompleter func(ctx context.Context, prompt, input string) (string, error)

// HostContext is the narrow facade extensions use to access host services.
type HostContext interface {
	Config() *config.Config
	WorkingDir() string
	Completer() TextCompleter
	ToolDefs() []processor.ToolDef
	SkillDefs() []processor.SkillDef
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
	Sessions    session.Service
	Messages    message.Service
	LSP         *lsp.Manager
	DB          *sql.DB
	Config      *config.ConfigStore
	Events      *pubsub.Broker[tea.Msg]
	WorkingDir  string
	Completer   TextCompleter
	ToolDefsFn  func() []processor.ToolDef
	SkillDefsFn func() []processor.SkillDef
}
