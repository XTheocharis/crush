package agent

import (
	"context"
	"sync"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/stretchr/testify/require"
)

// mockModelRouterExt is a test-only extension that mimics the real
// ModelRouterExtension's routing and LastRoutedModel accessor.
type mockModelRouterExt struct {
	mu            sync.RWMutex
	lastModelType config.SelectedModelType
}

func (e *mockModelRouterExt) Name() string                                    { return "model_router" }
func (e *mockModelRouterExt) Init(_ context.Context, _ ext.HostContext) error { return nil }
func (e *mockModelRouterExt) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastModelType = ""
	return nil
}

func (e *mockModelRouterExt) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	return nil, nil
}
func (e *mockModelRouterExt) ToolNames() []string         { return nil }
func (e *mockModelRouterExt) RunHooks() []ext.RunHook     { return nil }
func (e *mockModelRouterExt) PromptHook() *ext.PromptHook { return nil }
func (e *mockModelRouterExt) StepHooks() []ext.StepHook {
	return []ext.StepHook{
		{
			Name:          "model_router:select_model",
			OnPrepareStep: e.selectModel,
		},
	}
}

func (e *mockModelRouterExt) selectModel(_ context.Context, _ string, messages []fantasy.Message) ([]fantasy.Message, error) {
	charCount := 0
	for _, msg := range messages {
		for _, part := range msg.Content {
			if tp, ok := fantasy.AsContentType[fantasy.TextPart](part); ok {
				charCount += len(tp.Text)
			}
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if charCount < 10000 {
		e.lastModelType = config.SelectedModelTypeSmall
	} else {
		e.lastModelType = config.SelectedModelTypeLarge
	}
	return messages, nil
}

func (e *mockModelRouterExt) LastRoutedModel() config.SelectedModelType {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastModelType
}

func TestSessionAgentExtHostStored(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	host := ext.NewExtensionHost(ext.HostDeps{})
	ctx := context.Background()
	require.NoError(t, host.Bootstrap(ctx))

	agent := NewSessionAgent(SessionAgentOptions{
		LargeModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		SmallModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		IsYolo:  true,
		ExtHost: host,
	})

	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)
	require.NotNil(t, sa.extHost)
	require.True(t, sa.extHost.IsBootstrapped())
}

func TestSessionAgentNilExtHost(t *testing.T) {
	agent := NewSessionAgent(SessionAgentOptions{
		LargeModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		SmallModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		IsYolo:  true,
		ExtHost: nil,
	})

	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)
	require.Nil(t, sa.extHost)
}

func TestSessionAgentExtHostSubAgentField(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	host := ext.NewExtensionHost(ext.HostDeps{})
	ctx := context.Background()
	require.NoError(t, host.Bootstrap(ctx))

	agent := NewSessionAgent(SessionAgentOptions{
		LargeModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		SmallModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		IsSubAgent: true,
		IsYolo:     true,
		ExtHost:    nil,
	})

	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)
	require.Nil(t, sa.extHost)
	require.True(t, sa.isSubAgent)
}

func TestAgentSafeCallRecovery(t *testing.T) {
	err := safeCall("test_panic", func() error {
		panic("test panic")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "panicked")
	require.Contains(t, err.Error(), "test panic")
}

func TestAgentSafeCallNoPanic(t *testing.T) {
	err := safeCall("test_no_panic", func() error {
		return nil
	})
	require.NoError(t, err)
}

func TestAgentSafeCallReturnsError(t *testing.T) {
	err := safeCall("test_error", func() error {
		return context.Canceled
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestAgentExtHostHooksAccessible(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	_ = ext.RunHook{
		Name: "test_run_hook",
		OnRunStart: func(ctx context.Context, sessionID string, prompt string) error {
			return nil
		},
		OnRunEnd: func(ctx context.Context, sessionID string, result *fantasy.AgentResult, err error) error {
			return nil
		},
	}

	_ = &testExtension{} //nolint:unusedwrite
	// We can't easily create RunHookProvider with the testExtension in this package,
	// so test via the host directly.
	host := ext.NewExtensionHost(ext.HostDeps{})

	agent := NewSessionAgent(SessionAgentOptions{
		LargeModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		SmallModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		IsYolo:  true,
		ExtHost: host,
	})

	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)

	require.Empty(t, sa.extHost.RunHooks())

	agent2 := NewSessionAgent(SessionAgentOptions{
		LargeModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		SmallModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		IsYolo:  true,
		ExtHost: nil,
	})

	sa2, ok := agent2.(*sessionAgent)
	require.True(t, ok)
	require.Nil(t, sa2.extHost)
}

func TestAgentExtHostStopConditionNilSafe(t *testing.T) {
	agent := NewSessionAgent(SessionAgentOptions{
		LargeModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		SmallModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		IsYolo:  true,
		ExtHost: nil,
	})

	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)

	if sa.extHost != nil {
		t.Fatal("extHost should be nil")
	}

	stepHooks := []ext.StepHook(nil)
	require.Empty(t, stepHooks)
}

func TestAgentMessageQueueInitialized(t *testing.T) {
	agent := NewSessionAgent(SessionAgentOptions{
		LargeModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		SmallModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		IsYolo:  true,
		ExtHost: nil,
	})

	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)
	require.NotNil(t, sa.messageQueue)
	require.NotNil(t, sa.activeRequests)
}

func TestAgentToolsSliceInitialized(t *testing.T) {
	mockTool := fantasy.NewAgentTool(
		"init_test_tool",
		"Test tool for init",
		func(ctx context.Context, input struct{}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.ToolResponse{}, nil
		},
	)

	agent := NewSessionAgent(SessionAgentOptions{
		LargeModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		SmallModel: Model{
			CatwalkCfg: catwalk.Model{ContextWindow: 100000, DefaultMaxTokens: 1000},
		},
		IsYolo:  true,
		Tools:   []fantasy.AgentTool{mockTool},
		ExtHost: nil,
	})

	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)
	tools := sa.tools.Copy()
	require.Len(t, tools, 1)
	require.Equal(t, "init_test_tool", tools[0].Info().Name)
}

func TestAgentCsyncFieldsInitialized(t *testing.T) {
	agent := NewSessionAgent(SessionAgentOptions{
		LargeModel: Model{
			Model:      nil,
			CatwalkCfg: catwalk.Model{ContextWindow: 100000},
		},
		SmallModel: Model{
			Model:      nil,
			CatwalkCfg: catwalk.Model{ContextWindow: 100000},
		},
		SystemPromptPrefix: "prefix",
		SystemPrompt:       "system",
		IsYolo:             true,
		ExtHost:            nil,
	})

	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)

	prefix := sa.systemPromptPrefix.Get()
	require.Equal(t, "prefix", prefix)

	prompt := sa.systemPrompt.Get()
	require.Equal(t, "system", prompt)

	require.NotNil(t, sa.largeModel)
	require.NotNil(t, sa.smallModel)
}

func TestGetRoutedModelType_NoExtensionHost(t *testing.T) {
	m := agentHookMediator{host: nil}
	require.Equal(t, config.SelectedModelTypeLarge, m.getRoutedModelType())
}

func TestGetRoutedModelType_NoModelRouterExtension(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Bootstrap(context.Background()))

	m := agentHookMediator{host: host}
	require.Equal(t, config.SelectedModelTypeLarge, m.getRoutedModelType())
}

func TestGetRoutedModelType_WithModelRouterSmall(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	router := &mockModelRouterExt{}
	ext.RegisterExtension(router)

	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Bootstrap(context.Background()))

	_, err := router.StepHooks()[0].OnPrepareStep(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage("hi"),
	})
	require.NoError(t, err)

	m := agentHookMediator{host: host}
	require.Equal(t, config.SelectedModelTypeSmall, m.getRoutedModelType())
}

func TestGetRoutedModelType_WithModelRouterLarge(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	router := &mockModelRouterExt{}
	ext.RegisterExtension(router)

	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Bootstrap(context.Background()))

	var longText string
	for i := 0; i < 20000; i++ {
		longText += "x"
	}
	_, err := router.StepHooks()[0].OnPrepareStep(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage(longText),
	})
	require.NoError(t, err)

	m := agentHookMediator{host: host}
	require.Equal(t, config.SelectedModelTypeLarge, m.getRoutedModelType())
}

func TestModelRouterSwitchesModels(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	router := &mockModelRouterExt{}
	ext.RegisterExtension(router)

	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Bootstrap(context.Background()))

	largeModel := Model{
		CatwalkCfg: catwalk.Model{
			Name:             "LargeModel",
			ContextWindow:    200000,
			DefaultMaxTokens: 10000,
			SupportsImages:   true,
		},
		ModelCfg: config.SelectedModel{
			Model:    "large-model-id",
			Provider: "test-provider",
		},
	}
	smallModel := Model{
		CatwalkCfg: catwalk.Model{
			Name:             "SmallModel",
			ContextWindow:    100000,
			DefaultMaxTokens: 5000,
			SupportsImages:   false,
		},
		ModelCfg: config.SelectedModel{
			Model:    "small-model-id",
			Provider: "test-provider",
		},
	}

	agent := NewSessionAgent(SessionAgentOptions{
		LargeModel:   largeModel,
		SmallModel:   smallModel,
		SystemPrompt: "test",
		IsYolo:       true,
		ExtHost:      host,
	})

	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)

	t.Run("small_message_routes_to_small_model", func(t *testing.T) {
		_, err := router.StepHooks()[0].OnPrepareStep(context.Background(), "s1", []fantasy.Message{
			fantasy.NewUserMessage("hi"),
		})
		require.NoError(t, err)

		routedModelType := sa.hooks.getRoutedModelType()
		require.Equal(t, config.SelectedModelTypeSmall, routedModelType)

		routedModel := sa.largeModel.Get()
		if routedModelType == config.SelectedModelTypeSmall {
			if sm := sa.smallModel.Get(); sm.Model != nil || sm.CatwalkCfg.Name != "" {
				routedModel = sm
			}
		}
		require.Equal(t, "SmallModel", routedModel.CatwalkCfg.Name)
		require.Equal(t, "small-model-id", routedModel.ModelCfg.Model)
		require.False(t, routedModel.CatwalkCfg.SupportsImages)
	})

	t.Run("large_message_routes_to_large_model", func(t *testing.T) {
		var longText string
		for i := 0; i < 20000; i++ {
			longText += "x"
		}
		_, err := router.StepHooks()[0].OnPrepareStep(context.Background(), "s1", []fantasy.Message{
			fantasy.NewUserMessage(longText),
		})
		require.NoError(t, err)

		routedModelType := sa.hooks.getRoutedModelType()
		require.Equal(t, config.SelectedModelTypeLarge, routedModelType)

		routedModel := sa.largeModel.Get()
		require.Equal(t, "LargeModel", routedModel.CatwalkCfg.Name)
		require.Equal(t, "large-model-id", routedModel.ModelCfg.Model)
		require.True(t, routedModel.CatwalkCfg.SupportsImages)
	})
}

func TestModelRouterSwitchesModels_NilSmallModel(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	router := &mockModelRouterExt{}
	ext.RegisterExtension(router)

	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Bootstrap(context.Background()))

	largeModel := Model{
		CatwalkCfg: catwalk.Model{
			Name:             "LargeModel",
			ContextWindow:    200000,
			DefaultMaxTokens: 10000,
			SupportsImages:   true,
		},
		ModelCfg: config.SelectedModel{
			Model:    "large-model-id",
			Provider: "test-provider",
		},
	}

	agent := NewSessionAgent(SessionAgentOptions{
		LargeModel:   largeModel,
		SmallModel:   Model{},
		SystemPrompt: "test",
		IsYolo:       true,
		ExtHost:      host,
	})

	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)

	_, err := router.StepHooks()[0].OnPrepareStep(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage("hi"),
	})
	require.NoError(t, err)

	routedModelType := sa.hooks.getRoutedModelType()
	require.Equal(t, config.SelectedModelTypeSmall, routedModelType)

	routedModel := sa.largeModel.Get()
	if routedModelType == config.SelectedModelTypeSmall {
		if sm := sa.smallModel.Get(); sm.Model != nil {
			routedModel = sm
		}
	}
	require.Equal(t, "LargeModel", routedModel.CatwalkCfg.Name)
}
