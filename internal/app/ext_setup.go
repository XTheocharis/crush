package app

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/extensions"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
)

func setupExtensions(ctx context.Context, app *App, conn *sql.DB, q db.Querier, sessions session.Service, messages message.Service, store *config.ConfigStore) {
	completer := newTextCompleter(store)
	extHost := ext.NewExtensionHost(ext.HostDeps{
		Sessions:   sessions,
		Messages:   messages,
		LSP:        app.LSPManager,
		DB:         conn,
		Config:     store,
		Events:     app.events,
		WorkingDir: store.WorkingDir(),
		Completer:  completer,
	})
	if err := extHost.Bootstrap(ctx); err != nil {
		slog.Warn("Extension host bootstrap failed", "error", err)
	} else {
		config.RegisterExtensionToolNames(extHost.ContributedToolNames)
		store.SetupAgents()
	}
	app.ExtHost = extHost

	// [XRUSH: begin: wire LCM LLM client]
	wireLCMLLMClient(store)
	// [XRUSH: end]

	// [XRUSH: begin: wire compact hook runners]
	wireCompactHookRunners(store)
	// [XRUSH: end]

	// [XRUSH: begin: wire orchestration tools]
	wireOrchestration()
	// [XRUSH: end]

	// [XRUSH: begin: wire prompt-assembly LCM]
	wirePromptAssembly(extHost)
	// [XRUSH: end]

	// [XRUSH: begin: rewind service initialization]
	app.RewindService = initRewindService(q, sessions, store)
	// [XRUSH: end]

	// [XRUSH: begin: wire compaction event to pill]
	if mgr := extensions.TheLCMExtension.Manager(); mgr != nil {
		setupSubscriber(app.eventsCtx, app.serviceEventsWG, "lcm-compaction", mgr.Subscribe, app.events)
	}
	// [XRUSH: end]

	app.cleanupFuncs = append(app.cleanupFuncs, func(_ context.Context) error {
		if app.ExtHost != nil {
			return app.ExtHost.Shutdown(context.Background())
		}
		return nil
	})
}
