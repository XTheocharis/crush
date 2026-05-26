package processor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMockProcessorDefaultBehavior(t *testing.T) {
	t.Parallel()

	p := &MockProcessor{IDField: "default"}
	ctx := context.Background()
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("hello")},
		State:    make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	AssertPhaseAction(t, result, ActionContinue)
	require.Equal(t, pctx.Messages, result.Messages)

	result, err = p.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	AssertPhaseAction(t, result, ActionContinue)

	result, err = p.ProcessOutputResult(ctx, pctx)
	require.NoError(t, err)
	AssertPhaseAction(t, result, ActionContinue)

	result, err = p.ProcessAPIError(ctx, pctx)
	require.NoError(t, err)
	AssertPhaseAction(t, result, ActionContinue)
}

func TestMockProcessorCustomBehavior(t *testing.T) {
	t.Parallel()

	p := &MockProcessor{
		IDField: "custom",
		InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action:   ActionRewrite,
				Messages: []Message{UserMessage("rewritten")},
				State:    map[string]any{"rewritten": true},
			}, nil
		},
	}

	ctx := context.Background()
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("original")},
		State:    make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	AssertPhaseAction(t, result, ActionRewrite)
	require.Equal(t, []Message{UserMessage("rewritten")}, result.Messages)
	require.Equal(t, true, result.State["rewritten"])
}

func TestMockProcessorAbort(t *testing.T) {
	t.Parallel()

	p := &MockProcessor{
		IDField: "aborter",
		APIErrorFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action: ActionAbort,
				Error:  errors.New("abort"),
			}, nil
		},
	}

	ctx := context.Background()
	pctx := ProcessorContext{Phase: APIErrorPhase}

	result, err := p.ProcessAPIError(ctx, pctx)
	require.NoError(t, err)
	AssertPhaseAction(t, result, ActionAbort)
	require.Equal(t, "abort", result.Error.Error())
}

func TestMockProcessorReturnsError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	p := &MockProcessor{
		IDField: "errorer",
		OutputResultFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{}, wantErr
		},
	}

	result, err := p.ProcessOutputResult(context.Background(), ProcessorContext{})
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, ProcessorResult{}, result)
}

func TestFakeMessages(t *testing.T) {
	t.Parallel()

	um := UserMessage("hi")
	require.Equal(t, "user", um.Role)
	require.Equal(t, "hi", um.Content)

	am := AssistantMessage("hello")
	require.Equal(t, "assistant", am.Role)
	require.Equal(t, "hello", am.Content)

	tum := ToolUseMessage("id1", "bash", "ls")
	require.Equal(t, "tool_use", tum.Role)
	require.Equal(t, "ls", tum.Content)
	require.Equal(t, "id1", tum.Meta["id"])
	require.Equal(t, "bash", tum.Meta["name"])

	trm := ToolResultMessage("id1", "output")
	require.Equal(t, "tool_result", trm.Role)
	require.Equal(t, "output", trm.Content)
	require.Equal(t, "id1", trm.Meta["tool_use_id"])
}

func TestMockLLMClient(t *testing.T) {
	t.Parallel()

	client := &MockLLMClient{Response: "world"}
	ctx := context.Background()

	resp, err := client.Complete(ctx, "hello", "input")
	require.NoError(t, err)
	require.Equal(t, "world", resp)
	require.Len(t, client.Calls, 1)
	require.Equal(t, "hello", client.Calls[0].Prompt)
	require.Equal(t, "input", client.Calls[0].Input)
}

func TestMockLLMClientError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("api down")
	client := &MockLLMClient{Err: wantErr}

	resp, err := client.Complete(context.Background(), "prompt", "input")
	require.ErrorIs(t, err, wantErr)
	require.Empty(t, resp)
	require.Len(t, client.Calls, 1, "call should be recorded even on error")
}

func TestProcessorTestHarness(t *testing.T) {
	t.Parallel()

	p := &MockProcessor{IDField: "harness"}
	pctx := NewTestContext()

	final := RunAllPhases(t, p, pctx)
	require.Equal(t, APIErrorPhase, final.Phase, "should end on last phase")
	require.NotNil(t, final.State)
	require.NotNil(t, final.Messages)
}

func TestNewTestContext(t *testing.T) {
	t.Parallel()

	pctx := NewTestContext()
	require.Equal(t, InputPhase, pctx.Phase)
	require.Equal(t, "test input", pctx.Input)
	require.Len(t, pctx.Messages, 2)
	require.Equal(t, "user", pctx.Messages[0].Role)
	require.Equal(t, "assistant", pctx.Messages[1].Role)
	require.NotNil(t, pctx.State)
	require.NotNil(t, pctx.Metadata)
	require.Empty(t, pctx.State)
	require.Empty(t, pctx.Metadata)
}
