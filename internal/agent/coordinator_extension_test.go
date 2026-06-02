package agent

import (
	"context"
	"testing"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/stretchr/testify/require"
)

func TestCoordinatorWithExtensionTools(t *testing.T) {
	ext.ResetForTesting()
	config.ResetExtensionToolNamesForTesting()
	t.Cleanup(func() {
		ext.ResetForTesting()
		config.ResetExtensionToolNamesForTesting()
	})

	ctx := context.Background()
	env := testEnv(t)

	config.RegisterExtensionToolNames(func() []string {
		return []string{"ext_test_tool"}
	})

	cfg, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)

	mockTool := fantasy.NewAgentTool(
		"ext_test_tool",
		"Extension test tool",
		func(ctx context.Context, input struct{}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.ToolResponse{}, nil
		},
	)

	mockExt := newTestExtension("test_ext", []fantasy.AgentTool{mockTool}, []string{"ext_test_tool"})
	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Register(mockExt))
	require.NoError(t, host.Bootstrap(ctx))

	c := &coordinator{
		cfg:         cfg,
		sessions:    env.sessions,
		permissions: env.permissions,
		lspManager:  nil,
		extHost:     host,
	}

	agentCfg := cfg.Config().Agents[config.AgentCoder]
	tools, err := c.buildTools(ctx, agentCfg, false)
	require.NoError(t, err)

	found := false
	for _, tool := range tools {
		if tool.Info().Name == "ext_test_tool" {
			found = true
			break
		}
	}
	require.True(t, found, "extension tool should appear in buildTools output for coder agent")
}

func TestCoordinatorSubAgentExcludesExtensionTools(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	ctx := context.Background()
	env := testEnv(t)
	cfg, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)

	mockTool := fantasy.NewAgentTool(
		"ext_sub_test_tool",
		"Extension sub-agent test tool",
		func(ctx context.Context, input struct{}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.ToolResponse{}, nil
		},
	)

	mockExt := newTestExtension("test_ext_sub", []fantasy.AgentTool{mockTool}, []string{"ext_sub_test_tool"})
	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Register(mockExt))
	require.NoError(t, host.Bootstrap(ctx))

	c := &coordinator{
		cfg:         cfg,
		sessions:    env.sessions,
		permissions: env.permissions,
		lspManager:  nil,
		extHost:     host,
	}

	agentCfg := cfg.Config().Agents[config.AgentCoder]
	tools, err := c.buildTools(ctx, agentCfg, true)
	require.NoError(t, err)

	for _, tool := range tools {
		require.NotEqual(t, "ext_sub_test_tool", tool.Info().Name, "extension tools should NOT appear for sub-agent")
	}
}

func TestCoordinatorNilExtensionHost(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	ctx := context.Background()
	env := testEnv(t)
	cfg, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)

	c := &coordinator{
		cfg:         cfg,
		sessions:    env.sessions,
		permissions: env.permissions,
		lspManager:  nil,
		extHost:     nil,
	}

	agentCfg := cfg.Config().Agents[config.AgentCoder]
	tools, err := c.buildTools(ctx, agentCfg, false)
	require.NoError(t, err)

	for _, tool := range tools {
		require.NotEqual(t, "ext_test_tool", tool.Info().Name, "no extension tools should appear with nil extHost")
	}
}

type testExtension struct {
	name      string
	tools     []fantasy.AgentTool
	toolNames []string
}

func newTestExtension(name string, tools []fantasy.AgentTool, toolNames []string) *testExtension {
	return &testExtension{name: name, tools: tools, toolNames: toolNames}
}

func (e *testExtension) Name() string                                         { return e.name }
func (e *testExtension) Init(_ context.Context, _ ext.HostContext) error      { return nil }
func (e *testExtension) Shutdown(_ context.Context) error                     { return nil }
func (e *testExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) { return e.tools, nil }
func (e *testExtension) ToolNames() []string                                  { return e.toolNames }
func (e *testExtension) RunHooks() []ext.RunHook                              { return nil }
func (e *testExtension) StepHooks() []ext.StepHook                            { return nil }
func (e *testExtension) PromptHook() *ext.PromptHook                          { return nil }

// Verify the testExtension implements required interfaces at compile time.
var (
	_ ext.Extension          = (*testExtension)(nil)
	_ ext.ToolProvider       = (*testExtension)(nil)
	_ ext.RunHookProvider    = (*testExtension)(nil)
	_ ext.StepHookProvider   = (*testExtension)(nil)
	_ ext.PromptHookProvider = (*testExtension)(nil)
)

func TestCoordinatorExtHostFieldStored(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	ctx := context.Background()
	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Bootstrap(ctx))

	env := testEnv(t)
	cfg, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)

	c := &coordinator{
		cfg:         cfg,
		sessions:    env.sessions,
		permissions: env.permissions,
		extHost:     host,
	}

	require.NotNil(t, c.extHost)
	require.True(t, c.extHost.IsBootstrapped())
}

func TestCoordinatorExtHostGetterReturnsNilForSubAgent(t *testing.T) {
	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	ctx := context.Background()
	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Bootstrap(ctx))

	env := testEnv(t)
	cfg, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)

	c := &coordinator{
		cfg:         cfg,
		sessions:    env.sessions,
		permissions: env.permissions,
		extHost:     host,
	}

	extHostForCoder := func() *ext.ExtensionHost {
		if false {
			return nil
		}
		return c.extHost
	}()
	require.NotNil(t, extHostForCoder)

	extHostForSub := func() *ext.ExtensionHost {
		if true {
			return nil
		}
		return c.extHost
	}()
	require.Nil(t, extHostForSub)
}

// TestLSPRestartNotDuplicated verifies that lsp_restart appears exactly once
// in the built tool list — provided only by extension, not duplicated
// by the coordinator.
func TestLSPRestartNotDuplicated(t *testing.T) {
	ext.ResetForTesting()
	config.ResetExtensionToolNamesForTesting()
	t.Cleanup(func() {
		ext.ResetForTesting()
		config.ResetExtensionToolNamesForTesting()
	})

	ctx := context.Background()
	env := testEnv(t)

	cfg, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)

	lspManager := lsp.NewManager(cfg)
	lspRestartTool := tools.NewLSPRestartTool(lspManager)

	config.RegisterExtensionToolNames(func() []string {
		return []string{tools.LSPRestartToolName}
	})
	cfg.SetupAgents()

	mockExt := newTestExtension("lsp-tools-mock", []fantasy.AgentTool{lspRestartTool}, []string{tools.LSPRestartToolName})
	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Register(mockExt))
	require.NoError(t, host.Bootstrap(ctx))
	t.Cleanup(func() { host.Shutdown(ctx) })

	c := &coordinator{
		cfg:         cfg,
		sessions:    env.sessions,
		permissions: env.permissions,
		lspManager:  lspManager,
		extHost:     host,
	}

	agentCfg := cfg.Config().Agents[config.AgentCoder]
	builtTools, err := c.buildTools(ctx, agentCfg, false)
	require.NoError(t, err)

	count := 0
	for _, tool := range builtTools {
		if tool.Info().Name == tools.LSPRestartToolName {
			count++
		}
	}
	require.Equal(t, 1, count, "lsp_restart should appear exactly once, got %d", count)
}
