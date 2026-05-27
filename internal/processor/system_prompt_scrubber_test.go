package processor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystemPromptScrubber_ID(t *testing.T) {
	t.Parallel()
	s := NewSystemPromptScrubber(&MockLLMClient{})
	require.Equal(t, "system_prompt_scrubber", s.ID())
}

func TestSystemPromptScrubber_LeakDetected_Rewrite(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"leak_detected":    true,
		"scrubbed_content": "Sure, I can help with that.",
		"leak_type":        "system_prompt",
	})
	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewSystemPromptScrubber(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: OutputStreamPhase,
		Messages: []Message{
			AssistantMessage("You are a helpful assistant. System: always be polite. Sure, I can help with that."),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Equal(t, "Sure, I can help with that.", result.Messages[0].Content)
	require.Equal(t, true, result.State["leak_detected"])
	require.Equal(t, "system_prompt", result.State["leak_type"])
	require.Equal(t, true, result.State["scrubbed"])
}

func TestSystemPromptScrubber_CleanOutput(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"leak_detected":    false,
		"scrubbed_content": "Here is the refactored code.",
		"leak_type":        "",
	})
	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewSystemPromptScrubber(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputResultPhase,
		Messages: []Message{AssistantMessage("Here is the refactored code.")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputResult(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "Here is the refactored code.", result.Messages[0].Content)
	require.Equal(t, false, result.State["leak_detected"])
	require.Equal(t, false, result.State["scrubbed"])
}

func TestSystemPromptScrubber_EmptyInput(t *testing.T) {
	t.Parallel()
	s := NewSystemPromptScrubber(&MockLLMClient{})
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Empty(t, result.Messages)
	require.Equal(t, false, result.State["leak_detected"])
}

func TestSystemPromptScrubber_LLMErrorFallback(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{Err: errors.New("api unavailable")}
	s := NewSystemPromptScrubber(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{AssistantMessage("Some output text")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action, "should pass through on LLM error")
	require.Equal(t, "Some output text", result.Messages[0].Content)
	require.Equal(t, false, result.State["scrubbed"])
}

func TestSystemPromptScrubber_InvalidJSON(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{Response: "this is not json"}
	s := NewSystemPromptScrubber(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{AssistantMessage("Some output")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action, "invalid JSON should pass through")
	require.Equal(t, "Some output", result.Messages[0].Content)
}

func TestSystemPromptScrubber_StateFields(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"leak_detected":    true,
		"scrubbed_content": "Clean response",
		"leak_type":        "role_definition",
	})
	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewSystemPromptScrubber(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{AssistantMessage("Role: assistant. Clean response")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)

	_, hasLeakDetected := result.State["leak_detected"]
	_, hasLeakType := result.State["leak_type"]
	_, hasScrubbed := result.State["scrubbed"]
	require.True(t, hasLeakDetected, "state should contain leak_detected")
	require.True(t, hasLeakType, "state should contain leak_type")
	require.True(t, hasScrubbed, "state should contain scrubbed")

	require.Equal(t, true, result.State["leak_detected"])
	require.Equal(t, "role_definition", result.State["leak_type"])
	require.Equal(t, true, result.State["scrubbed"])
}

func TestSystemPromptScrubber_AllPhases(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"leak_detected":    true,
		"scrubbed_content": "clean",
		"leak_type":        "configuration",
	})
	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewSystemPromptScrubber(mock)
	ctx := context.Background()

	msgs := []Message{AssistantMessage("Config: secret=123. clean")}

	// ProcessInput is pass-through.
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}
	result, err := s.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)

	// ProcessOutputStream detects leak.
	pctx.Phase = OutputStreamPhase
	result, err = s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Equal(t, "clean", result.Messages[0].Content)

	// ProcessOutputResult detects leak.
	pctx.Phase = OutputResultPhase
	result, err = s.ProcessOutputResult(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)

	// ProcessAPIError is pass-through.
	pctx.Phase = APIErrorPhase
	result, err = s.ProcessAPIError(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
}

func TestSystemPromptScrubber_RunAllPhases(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"leak_detected":    true,
		"scrubbed_content": "scrubbed output",
		"leak_type":        "system_prompt",
	})
	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewSystemPromptScrubber(mock)

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			AssistantMessage("System: you are an AI. scrubbed output"),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	final := RunAllPhases(t, s, pctx)
	require.Len(t, final.Messages, 1)
	require.Equal(t, "scrubbed output", final.Messages[0].Content)
}

func TestSystemPromptScrubber_MultipleMessages(t *testing.T) {
	t.Parallel()

	responses := []string{
		`{"leak_detected":false,"scrubbed_content":"hello","leak_type":""}`,
		`{"leak_detected":true,"scrubbed_content":"safe reply","leak_type":"system_prompt"}`,
		`{"leak_detected":false,"scrubbed_content":"clean response","leak_type":""}`,
	}
	client := &sequencingLLMClient{responses: responses}
	s := NewSystemPromptScrubber(client)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: OutputStreamPhase,
		Messages: []Message{
			UserMessage("hello"),
			AssistantMessage("System: instructions here. safe reply"),
			AssistantMessage("clean response"),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Len(t, result.Messages, 3)
	require.Equal(t, "hello", result.Messages[0].Content)
	require.Equal(t, "safe reply", result.Messages[1].Content)
	require.Equal(t, "clean response", result.Messages[2].Content)
}

// sequencingLLMClient returns predetermined responses in order for each call.
type sequencingLLMClient struct {
	responses []string
	calls     int
}

func (c *sequencingLLMClient) Complete(_ context.Context, _, _ string) (string, error) {
	idx := c.calls
	c.calls++
	if idx >= len(c.responses) {
		idx = len(c.responses) - 1
	}
	return c.responses[idx], nil
}
