package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/extensions"
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/charmbracelet/crush/internal/lcm"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/require"
)

type stubLM struct{}

func (s *stubLM) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return &fantasy.Response{}, nil
}

func (s *stubLM) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, nil
}

func (s *stubLM) GenerateObject(_ context.Context, _ fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, nil
}

func (s *stubLM) StreamObject(_ context.Context, _ fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, nil
}

func (s *stubLM) Provider() string { return "test" }
func (s *stubLM) Model() string    { return "test-model" }

func TestSetLLMClientCreatesAdapter(t *testing.T) {
	t.Parallel()

	model := agent.Model{
		Model:      &stubLM{},
		CatwalkCfg: catwalk.Model{ContextWindow: 128000, DefaultMaxTokens: 4096},
		ModelCfg: config.SelectedModel{
			Model:    "test-model",
			Provider: "test",
		},
	}
	providerCfg := config.ProviderConfig{
		ID:   "test",
		Name: "Test Provider",
	}

	adapter := agent.NewLCMLLMClient(model, providerCfg)
	require.NotNil(t, adapter, "NewLCMLLMClient should return a non-nil adapter")

	var _ lcm.LLMClient = adapter
}

func TestLCMLLMClientWired(t *testing.T) {
	origExt := extensions.TheLCMExtension
	t.Cleanup(func() { extensions.TheLCMExtension = origExt })
	freshExt := &extensions.LCMExtension{}
	extensions.TheLCMExtension = freshExt

	ext.ResetForTesting()
	ext.RegisterExtension(freshExt)

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)

	cfg := &config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Options: &config.Options{
			LCM: &config.LCMOptions{},
		},
	}
	store := config.NewTestStore(cfg)

	events := pubsub.NewBroker[tea.Msg]()
	t.Cleanup(func() { events.Shutdown() })

	app := &App{
		globalCtx:       t.Context(),
		serviceEventsWG: &sync.WaitGroup{},
		eventsCtx:       t.Context(),
		events:          events,
	}

	setupExtensions(t.Context(), app, conn, q, nil, nil, store)

	mgr := extensions.TheLCMExtension.Manager()
	require.NotNil(t, mgr, "LCM manager should be initialized after setupExtensions")

	_, err = mgr.CompressWith(t.Context(), "test input")
	require.False(t,
		errors.Is(err, lcm.ErrNoCompressor),
		"LCM manager has nil compressor after setup — SetLLMClient was never called",
	)
}

func TestHookRunnersNilManager(t *testing.T) {
	origExt := extensions.TheLCMExtension
	t.Cleanup(func() { extensions.TheLCMExtension = origExt })
	extensions.TheLCMExtension = &extensions.LCMExtension{}

	cfg := &config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
	}
	store := config.NewTestStore(cfg)

	require.NotPanics(t, func() {
		wireCompactHookRunners(store)
	}, "wireCompactHookRunners should not panic when LCM manager is nil")
}

func TestHookRunnersWired(t *testing.T) {
	origExt := extensions.TheLCMExtension
	t.Cleanup(func() { extensions.TheLCMExtension = origExt })
	freshExt := &extensions.LCMExtension{}
	extensions.TheLCMExtension = freshExt

	ext.ResetForTesting()
	ext.RegisterExtension(freshExt)

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)

	markerDir := t.TempDir()

	cfg := &config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Options: &config.Options{
			LCM: &config.LCMOptions{},
		},
		Hooks: map[string][]config.HookConfig{
			hooks.EventPreCompact: {
				{Command: "touch " + filepath.Join(markerDir, "pre-fired")},
			},
			hooks.EventPostCompact: {
				{Command: "touch " + filepath.Join(markerDir, "post-fired")},
			},
		},
	}
	store := config.NewTestStore(cfg)

	events := pubsub.NewBroker[tea.Msg]()
	t.Cleanup(func() { events.Shutdown() })

	app := &App{
		globalCtx:       t.Context(),
		serviceEventsWG: &sync.WaitGroup{},
		eventsCtx:       t.Context(),
		events:          events,
	}

	setupExtensions(t.Context(), app, conn, q, nil, nil, store)

	mgr := extensions.TheLCMExtension.Manager()
	require.NotNil(t, mgr, "LCM manager should be initialized after setupExtensions")
}

func TestOrchestrationWiringComplete(t *testing.T) {
	origXrush := extensions.TheXrushExtension
	origOrch := extensions.TheOrchestrationExtension
	origLCM := extensions.TheLCMExtension
	t.Cleanup(func() {
		extensions.TheXrushExtension = origXrush
		extensions.TheOrchestrationExtension = origOrch
		extensions.TheLCMExtension = origLCM
	})

	freshXrush := &extensions.XrushExtension{}
	freshOrch := &extensions.OrchestrationExtension{}
	freshLCM := &extensions.LCMExtension{}
	extensions.TheXrushExtension = freshXrush
	extensions.TheOrchestrationExtension = freshOrch
	extensions.TheLCMExtension = freshLCM

	ext.ResetForTesting()
	ext.RegisterExtension(freshXrush)
	ext.RegisterExtension(freshLCM)
	ext.RegisterExtension(freshOrch)

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)

	cfg := &config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Options: &config.Options{
			LCM: &config.LCMOptions{},
		},
	}
	store := config.NewTestStore(cfg)

	events := pubsub.NewBroker[tea.Msg]()
	t.Cleanup(func() { events.Shutdown() })

	app := &App{
		globalCtx:       t.Context(),
		serviceEventsWG: &sync.WaitGroup{},
		eventsCtx:       t.Context(),
		events:          events,
	}

	setupExtensions(t.Context(), app, conn, q, nil, nil, store)

	require.NotNil(t, extensions.TheXrushExtension.Registry(),
		"XrushExtension registry should be non-nil after bootstrap+wiring")
	require.NotNil(t, extensions.TheXrushExtension.Mailbox(),
		"XrushExtension mailbox should be non-nil after bootstrap+wiring")

	orchTools, err := extensions.TheOrchestrationExtension.Tools(t.Context())
	require.NoError(t, err)
	require.Len(t, orchTools, 4,
		"OrchestrationExtension should contribute 4 tools after wiring")

	expectedNames := map[string]bool{
		"send_message": true,
		"team_create":  true,
		"team_delete":  true,
		"task_stop":    true,
	}
	for _, tool := range orchTools {
		name := tool.Info().Name
		require.True(t, expectedNames[name],
			"unexpected orchestration tool: %s", name)
	}

	surface := agent.NewToolSurface()
	for _, name := range []string{"send_message", "team_create", "team_delete", "task_stop"} {
		caps := surface.GetToolCapabilities(name)
		require.Empty(t, caps,
			"tool %q should NOT be registered in ToolSurface defaults (provided by extension)", name)
	}
}

func TestLCMExtensionWiredToPromptAssembly(t *testing.T) {
	origLCM := extensions.TheLCMExtension
	t.Cleanup(func() { extensions.TheLCMExtension = origLCM })

	freshLCM := &extensions.LCMExtension{}
	extensions.TheLCMExtension = freshLCM

	ext.ResetForTesting()
	ext.RegisterExtension(freshLCM)
	ext.RegisterExtension(&extensions.PromptAssemblyExtension{})

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)

	cfg := &config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Options:   &config.Options{LCM: &config.LCMOptions{}},
	}
	store := config.NewTestStore(cfg)

	events := pubsub.NewBroker[tea.Msg]()
	t.Cleanup(func() { events.Shutdown() })

	app := &App{
		globalCtx:       t.Context(),
		serviceEventsWG: &sync.WaitGroup{},
		eventsCtx:       t.Context(),
		events:          events,
	}

	setupExtensions(t.Context(), app, conn, q, nil, nil, store)

	raw := app.ExtHost.ExtensionByName("prompt-assembly")
	require.NotNil(t, raw, "prompt-assembly extension should be registered")

	hook := raw.(interface{ PromptHook() *ext.PromptHook }).PromptHook()
	require.NotNil(t, hook, "PromptHook should be non-nil")
	require.NotNil(t, hook.SystemPromptModifier, "SystemPromptModifier should be non-nil")

	mgr := extensions.TheLCMExtension.Manager()
	require.NotNil(t, mgr, "LCM manager should exist after bootstrap")

	modified, err := hook.SystemPromptModifier(t.Context(), "", "base-prompt")
	require.NoError(t, err)
	require.NotEqual(t, "base-prompt", modified,
		"SystemPromptModifier should modify prompt when LCM extension is wired "+
			"(LCM manager has built-in context files)")
	require.Contains(t, modified, "base-prompt",
		"Modified prompt should contain the original base prompt")
}

func TestRepoMapPromptInjectionWiring(t *testing.T) {
	origLCM := extensions.TheLCMExtension
	origRepomap := extensions.TheRepomapExtension
	t.Cleanup(func() {
		extensions.TheLCMExtension = origLCM
		extensions.TheRepomapExtension = origRepomap
	})

	freshLCM := &extensions.LCMExtension{}
	freshRepomap := &extensions.RepomapExtension{}
	extensions.TheLCMExtension = freshLCM
	extensions.TheRepomapExtension = freshRepomap

	ext.ResetForTesting()
	ext.RegisterExtension(freshLCM)
	ext.RegisterExtension(freshRepomap)
	ext.RegisterExtension(&extensions.PromptAssemblyExtension{})

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)

	cfg := &config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Options:   &config.Options{LCM: &config.LCMOptions{}},
	}
	store := config.NewTestStore(cfg)

	events := pubsub.NewBroker[tea.Msg]()
	t.Cleanup(func() { events.Shutdown() })

	app := &App{
		globalCtx:       t.Context(),
		serviceEventsWG: &sync.WaitGroup{},
		eventsCtx:       t.Context(),
		events:          events,
	}

	setupExtensions(t.Context(), app, conn, q, nil, nil, store)

	raw := app.ExtHost.ExtensionByName("prompt-assembly")
	require.NotNil(t, raw, "prompt-assembly extension should be registered")

	hook := raw.(interface{ PromptHook() *ext.PromptHook }).PromptHook()
	require.NotNil(t, hook, "PromptHook should be non-nil")

	// Verify no panic when calling SystemPromptModifier without cached map.
	modified, err := hook.SystemPromptModifier(t.Context(), "test-session", "base-prompt")
	require.NoError(t, err)
	require.Contains(t, modified, "base-prompt",
		"SystemPromptModifier should not corrupt prompt when no cached map exists")

	// Verify ShouldInjectMap returns false without run key in context.
	_, tokenCount := extensions.TheRepomapExtension.LoadCachedMap("test-session")
	require.Equal(t, 0, tokenCount,
		"LoadCachedMap should return 0 tokens for session with no cached map")
}

func TestRepoMapPromptInjectionNoPanicWithoutRepomap(t *testing.T) {
	origLCM := extensions.TheLCMExtension
	t.Cleanup(func() { extensions.TheLCMExtension = origLCM })

	freshLCM := &extensions.LCMExtension{}
	extensions.TheLCMExtension = freshLCM

	ext.ResetForTesting()
	ext.RegisterExtension(freshLCM)
	ext.RegisterExtension(&extensions.PromptAssemblyExtension{})

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)

	cfg := &config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Options:   &config.Options{LCM: &config.LCMOptions{}},
	}
	store := config.NewTestStore(cfg)

	events := pubsub.NewBroker[tea.Msg]()
	t.Cleanup(func() { events.Shutdown() })

	app := &App{
		globalCtx:       t.Context(),
		serviceEventsWG: &sync.WaitGroup{},
		eventsCtx:       t.Context(),
		events:          events,
	}

	require.NotPanics(t, func() {
		setupExtensions(t.Context(), app, conn, q, nil, nil, store)
	}, "setupExtensions should not panic when RepomapExtension is nil")
}

func TestWireOrchestrationNilSafe(t *testing.T) {
	origXrush := extensions.TheXrushExtension
	origOrch := extensions.TheOrchestrationExtension
	t.Cleanup(func() {
		extensions.TheXrushExtension = origXrush
		extensions.TheOrchestrationExtension = origOrch
	})

	extensions.TheXrushExtension = &extensions.XrushExtension{}
	extensions.TheOrchestrationExtension = &extensions.OrchestrationExtension{}

	require.NotPanics(t, func() {
		wireOrchestration()
	}, "wireOrchestration should not panic when xrush registry/mailbox are nil (pre-bootstrap)")
}

var _ tools.AgentRegistry = (*stubAgentRegistry)(nil)

type stubAgentRegistry struct{}

func (stubAgentRegistry) Get(string) (tools.AgentHandle, bool) { return nil, false }
func (stubAgentRegistry) HasAgent(string) bool                 { return false }
func (stubAgentRegistry) List() []string                       { return nil }

var _ tools.Mailbox = (*stubMailbox)(nil)

type stubMailbox struct{}

func (stubMailbox) Send(tools.MailboxMessage) error { return nil }
func (stubMailbox) HasInbox(string) bool            { return false }

func TestMessageDecoratorWired(t *testing.T) {
	origLCM := extensions.TheLCMExtension
	t.Cleanup(func() { extensions.TheLCMExtension = origLCM })
	freshLCM := &extensions.LCMExtension{}
	extensions.TheLCMExtension = freshLCM

	ext.ResetForTesting()
	ext.RegisterExtension(freshLCM)

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)
	msgSvc := message.NewService(q)

	cfg := &config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Options: &config.Options{
			LCM: &config.LCMOptions{},
		},
	}
	store := config.NewTestStore(cfg)

	events := pubsub.NewBroker[tea.Msg]()
	t.Cleanup(func() { events.Shutdown() })

	app := &App{
		Messages:        msgSvc,
		globalCtx:       t.Context(),
		serviceEventsWG: &sync.WaitGroup{},
		eventsCtx:       t.Context(),
		events:          events,
	}

	setupExtensions(t.Context(), app, conn, q, nil, msgSvc, store)

	mgr := extensions.TheLCMExtension.Manager()
	require.NotNil(t, mgr, "LCM manager should be initialized after setupExtensions")
	require.NotNil(t, app.Messages, "app.Messages should be non-nil after wiring")

	msgs, err := app.Messages.List(t.Context(), "nonexistent-session")
	require.NoError(t, err, "decorated message service List should not error")
	require.Empty(t, msgs, "List for nonexistent session should return empty")
}

func TestMessageDecoratorNilSafe(t *testing.T) {
	origLCM := extensions.TheLCMExtension
	t.Cleanup(func() { extensions.TheLCMExtension = origLCM })
	extensions.TheLCMExtension = &extensions.LCMExtension{}

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)
	msgSvc := message.NewService(q)

	cfg := &config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
	}
	store := config.NewTestStore(cfg)

	require.NotPanics(t, func() {
		wireMessageDecorator(&App{Messages: msgSvc}, q, conn, store)
	}, "wireMessageDecorator should not panic when LCM manager is nil")
}
