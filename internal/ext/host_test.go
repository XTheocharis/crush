package ext

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"charm.land/fantasy"

	"github.com/stretchr/testify/require"
)

type mockToolInput struct {
	Value string `json:"value,omitempty"`
}

type mockExtension struct {
	name            string
	initCalled      bool
	shutdownCalled  bool
	tools           []fantasy.AgentTool
	toolNames       []string
	runHooks        []RunHook
	stepHooks       []StepHook
	promptHook      *PromptHook
	initErr         error
	shutdownErr     error
	mu              sync.Mutex
	panicInInit     bool
	panicInShutdown bool
	panicInTools    bool
	onShutdown      func(context.Context) error
}

func (m *mockExtension) Name() string {
	return m.name
}

func (m *mockExtension) Init(ctx context.Context, host HostContext) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initCalled = true
	if m.panicInInit {
		panic("panic in Init for testing")
	}
	return m.initErr
}

func (m *mockExtension) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownCalled = true
	if m.panicInShutdown {
		panic("panic in Shutdown for testing")
	}
	if m.onShutdown != nil {
		if err := m.onShutdown(ctx); err != nil {
			return err
		}
	}
	return m.shutdownErr
}

func (m *mockExtension) Tools(ctx context.Context) ([]fantasy.AgentTool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.panicInTools {
		panic("panic in Tools for testing")
	}
	return m.tools, nil
}

func (m *mockExtension) ToolNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.toolNames
}

func (m *mockExtension) RunHooks() []RunHook {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runHooks
}

func (m *mockExtension) StepHooks() []StepHook {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stepHooks
}

func (m *mockExtension) PromptHook() *PromptHook {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.promptHook
}

func newMockExtension(name string) *mockExtension {
	return &mockExtension{
		name: name,
	}
}

func (m *mockExtension) withTools(tools []fantasy.AgentTool, names []string) *mockExtension {
	m.tools = tools
	m.toolNames = names
	return m
}

func (m *mockExtension) withRunHooks(hooks []RunHook) *mockExtension {
	m.runHooks = hooks
	return m
}

func (m *mockExtension) withStepHooks(hooks []StepHook) *mockExtension {
	m.stepHooks = hooks
	return m
}

func (m *mockExtension) withPromptHook(hook *PromptHook) *mockExtension {
	m.promptHook = hook
	return m
}

func (m *mockExtension) withInitError(err error) *mockExtension {
	m.initErr = err
	return m
}

func (m *mockExtension) withShutdownError(err error) *mockExtension {
	m.shutdownErr = err
	return m
}

func (m *mockExtension) withPanicInInit() *mockExtension {
	m.panicInInit = true
	return m
}

func (m *mockExtension) withPanicInShutdown() *mockExtension {
	m.panicInShutdown = true
	return m
}

func (m *mockExtension) withPanicInTools() *mockExtension {
	m.panicInTools = true
	return m
}

func (m *mockExtension) withOnShutdown(fn func(context.Context) error) *mockExtension {
	m.onShutdown = fn
	return m
}

func (m *mockExtension) wasInitCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.initCalled
}

func (m *mockExtension) wasShutdownCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownCalled
}

func createMockTool(name string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		name,
		fmt.Sprintf("Mock tool %s for testing", name),
		func(ctx context.Context, input mockToolInput, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.ToolResponse{}, nil
		},
	)
}

func setupTest(t *testing.T) {
	ResetForTesting()
}

func TestNoOpHost(t *testing.T) {
	setupTest(t)

	host := NewExtensionHost(HostDeps{})

	require.False(t, host.IsBootstrapped())
	require.Empty(t, host.ContributedTools())
	require.Empty(t, host.ContributedToolNames())
	require.Empty(t, host.RunHooks())
	require.Empty(t, host.StepHooks())
	require.Nil(t, host.GetPromptHook())
}

func TestBootstrapLifecycle(t *testing.T) {
	setupTest(t)

	tests := []struct {
		name          string
		extensions    []*mockExtension
		expectInit    []bool
		expectError   bool
		errorContains string
	}{
		{
			name:        "single extension",
			extensions:  []*mockExtension{newMockExtension("ext1")},
			expectInit:  []bool{true},
			expectError: false,
		},
		{
			name: "multiple extensions",
			extensions: []*mockExtension{
				newMockExtension("ext1"),
				newMockExtension("ext2"),
				newMockExtension("ext3"),
			},
			expectInit:  []bool{true, true, true},
			expectError: false,
		},
		{
			name: "extension init error",
			extensions: []*mockExtension{
				newMockExtension("ext1"),
				newMockExtension("ext2").withInitError(errors.New("init failed")),
			},
			expectInit:    []bool{true, true},
			expectError:   true,
			errorContains: "init failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := NewExtensionHost(HostDeps{})
			for _, ext := range tt.extensions {
				err := host.Register(ext)
				require.NoError(t, err)
			}

			ctx := context.Background()
			err := host.Bootstrap(ctx)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			require.True(t, host.IsBootstrapped())

			for i, ext := range tt.extensions {
				if i < len(tt.expectInit) && tt.expectInit[i] {
					require.True(t, ext.wasInitCalled(), "extension %d should have been inited", i)
				}
			}

			err = host.Shutdown(ctx)
			require.NoError(t, err)
			for _, ext := range tt.extensions {
				require.True(t, ext.wasShutdownCalled(), "extension %s should have been shutdown", ext.Name())
			}
		})
	}
}

func TestToolContribution(t *testing.T) {
	setupTest(t)

	ctx := context.Background()

	mockTool1 := createMockTool("mock_tool_1")
	mockTool2 := createMockTool("mock_tool_2")

	tests := []struct {
		name            string
		extensions      []*mockExtension
		expectToolCount int
		expectToolNames []string
		expectError     bool
		errorContains   string
	}{
		{
			name: "single tool provider",
			extensions: []*mockExtension{
				newMockExtension("tool_ext").withTools([]fantasy.AgentTool{mockTool1}, []string{"mock_tool_1"}),
			},
			expectToolCount: 1,
			expectToolNames: []string{"mock_tool_1"},
			expectError:     false,
		},
		{
			name: "multiple tools from single provider",
			extensions: []*mockExtension{
				newMockExtension("tool_ext").withTools([]fantasy.AgentTool{mockTool1, mockTool2}, []string{"mock_tool_1", "mock_tool_2"}),
			},
			expectToolCount: 2,
			expectToolNames: []string{"mock_tool_1", "mock_tool_2"},
			expectError:     false,
		},
		{
			name: "multiple tool providers",
			extensions: []*mockExtension{
				newMockExtension("tool_ext1").withTools([]fantasy.AgentTool{mockTool1}, []string{"mock_tool_1"}),
				newMockExtension("tool_ext2").withTools([]fantasy.AgentTool{mockTool2}, []string{"mock_tool_2"}),
			},
			expectToolCount: 2,
			expectToolNames: []string{"mock_tool_1", "mock_tool_2"},
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := NewExtensionHost(HostDeps{})
			for _, ext := range tt.extensions {
				err := host.Register(ext)
				require.NoError(t, err)
			}

			err := host.Bootstrap(ctx)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)

			tools := host.ContributedTools()
			toolNames := host.ContributedToolNames()

			require.Len(t, tools, tt.expectToolCount)
			require.Len(t, toolNames, tt.expectToolCount)

			if tt.expectToolNames != nil {
				require.Equal(t, tt.expectToolNames, toolNames)
			}

			for _, name := range toolNames {
				found := false
				for _, tool := range tools {
					if tool.Info().Name == name {
						found = true
						break
					}
				}
				require.True(t, found, "tool %s should be in contributed tools", name)
			}
		})
	}
}

func TestRunHooks(t *testing.T) {
	setupTest(t)

	ctx := context.Background()

	tests := []struct {
		name            string
		extensions      []*mockExtension
		expectHookCount int
	}{
		{
			name: "single run hook provider",
			extensions: []*mockExtension{
				newMockExtension("run_hook_ext").withRunHooks([]RunHook{
					{
						Name: "hook1",
						OnRunStart: func(ctx context.Context, sessionID string, prompt string) error {
							return nil
						},
						OnRunEnd: func(ctx context.Context, sessionID string, result *fantasy.AgentResult, err error) error {
							return nil
						},
					},
				}),
			},
			expectHookCount: 1,
		},
		{
			name: "multiple run hooks from single provider",
			extensions: []*mockExtension{
				newMockExtension("run_hook_ext").withRunHooks([]RunHook{
					{
						Name: "hook1",
						OnRunStart: func(ctx context.Context, sessionID string, prompt string) error {
							return nil
						},
						OnRunEnd: func(ctx context.Context, sessionID string, result *fantasy.AgentResult, err error) error {
							return nil
						},
					},
					{
						Name: "hook2",
						OnRunStart: func(ctx context.Context, sessionID string, prompt string) error {
							return nil
						},
						OnRunEnd: func(ctx context.Context, sessionID string, result *fantasy.AgentResult, err error) error {
							return nil
						},
					},
				}),
			},
			expectHookCount: 2,
		},
		{
			name: "multiple run hook providers",
			extensions: []*mockExtension{
				newMockExtension("run_hook_ext1").withRunHooks([]RunHook{
					{
						Name: "hook1",
						OnRunStart: func(ctx context.Context, sessionID string, prompt string) error {
							return nil
						},
						OnRunEnd: func(ctx context.Context, sessionID string, result *fantasy.AgentResult, err error) error {
							return nil
						},
					},
				}),
				newMockExtension("run_hook_ext2").withRunHooks([]RunHook{
					{
						Name: "hook2",
						OnRunStart: func(ctx context.Context, sessionID string, prompt string) error {
							return nil
						},
						OnRunEnd: func(ctx context.Context, sessionID string, result *fantasy.AgentResult, err error) error {
							return nil
						},
					},
				}),
			},
			expectHookCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := NewExtensionHost(HostDeps{})
			for _, ext := range tt.extensions {
				err := host.Register(ext)
				require.NoError(t, err)
			}

			err := host.Bootstrap(ctx)
			require.NoError(t, err)

			hooks := host.RunHooks()
			require.Len(t, hooks, tt.expectHookCount)
		})
	}
}

func TestStepHooks(t *testing.T) {
	setupTest(t)

	ctx := context.Background()

	tests := []struct {
		name            string
		extensions      []*mockExtension
		expectHookCount int
	}{
		{
			name: "single step hook provider",
			extensions: []*mockExtension{
				newMockExtension("step_hook_ext").withStepHooks([]StepHook{
					{
						Name: "step_hook1",
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
				}),
			},
			expectHookCount: 1,
		},
		{
			name: "multiple step hooks",
			extensions: []*mockExtension{
				newMockExtension("step_hook_ext").withStepHooks([]StepHook{
					{
						Name: "step_hook1",
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
					{
						Name: "step_hook2",
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
				}),
			},
			expectHookCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := NewExtensionHost(HostDeps{})
			for _, ext := range tt.extensions {
				err := host.Register(ext)
				require.NoError(t, err)
			}

			err := host.Bootstrap(ctx)
			require.NoError(t, err)

			hooks := host.StepHooks()
			require.Len(t, hooks, tt.expectHookCount)
		})
	}
}

func TestPromptHook(t *testing.T) {
	setupTest(t)

	ctx := context.Background()

	tests := []struct {
		name             string
		extensions       []*mockExtension
		expectPromptHook bool
		expectError      bool
		errorContains    string
	}{
		{
			name: "single prompt hook provider",
			extensions: []*mockExtension{
				newMockExtension("prompt_hook_ext").withPromptHook(&PromptHook{
					Name: "prompt_hook1",
					OnPreparePrompt: func(ctx context.Context, sessionID string, messages []fantasy.Message) ([]fantasy.Message, error) {
						return messages, nil
					},
					SystemPromptModifier: func(ctx context.Context, sessionID string, systemPrompt string) (string, error) {
						return systemPrompt, nil
					},
				}),
			},
			expectPromptHook: true,
			expectError:      false,
		},
		{
			name: "no prompt hook",
			extensions: []*mockExtension{
				newMockExtension("no_prompt_ext"),
			},
			expectPromptHook: false,
			expectError:      false,
		},
		{
			name: "multiple prompt hooks should error",
			extensions: []*mockExtension{
				newMockExtension("prompt_hook_ext1").withPromptHook(&PromptHook{
					Name: "prompt_hook1",
				}),
				newMockExtension("prompt_hook_ext2").withPromptHook(&PromptHook{
					Name: "prompt_hook2",
				}),
			},
			expectPromptHook: false,
			expectError:      true,
			errorContains:    "multiple prompt hooks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := NewExtensionHost(HostDeps{})
			for _, ext := range tt.extensions {
				err := host.Register(ext)
				require.NoError(t, err)
			}

			err := host.Bootstrap(ctx)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)

			hook := host.GetPromptHook()
			if tt.expectPromptHook {
				require.NotNil(t, hook)
			} else {
				require.Nil(t, hook)
			}
		})
	}
}

func TestPanicRecovery(t *testing.T) {
	setupTest(t)

	tests := []struct {
		name                 string
		extensions           []*mockExtension
		expectBootstrapErr   bool
		bootstrapErrContains string
		expectShutdownErr    bool
		shutdownErrContains  string
	}{
		{
			name: "panic in Init",
			extensions: []*mockExtension{
				newMockExtension("panic_init_ext").withPanicInInit(),
			},
			expectBootstrapErr:   true,
			bootstrapErrContains: "panicked",
			expectShutdownErr:    false,
		},
		{
			name: "panic in Shutdown",
			extensions: []*mockExtension{
				newMockExtension("panic_shutdown_ext").withPanicInShutdown(),
			},
			expectBootstrapErr:  false,
			expectShutdownErr:   true,
			shutdownErrContains: "panicked",
		},
		{
			name: "panic in Tools",
			extensions: []*mockExtension{
				newMockExtension("panic_tools_ext").withTools([]fantasy.AgentTool{createMockTool("mock_tool")}, []string{"mock_tool"}).withPanicInTools(),
			},
			expectBootstrapErr:   true,
			bootstrapErrContains: "panicked",
			expectShutdownErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := NewExtensionHost(HostDeps{})
			for _, ext := range tt.extensions {
				err := host.Register(ext)
				require.NoError(t, err)
			}

			ctx := context.Background()
			err := host.Bootstrap(ctx)

			if tt.expectBootstrapErr {
				require.Error(t, err)
				if tt.bootstrapErrContains != "" {
					require.Contains(t, err.Error(), tt.bootstrapErrContains)
				}
			} else {
				require.NoError(t, err)
			}

			err = host.Shutdown(ctx)
			if tt.expectShutdownErr {
				require.Error(t, err)
				if tt.shutdownErrContains != "" {
					require.Contains(t, err.Error(), tt.shutdownErrContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDuplicateToolNames(t *testing.T) {
	setupTest(t)

	ctx := context.Background()

	mockTool := createMockTool("duplicate_tool")

	host := NewExtensionHost(HostDeps{})
	ext1 := newMockExtension("ext1").withTools([]fantasy.AgentTool{mockTool}, []string{"duplicate_tool"})
	ext2 := newMockExtension("ext2").withTools([]fantasy.AgentTool{createMockTool("duplicate_tool")}, []string{"duplicate_tool"})

	err := host.Register(ext1)
	require.NoError(t, err)

	err = host.Register(ext2)
	require.NoError(t, err)

	err = host.Bootstrap(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate tool name")
	require.Contains(t, err.Error(), "ext1")
	require.Contains(t, err.Error(), "ext2")
}

func TestConcurrentAccess(t *testing.T) {
	setupTest(t)

	ctx := context.Background()

	mockTool := createMockTool("concurrent_tool")

	host := NewExtensionHost(HostDeps{})
	ext := newMockExtension("concurrent_ext").withTools([]fantasy.AgentTool{mockTool}, []string{"concurrent_tool"})
	err := host.Register(ext)
	require.NoError(t, err)

	err = host.Bootstrap(ctx)
	require.NoError(t, err)

	var wg sync.WaitGroup
	iterations := 100

	for range iterations {
		wg.Go(func() {
			tools := host.ContributedTools()
			require.Len(t, tools, 1)

			names := host.ContributedToolNames()
			require.Len(t, names, 1)
			require.Equal(t, "concurrent_tool", names[0])

			hooks := host.RunHooks()
			require.Empty(t, hooks)

			stepHooks := host.StepHooks()
			require.Empty(t, stepHooks)

			promptHook := host.GetPromptHook()
			require.Nil(t, promptHook)

			isBootstrapped := host.IsBootstrapped()
			require.True(t, isBootstrapped)
		})
	}

	wg.Wait()
}

func TestShutdownOrder(t *testing.T) {
	setupTest(t)

	ctx := context.Background()

	var shutdownOrder []string

	ext1 := newMockExtension("ext1").withOnShutdown(func(ctx context.Context) error {
		shutdownOrder = append(shutdownOrder, "ext1")
		return nil
	})
	ext2 := newMockExtension("ext2").withOnShutdown(func(ctx context.Context) error {
		shutdownOrder = append(shutdownOrder, "ext2")
		return nil
	})
	ext3 := newMockExtension("ext3").withOnShutdown(func(ctx context.Context) error {
		shutdownOrder = append(shutdownOrder, "ext3")
		return nil
	})

	host := NewExtensionHost(HostDeps{})
	err := host.Register(ext1)
	require.NoError(t, err)
	err = host.Register(ext2)
	require.NoError(t, err)
	err = host.Register(ext3)
	require.NoError(t, err)

	err = host.Bootstrap(ctx)
	require.NoError(t, err)

	err = host.Shutdown(ctx)
	require.NoError(t, err)

	require.Equal(t, []string{"ext3", "ext2", "ext1"}, shutdownOrder)
}

func TestRegisterAfterBootstrap(t *testing.T) {
	setupTest(t)

	ctx := context.Background()

	host := NewExtensionHost(HostDeps{})
	ext1 := newMockExtension("ext1")
	err := host.Register(ext1)
	require.NoError(t, err)

	err = host.Bootstrap(ctx)
	require.NoError(t, err)

	ext2 := newMockExtension("ext2")
	err = host.Register(ext2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot register extension")
	require.Contains(t, err.Error(), "after bootstrap")
}

func TestRefreshContributedTools_basic(t *testing.T) {
	setupTest(t)

	ctx := context.Background()

	tool1 := createMockTool("refresh_tool_1")
	ext := newMockExtension("refresh_ext").withTools([]fantasy.AgentTool{tool1}, []string{"refresh_tool_1"})

	host := NewExtensionHost(HostDeps{})
	require.NoError(t, host.Register(ext))
	require.NoError(t, host.Bootstrap(ctx))

	require.Equal(t, []string{"refresh_tool_1"}, host.ContributedToolNames())
	require.Len(t, host.ContributedTools(), 1)

	tool2 := createMockTool("refresh_tool_2")
	ext.mu.Lock()
	ext.tools = []fantasy.AgentTool{tool1, tool2}
	ext.toolNames = []string{"refresh_tool_1", "refresh_tool_2"}
	ext.mu.Unlock()

	require.NoError(t, host.RefreshContributedTools(ctx))

	require.Equal(t, []string{"refresh_tool_1", "refresh_tool_2"}, host.ContributedToolNames())
	require.Len(t, host.ContributedTools(), 2)
}

func TestRefreshContributedTools_threadSafety(t *testing.T) {
	setupTest(t)

	ctx := context.Background()

	tool1 := createMockTool("ts_tool_1")
	ext := newMockExtension("ts_ext").withTools([]fantasy.AgentTool{tool1}, []string{"ts_tool_1"})

	host := NewExtensionHost(HostDeps{})
	require.NoError(t, host.Register(ext))
	require.NoError(t, host.Bootstrap(ctx))

	var wg sync.WaitGroup
	iterations := 50

	for i := range iterations {
		wg.Go(func() {
			_ = host.ContributedTools()
			_ = host.ContributedToolNames()
		})

		if i%10 == 0 {
			wg.Go(func() {
				_ = host.RefreshContributedTools(ctx)
			})
		}
	}

	wg.Wait()

	require.Equal(t, []string{"ts_tool_1"}, host.ContributedToolNames())
}

func TestRefreshContributedTools_beforeBootstrap(t *testing.T) {
	setupTest(t)

	host := NewExtensionHost(HostDeps{})
	err := host.RefreshContributedTools(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "before bootstrap")
}

func TestResetForTesting(t *testing.T) {
	ResetForTesting()

	ext1 := newMockExtension("compiled_ext1")
	RegisterExtension(ext1)

	ResetForTesting()

	host := NewExtensionHost(HostDeps{})
	ext2 := newMockExtension("runtime_ext")
	err := host.Register(ext2)
	require.NoError(t, err)

	ctx := context.Background()
	err = host.Bootstrap(ctx)
	require.NoError(t, err)

	require.Len(t, host.ContributedTools(), 0)

	ResetForTesting()
}

func TestExtensionHost_StopConditionFlag(t *testing.T) {
	t.Parallel()

	h := NewExtensionHost(HostDeps{})
	require.False(t, h.WasStoppedByCondition())

	h.MarkStoppedByCondition()
	require.True(t, h.WasStoppedByCondition())

	h.ClearStoppedByCondition()
	require.False(t, h.WasStoppedByCondition())

	var nilHost *ExtensionHost
	require.False(t, nilHost.WasStoppedByCondition())
	nilHost.MarkStoppedByCondition()
	nilHost.ClearStoppedByCondition()
}
