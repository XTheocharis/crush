package processor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageHistory_ProcessInput_SavesMessages(t *testing.T) {
	t.Parallel()
	store := &InMemoryStore{}
	h := &MessageHistory{Store: store}

	msgs := []Message{
		UserMessage("hello"),
		AssistantMessage("hi"),
	}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}

	result, err := h.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)

	loaded, loadErr := store.LoadMessages(context.Background())
	require.NoError(t, loadErr)
	require.Equal(t, msgs, loaded)
}

func TestMessageHistory_ProcessOutputResult_SavesMessages(t *testing.T) {
	t.Parallel()
	store := &InMemoryStore{}
	h := &MessageHistory{Store: store}

	msgs := []Message{
		UserMessage("ping"),
		AssistantMessage("pong"),
		ToolUseMessage("tu-1", "bash", "ls"),
	}
	pctx := ProcessorContext{
		Phase:    OutputResultPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}

	result, err := h.ProcessOutputResult(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)

	loaded, loadErr := store.LoadMessages(context.Background())
	require.NoError(t, loadErr)
	require.Equal(t, msgs, loaded)
}

func TestMessageHistory_InMemoryStore_RoundTrip(t *testing.T) {
	t.Parallel()
	store := &InMemoryStore{}
	ctx := context.Background()

	original := []Message{
		UserMessage("first"),
		AssistantMessage("second"),
		ToolUseMessage("tu-1", "grep", "pattern"),
		ToolResultMessage("tu-1", "match"),
	}

	require.NoError(t, store.SaveMessages(ctx, original))
	loaded, err := store.LoadMessages(ctx)
	require.NoError(t, err)
	require.Equal(t, original, loaded)
}

func TestMessageHistory_InMemoryStore_EmptyRoundTrip(t *testing.T) {
	t.Parallel()
	store := &InMemoryStore{}
	ctx := context.Background()

	require.NoError(t, store.SaveMessages(ctx, []Message{}))
	loaded, err := store.LoadMessages(ctx)
	require.NoError(t, err)
	require.Empty(t, loaded)

	require.NoError(t, store.SaveMessages(ctx, nil))
	loaded, err = store.LoadMessages(ctx)
	require.NoError(t, err)
	require.Empty(t, loaded)
}

func TestMessageHistory_StateContainsStats_ProcessInput(t *testing.T) {
	t.Parallel()
	store := &InMemoryStore{}
	h := &MessageHistory{Store: store}

	msgs := []Message{UserMessage("a"), AssistantMessage("b"), UserMessage("c")}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}

	result, err := h.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, 3, result.State["input_messages_saved"])
	require.Equal(t, "save_input", result.State["history_action"])
}

func TestMessageHistory_StateContainsStats_ProcessOutputResult(t *testing.T) {
	t.Parallel()
	store := &InMemoryStore{}
	h := &MessageHistory{Store: store}

	msgs := []Message{AssistantMessage("done")}
	pctx := ProcessorContext{
		Phase:    OutputResultPhase,
		Messages: msgs,
		State:    make(map[string]any),
	}

	result, err := h.ProcessOutputResult(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, 1, result.State["output_messages_saved"])
	require.Equal(t, "save_output", result.State["history_action"])
}

func TestMessageHistory_RunAllPhases(t *testing.T) {
	t.Parallel()
	store := &InMemoryStore{}
	h := &MessageHistory{Store: store}

	pctx := NewTestContext()
	final := RunAllPhases(t, h, pctx)

	require.Equal(t, []Message{UserMessage("hello"), AssistantMessage("hi there")}, final.Messages)
	require.Equal(t, "save_output", final.State["history_action"])

	loaded, err := store.LoadMessages(context.Background())
	require.NoError(t, err)
	require.Equal(t, []Message{UserMessage("hello"), AssistantMessage("hi there")}, loaded)
}

func TestMessageHistory_MessagesNotModified(t *testing.T) {
	t.Parallel()
	store := &InMemoryStore{}
	h := &MessageHistory{Store: store}

	original := []Message{
		UserMessage("unchanged"),
		AssistantMessage("also unchanged"),
	}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: original,
		State:    make(map[string]any),
	}

	result, err := h.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, original, result.Messages)

	result2, err2 := h.ProcessOutputResult(context.Background(), pctx)
	require.NoError(t, err2)
	require.Equal(t, original, result2.Messages)
}

func TestMessageHistory_StoreCalledWithCorrectMessages(t *testing.T) {
	t.Parallel()

	var inputSaved, outputSaved []Message
	store := &trackingStore{
		onSave: func(messages []Message) {
			if inputSaved == nil {
				inputSaved = messages
			} else {
				outputSaved = messages
			}
		},
	}
	h := &MessageHistory{Store: store}

	inputMsgs := []Message{UserMessage("q1"), AssistantMessage("a1")}
	outputMsgs := []Message{AssistantMessage("final answer")}

	_, err := h.ProcessInput(context.Background(), ProcessorContext{
		Phase:    InputPhase,
		Messages: inputMsgs,
		State:    make(map[string]any),
	})
	require.NoError(t, err)

	_, err = h.ProcessOutputResult(context.Background(), ProcessorContext{
		Phase:    OutputResultPhase,
		Messages: outputMsgs,
		State:    make(map[string]any),
	})
	require.NoError(t, err)

	require.Equal(t, inputMsgs, inputSaved)
	require.Equal(t, outputMsgs, outputSaved)
}

func TestMessageHistory_OutputStreamAndAPIError_PassThrough(t *testing.T) {
	t.Parallel()
	store := &InMemoryStore{}
	h := &MessageHistory{Store: store}

	msgs := []Message{UserMessage("test")}
	pctx := ProcessorContext{
		Messages: msgs,
		State:    make(map[string]any),
	}

	streamResult, err := h.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, streamResult.Action)
	require.Equal(t, msgs, streamResult.Messages)
	require.Empty(t, streamResult.State)

	errorResult, err := h.ProcessAPIError(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, errorResult.Action)
	require.Equal(t, msgs, errorResult.Messages)
	require.Empty(t, errorResult.State)
}

// trackingStore wraps InMemoryStore and calls onSave each time SaveMessages is
// invoked, allowing tests to inspect what was persisted.
type trackingStore struct {
	InMemoryStore
	onSave func(messages []Message)
}

func (s *trackingStore) SaveMessages(ctx context.Context, messages []Message) error {
	s.onSave(messages)
	return s.InMemoryStore.SaveMessages(ctx, messages)
}
