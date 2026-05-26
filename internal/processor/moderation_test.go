package processor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModeration_ID(t *testing.T) {
	t.Parallel()
	m := NewModerationProcessor(nil, 0.7)
	require.Equal(t, "moderation", m.ID())
}

func TestModeration_ToxicContent_Aborts(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"score":      0.95,
		"categories": []string{"hate_speech", "violence"},
		"filtered":   "I will [FILTERED] help with [FILTERED] content",
	})
	mock := &MockLLMClient{Response: string(resp)}
	m := NewModerationProcessor(mock, 0.7)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("I will harm people with violence")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionAbort, result.Action)
	require.Equal(t, 0.95, result.State["toxicity_score"])
	require.Equal(t, "abort", result.State["action_taken"])

	categories := result.State["categories"].([]string)
	require.Contains(t, categories, "hate_speech")
	require.Contains(t, categories, "violence")
	require.Error(t, result.Error)
}

func TestModeration_CleanContent_Continues(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"score":      0.0,
		"categories": []string{},
		"filtered":   "",
	})
	mock := &MockLLMClient{Response: string(resp)}
	m := NewModerationProcessor(mock, 0.7)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Please help me refactor the auth module")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, 0.0, result.State["toxicity_score"])
	require.Equal(t, "continue", result.State["action_taken"])
	require.NoError(t, result.Error)
}

func TestModeration_BorderlineScore_Rewrites(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"score":      0.5,
		"categories": []string{"harassment"},
		"filtered":   "You are a [FILTERED] person",
	})
	mock := &MockLLMClient{Response: string(resp)}
	m := NewModerationProcessor(mock, 0.7)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("You are a terrible person")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Equal(t, 0.5, result.State["toxicity_score"])
	require.Equal(t, "rewrite", result.State["action_taken"])
	require.Equal(t, "You are a [FILTERED] person", result.Messages[0].Content)
}

func TestModeration_CustomThreshold(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"score":      0.4,
		"categories": []string{"mild_toxicity"},
		"filtered":   "some [FILTERED] text",
	})
	mock := &MockLLMClient{Response: string(resp)}

	t.Run("low_threshold_aborts", func(t *testing.T) {
		t.Parallel()
		m := NewModerationProcessor(mock, 0.3)
		ctx := context.Background()

		pctx := ProcessorContext{
			Phase:    InputPhase,
			Messages: []Message{UserMessage("some text")},
			State:    make(map[string]any),
			Metadata: make(map[string]any),
		}

		result, err := m.ProcessInput(ctx, pctx)
		require.NoError(t, err)
		require.Equal(t, ActionAbort, result.Action, "score 0.4 > threshold 0.3 should abort")
	})

	t.Run("high_threshold_continues", func(t *testing.T) {
		t.Parallel()
		m := NewModerationProcessor(mock, 0.8)
		ctx := context.Background()

		pctx := ProcessorContext{
			Phase:    InputPhase,
			Messages: []Message{UserMessage("some text")},
			State:    make(map[string]any),
			Metadata: make(map[string]any),
		}

		result, err := m.ProcessInput(ctx, pctx)
		require.NoError(t, err)
		require.Equal(t, ActionRewrite, result.Action, "score 0.4 > 0 but < 0.8 should rewrite")
	})
}

func TestModeration_EmptyInput(t *testing.T) {
	t.Parallel()
	m := NewModerationProcessor(nil, 0.7)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Empty(t, result.Messages)
	require.Equal(t, 0.0, result.State["toxicity_score"])
	require.Equal(t, "continue", result.State["action_taken"])
}

func TestModeration_EmptyMessageContent(t *testing.T) {
	t.Parallel()
	m := NewModerationProcessor(nil, 0.7)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
}

func TestModeration_LLMError_Fallback(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{Err: errors.New("LLM unavailable")}
	m := NewModerationProcessor(mock, 0.7)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Any content here")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action, "LLM error should fall back to continue")
	require.Equal(t, 0.0, result.State["toxicity_score"])
	require.Equal(t, "LLM unavailable", result.State["llm_error"])
}

func TestModeration_LLMInvalidJSON_Fallback(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{Response: "not valid json at all"}
	m := NewModerationProcessor(mock, 0.7)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Some content")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action, "invalid JSON should fall back to continue")
	require.Equal(t, 0.0, result.State["toxicity_score"])
}

func TestModeration_OutputStream_PassThrough(t *testing.T) {
	t.Parallel()
	m := NewModerationProcessor(nil, 0.7)
	ctx := context.Background()

	msgs := []Message{AssistantMessage("Some output")}
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: msgs,
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
}

func TestModeration_OutputResult_PassThrough(t *testing.T) {
	t.Parallel()
	m := NewModerationProcessor(nil, 0.7)
	ctx := context.Background()

	msgs := []Message{AssistantMessage("Final result")}
	pctx := ProcessorContext{
		Phase:    OutputResultPhase,
		Messages: msgs,
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessOutputResult(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
}

func TestModeration_APIError_PassThrough(t *testing.T) {
	t.Parallel()
	m := NewModerationProcessor(nil, 0.7)
	ctx := context.Background()

	msgs := []Message{UserMessage("error occurred")}
	pctx := ProcessorContext{
		Phase:    APIErrorPhase,
		Messages: msgs,
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessAPIError(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
}

func TestModeration_StateFields(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"score":      0.85,
		"categories": []string{"hate_speech"},
		"filtered":   "[FILTERED] content",
	})
	mock := &MockLLMClient{Response: string(resp)}
	m := NewModerationProcessor(mock, 0.7)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("bad content")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessInput(ctx, pctx)
	require.NoError(t, err)

	_, hasScore := result.State["toxicity_score"]
	_, hasAction := result.State["action_taken"]
	_, hasCats := result.State["categories"]
	require.True(t, hasScore, "state should contain toxicity_score")
	require.True(t, hasAction, "state should contain action_taken")
	require.True(t, hasCats, "state should contain categories")
}

func TestModeration_MultipleMessages(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"score":      0.6,
		"categories": []string{"harassment"},
		"filtered":   "You are [FILTERED]",
	})
	mock := &MockLLMClient{Response: string(resp)}
	m := NewModerationProcessor(mock, 0.7)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			UserMessage("Hello"),
			AssistantMessage("Hi there"),
			UserMessage("You are terrible"),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionRewrite, result.Action)
	require.Len(t, result.Messages, 3)
	require.Len(t, mock.Calls, 1)
}

func TestModeration_RunAllPhases(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"score":      0.0,
		"categories": []string{},
		"filtered":   "",
	})
	mock := &MockLLMClient{Response: string(resp)}
	m := NewModerationProcessor(mock, 0.7)

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Clean message")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	final := RunAllPhases(t, m, pctx)
	require.Len(t, final.Messages, 1)
}

func TestModeration_ThresholdClamping(t *testing.T) {
	t.Parallel()

	t.Run("negative_clamped", func(t *testing.T) {
		t.Parallel()
		m := NewModerationProcessor(nil, -0.5)
		require.Equal(t, 0.7, m.threshold)
	})

	t.Run("over_one_clamped", func(t *testing.T) {
		t.Parallel()
		m := NewModerationProcessor(nil, 1.5)
		require.Equal(t, 1.0, m.threshold)
	})
}

func TestModeration_NilCategoriesInResponse(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"score":      0.0,
		"categories": nil,
		"filtered":   "",
	})
	mock := &MockLLMClient{Response: string(resp)}
	m := NewModerationProcessor(mock, 0.7)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("clean content")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := m.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	categories := result.State["categories"].([]string)
	require.Empty(t, categories)
}
