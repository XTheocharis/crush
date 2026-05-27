package processor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageSelectionRecencyKeepsMostRecent(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		UserMessage("first"),
		AssistantMessage("second"),
		UserMessage("third"),
		AssistantMessage("fourth"),
		UserMessage("fifth"),
	}
	ms := &MessageSelection{MaxMessages: 2, Strategy: "recency"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := ms.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, 2, len(result.Messages))
	require.Equal(t, "fourth", result.Messages[0].Content)
	require.Equal(t, "fifth", result.Messages[1].Content)
}

func TestMessageSelectionRecencyPreservesFirstUserMessage(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		UserMessage("original request"),
		AssistantMessage("reply 1"),
		AssistantMessage("reply 2"),
		AssistantMessage("reply 3"),
		AssistantMessage("reply 4"),
	}
	ms := &MessageSelection{MaxMessages: 2, Strategy: "recency"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := ms.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, 2, len(result.Messages))
	require.Equal(t, "original request", result.Messages[0].Content)
	require.Equal(t, "reply 4", result.Messages[1].Content)
}

func TestMessageSelectionRelevanceDropsToolUseFirst(t *testing.T) {
	t.Parallel()
	f := NewMessageFactory()
	msgs := []Message{
		f.UserMsg("question"),
		f.AssistantMsg("let me check"),
		f.ToolUse("bash", "ls"),
		f.ToolResult("file1.txt"),
		f.AssistantMsg("here's the answer"),
	}
	ms := &MessageSelection{MaxMessages: 3, Strategy: "relevance"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := ms.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, 3, len(result.Messages))
	require.Equal(t, "user", result.Messages[0].Role)
	require.Equal(t, "assistant", result.Messages[1].Role)
	require.Equal(t, "assistant", result.Messages[2].Role)
	roles := map[string]bool{}
	for _, m := range result.Messages {
		roles[m.Role] = true
	}
	require.False(t, roles["tool_use"], "tool_use should have been dropped")
}

func TestMessageSelectionRelevanceDropsToolResultBeforeAssistant(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		UserMessage("question"),
		AssistantMessage("checking"),
		ToolUseMessage("t1", "bash", "ls"),
		ToolResultMessage("t1", "output"),
		UserMessage("follow-up"),
	}
	ms := &MessageSelection{MaxMessages: 3, Strategy: "relevance"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := ms.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, 3, len(result.Messages))

	remaining := roleCounts(result.Messages)
	require.Equal(t, 2, remaining["user"], "both user messages should be kept")
	require.Equal(t, 1, remaining["assistant"], "one assistant message should be kept")
	_, hasToolUse := remaining["tool_use"]
	require.False(t, hasToolUse, "tool_use should have been dropped")
	_, hasToolResult := remaining["tool_result"]
	require.False(t, hasToolResult, "tool_result should have been dropped")
}

func TestMessageSelectionMaxMessagesZeroNoFiltering(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		UserMessage("a"),
		AssistantMessage("b"),
		UserMessage("c"),
	}
	ms := &MessageSelection{MaxMessages: 0, Strategy: "recency"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := ms.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
}

func TestMessageSelectionWithinBudgetNoFiltering(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		UserMessage("a"),
		AssistantMessage("b"),
	}
	ms := &MessageSelection{MaxMessages: 10, Strategy: "recency"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := ms.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
}

func TestMessageSelectionEmptyMessages(t *testing.T) {
	t.Parallel()
	ms := &MessageSelection{MaxMessages: 5, Strategy: "recency"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{},
		State:    make(map[string]any),
	}
	result, err := ms.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Empty(t, result.Messages)
}

func TestMessageSelectionAllToolUse(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		ToolUseMessage("t1", "bash", "ls"),
		ToolUseMessage("t2", "grep", "pattern"),
		ToolUseMessage("t3", "cat", "file"),
		ToolUseMessage("t4", "edit", "fix"),
	}
	ms := &MessageSelection{MaxMessages: 2, Strategy: "relevance"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := ms.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, 2, len(result.Messages))
	require.Equal(t, "file", result.Messages[0].Content)
	require.Equal(t, "fix", result.Messages[1].Content)
}

func TestMessageSelectionStateContainsStats(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		UserMessage("question"),
		AssistantMessage("checking"),
		ToolUseMessage("t1", "bash", "ls"),
		ToolResultMessage("t1", "output"),
		UserMessage("follow-up"),
	}
	ms := &MessageSelection{MaxMessages: 3, Strategy: "relevance"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := ms.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)

	require.Equal(t, 5, result.State["messages_before"])
	require.Equal(t, 3, result.State["messages_after"])
	require.Equal(t, "relevance", result.State["strategy"])

	removedRoles, ok := result.State["removed_roles"].(map[string]int)
	require.True(t, ok, "removed_roles should be map[string]int")
	require.Equal(t, 1, removedRoles["tool_use"])
	require.Equal(t, 1, removedRoles["tool_result"])
}

func TestMessageSelectionRunAllPhases(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		UserMessage("hello"),
		AssistantMessage("hi"),
		AssistantMessage("how are you"),
		AssistantMessage("fine"),
	}
	ms := &MessageSelection{MaxMessages: 2, Strategy: "recency"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Input:    "test",
		Messages: msgs,
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}
	final := RunAllPhases(t, ms, pctx)
	require.Equal(t, 2, len(final.Messages))
	require.Equal(t, "hello", final.Messages[0].Content)
	require.Equal(t, "fine", final.Messages[1].Content)
}

func TestMessageSelectionPassThroughPhases(t *testing.T) {
	t.Parallel()
	ms := &MessageSelection{MaxMessages: 1, Strategy: "recency"}
	msgs := []Message{UserMessage("keep"), AssistantMessage("drop")}
	ctx := context.Background()

	streamPctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := ms.ProcessOutputStream(ctx, streamPctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)

	resultPctx := ProcessorContext{
		Phase:    OutputResultPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err = ms.ProcessOutputResult(ctx, resultPctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)

	errorPctx := ProcessorContext{
		Phase:    APIErrorPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err = ms.ProcessAPIError(ctx, errorPctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
}

func TestMessageSelectionRelevancePreservesFirstUserMessage(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		UserMessage("original request"),
		ToolUseMessage("t1", "bash", "ls"),
		ToolResultMessage("t1", "file1.txt"),
		ToolUseMessage("t2", "grep", "TODO"),
		ToolResultMessage("t2", "found 3"),
	}
	ms := &MessageSelection{MaxMessages: 2, Strategy: "relevance"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := ms.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, 2, len(result.Messages))
	require.Equal(t, "user", result.Messages[0].Role)
	require.Equal(t, "original request", result.Messages[0].Content)
}

func TestMessageSelectionRecencyStateStats(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		UserMessage("a"),
		AssistantMessage("b"),
		UserMessage("c"),
		ToolUseMessage("t1", "bash", "ls"),
		ToolResultMessage("t1", "out"),
	}
	ms := &MessageSelection{MaxMessages: 2, Strategy: "recency"}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := ms.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)

	require.Equal(t, 5, result.State["messages_before"])
	require.Equal(t, 2, result.State["messages_after"])
	require.Equal(t, "recency", result.State["strategy"])

	removedRoles, ok := result.State["removed_roles"].(map[string]int)
	require.True(t, ok)
	require.Equal(t, 1, removedRoles["user"])
	require.Equal(t, 1, removedRoles["assistant"])
	require.Equal(t, 1, removedRoles["tool_use"])
}

func roleCounts(msgs []Message) map[string]int {
	counts := make(map[string]int)
	for _, m := range msgs {
		counts[m.Role]++
	}
	return counts
}
