package processor

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenLimiter_ID(t *testing.T) {
	t.Parallel()
	tl := &TokenLimiter{Budget: 100}
	require.Equal(t, "token_limiter", tl.ID())
}

func TestTokenLimiter_UnderBudget(t *testing.T) {
	t.Parallel()
	tl := &TokenLimiter{Budget: 100}
	msgs := []Message{
		UserMessage("short message"),
		AssistantMessage("reply"),
	}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := tl.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
	require.Nil(t, result.State, "no state when within budget")
}

func TestTokenLimiter_OverBudget_TruncatesOldest(t *testing.T) {
	t.Parallel()
	// Each message is 40 chars = 10 tokens. Two messages = 20 tokens.
	// Budget of 15 means the oldest message should be removed.
	tl := &TokenLimiter{Budget: 15}
	msgs := []Message{
		UserMessage(strings.Repeat("a", 40)),      // 10 tokens
		AssistantMessage(strings.Repeat("b", 40)), // 10 tokens
	}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := tl.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Len(t, result.Messages, 1)
	require.Equal(t, "assistant", result.Messages[0].Role)
	require.Equal(t, 20, result.State["tokens_before"])
	require.Equal(t, 10, result.State["tokens_after"])
	require.Equal(t, 1, result.State["messages_removed"])
}

func TestTokenLimiter_ExactBudget(t *testing.T) {
	t.Parallel()
	// 40 chars = 10 tokens. Budget = 10. Should pass through.
	tl := &TokenLimiter{Budget: 10}
	msgs := []Message{
		UserMessage(strings.Repeat("a", 40)),
	}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := tl.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
	require.Nil(t, result.State)
}

func TestTokenLimiter_EmptyMessages(t *testing.T) {
	t.Parallel()
	tl := &TokenLimiter{Budget: 100}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{},
		State:    make(map[string]any),
	}
	result, err := tl.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Empty(t, result.Messages)
	require.Nil(t, result.State)
}

func TestTokenLimiter_SingleMessageExceedsBudget(t *testing.T) {
	t.Parallel()
	// 200 chars = 50 tokens. Budget = 10 → maxChars = 40.
	tl := &TokenLimiter{Budget: 10}
	msgs := []Message{
		UserMessage(strings.Repeat("x", 200)),
	}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := tl.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Len(t, result.Messages, 1)
	// Content should be truncated to 37 chars + "..." = 40 chars.
	require.Len(t, result.Messages[0].Content, 40)
	require.True(t, strings.HasSuffix(result.Messages[0].Content, "..."))
	require.Equal(t, 50, result.State["tokens_before"])
	require.Equal(t, 0, result.State["messages_removed"])
}

func TestTokenLimiter_StateContainsTokenCounts(t *testing.T) {
	t.Parallel()
	tl := &TokenLimiter{Budget: 5}
	msgs := []Message{
		UserMessage(strings.Repeat("a", 40)),      // 10 tokens
		AssistantMessage(strings.Repeat("b", 40)), // 10 tokens
	}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	result, err := tl.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Contains(t, result.State, "tokens_before")
	require.Contains(t, result.State, "tokens_after")
	require.Contains(t, result.State, "messages_removed")
	require.Equal(t, 20, result.State["tokens_before"])
}

func TestTokenLimiter_PassThroughPhases(t *testing.T) {
	t.Parallel()
	tl := &TokenLimiter{Budget: 100}
	msgs := []Message{UserMessage("hello")}
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}
	for _, fn := range []func(context.Context, ProcessorContext) (ProcessorResult, error){
		tl.ProcessOutputStream,
		tl.ProcessOutputResult,
		tl.ProcessAPIError,
	} {
		result, err := fn(ctx, pctx)
		require.NoError(t, err)
		require.Equal(t, ActionContinue, result.Action)
		require.Equal(t, msgs, result.Messages)
	}
}

func TestTokenLimiter_RunAllPhases(t *testing.T) {
	t.Parallel()
	tl := &TokenLimiter{Budget: 100}
	pctx := NewTestContext()
	final := RunAllPhases(t, tl, pctx)
	require.Len(t, final.Messages, 2)
}

func TestTokenLimiter_ImplementsInterface(t *testing.T) {
	t.Parallel()
	var _ Processor = (*TokenLimiter)(nil)
}
