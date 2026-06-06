// [XRUSH: begin: rewind service and agent config restoration]
package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/extensions"
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/charmbracelet/crush/internal/lcm"
	"github.com/charmbracelet/crush/internal/lcm/explorer"
	"github.com/charmbracelet/crush/internal/lcm/nudge"
	"github.com/charmbracelet/crush/internal/rewind"
	"github.com/charmbracelet/crush/internal/session"
)

// [XRUSH: begin: initRewindService]
// initRewindService creates the rewind service from the database and config.
// The snapshot config's MaxPerSession controls retention limit; the service is always active.
func initRewindService(q db.Querier, sessions session.Service, store *config.ConfigStore) rewind.Service {
	cfg := store.Config()
	var opts []rewind.SnapshotterOption
	if cfg.Options.Snapshot != nil && cfg.Options.Snapshot.MaxPerSession > 0 {
		opts = append(opts, rewind.WithMaxPerSession(cfg.Options.Snapshot.MaxPerSession))
	}
	return rewind.NewService(q, sessions, store.WorkingDir(), opts...)
}

// [XRUSH: end]

// [XRUSH: begin: wireAgentConfigRestorer]
// WireAgentConfigRestorer connects the agent config restorer to the LCM
// manager for post-compaction skill restoration.
func wireAgentConfigRestorer(coord agent.Coordinator) {
	if mgr := extensions.TheLCMExtension.Manager(); mgr != nil {
		mgr.SetAgentConfigRestorer(coord)
	}
}

// [XRUSH: end]

// [XRUSH: begin: wireLCMLLMClient]
// wireLCMLLMClient resolves the LLM model for LCM summarization and wires the
// adapter into the LCM manager. It prefers Options.LCM.SummarizerModel and
// falls back to the configured large model. When neither is available it
// installs a fallback adapter so the manager is never left without a
// compressor (which would block compaction entirely).
func wireLCMLLMClient(ctx context.Context, store *config.ConfigStore, coord agent.Coordinator) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		slog.Warn("LCM manager is nil, skipping LLM client wiring")
		return
	}

	cfg := store.Config()

	var selected *config.SelectedModel
	if cfg.Options != nil && cfg.Options.LCM != nil && cfg.Options.LCM.SummarizerModel != nil {
		selected = cfg.Options.LCM.SummarizerModel
	}
	if selected == nil {
		if sm, ok := cfg.Models[config.SelectedModelTypeLarge]; ok {
			selected = &sm
		}
	}

	if selected == nil {
		slog.Warn("No LCM summarizer model configured, using fallback adapter")
		mgr.SetLLMClient(fallbackLCMClient{})
		slog.Info("LCM LLM client wired (fallback)")
		return
	}

	providerCfg, ok := cfg.Providers.Get(selected.Provider)
	if !ok {
		slog.Warn("LCM summarizer model provider not found, using fallback adapter", "provider", selected.Provider)
		mgr.SetLLMClient(fallbackLCMClient{})
		slog.Info("LCM LLM client wired (fallback)")
		return
	}

	model, err := resolveLCMModel(ctx, *selected, providerCfg, coord)
	if err != nil {
		slog.Warn("Failed to resolve LCM summarizer model, using fallback adapter", "error", err)
		mgr.SetLLMClient(fallbackLCMClient{})
		slog.Info("LCM LLM client wired (fallback)")
		return
	}

	adapter := agent.NewLCMLLMClient(model, providerCfg)
	mgr.SetLLMClient(adapter)
	slog.Info("LCM LLM client wired successfully",
		"provider", selected.Provider,
		"model", selected.Model,
	)
}

// resolveLCMModel builds an agent.Model from the config for LCM summarization.
func resolveLCMModel(ctx context.Context, selected config.SelectedModel, providerCfg config.ProviderConfig, coord agent.Coordinator) (agent.Model, error) {
	return coord.ResolveLCMModel(ctx, selected, providerCfg)
}

// fallbackLCMClient is a minimal LLM client used when no real model is
// available. It satisfies lcm.LLMClient so that the LCM manager always has a
// compressor wired, preventing ErrNoCompressor from blocking compaction.
type fallbackLCMClient struct{}

func (fallbackLCMClient) Complete(_ context.Context, _, _ string) (string, error) {
	return "", fmt.Errorf("LCM: no summarizer model configured")
}

// lcmFallbackLM is a minimal fantasy.LanguageModel used to construct an
// agent.Model when we need metadata (catwalk config, selected model) but
// don't have a real provider-backed language model. It satisfies the
// LanguageModel interface so NewLCMLLMClient can wrap it.
type lcmFallbackLM struct{}

func (*lcmFallbackLM) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return &fantasy.Response{}, nil
}

func (*lcmFallbackLM) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, nil
}

func (*lcmFallbackLM) GenerateObject(_ context.Context, _ fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, nil
}

func (*lcmFallbackLM) StreamObject(_ context.Context, _ fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, nil
}

func (*lcmFallbackLM) Provider() string { return "lcm-fallback" }

func (*lcmFallbackLM) Model() string { return "lcm-fallback" }

// [XRUSH: begin: wireLCMContextWindow]
// wireLCMContextWindow reads the large model's context window from the config
// and propagates it to the LCM manager so that budget calculations use the
// actual model limits instead of the hardcoded 128000 default. When the model
// reports 0 or is not available, the default is kept.
func wireLCMContextWindow(store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	cfg := store.Config()

	// Prefer LCM summarizer model, fall back to the large model — same
	// resolution logic as wireLCMLLMClient.
	var catModel *catwalk.Model
	if cfg.Options != nil && cfg.Options.LCM != nil && cfg.Options.LCM.SummarizerModel != nil {
		catModel = cfg.GetModel(cfg.Options.LCM.SummarizerModel.Provider, cfg.Options.LCM.SummarizerModel.Model)
	}
	if catModel == nil {
		catModel = cfg.LargeModel()
	}

	if catModel == nil || catModel.ContextWindow <= 0 {
		// Keep the 128000 default.
		slog.Info("LCM context window kept at default (model unavailable or reports 0)")
		return
	}

	mgr.SetDefaultContextWindow(catModel.ContextWindow)
	slog.Info("LCM context window set from model metadata",
		"context_window", catModel.ContextWindow,
	)
}

// [XRUSH: end]

// [XRUSH: begin: wireLCMCutoffThreshold]
// wireLCMCutoffThreshold reads the ctx_cutoff_threshold from the LCM config
// and propagates it to the LCM manager so that budget calculations use the
// configured value instead of the hardcoded 0.6 default.
func wireLCMCutoffThreshold(store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	cfg := store.Config()
	if cfg.Options == nil || cfg.Options.LCM == nil {
		return
	}
	if cfg.Options.LCM.CtxCutoffThreshold <= 0 {
		return
	}

	mgr.SetCutoffThreshold(cfg.Options.LCM.CtxCutoffThreshold)
	slog.Info("LCM cutoff threshold set from config",
		"cutoff_threshold", cfg.Options.LCM.CtxCutoffThreshold,
	)
}

// [XRUSH: end]

// [XRUSH: begin: wireLCMLargeOutputThreshold]
// wireLCMLargeOutputThreshold reads large_tool_output_token_threshold from the
// LCM config and propagates it to the LCM manager. When zero or absent the
// hardcoded default (50000) is used.
func wireLCMLargeOutputThreshold(store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	cfg := store.Config()
	if cfg.Options == nil || cfg.Options.LCM == nil {
		return
	}
	if cfg.Options.LCM.LargeToolOutputTokenThreshold <= 0 {
		return
	}

	mgr.SetLargeOutputThreshold(int64(cfg.Options.LCM.LargeToolOutputTokenThreshold))
	slog.Info("LCM large output threshold set from config",
		"large_output_threshold", cfg.Options.LCM.LargeToolOutputTokenThreshold,
	)
}

// [XRUSH: end]

// [XRUSH: begin: wireLCMSessionBudget]
func wireLCMSessionBudget(store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	cfg := store.Config()
	if cfg.Options == nil || cfg.Options.LCM == nil {
		return
	}
	if cfg.Options.LCM.SessionBudget <= 0 {
		return
	}

	mgr.SetSessionBudget(cfg.Options.LCM.SessionBudget)
	slog.Info("LCM session budget set from config",
		"session_budget", cfg.Options.LCM.SessionBudget,
	)
}

// [XRUSH: end]

// [XRUSH: begin: wireLCMPostCompactConfig]
func wireLCMPostCompactConfig(store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	cfg := store.Config()
	if cfg.Options == nil || cfg.Options.LCM == nil {
		return
	}
	if cfg.Options.LCM.PostCompactMaxFiles == 0 && cfg.Options.LCM.PostCompactTokenBudget == 0 {
		return
	}

	mgr.SetPostCompactConfig(cfg.Options.LCM.PostCompactMaxFiles, cfg.Options.LCM.PostCompactTokenBudget)
	slog.Info("LCM post-compact config set from config",
		"max_files", cfg.Options.LCM.PostCompactMaxFiles,
		"token_budget", cfg.Options.LCM.PostCompactTokenBudget,
	)
}

// [XRUSH: end]

// [XRUSH: begin: wireNudgeConfig]
// wireNudgeConfig reads nudge options from the LCM config and creates a
// NudgeInjector wired into the LCM manager. When nudge config is nil, defaults
// are used. The pressure-tier function is bridged from lcm.CalculatePressureTier.
func wireNudgeConfig(store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	cfg := store.Config()

	var nudgeCfg nudge.NudgeConfig
	if cfg.Options != nil && cfg.Options.LCM != nil && cfg.Options.LCM.Nudge != nil {
		opts := cfg.Options.LCM.Nudge
		nudgeCfg = nudge.DefaultNudgeConfig()
		if opts.MinContextLimit > 0 {
			nudgeCfg.MinContextLimit = opts.MinContextLimit
		}
		if opts.MaxContextLimit > 0 {
			nudgeCfg.MaxContextLimit = opts.MaxContextLimit
		}
		if opts.NudgeFrequency > 0 {
			nudgeCfg.NudgeFrequency = opts.NudgeFrequency
		}
		if opts.NudgeForce != "" {
			nudgeCfg.NudgeForce = opts.NudgeForce
		}
	} else {
		nudgeCfg = nudge.DefaultNudgeConfig()
	}

	tierFn := func(currentTokens, contextWindow int64) nudge.PressureTier {
		_, tier := lcm.CalculatePressureTier(currentTokens, contextWindow, lcm.DefaultPressureConfig())
		switch tier {
		case lcm.PressureHigh:
			return nudge.PressureHigh
		case lcm.PressureMedium:
			return nudge.PressureMedium
		default:
			return nudge.PressureLow
		}
	}

	injector := nudge.NewNudgeInjector(&nudgeCfg, tierFn)
	mgr.SetNudgeInjector(injector)
	slog.Info("LCM nudge injector wired",
		"min_context_limit", nudgeCfg.MinContextLimit,
		"max_context_limit", nudgeCfg.MaxContextLimit,
		"frequency", nudgeCfg.NudgeFrequency,
		"force", nudgeCfg.NudgeForce,
	)
}

// [XRUSH: end]

// [XRUSH: begin: wireLCMOperationalMemory]
// wireLCMOperationalMemory reads the operational_memory_enabled flag from the
// LCM config. When enabled it creates a session.OperationalMemory backed by the
// database and wires it into the LCM manager, then sets the enabled flag. When
// disabled (the default) no store is wired and lifecycle hooks are no-ops.
func wireLCMOperationalMemory(conn *sql.DB, store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	cfg := store.Config()
	enabled := cfg.Options != nil && cfg.Options.LCM != nil && cfg.Options.LCM.OperationalMemoryEnabled
	if !enabled {
		return
	}

	om := session.NewOperationalMemory(conn)
	mgr.SetOperationalMemory(om)
	mgr.SetOperationalMemoryEnabled(true)
	slog.Info("LCM operational memory wired and enabled")
}

// [XRUSH: end]

// [XRUSH: begin: wireLCMObservationConfig]
// wireLCMObservationConfig reads observation options from the LCM config and
// wires them into the LCM manager's observation coordinator. When observation
// config is nil the default strategy and threshold are used (backward
// compatible).
func wireLCMObservationConfig(store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	cfg := store.Config()
	if cfg.Options == nil || cfg.Options.LCM == nil || cfg.Options.LCM.Observation == nil {
		return
	}

	obs := cfg.Options.LCM.Observation
	mgr.SetObservationConfig(
		obs.Strategy,
		int64(obs.ObserverMessageTokens),
		obs.ObserverModel,
		obs.ReflectorModel,
		obs.ObserverBufferRatio,
		int64(obs.ReflectorObservationTokens),
		obs.ReflectorBufferActivation,
	)
	slog.Info("LCM observation config wired",
		"strategy", obs.Strategy,
		"threshold", obs.ObserverMessageTokens,
		"observer_model", obs.ObserverModel,
		"reflector_model", obs.ReflectorModel,
		"observer_buffer_ratio", obs.ObserverBufferRatio,
		"reflector_observation_tokens", obs.ReflectorObservationTokens,
		"reflector_buffer_activation", obs.ReflectorBufferActivation,
	)
}

// [XRUSH: end]

// [XRUSH: begin: wireLCMDedupConfig]
// wireLCMDedupConfig reads deduplication and purge-errors flags from the LCM
// config and wires them into the LCM manager. When either flag is nil (not
// explicitly set), the default is true (enabled).
func wireLCMDedupConfig(store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	cfg := store.Config()
	if cfg.Options == nil || cfg.Options.LCM == nil {
		// Both default to true when no LCM config is present.
		mgr.SetDeduplicationEnabled(true)
		mgr.SetPurgeErrorsEnabled(true)
		return
	}

	dedup := true
	if cfg.Options.LCM.DeduplicationEnabled != nil {
		dedup = *cfg.Options.LCM.DeduplicationEnabled
	}
	purge := true
	if cfg.Options.LCM.PurgeErrorsEnabled != nil {
		purge = *cfg.Options.LCM.PurgeErrorsEnabled
	}

	mgr.SetDeduplicationEnabled(dedup)
	mgr.SetPurgeErrorsEnabled(purge)
	slog.Info("LCM dedup config wired",
		"deduplication_enabled", dedup,
		"purge_errors_enabled", purge,
	)
}

// [XRUSH: end]

// [XRUSH: begin: wireCompactHookRunners]
// wireCompactHookRunners creates hooks.Runner instances for PreCompact and
// PostCompact events from the config and wires them into the LCM manager.
// When the manager is nil or no hook configs exist, the function logs and
// returns without error.
func wireCompactHookRunners(store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		slog.Warn("LCM manager is nil, skipping compact hook runner wiring")
		return
	}

	cfg := store.Config()
	cwd := store.WorkingDir()

	var preRunner, postRunner *hooks.Runner
	if cfg.Hooks != nil {
		if preHooks, ok := cfg.Hooks[hooks.EventPreCompact]; ok && len(preHooks) > 0 {
			preRunner = hooks.NewRunner(preHooks, cwd, cwd)
		}
		if postHooks, ok := cfg.Hooks[hooks.EventPostCompact]; ok && len(postHooks) > 0 {
			postRunner = hooks.NewRunner(postHooks, cwd, cwd)
		}
	}

	mgr.SetHookRunners(preRunner, postRunner)
	slog.Info("LCM compact hook runners wired",
		"pre_compact", preRunner != nil,
		"post_compact", postRunner != nil,
	)
}

// [XRUSH: end]

// [XRUSH: begin: wireOrchestration]
// wireOrchestration connects the XrushExtension's registry and mailbox to
// the OrchestrationExtension, then rebuilds orchestration tools with the
// non-nil dependencies.
func wireOrchestration() {
	xrush := extensions.TheXrushExtension
	if xrush == nil {
		slog.Warn("XrushExtension singleton is nil, skipping orchestration wiring")
		return
	}

	registry := xrush.Registry()
	if registry == nil {
		slog.Warn("XrushExtension registry is nil, skipping orchestration wiring")
		return
	}

	mailbox := xrush.Mailbox()
	if mailbox == nil {
		slog.Warn("XrushExtension mailbox is nil, skipping orchestration wiring")
		return
	}

	orch := extensions.TheOrchestrationExtension
	if orch == nil {
		slog.Warn("OrchestrationExtension singleton is nil, skipping orchestration wiring")
		return
	}

	orch.SetRegistry(registry)
	orch.SetMailbox(mailbox)
	orch.SetTeamManager(registry)
	orch.RebuildTools()
	slog.Info("Orchestration tools wired with registry and mailbox")
}

// [XRUSH: begin: wireSwarm]
// wireSwarm connects the XrushExtension's registry and mailbox to the
// SwarmExtension for future teammate tool support.
func wireSwarm() {
	xrush := extensions.TheXrushExtension
	if xrush == nil {
		slog.Warn("XrushExtension singleton is nil, skipping swarm wiring")
		return
	}

	registry := xrush.Registry()
	if registry == nil {
		slog.Warn("XrushExtension registry is nil, skipping swarm wiring")
		return
	}

	mailbox := xrush.Mailbox()
	if mailbox == nil {
		slog.Warn("XrushExtension mailbox is nil, skipping swarm wiring")
		return
	}

	swarm := extensions.TheSwarmExtension
	if swarm == nil {
		slog.Warn("SwarmExtension singleton is nil, skipping swarm wiring")
		return
	}

	swarm.SetRegistry(registry)
	swarm.SetMailbox(mailbox)
	slog.Info("Swarm registry and mailbox wired")
}

// [XRUSH: end]

// [XRUSH: begin: wireSwarmFactory]
// wireSwarmFactory extracts the StructuredSubagentFactory from the coordinator
// and wires it into the SwarmExtension, then rebuilds tools so swarm_execute
// becomes available.
func wireSwarmFactory(coord agent.Coordinator) {
	swarm := extensions.TheSwarmExtension
	if swarm == nil {
		slog.Warn("SwarmExtension singleton is nil, skipping factory wiring")
		return
	}

	factory := coord.StructuredSubagentFactory()
	if factory == nil {
		slog.Warn("Coordinator StructuredSubagentFactory is nil, skipping swarm factory wiring")
		return
	}

	swarm.SetFactory(factory)
	swarm.RebuildTools()
	slog.Info("Swarm factory wired and tools rebuilt")
}

// [XRUSH: end]

// [XRUSH: begin: wireProductiveFactory]
func wireProductiveFactory(coord agent.Coordinator) {
	prod := extensions.TheProductiveExtension
	if prod == nil {
		slog.Warn("ProductiveExtension singleton is nil, skipping factory wiring")
		return
	}

	factory := coord.StructuredSubagentFactory()
	if factory == nil {
		slog.Warn("Coordinator StructuredSubagentFactory is nil, skipping productive factory wiring")
		return
	}

	prod.SetFactory(factory)
	prod.RebuildTools()
	slog.Info("Productive factory wired and tools rebuilt")
}

// [XRUSH: end]

// [XRUSH: begin: wirePromptAssembly]
// wirePromptAssembly connects the LCM extension to the PromptAssemblyExtension
// so that the system prompt modifier can inject LCM context files.
func wirePromptAssembly(extHost *ext.ExtensionHost) {
	if extHost == nil {
		slog.Warn("Extension host is nil, skipping prompt-assembly wiring")
		return
	}

	raw := extHost.ExtensionByName("prompt-assembly")
	if raw == nil {
		slog.Warn("PromptAssemblyExtension not found, skipping LCM wiring")
		return
	}

	pa, ok := raw.(*extensions.PromptAssemblyExtension)
	if !ok {
		slog.Warn("ExtensionByName(\"prompt-assembly\") returned unexpected type", "type", fmt.Sprintf("%T", raw))
		return
	}

	if extensions.TheLCMExtension == nil {
		slog.Warn("LCMExtension singleton is nil, skipping prompt-assembly LCM wiring")
		return
	}

	pa.SetLCMExtension(extensions.TheLCMExtension)
	slog.Info("PromptAssemblyExtension wired with LCM extension")
}

// [XRUSH: end]

// [XRUSH: begin: wireRepoMapPromptInjection]
// wireRepoMapPromptInjection connects the RepomapExtension to the
// PromptAssemblyExtension so that the cached repo map can be injected into
// the system prompt when ShouldInject returns true.
func wireRepoMapPromptInjection(extHost *ext.ExtensionHost) {
	if extHost == nil {
		slog.Warn("Extension host is nil, skipping repo map prompt injection wiring")
		return
	}

	raw := extHost.ExtensionByName("prompt-assembly")
	if raw == nil {
		slog.Warn("PromptAssemblyExtension not found, skipping repo map wiring")
		return
	}

	pa, ok := raw.(*extensions.PromptAssemblyExtension)
	if !ok {
		slog.Warn("ExtensionByName(\"prompt-assembly\") returned unexpected type", "type", fmt.Sprintf("%T", raw))
		return
	}

	if extensions.TheRepomapExtension == nil {
		slog.Warn("RepomapExtension singleton is nil, skipping repo map prompt wiring")
		return
	}

	pa.SetRepomapExtension(extensions.TheRepomapExtension)
	slog.Info("PromptAssemblyExtension wired with repo map extension")
}

// [XRUSH: end]

// [XRUSH: begin: wireMessageDecorator]
// wireMessageDecorator wraps the raw message service with LCM-aware behaviour
// (large-output storage, token tracking, compaction scheduling, summary
// injection). When the LCM manager is nil the function logs and returns without
// error so that non-LCM builds remain unaffected.
func wireMessageDecorator(app *App, q db.Querier, conn *sql.DB, store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		slog.Warn("LCM manager is nil, skipping message decorator wiring")
		return
	}

	queries, ok := q.(*db.Queries)
	if !ok {
		slog.Warn("DB Querier is not *db.Queries, skipping message decorator wiring")
		return
	}

	cfg := store.Config()

	decoratorCfg := lcm.MessageDecoratorConfig{}
	if cfg.Options != nil && cfg.Options.LCM != nil {
		decoratorCfg.DisableLargeToolOutput = cfg.Options.LCM.DisableLargeToolOutput
		decoratorCfg.LargeToolOutputTokenThreshold = cfg.Options.LCM.LargeToolOutputTokenThreshold
		if cfg.Options.LCM.ExplorerOutputProfile != "" {
			decoratorCfg.ExplorerOutputProfile = explorer.OutputProfile(cfg.Options.LCM.ExplorerOutputProfile)
		}
	}

	app.Messages = lcm.NewMessageDecorator(app.Messages, mgr, queries, conn, decoratorCfg)
	slog.Info("Message decorator wired with LCM support")
}

// [XRUSH: end]

// [XRUSH: begin: wireLCMModelOutputLimit]
// wireLCMModelOutputLimit reads the large model's default_max_tokens from the
// config and propagates it to the LCM manager so that budget calculations
// reserve the correct output token quota. When the model reports 0 or is not
// available, the default (0) is kept, which causes the budget formula to fall
// back to min(20000, contextWindow*0.25).
func wireLCMModelOutputLimit(store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	cfg := store.Config()

	// Use the same model resolution as wireLCMContextWindow.
	var catModel *catwalk.Model
	if cfg.Options != nil && cfg.Options.LCM != nil && cfg.Options.LCM.SummarizerModel != nil {
		catModel = cfg.GetModel(cfg.Options.LCM.SummarizerModel.Provider, cfg.Options.LCM.SummarizerModel.Model)
	}
	if catModel == nil {
		catModel = cfg.LargeModel()
	}

	if catModel == nil || catModel.DefaultMaxTokens <= 0 {
		slog.Info("LCM model output limit kept at default (model unavailable or reports 0)")
		return
	}

	mgr.SetModelOutputLimit(catModel.DefaultMaxTokens)
	slog.Info("LCM model output limit set from model metadata",
		"model_output_limit", catModel.DefaultMaxTokens,
	)
}

// [XRUSH: end]

// [XRUSH: begin: wireLCMOverheadTokens]
// wireLCMOverheadTokens sets default system prompt and tool token overhead
// estimates for budget computation. These defaults account for the system
// prompt template, tool definitions, and per-step injection overhead.
func wireLCMOverheadTokens(_ *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	const defaultSystemPromptTokens = 4000
	const defaultToolTokens = 8000

	mgr.SetOverheadTokens(defaultSystemPromptTokens, defaultToolTokens)
	slog.Info("LCM overhead tokens set",
		"system_prompt_tokens", defaultSystemPromptTokens,
		"tool_tokens", defaultToolTokens,
	)
}

// [XRUSH: end]

// [XRUSH: begin: wireLCMProviderType]
// wireLCMProviderType reads the provider type from the selected model config
// and propagates it to the LCM manager for cache-optimization decisions (e.g.
// "anthropic" enables Anthropic prefix caching heuristics).
func wireLCMProviderType(store *config.ConfigStore) {
	mgr := extensions.TheLCMExtension.Manager()
	if mgr == nil {
		return
	}

	cfg := store.Config()

	// Use the same model resolution as wireLCMLLMClient.
	var selected *config.SelectedModel
	if cfg.Options != nil && cfg.Options.LCM != nil && cfg.Options.LCM.SummarizerModel != nil {
		selected = cfg.Options.LCM.SummarizerModel
	}
	if selected == nil {
		if sm, ok := cfg.Models[config.SelectedModelTypeLarge]; ok {
			selected = &sm
		}
	}

	if selected == nil {
		return
	}

	providerCfg, ok := cfg.Providers.Get(selected.Provider)
	if !ok {
		return
	}

	mgr.SetProviderType(string(providerCfg.Type))
	slog.Info("LCM provider type set",
		"provider_type", providerCfg.Type,
	)
}

// [XRUSH: end]

// [XRUSH: end]

// Verify interface compliance.
var (
	_ lcm.LLMClient         = fallbackLCMClient{}
	_ fantasy.LanguageModel = (*lcmFallbackLM)(nil)
)

// [XRUSH: end]
// [XRUSH: end: rewind service and agent config restoration]
