package processor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolCallFilterEmptyLists(t *testing.T) {
	t.Parallel()
	f := &ToolCallFilter{}
	msgs := []Message{
		UserMessage("hello"),
		ToolUseMessage("tu-1", "bash", "ls"),
		ToolResultMessage("tu-1", "file.txt"),
		AssistantMessage("done"),
	}
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := f.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Len(t, result.Messages, 4)
}

func TestToolCallFilterAllowOnlyMode(t *testing.T) {
	t.Parallel()
	f := &ToolCallFilter{AllowList: []string{"bash", "edit"}}
	msgs := []Message{
		UserMessage("hello"),
		ToolUseMessage("tu-1", "bash", "ls"),
		ToolUseMessage("tu-2", "rm_rf", "rm -rf /"),
		ToolUseMessage("tu-3", "edit", "fix code"),
		AssistantMessage("done"),
	}
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := f.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Len(t, result.Messages, 4)
	roles := make([]string, len(result.Messages))
	for i, m := range result.Messages {
		roles[i] = m.Role
	}
	require.Equal(t, []string{"user", "tool_use", "tool_use", "assistant"}, roles)
	require.Equal(t, "bash", result.Messages[1].Meta["name"])
	require.Equal(t, "edit", result.Messages[2].Meta["name"])
}

func TestToolCallFilterDenyMode(t *testing.T) {
	t.Parallel()
	f := &ToolCallFilter{DenyList: []string{"bash"}}
	msgs := []Message{
		UserMessage("hello"),
		ToolUseMessage("tu-1", "bash", "ls"),
		ToolResultMessage("tu-1", "output"),
		ToolUseMessage("tu-2", "edit", "fix"),
		ToolResultMessage("tu-2", "ok"),
	}
	pctx := ProcessorContext{
		Phase:    OutputResultPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := f.ProcessOutputResult(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Len(t, result.Messages, 3)
	require.Equal(t, "user", result.Messages[0].Role)
	require.Equal(t, "tool_use", result.Messages[1].Role)
	require.Equal(t, "edit", result.Messages[1].Meta["name"])
	require.Equal(t, "tool_result", result.Messages[2].Role)
}

func TestToolCallFilterCombinedAllowAndDeny(t *testing.T) {
	t.Parallel()
	f := &ToolCallFilter{
		AllowList: []string{"bash", "edit", "view"},
		DenyList:  []string{"edit"},
	}
	msgs := []Message{
		ToolUseMessage("tu-1", "bash", "ls"),
		ToolUseMessage("tu-2", "edit", "change"),
		ToolUseMessage("tu-3", "view", "read"),
		ToolUseMessage("tu-4", "rm_rf", "danger"),
	}
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := f.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Len(t, result.Messages, 2)
	require.Equal(t, "bash", result.Messages[0].Meta["name"])
	require.Equal(t, "view", result.Messages[1].Meta["name"])
}

func TestToolCallFilterToolResultPairRemoval(t *testing.T) {
	t.Parallel()
	f := &ToolCallFilter{DenyList: []string{"bash"}}
	msgs := []Message{
		UserMessage("go"),
		ToolUseMessage("tu-1", "bash", "ls"),
		ToolResultMessage("tu-1", "output"),
		ToolUseMessage("tu-2", "edit", "fix"),
		ToolResultMessage("tu-2", "ok"),
		AssistantMessage("all done"),
	}
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := f.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Len(t, result.Messages, 4)
	require.Equal(t, "user", result.Messages[0].Role)
	require.Equal(t, "tool_use", result.Messages[1].Role)
	require.Equal(t, "edit", result.Messages[1].Meta["name"])
	require.Equal(t, "tool_result", result.Messages[2].Role)
	require.Equal(t, "assistant", result.Messages[3].Role)
}

func TestToolCallFilterNoToolUseMessages(t *testing.T) {
	t.Parallel()
	f := &ToolCallFilter{DenyList: []string{"bash"}}
	msgs := []Message{
		UserMessage("hello"),
		AssistantMessage("hi there"),
	}
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := f.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Len(t, result.Messages, 2)
}

func TestToolCallFilterStateStats(t *testing.T) {
	t.Parallel()
	f := &ToolCallFilter{AllowList: []string{"bash"}}
	msgs := []Message{
		ToolUseMessage("tu-1", "bash", "ls"),
		ToolUseMessage("tu-2", "rm_rf", "rm"),
		ToolUseMessage("tu-3", "edit", "fix"),
	}
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := f.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, 1, result.State["tools_allowed"])
	require.Equal(t, 2, result.State["tools_blocked"])
	blockedNames, ok := result.State["blocked_names"].([]string)
	require.True(t, ok)
	require.ElementsMatch(t, []string{"rm_rf", "edit"}, blockedNames)
	require.Equal(t, "tool_call_filter", result.State["filter_id"])
}

func TestToolCallFilterProcessInputPassthrough(t *testing.T) {
	t.Parallel()
	f := &ToolCallFilter{DenyList: []string{"bash"}}
	msgs := []Message{ToolUseMessage("tu-1", "bash", "ls")}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := f.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Len(t, result.Messages, 1)
}

func TestToolCallFilterProcessAPIErrorPassthrough(t *testing.T) {
	t.Parallel()
	f := &ToolCallFilter{DenyList: []string{"bash"}}
	msgs := []Message{ToolUseMessage("tu-1", "bash", "ls")}
	pctx := ProcessorContext{
		Phase:    APIErrorPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := f.ProcessAPIError(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Len(t, result.Messages, 1)
}

func TestToolCallFilterRunAllPhases(t *testing.T) {
	t.Parallel()
	f := &ToolCallFilter{DenyList: []string{"bash"}}
	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			UserMessage("hello"),
			ToolUseMessage("tu-1", "bash", "ls"),
			ToolResultMessage("tu-1", "output"),
			ToolUseMessage("tu-2", "edit", "fix"),
			ToolResultMessage("tu-2", "ok"),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}
	final := RunAllPhases(t, f, pctx)
	// After filtering: bash and its result are removed, edit passes through.
	require.Len(t, final.Messages, 3)
	require.Equal(t, "user", final.Messages[0].Role)
	require.Equal(t, "tool_use", final.Messages[1].Role)
	require.Equal(t, "edit", final.Messages[1].Meta["name"])
	require.Equal(t, "tool_result", final.Messages[2].Role)
	require.NotNil(t, final.State["tools_blocked"])
}
