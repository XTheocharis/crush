package ext

import (
	"context"
	"sync"
	"testing"

	"charm.land/fantasy"

	"github.com/stretchr/testify/require"
)

func TestIntegrationFullPipeline(t *testing.T) {
	ResetForTesting()

	ctx := context.Background()
	mockTool := createMockTool("pipeline_tool")

	ext1 := newMockExtension("pipeline_ext").
		withTools([]fantasy.AgentTool{mockTool}, []string{"pipeline_tool"})

	host := NewExtensionHost(HostDeps{})
	require.NoError(t, host.Register(ext1))

	require.NoError(t, host.Bootstrap(ctx))
	require.True(t, host.IsBootstrapped())
	require.True(t, ext1.wasInitCalled())

	tools := host.ContributedTools()
	require.Len(t, tools, 1)
	require.Equal(t, "pipeline_tool", tools[0].Info().Name)

	names := host.ContributedToolNames()
	require.Equal(t, []string{"pipeline_tool"}, names)

	require.NoError(t, host.Shutdown(ctx))
	require.True(t, ext1.wasShutdownCalled())
}

func TestIntegrationMultipleExtensions(t *testing.T) {
	ResetForTesting()

	ctx := context.Background()

	tool1 := createMockTool("ext_tool_1")
	tool2 := createMockTool("ext_tool_2")

	extTool := newMockExtension("tool_provider").
		withTools([]fantasy.AgentTool{tool1, tool2}, []string{"ext_tool_1", "ext_tool_2"})

	extHook := newMockExtension("hook_provider").
		withRunHooks([]RunHook{
			{
				Name: "run_hook_1",
				OnRunStart: func(ctx context.Context, sessionID string, prompt string) error {
					return nil
				},
				OnRunEnd: func(ctx context.Context, sessionID string, result *fantasy.AgentResult, err error) error {
					return nil
				},
			},
		}).
		withStepHooks([]StepHook{
			{
				Name: "step_hook_1",
				OnPrepareStep: func(ctx context.Context, sessionID string, messages []fantasy.Message) ([]fantasy.Message, error) {
					return messages, nil
				},
				OnStepFinish: func(ctx context.Context, sessionID string, step fantasy.StepResult) error {
					return nil
				},
				StopCondition: func(ctx context.Context, steps []fantasy.StepResult) bool {
					return false
				},
			},
		})

	extPrompt := newMockExtension("prompt_provider").withPromptHook(&PromptHook{
		Name: "prompt_hook_1",
		OnPreparePrompt: func(ctx context.Context, sessionID string, messages []fantasy.Message) ([]fantasy.Message, error) {
			return messages, nil
		},
		SystemPromptModifier: func(ctx context.Context, sessionID string, systemPrompt string) (string, error) {
			return systemPrompt, nil
		},
	})

	host := NewExtensionHost(HostDeps{})
	require.NoError(t, host.Register(extTool))
	require.NoError(t, host.Register(extHook))
	require.NoError(t, host.Register(extPrompt))

	require.NoError(t, host.Bootstrap(ctx))

	require.Len(t, host.ContributedTools(), 2)
	require.Equal(t, []string{"ext_tool_1", "ext_tool_2"}, host.ContributedToolNames())

	require.Len(t, host.RunHooks(), 1)
	require.Equal(t, "run_hook_1", host.RunHooks()[0].Name)
	require.Len(t, host.StepHooks(), 1)
	require.Equal(t, "step_hook_1", host.StepHooks()[0].Name)

	require.NotNil(t, host.GetPromptHook())
	require.Equal(t, "prompt_hook_1", host.GetPromptHook().Name)

	require.True(t, extTool.wasInitCalled())
	require.True(t, extHook.wasInitCalled())
	require.True(t, extPrompt.wasInitCalled())

	require.NoError(t, host.Shutdown(ctx))
	require.True(t, extTool.wasShutdownCalled())
	require.True(t, extHook.wasShutdownCalled())
	require.True(t, extPrompt.wasShutdownCalled())
}

func TestIntegrationHookOrder(t *testing.T) {
	ResetForTesting()

	ctx := context.Background()
	var mu sync.Mutex
	var order []string

	ext1 := newMockExtension("first").withRunHooks([]RunHook{
		{
			Name: "first_run_hook",
			OnRunStart: func(ctx context.Context, sessionID string, prompt string) error {
				mu.Lock()
				order = append(order, "first:start")
				mu.Unlock()
				return nil
			},
			OnRunEnd: func(ctx context.Context, sessionID string, result *fantasy.AgentResult, err error) error {
				return nil
			},
		},
	})

	ext2 := newMockExtension("second").withRunHooks([]RunHook{
		{
			Name: "second_run_hook",
			OnRunStart: func(ctx context.Context, sessionID string, prompt string) error {
				mu.Lock()
				order = append(order, "second:start")
				mu.Unlock()
				return nil
			},
			OnRunEnd: func(ctx context.Context, sessionID string, result *fantasy.AgentResult, err error) error {
				return nil
			},
		},
	})

	host := NewExtensionHost(HostDeps{})
	require.NoError(t, host.Register(ext1))
	require.NoError(t, host.Register(ext2))
	require.NoError(t, host.Bootstrap(ctx))

	hooks := host.RunHooks()
	require.Len(t, hooks, 2)
	require.Equal(t, "first_run_hook", hooks[0].Name)
	require.Equal(t, "second_run_hook", hooks[1].Name)

	for _, hook := range hooks {
		require.NoError(t, hook.OnRunStart(ctx, "test-session", "hello"))
	}

	mu.Lock()
	require.Equal(t, []string{"first:start", "second:start"}, order)
	mu.Unlock()
}

func TestIntegrationCompiledInExtensions(t *testing.T) {
	ResetForTesting()

	ctx := context.Background()
	mockTool := createMockTool("compiled_tool")

	compiledExt := newMockExtension("compiled_ext").
		withTools([]fantasy.AgentTool{mockTool}, []string{"compiled_tool"})
	RegisterExtension(compiledExt)

	host := NewExtensionHost(HostDeps{})
	runtimeExt := newMockExtension("runtime_ext")
	require.NoError(t, host.Register(runtimeExt))

	require.NoError(t, host.Bootstrap(ctx))

	require.True(t, compiledExt.wasInitCalled())
	require.True(t, runtimeExt.wasInitCalled())
	require.Equal(t, []string{"compiled_tool"}, host.ContributedToolNames())
}

func TestIntegrationBootstrapIdempotency(t *testing.T) {
	ResetForTesting()

	ctx := context.Background()
	host := NewExtensionHost(HostDeps{})
	require.NoError(t, host.Register(newMockExtension("ext1")))
	require.NoError(t, host.Bootstrap(ctx))

	err := host.Bootstrap(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already bootstrapped")
}

func TestIntegrationShutdownReverseOrder(t *testing.T) {
	ResetForTesting()

	ctx := context.Background()
	var mu sync.Mutex
	var shutdownOrder []string

	makeExt := func(name string) *mockExtension {
		return newMockExtension(name).withOnShutdown(func(ctx context.Context) error {
			mu.Lock()
			shutdownOrder = append(shutdownOrder, name)
			mu.Unlock()
			return nil
		})
	}

	host := NewExtensionHost(HostDeps{})
	require.NoError(t, host.Register(makeExt("a")))
	require.NoError(t, host.Register(makeExt("b")))
	require.NoError(t, host.Register(makeExt("c")))
	require.NoError(t, host.Bootstrap(ctx))
	require.NoError(t, host.Shutdown(ctx))

	mu.Lock()
	require.Equal(t, []string{"c", "b", "a"}, shutdownOrder)
	mu.Unlock()
}
