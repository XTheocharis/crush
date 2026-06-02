package app

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/charmbracelet/crush/internal/lcm"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/require"
)

type stubCoordinator struct {
	model      agent.Model
	resolveErr error
}

func (s *stubCoordinator) Run(_ context.Context, _, _ string, _ ...message.Attachment) (*fantasy.AgentResult, error) {
	return nil, nil
}
func (s *stubCoordinator) Cancel(string)                               {}
func (s *stubCoordinator) CancelAll()                                  {}
func (s *stubCoordinator) IsSessionBusy(string) bool                   { return false }
func (s *stubCoordinator) IsBusy() bool                                { return false }
func (s *stubCoordinator) QueuedPrompts(string) int                    { return 0 }
func (s *stubCoordinator) QueuedPromptsList(string) []string           { return nil }
func (s *stubCoordinator) ClearQueue(string)                           {}
func (s *stubCoordinator) Summarize(_ context.Context, _ string) error { return nil }
func (s *stubCoordinator) Model() agent.Model                          { return s.model }
func (s *stubCoordinator) UpdateModels(_ context.Context) error        { return nil }
func (s *stubCoordinator) RecoverSession(_ context.Context, _ string) error {
	return nil
}

func (s *stubCoordinator) RepoMapRefresh(_ context.Context, _ string) error {
	return nil
}

func (s *stubCoordinator) RestoreAgentConfig(_ context.Context, _ map[string][]string) error {
	return nil
}

func (s *stubCoordinator) StructuredSubagentFactory() agent.StructuredSubagentFactory {
	return nil
}

func (s *stubCoordinator) ResolveLCMModel(_ context.Context, selected config.SelectedModel, _ config.ProviderConfig) (agent.Model, error) {
	if s.resolveErr != nil {
		return agent.Model{}, s.resolveErr
	}
	return s.model, nil
}

type stubLanguageModel struct {
	resp string
}

func (s *stubLanguageModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return &fantasy.Response{Content: fantasy.ResponseContent{fantasy.TextContent{Text: s.resp}}}, nil
}

func (s *stubLanguageModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, nil
}

func (s *stubLanguageModel) GenerateObject(_ context.Context, _ fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, nil
}

func (s *stubLanguageModel) StreamObject(_ context.Context, _ fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, nil
}
func (s *stubLanguageModel) Provider() string { return "test-provider" }
func (s *stubLanguageModel) Model() string    { return "test-model" }

func TestResolveLCMModelReturnsRealProvider(t *testing.T) {
	t.Parallel()

	lm := &stubLanguageModel{resp: "summarized content"}
	coord := &stubCoordinator{
		model: agent.Model{
			Model: lm,
			CatwalkCfg: catwalk.Model{
				ID:               "test-model",
				ContextWindow:    128000,
				DefaultMaxTokens: 4096,
			},
			ModelCfg: config.SelectedModel{
				Provider: "test-provider",
				Model:    "test-model",
			},
		},
	}

	selected := config.SelectedModel{
		Provider: "test-provider",
		Model:    "test-model",
	}
	providerCfg := config.ProviderConfig{
		ID:   "test-provider",
		Type: "openai-compat",
		Models: []catwalk.Model{
			{ID: "test-model", ContextWindow: 128000},
		},
	}

	model, err := resolveLCMModel(t.Context(), selected, providerCfg, coord)
	require.NoError(t, err)
	require.NotNil(t, model.Model, "Model.LanguageModel should be non-nil")
	require.Equal(t, "test-model", model.ModelCfg.Model)
	require.Equal(t, "test-provider", model.ModelCfg.Provider)

	resp, respErr := model.Model.Generate(t.Context(), fantasy.Call{})
	require.NoError(t, respErr)
	require.Equal(t, "summarized content", resp.Content.Text(),
		"LanguageModel should return real response, not empty stub")
}

func TestResolveLCMModelFallbackOnProviderError(t *testing.T) {
	t.Parallel()

	coord := &stubCoordinator{
		resolveErr: fmt.Errorf("provider construction failed"),
	}

	selected := config.SelectedModel{
		Provider: "bogus-provider",
		Model:    "no-such-model",
	}
	providerCfg := config.ProviderConfig{
		ID:   "bogus-provider",
		Type: "openai-compat",
	}

	_, err := resolveLCMModel(t.Context(), selected, providerCfg, coord)
	require.Error(t, err, "resolveLCMModel should return error when provider fails")
	require.Contains(t, err.Error(), "provider construction failed")
}

func TestWireLCMLLMClientWithRealProvider(t *testing.T) {
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

	providers := csync.NewMap[string, config.ProviderConfig]()
	providers.Set("test", config.ProviderConfig{
		ID:   "test",
		Type: "openai-compat",
		Models: []catwalk.Model{
			{ID: "test-model", ContextWindow: 128000, DefaultMaxTokens: 4096},
		},
	})

	cfg := &config.Config{
		Providers: providers,
		Models: map[config.SelectedModelType]config.SelectedModel{
			config.SelectedModelTypeLarge: {Provider: "test", Model: "test-model"},
		},
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

	lm := &stubLanguageModel{resp: "lcm summary"}
	coord := &stubCoordinator{
		model: agent.Model{
			Model:      lm,
			CatwalkCfg: catwalk.Model{ID: "test-model", ContextWindow: 128000, DefaultMaxTokens: 4096},
			ModelCfg:   config.SelectedModel{Provider: "test", Model: "test-model"},
		},
	}

	wireLCMLLMClient(t.Context(), store, coord)

	mgr := extensions.TheLCMExtension.Manager()
	require.NotNil(t, mgr)

	_, compressErr := mgr.CompressWith(t.Context(), "test input")
	require.False(t, errors.Is(compressErr, lcm.ErrNoCompressor),
		"LLMClient should be wired after wireLCMLLMClient, not nil")
}

func TestWireLCMLLMClientFallbackOnError(t *testing.T) {
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

	providers := csync.NewMap[string, config.ProviderConfig]()
	providers.Set("test", config.ProviderConfig{
		ID:   "test",
		Type: "openai-compat",
		Models: []catwalk.Model{
			{ID: "test-model"},
		},
	})

	cfg := &config.Config{
		Providers: providers,
		Models: map[config.SelectedModelType]config.SelectedModel{
			config.SelectedModelTypeLarge: {Provider: "test", Model: "test-model"},
		},
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

	coord := &stubCoordinator{
		resolveErr: fmt.Errorf("provider unavailable"),
	}

	wireLCMLLMClient(t.Context(), store, coord)

	mgr := extensions.TheLCMExtension.Manager()
	require.NotNil(t, mgr)

	_, compressErr := mgr.CompressWith(t.Context(), "test input")
	require.Error(t, compressErr,
		"CompressWith should error when using fallbackLCMClient")
	require.False(t, errors.Is(compressErr, lcm.ErrNoCompressor),
		"should NOT be ErrNoCompressor — fallback client should be wired")
}

func TestWireLCMLLMClientNoModelConfigured(t *testing.T) {
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

	coord := &stubCoordinator{}

	wireLCMLLMClient(t.Context(), store, coord)

	mgr := extensions.TheLCMExtension.Manager()
	require.NotNil(t, mgr)

	_, compressErr := mgr.CompressWith(t.Context(), "test input")
	require.Error(t, compressErr)
	require.False(t, errors.Is(compressErr, lcm.ErrNoCompressor),
		"should use fallbackLCMClient (error client), not leave compressor nil")
}
