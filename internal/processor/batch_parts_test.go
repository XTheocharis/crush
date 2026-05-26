package processor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBatchParts_MergesAdjacentSameRole(t *testing.T) {
	t.Parallel()
	p := BatchParts{}
	pctx := ProcessorContext{
		Phase: OutputStreamPhase,
		Messages: []Message{
			UserMessage("hello"),
			UserMessage("world"),
		},
		State: make(map[string]any),
	}

	result, err := p.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Len(t, result.Messages, 1)
	require.Equal(t, "hello\nworld", result.Messages[0].Content)
	require.Equal(t, "user", result.Messages[0].Role)
}

func TestBatchParts_DifferentRolesNotMerged(t *testing.T) {
	t.Parallel()
	p := BatchParts{}
	pctx := ProcessorContext{
		Phase: OutputStreamPhase,
		Messages: []Message{
			UserMessage("hello"),
			AssistantMessage("hi"),
		},
		State: make(map[string]any),
	}

	result, err := p.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Len(t, result.Messages, 2)
	require.Equal(t, "hello", result.Messages[0].Content)
	require.Equal(t, "hi", result.Messages[1].Content)
}

func TestBatchParts_ThreeConsecutiveSameRole(t *testing.T) {
	t.Parallel()
	p := BatchParts{}
	pctx := ProcessorContext{
		Phase: OutputStreamPhase,
		Messages: []Message{
			AssistantMessage("a"),
			AssistantMessage("b"),
			AssistantMessage("c"),
		},
		State: make(map[string]any),
	}

	result, err := p.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Len(t, result.Messages, 1)
	require.Equal(t, "a\nb\nc", result.Messages[0].Content)
}

func TestBatchParts_EmptyMessages(t *testing.T) {
	t.Parallel()
	p := BatchParts{}
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{},
		State:    make(map[string]any),
	}

	result, err := p.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Empty(t, result.Messages)
}

func TestBatchParts_SingleMessage(t *testing.T) {
	t.Parallel()
	p := BatchParts{}
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{UserMessage("solo")},
		State:    make(map[string]any),
	}

	result, err := p.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Len(t, result.Messages, 1)
	require.Equal(t, "solo", result.Messages[0].Content)
}

func TestBatchParts_AlternatingRoles(t *testing.T) {
	t.Parallel()
	p := BatchParts{}
	pctx := ProcessorContext{
		Phase: OutputStreamPhase,
		Messages: []Message{
			UserMessage("u1"),
			AssistantMessage("a1"),
			UserMessage("u2"),
			AssistantMessage("a2"),
		},
		State: make(map[string]any),
	}

	result, err := p.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Len(t, result.Messages, 4)
}

func TestBatchParts_MetaMerging(t *testing.T) {
	t.Parallel()
	p := BatchParts{}
	pctx := ProcessorContext{
		Phase: OutputStreamPhase,
		Messages: []Message{
			{Role: "user", Content: "first", Meta: map[string]any{"k1": "v1", "shared": "first"}},
			{Role: "user", Content: "second", Meta: map[string]any{"k2": "v2", "shared": "second"}},
		},
		State: make(map[string]any),
	}

	result, err := p.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Len(t, result.Messages, 1)
	require.Equal(t, "v1", result.Messages[0].Meta["k1"])
	require.Equal(t, "v2", result.Messages[0].Meta["k2"])
	require.Equal(t, "second", result.Messages[0].Meta["shared"])
}

func TestBatchParts_StateContainsBatchStats(t *testing.T) {
	t.Parallel()
	p := BatchParts{}
	pctx := ProcessorContext{
		Phase: OutputStreamPhase,
		Messages: []Message{
			UserMessage("a"),
			UserMessage("b"),
			AssistantMessage("c"),
		},
		State: make(map[string]any),
	}

	result, err := p.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, 3, result.State["messages_before"])
	require.Equal(t, 2, result.State["messages_after"])
	require.Equal(t, 1, result.State["merges_performed"])
}

func TestBatchParts_RunAllPhases(t *testing.T) {
	t.Parallel()
	p := BatchParts{}
	pctx := NewTestContext()

	final := RunAllPhases(t, p, pctx)
	require.NotNil(t, final.State)
}

func TestBatchParts_ProcessInputPassthrough(t *testing.T) {
	t.Parallel()
	p := BatchParts{}
	msgs := []Message{UserMessage("a"), UserMessage("b")}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}

	result, err := p.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
}

func TestBatchParts_ProcessAPIErrorPassthrough(t *testing.T) {
	t.Parallel()
	p := BatchParts{}
	msgs := []Message{UserMessage("a"), UserMessage("b")}
	pctx := ProcessorContext{
		Phase:    APIErrorPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}

	result, err := p.ProcessAPIError(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
}
