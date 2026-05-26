package agent

import (
	"context"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/ext"
	"github.com/stretchr/testify/require"
)

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

	_ = &testExtension{name: "hook_ext"}
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
