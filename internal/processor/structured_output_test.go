package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStructuredOutput_ID(t *testing.T) {
	t.Parallel()
	s := NewStructuredOutput(&MockLLMClient{}, StructuredOutputConfig{})
	require.Equal(t, "structured_output", s.ID())
}

func TestStructuredOutput_StructuredRewrite(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"structured":  true,
		"json_output": `{"name":"Alice","age":30}`,
		"schema":      "person object with name and age",
	})

	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{AssistantMessage("Name: Alice, Age: 30")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Equal(t, `{"name":"Alice","age":30}`, result.Messages[0].Content)
	require.Equal(t, true, result.State["structured"])
	require.Equal(t, "person object with name and age", result.State["schema"])
	require.Equal(t, 20, result.State["output_length"])
}

func TestStructuredOutput_AlreadyStructured(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"structured":  false,
		"json_output": "",
		"schema":      "",
	})

	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{AssistantMessage(`{"already":"structured"}`)},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, `{"already":"structured"}`, result.Messages[0].Content)
	require.Equal(t, false, result.State["structured"])
}

func TestStructuredOutput_EmptyInput(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
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
	require.Equal(t, false, result.State["structured"])
	require.Equal(t, 0, result.State["output_length"])
	require.Empty(t, mock.Calls, "LLM should not be called for empty input")
}

func TestStructuredOutput_LLMErrorFallback(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{Err: fmt.Errorf("LLM unavailable")}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{AssistantMessage("Some unstructured text")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "Some unstructured text", result.Messages[0].Content)
	require.Equal(t, false, result.State["structured"])
}

func TestStructuredOutput_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{Response: "not valid json at all"}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{AssistantMessage("Some text here")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "Some text here", result.Messages[0].Content)
	require.Equal(t, false, result.State["structured"])
}

func TestStructuredOutput_StateFields(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"structured":  true,
		"json_output": `{"items":[1,2,3]}`,
		"schema":      "array of integers",
	})

	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputResultPhase,
		Messages: []Message{AssistantMessage("The numbers are 1, 2, and 3.")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputResult(ctx, pctx)
	require.NoError(t, err)

	_, hasStructured := result.State["structured"]
	_, hasSchema := result.State["schema"]
	_, hasOutputLen := result.State["output_length"]
	require.True(t, hasStructured, "state should contain structured")
	require.True(t, hasSchema, "state should contain schema")
	require.True(t, hasOutputLen, "state should contain output_length")
	require.Equal(t, true, result.State["structured"])
	require.Equal(t, "array of integers", result.State["schema"])
}

func TestStructuredOutput_AllPhases(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"structured":  true,
		"json_output": `{"status":"ok"}`,
		"schema":      "status object",
	})

	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
	ctx := context.Background()

	msgs := []Message{AssistantMessage("Status is OK")}

	// ProcessInput — pass-through.
	inputResult, err := s.ProcessInput(ctx, ProcessorContext{
		Phase:    InputPhase,
		Messages: msgs,
		State:    make(map[string]any),
	})
	require.NoError(t, err)
	require.Equal(t, ActionContinue, inputResult.Action)
	require.Equal(t, msgs, inputResult.Messages)

	// ProcessOutputStream — structured rewrite.
	streamResult, err := s.ProcessOutputStream(ctx, ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: msgs,
		State:    make(map[string]any),
	})
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, streamResult.Action)

	// ProcessOutputResult — structured rewrite.
	resultResult, err := s.ProcessOutputResult(ctx, ProcessorContext{
		Phase:    OutputResultPhase,
		Messages: msgs,
		State:    make(map[string]any),
	})
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, resultResult.Action)

	// ProcessAPIError — pass-through.
	errResult, err := s.ProcessAPIError(ctx, ProcessorContext{
		Phase:    APIErrorPhase,
		Messages: msgs,
		State:    make(map[string]any),
	})
	require.NoError(t, err)
	require.Equal(t, ActionContinue, errResult.Action)
	require.Equal(t, msgs, errResult.Messages)
}

func TestStructuredOutput_RunAllPhases(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"structured":  true,
		"json_output": `{"greeting":"hello"}`,
		"schema":      "greeting object",
	})

	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{AssistantMessage("Hello world")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	final := RunAllPhases(t, s, pctx)
	require.Len(t, final.Messages, 1)
	require.Equal(t, `{"greeting":"hello"}`, final.Messages[0].Content)
}

func TestStructuredOutput_MultipleMessages(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"structured":  true,
		"json_output": `{"combined":"structured"}`,
		"schema":      "combined object",
	})

	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: OutputStreamPhase,
		Messages: []Message{
			UserMessage("Tell me about fruits"),
			AssistantMessage("Apples are red"),
			AssistantMessage("Bananas are yellow"),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Len(t, result.Messages, 3)
	require.Equal(t, "Tell me about fruits", result.Messages[0].Content,
		"user messages should not be rewritten")
	require.Equal(t, `{"combined":"structured"}`, result.Messages[1].Content)
	require.Equal(t, `{"combined":"structured"}`, result.Messages[2].Content)
	require.Equal(t, true, result.State["structured"])

	// Verify the LLM received the combined assistant content.
	require.Len(t, mock.Calls, 1)
	require.Contains(t, mock.Calls[0].Input, "Apples are red")
	require.Contains(t, mock.Calls[0].Input, "Bananas are yellow")
}

func TestStructuredOutput_WithSchemaHint(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"structured":  true,
		"json_output": `{"temperature":72,"unit":"fahrenheit"}`,
		"schema":      "weather reading",
	})

	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewStructuredOutput(mock, StructuredOutputConfig{
		Schema: `{"type":"object","properties":{"temperature":{"type":"number"},"unit":{"type":"string"}}}`,
	})
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputResultPhase,
		Messages: []Message{AssistantMessage("It's 72 degrees Fahrenheit")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputResult(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Equal(t, `{"temperature":72,"unit":"fahrenheit"}`, result.Messages[0].Content)

	// Verify the schema hint was included in the prompt.
	require.Len(t, mock.Calls, 1)
	require.Contains(t, mock.Calls[0].Prompt, "conform to:")
	require.Contains(t, mock.Calls[0].Prompt, "temperature")
}

func TestStructuredOutput_InputPassthrough(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
	ctx := context.Background()

	msgs := []Message{UserMessage("Hello"), AssistantMessage("Hi there")}
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
	require.Empty(t, mock.Calls, "LLM should not be called during input phase")
}

func TestStructuredOutput_APIErrorPassthrough(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
	ctx := context.Background()

	msgs := []Message{AssistantMessage("Some content")}
	pctx := ProcessorContext{
		Phase:    APIErrorPhase,
		Messages: msgs,
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessAPIError(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
	require.Empty(t, mock.Calls, "LLM should not be called during error phase")
}

func TestStructuredOutput_StructuredFalseWithEmptyOutput(t *testing.T) {
	t.Parallel()

	llmResp, _ := json.Marshal(map[string]any{
		"structured":  true,
		"json_output": "",
		"schema":      "",
	})

	mock := &MockLLMClient{Response: string(llmResp)}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{AssistantMessage("Plain text that LLM decided not to structure")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action,
		"should continue when structured is true but json_output is empty")
	require.Equal(t, false, result.State["structured"])
}

func TestStructuredOutput_EmptyAssistantContent(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{}
	s := NewStructuredOutput(mock, StructuredOutputConfig{})
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: OutputStreamPhase,
		Messages: []Message{
			UserMessage("hello"),
			AssistantMessage(""),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := s.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Empty(t, mock.Calls, "LLM should not be called when assistant content is empty")
	require.Equal(t, 0, result.State["output_length"])
}
