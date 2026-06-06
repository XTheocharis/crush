package extensions

import (
	"context"
	"fmt"
	"testing"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/stretchr/testify/require"
)

func newTestDoomExt(t *testing.T) (*DoomExtension, ext.StepHook) {
	t.Helper()
	e := &DoomExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	hooks := e.StepHooks()
	require.Len(t, hooks, 1)
	return e, hooks[0]
}

// makeRepetitiveStep creates a StepResult with a single tool call and its
// matching result, using the given tool name, input, output text, and call ID.
func makeRepetitiveStep(toolName, input, output, callID string) fantasy.StepResult {
	return fantasy.StepResult{
		Response: fantasy.Response{
			Content: fantasy.ResponseContent{
				fantasy.ToolCallContent{
					ToolCallID: callID,
					ToolName:   toolName,
					Input:      input,
				},
				fantasy.ToolResultContent{
					ToolCallID: callID,
					ToolName:   toolName,
					Result:     fantasy.ToolResultOutputContentText{Text: output},
				},
			},
		},
	}
}

func TestDoomWindowConsistency(t *testing.T) {
	t.Parallel()

	e, hook := newTestDoomExt(t)
	defer e.Shutdown(context.Background())

	// DoomLoopDetector defaults: Soft=3, Medium=5, Hard=7, WindowSize=10.
	// Detection requires len(steps) >= WindowSize (10), so hard doom triggers
	// at step 10 (10 identical calls >= Hard threshold of 7).
	for i := range 15 {
		step := makeRepetitiveStep("edit", `{"file":"main.go","old":"foo","new":"bar"}`, "ok", "call-1")
		err := hook.OnStepFinish(context.Background(), "test-session", step)
		require.NoError(t, err)

		if i >= 9 {
			stopped := hook.StopCondition(context.Background(), nil)
			require.True(t, stopped, "StopCondition should return true after step %d (hard doom at threshold)", i+1)
		} else {
			stopped := hook.StopCondition(context.Background(), nil)
			require.False(t, stopped, "StopCondition should return false after step %d (below WindowSize)", i+1)
		}
	}
}

func TestDoomStopConditionAfterShutdown(t *testing.T) {
	t.Parallel()

	e, hook := newTestDoomExt(t)

	for range 12 {
		s := makeRepetitiveStep("bash", `{"command":"make test"}`, "FAIL", "call-1")
		err := hook.OnStepFinish(context.Background(), "s", s)
		require.NoError(t, err)
	}

	require.True(t, hook.StopCondition(context.Background(), nil))

	e.Shutdown(context.Background())
	require.False(t, hook.StopCondition(context.Background(), nil))
}

func TestDoomStopConditionResetsOnDiverseSteps(t *testing.T) {
	t.Parallel()

	e, hook := newTestDoomExt(t)
	defer e.Shutdown(context.Background())

	// Feed 8 repetitive steps to trigger doom.
	for range 12 {
		s := makeRepetitiveStep("grep", `{"pattern":"TODO"}`, "no matches", "call-1")
		err := hook.OnStepFinish(context.Background(), "s", s)
		require.NoError(t, err)
	}
	require.True(t, hook.StopCondition(context.Background(), nil), "doom should be detected after 12 identical steps")

	for i := range 10 {
		step := makeRepetitiveStep("view", fmt.Sprintf(`{"file":"completely_unique_path_%d.go"}`, i), fmt.Sprintf("unique_output_%d", i), fmt.Sprintf("call-%d", i))
		err := hook.OnStepFinish(context.Background(), "s", step)
		require.NoError(t, err)
	}

	require.False(t, hook.StopCondition(context.Background(), nil), "doom should clear after diverse steps")
}
