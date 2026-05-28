package agent

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/stretchr/testify/require"
)

// fakeTool records the context it was invoked with so tests can assert on
// values stamped onto it by the hookedTool decorator.
type fakeTool struct {
	name   string
	called bool
	gotCtx context.Context
	resp   fantasy.ToolResponse
}

func (f *fakeTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{Name: f.name}
}

func (f *fakeTool) Run(ctx context.Context, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	f.called = true
	f.gotCtx = ctx
	return f.resp, nil
}

func (f *fakeTool) ProviderOptions() fantasy.ProviderOptions     { return nil }
func (f *fakeTool) SetProviderOptions(_ fantasy.ProviderOptions) {}

// newPreRunner builds a hooks.Runner from a single HookConfig for
// PreToolUse, running the config-loader path that compiles the matcher
// regex.
func newPreRunner(t *testing.T, cmd string) *hooks.Runner {
	t.Helper()
	cfg := &config.Config{
		Hooks: map[string][]config.HookConfig{
			hooks.EventPreToolUse: {{Command: cmd}},
		},
	}
	require.NoError(t, cfg.ValidateHooks())
	return hooks.NewRunner(cfg.Hooks[hooks.EventPreToolUse], t.TempDir(), t.TempDir())
}

// newPostRunner builds a hooks.Runner from a single HookConfig for
// PostToolUse.
func newPostRunner(t *testing.T, cmd string) *hooks.Runner {
	t.Helper()
	cfg := &config.Config{
		Hooks: map[string][]config.HookConfig{
			hooks.EventPostToolUse: {{Command: cmd}},
		},
	}
	require.NoError(t, cfg.ValidateHooks())
	return hooks.NewRunner(cfg.Hooks[hooks.EventPostToolUse], t.TempDir(), t.TempDir())
}

func TestHookedTool_AllowStampsHookApproval(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	preRunner := newPreRunner(t, `echo '{"decision":"allow"}'`)
	tool := newHookedTool(inner, preRunner, nil)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-1", Name: "view"})
	require.NoError(t, err)
	require.True(t, inner.called, "inner tool should have run")

	// The inner tool's permission service can now treat call-1 as pre-approved.
	svc := permission.NewPermissionService(t.TempDir(), false, nil)
	granted, err := svc.Request(inner.gotCtx, permission.CreatePermissionRequest{
		SessionID:  "s1",
		ToolCallID: "call-1",
		ToolName:   "view",
		Action:     "read",
		Path:       t.TempDir(),
	})
	require.NoError(t, err)
	require.True(t, granted, "hook allow should bypass the permission prompt")
}

func TestHookedTool_SilentDoesNotStampApproval(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	preRunner := newPreRunner(t, `exit 0`) // no stdout, no decision
	tool := newHookedTool(inner, preRunner, nil)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-2", Name: "view"})
	require.NoError(t, err)
	require.True(t, inner.called)

	// With no hook opinion, a fresh permission request has nothing stamped
	// and must fall through to the normal flow. We verify by checking that
	// the context does not look pre-approved for this call ID: sending a
	// request that no subscriber resolves will block until cancelled.
	svc := permission.NewPermissionService(t.TempDir(), false, nil)
	ctx, cancel := context.WithCancel(inner.gotCtx)
	cancel()
	granted, err := svc.Request(ctx, permission.CreatePermissionRequest{
		SessionID:  "s1",
		ToolCallID: "call-2",
		ToolName:   "view",
		Action:     "read",
		Path:       t.TempDir(),
	})
	require.Error(t, err, "no approval stamped => request should reach the prompt path")
	require.False(t, granted)
}

func TestHookedTool_DenySkipsInnerTool(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash"}
	preRunner := newPreRunner(t, `echo "blocked" >&2; exit 2`)
	tool := newHookedTool(inner, preRunner, nil)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-3", Name: "bash"})
	require.NoError(t, err)
	require.False(t, inner.called, "denied call must not reach the inner tool")
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "blocked")
}

func TestWrapToolsWithHooks(t *testing.T) {
	t.Parallel()

	preRunner := newPreRunner(t, `exit 0`)
	inputs := []fantasy.AgentTool{&fakeTool{name: "a"}, &fakeTool{name: "b"}}

	t.Run("top-level agent wraps every tool", func(t *testing.T) {
		t.Parallel()
		out := wrapToolsWithHooks(inputs, preRunner, nil, false)
		require.Len(t, out, len(inputs))
		for i, tool := range out {
			_, ok := tool.(*hookedTool)
			require.Truef(t, ok, "tool %d should be a *hookedTool", i)
		}
	})

	t.Run("sub-agent skips the wrap", func(t *testing.T) {
		t.Parallel()
		out := wrapToolsWithHooks(inputs, preRunner, nil, true)
		require.Equal(t, inputs, out, "sub-agent tools should be returned unwrapped")
		for _, tool := range out {
			_, isHooked := tool.(*hookedTool)
			require.False(t, isHooked, "sub-agent tool should not be wrapped")
		}
	})

	t.Run("both nil runners skips the wrap for both agent kinds", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, inputs, wrapToolsWithHooks(inputs, nil, nil, false))
		require.Equal(t, inputs, wrapToolsWithHooks(inputs, nil, nil, true))
	})

	t.Run("only postRunner wraps tools", func(t *testing.T) {
		t.Parallel()
		postRunner := newPostRunner(t, `exit 0`)
		out := wrapToolsWithHooks(inputs, nil, postRunner, false)
		require.Len(t, out, len(inputs))
		for i, tool := range out {
			_, ok := tool.(*hookedTool)
			require.Truef(t, ok, "tool %d should be a *hookedTool with only postRunner", i)
		}
	})
}

func TestPreToolUseHookFires(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("ok")}
	preRunner := newPreRunner(t, `echo '{"decision":"allow"}'`)
	// Only preRunner configured; postRunner is nil.
	tool := newHookedTool(inner, preRunner, nil)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-pre", Name: "bash", Input: `{}`})
	require.NoError(t, err)
	require.True(t, inner.called, "inner tool should have run after pre-hook allow")
	require.Equal(t, "ok", resp.Content)
}

func TestPostToolUseHookFires(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("original output")}
	// Only postRunner configured; preRunner is nil.
	postRunner := newPostRunner(t, `echo '{"modified_output":"rewritten by post-hook"}'`)
	tool := newHookedTool(inner, nil, postRunner)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-post", Name: "bash", Input: `{}`})
	require.NoError(t, err)
	require.True(t, inner.called, "inner tool should have run (no pre-hook to block it)")
	require.Equal(t, "rewritten by post-hook", resp.Content, "post-hook should rewrite output")
}

func TestBothHookRunnersFire(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("raw")}
	preRunner := newPreRunner(t, `echo '{"decision":"allow"}'`)
	postRunner := newPostRunner(t, `echo '{"modified_output":"post-processed"}'`)
	tool := newHookedTool(inner, preRunner, postRunner)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-both", Name: "bash", Input: `{}`})
	require.NoError(t, err)
	require.True(t, inner.called)
	require.Equal(t, "post-processed", resp.Content, "post-hook should rewrite the inner tool output")
}
