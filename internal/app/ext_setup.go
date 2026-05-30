package app

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/extensions"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/processor"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/skills"
)

func setupExtensions(ctx context.Context, app *App, conn *sql.DB, q db.Querier, sessions session.Service, messages message.Service, store *config.ConfigStore) {
	completer := newTextCompleter(store)

	// [XRUSH: begin: create rewind service before bootstrap]
	// Create the rewind service before bootstrap so that the RewindExtension
	// receives the same instance via HostDeps instead of creating its own.
	rewindSvc := initRewindService(q, sessions, store)
	// [XRUSH: end]

	extHost := ext.NewExtensionHost(ext.HostDeps{
		Sessions:      sessions,
		Messages:      messages,
		LSP:           app.LSPManager,
		DB:            conn,
		Config:        store,
		Events:        app.events,
		WorkingDir:    store.WorkingDir(),
		Completer:     completer,
		ToolDefsFn:    newToolDefsProvider(store.WorkingDir()),
		SkillDefsFn:   newSkillDefsProvider(app.Skills),
		RewindService: rewindSvc,
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

	// [XRUSH: begin: wire LCM context window from model metadata]
	wireLCMContextWindow(store)
	// [XRUSH: end]

	// [XRUSH: begin: wire LCM cutoff threshold from config]
	wireLCMCutoffThreshold(store)
	// [XRUSH: end]

	// [XRUSH: begin: wire LCM large output threshold from config]
	wireLCMLargeOutputThreshold(store)
	// [XRUSH: end]

	// [XRUSH: begin: wire nudge config from LCM options]
	wireNudgeConfig(store)
	// [XRUSH: end]

	// [XRUSH: begin: wire LCM operational memory from config]
	wireLCMOperationalMemory(conn, store)
	// [XRUSH: end]

	// [XRUSH: begin: wire compact hook runners]
	wireCompactHookRunners(store)
	// [XRUSH: end]

	// [XRUSH: begin: wire orchestration tools]
	wireOrchestration()
	// [XRUSH: end]

	// [XRUSH: begin: wire swarm registry + mailbox]
	wireSwarm()
	// [XRUSH: end]

	// [XRUSH: begin: wire prompt-assembly LCM]
	wirePromptAssembly(extHost)
	// [XRUSH: end]

	// [XRUSH: begin: wire repo map prompt injection]
	wireRepoMapPromptInjection(extHost)
	// [XRUSH: end]

	// [XRUSH: begin: rewind service assignment]
	app.RewindService = rewindSvc
	// [XRUSH: end]

	// [XRUSH: begin: wire message decorator]
	wireMessageDecorator(app, q, conn, store)
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

func newToolDefsProvider(workingDir string) func() []processor.ToolDef {
	return func() []processor.ToolDef {
		toolsDir := filepath.Join(workingDir, "internal", "agent", "tools")
		return extensions.LoadToolDefsFromMD(toolsDir)
	}
}

func newSkillDefsProvider(m *skills.Manager) func() []processor.SkillDef {
	return func() []processor.SkillDef {
		return extensions.SkillDefsFromManager(m)
	}
}
