package extensions

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestToolSurfaceExtension_Name(t *testing.T) {
	e := &ToolSurfaceExtension{}
	require.Equal(t, "tool-surface", e.Name())
}

func TestToolSurfaceExtension_InitAndShutdown(t *testing.T) {
	e := &ToolSurfaceExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.True(t, e.active)
	require.NotNil(t, e.surface)

	err = e.Shutdown(context.Background())
	require.NoError(t, err)
	require.False(t, e.active)
	require.Nil(t, e.surface)
}

func TestToolSurfaceExtension_RunHooksInactive(t *testing.T) {
	e := &ToolSurfaceExtension{}
	require.Nil(t, e.RunHooks())
}

func TestToolSurfaceExtension_OnRunStart_AllFlagsFalseByDefault(t *testing.T) {
	e := &ToolSurfaceExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	hooks := e.RunHooks()
	require.Len(t, hooks, 1)

	err = hooks[0].OnRunStart(context.Background(), "session", "")
	require.NoError(t, err)

	require.False(t, e.surface.IsVisible("lsp_diagnostics"))
	require.False(t, e.surface.IsVisible("lcm_grep"))
	require.False(t, e.surface.IsVisible("list_mcp_resources"))
}

func TestToolSurfaceExtension_OnRunStart_NilAfterShutdown(t *testing.T) {
	e := &ToolSurfaceExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	err = e.Shutdown(context.Background())
	require.NoError(t, err)

	hooks := e.RunHooks()
	require.Nil(t, hooks)
}

func TestToolSurfaceExtension_OnRunStart_LCMFlagFromSingleton(t *testing.T) {
	origLCM := TheLCMExtension
	defer func() { TheLCMExtension = origLCM }()

	TheLCMExtension = &LCMExtension{}

	e := &ToolSurfaceExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	require.NoError(t, e.Init(context.Background(), host))

	hooks := e.RunHooks()
	require.Len(t, hooks, 1)
	require.NoError(t, hooks[0].OnRunStart(context.Background(), "session", ""))

	require.False(t, e.surface.IsVisible("lcm_grep"))
}

func TestToolSurfaceExtension_OnRunStart_RepomapFlagFromSingleton(t *testing.T) {
	origRepo := TheRepomapExtension
	defer func() { TheRepomapExtension = origRepo }()

	TheRepomapExtension = &RepomapExtension{}

	e := &ToolSurfaceExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	require.NoError(t, e.Init(context.Background(), host))

	hooks := e.RunHooks()
	require.Len(t, hooks, 1)
	require.NoError(t, hooks[0].OnRunStart(context.Background(), "session", ""))

	require.False(t, e.surface.IsVisible("llm_map"))
	require.False(t, e.surface.IsVisible("agentic_map"))
	require.False(t, e.surface.IsVisible("map_refresh"))
}

func TestToolSurfaceExtension_OnRunEnd_NoOp(t *testing.T) {
	e := &ToolSurfaceExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	require.NoError(t, e.Init(context.Background(), host))

	hooks := e.RunHooks()
	require.Len(t, hooks, 1)
	require.NoError(t, hooks[0].OnRunEnd(context.Background(), "session", nil, nil))
}
