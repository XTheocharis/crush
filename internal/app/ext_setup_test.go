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
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/extensions"
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/charmbracelet/crush/internal/lcm"
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

