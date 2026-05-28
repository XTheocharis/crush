package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/stretchr/testify/require"
)

// TestIntegration_ModelRoutingWithHooks verifies that PreToolUse hooks,
// model routing, and PostToolUse hooks all fire in the correct order
// without cross-contamination.
func TestIntegration_ModelRoutingWithHooks(t *testing.T) {
	t.Parallel()

	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	// Step 1: Create separate PreToolUse and PostToolUse hook runners.
	preRunner := newPreRunner(t, `echo '{"decision":"allow"}'`)
	postRunner := newPostRunner(t, `echo '{"modified_output":"post-processed"}'`)

	// Step 2: Set up a mock model router extension that returns "small"
	// for short inputs.
	router := &mockModelRouterExt{}
	ext.RegisterExtension(router)

	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Bootstrap(context.Background()))

	// Feed a short message so the router selects "small".
	_, err := router.StepHooks()[0].OnPrepareStep(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage("hi"),
	})
	require.NoError(t, err)

	// Step 3: Create hooked tools with both runners.
	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("raw output")}
	tool := newHookedTool(inner, preRunner, postRunner)

	// Step 4: Verify PreToolUse hook fires before the tool.
	resp, err := tool.Run(t.Context(), fantasy.ToolCall{
		ID:    "call-int-1",
		Name:  "bash",
		Input: `{}`,
	})
	require.NoError(t, err)
	require.True(t, inner.called, "inner tool should have run after pre-hook allow")

	// Step 5: Verify the routing decision returns small for short input.
	m := agentHookMediator{host: host}
	routedType := m.getRoutedModelType()
	require.Equal(t, config.SelectedModelTypeSmall, routedType,
		"short input should route to small model")

	// Step 6: Verify PostToolUse hook fires after the tool (rewrites output).
	require.Equal(t, "post-processed", resp.Content,
		"post-hook should rewrite the inner tool output")

	// Step 7: Verify no cross-event firing — create a tool with only a
	// preRunner and confirm post-processing does not occur.
	inner2 := &fakeTool{name: "view", resp: fantasy.NewTextResponse("original")}
	toolPreOnly := newHookedTool(inner2, preRunner, nil)

	resp2, err := toolPreOnly.Run(t.Context(), fantasy.ToolCall{
		ID:    "call-int-2",
		Name:  "view",
		Input: `{}`,
	})
	require.NoError(t, err)
	require.True(t, inner2.called)
	require.Equal(t, "original", resp2.Content,
		"without postRunner, output should be unchanged")
}

// TestIntegration_BothHooksConfigured verifies that when both Pre and Post
// hook runners are configured, each fires only for its own event type,
// and tools are correctly wrapped.
func TestIntegration_BothHooksConfigured(t *testing.T) {
	t.Parallel()

	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	// 1. Both runners are non-nil.
	preRunner := newPreRunner(t, `echo '{"decision":"allow"}'`)
	postRunner := newPostRunner(t, `echo '{"modified_output":"rewritten"}'`)

	require.NotNil(t, preRunner, "preRunner should be non-nil")
	require.NotNil(t, postRunner, "postRunner should be non-nil")

	// 2. Each fires only for its own event type — tracked via counters.
	var preFired, postFired atomic.Int32

	// Pre-only tool: pre-hook fires, no post-processing.
	innerPre := &trackingFakeTool{
		name: "pre_only",
		resp: fantasy.NewTextResponse("pre-output"),
		onRun: func() {
			preFired.Add(1)
		},
	}
	toolPre := newHookedTool(innerPre, preRunner, nil)
	resp, err := toolPre.Run(t.Context(), fantasy.ToolCall{
		ID: "c1", Name: "pre_only", Input: `{}`,
	})
	require.NoError(t, err)
	require.True(t, innerPre.called)
	require.Equal(t, "pre-output", resp.Content,
		"pre-only tool should not have its output rewritten")
	require.Equal(t, int32(1), preFired.Load(),
		"inner tool should have run exactly once")

	// Post-only tool: no pre-hook blocking, post-hook rewrites.
	innerPost := &trackingFakeTool{
		name: "post_only",
		resp: fantasy.NewTextResponse("raw"),
		onRun: func() {
			postFired.Add(1)
		},
	}
	toolPost := newHookedTool(innerPost, nil, postRunner)
	resp2, err := toolPost.Run(t.Context(), fantasy.ToolCall{
		ID: "c2", Name: "post_only", Input: `{}`,
	})
	require.NoError(t, err)
	require.True(t, innerPost.called)
	require.Equal(t, "rewritten", resp2.Content,
		"post-only tool should have its output rewritten")
	require.Equal(t, int32(1), postFired.Load(),
		"inner tool should have run exactly once")

	// 3. Tools are wrapped (not nil) when both runners are provided.
	inputs := []fantasy.AgentTool{
		&fakeTool{name: "a"},
		&fakeTool{name: "b"},
	}
	wrapped := wrapToolsWithHooks(inputs, preRunner, postRunner, false)
	require.Len(t, wrapped, len(inputs))
	for i, tool := range wrapped {
		_, ok := tool.(*hookedTool)
		require.Truef(t, ok, "tool %d should be a *hookedTool", i)
	}

	// Verify sub-agents skip wrapping.
	unwrapped := wrapToolsWithHooks(inputs, preRunner, postRunner, true)
	require.Equal(t, inputs, unwrapped,
		"sub-agent tools should be returned unwrapped")
}

// TestIntegration_ModelRoutingWithHookDenial verifies that a hook deny
// prevents the inner tool from executing while model routing still
// functions correctly.
func TestIntegration_ModelRoutingWithHookDenial(t *testing.T) {
	t.Parallel()

	ext.ResetForTesting()
	t.Cleanup(func() { ext.ResetForTesting() })

	preRunner := newPreRunner(t, `echo "denied" >&2; exit 2`)
	postRunner := newPostRunner(t, `exit 0`)

	router := &mockModelRouterExt{}
	ext.RegisterExtension(router)

	host := ext.NewExtensionHost(ext.HostDeps{})
	require.NoError(t, host.Bootstrap(context.Background()))

	// Route a short message.
	_, err := router.StepHooks()[0].OnPrepareStep(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage("short"),
	})
	require.NoError(t, err)

	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("should not appear")}
	tool := newHookedTool(inner, preRunner, postRunner)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{
		ID:    "call-deny",
		Name:  "bash",
		Input: `{}`,
	})
	require.NoError(t, err)
	require.False(t, inner.called, "denied call must not reach the inner tool")
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "denied")

	// Model routing decision is independent of hook denial.
	m := agentHookMediator{host: host}
	require.Equal(t, config.SelectedModelTypeSmall, m.getRoutedModelType(),
		"routing should still report small regardless of hook denial")
}

// TestIntegration_FullAgentRoutingWithHooks exercises the full SessionAgent
// wiring: model router extension + hook wrapping + model selection.
func TestIntegration_FullAgentRoutingWithHooks(t *testing.T) {
	t.Parallel()

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
		},
		ModelCfg: config.SelectedModel{
			Model:    "small-model-id",
			Provider: "test-provider",
		},
	}

	agent := NewSessionAgent(SessionAgentOptions{
		LargeModel: largeModel,
		SmallModel: smallModel,
		IsYolo:     true,
		ExtHost:    host,
	})
	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)

	// Build hook runners.
	preRunner := newPreRunner(t, `echo '{"decision":"allow"}'`)
	postRunner := newPostRunner(t, `echo '{"modified_output":"post-ok"}'`)

	// Route a short input.
	_, err := router.StepHooks()[0].OnPrepareStep(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage("hello"),
	})
	require.NoError(t, err)

	// Verify model routing.
	routedType := sa.hooks.getRoutedModelType()
	require.Equal(t, config.SelectedModelTypeSmall, routedType)

	// Wrap a tool and verify hooks fire correctly.
	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("raw")}
	tool := newHookedTool(inner, preRunner, postRunner)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{
		ID: "c-full", Name: "bash", Input: `{}`,
	})
	require.NoError(t, err)
	require.True(t, inner.called)
	require.Equal(t, "post-ok", resp.Content,
		"post-hook should rewrite output in full agent wiring")

	// Now route a long input.
	var longText string
	for i := 0; i < 20000; i++ {
		longText += "x"
	}
	_, err = router.StepHooks()[0].OnPrepareStep(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage(longText),
	})
	require.NoError(t, err)
	require.Equal(t, config.SelectedModelTypeLarge, sa.hooks.getRoutedModelType())
}

// trackingFakeTool extends fakeTool with an onRun callback for tracking
// invocations.
type trackingFakeTool struct {
	name   string
	called bool
	resp   fantasy.ToolResponse
	onRun  func()
}

func (f *trackingFakeTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{Name: f.name}
}

func (f *trackingFakeTool) Run(ctx context.Context, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	f.called = true
	if f.onRun != nil {
		f.onRun()
	}
	return f.resp, nil
}

func (f *trackingFakeTool) ProviderOptions() fantasy.ProviderOptions     { return nil }
func (f *trackingFakeTool) SetProviderOptions(_ fantasy.ProviderOptions) {}

// Compile-time check that newPreRunner and newPostRunner produce *hooks.Runner.
var _ *hooks.Runner = (*hooks.Runner)(nil)
