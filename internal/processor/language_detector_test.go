package processor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLanguageDetector_ID(t *testing.T) {
	t.Parallel()
	d := NewLanguageDetector(&MockLLMClient{})
	require.Equal(t, "language_detector", d.ID())
}

func TestLanguageDetector_EnglishDetection(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"language":     "en",
		"confidence":   0.99,
		"alternatives": []string{"de"},
	})

	mock := &MockLLMClient{Response: string(resp)}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Hello, how are you doing today?")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "en", result.State["detected_language"])
	require.Equal(t, 0.99, result.State["confidence"])
	require.Equal(t, []string{"de"}, result.State["alternatives"])
	require.Equal(t, "en", result.Messages[0].Meta["detected_language"])
	require.Len(t, mock.Calls, 1)
	require.Equal(t, "Hello, how are you doing today?", mock.Calls[0].Input)
}

func TestLanguageDetector_SpanishDetection(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"language":     "es",
		"confidence":   0.95,
		"alternatives": []string{"pt", "it"},
	})

	mock := &MockLLMClient{Response: string(resp)}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Hola, ¿cómo estás hoy?")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, "es", result.State["detected_language"])
	require.Equal(t, "es", result.Messages[0].Meta["detected_language"])
}

func TestLanguageDetector_ChineseDetection(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"language":     "zh",
		"confidence":   0.97,
		"alternatives": []string{"ja", "ko"},
	})

	mock := &MockLLMClient{Response: string(resp)}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("你好，今天天气怎么样？")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, "zh", result.State["detected_language"])
	require.Equal(t, 0.97, result.State["confidence"])
	require.Equal(t, "zh", result.Messages[0].Meta["detected_language"])
}

func TestLanguageDetector_EmptyInput(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Empty(t, result.Messages)
	require.Equal(t, "", result.State["detected_language"])
	require.Empty(t, mock.Calls, "LLM should not be called for empty input")
}

func TestLanguageDetector_LLMErrorFallback(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{Err: errors.New("LLM unavailable")}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Hello there")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "", result.State["detected_language"])
	require.Equal(t, 0.0, result.State["confidence"])
	require.Equal(t, "Hello there", result.Messages[0].Content)
}

func TestLanguageDetector_InvalidJSON(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{Response: "not valid json at all"}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Some text here")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "", result.State["detected_language"])
	require.Equal(t, "Some text here", result.Messages[0].Content)
}

func TestLanguageDetector_StateFields(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"language":     "fr",
		"confidence":   0.88,
		"alternatives": []string{"en"},
	})

	mock := &MockLLMClient{Response: string(resp)}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("Bonjour le monde")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)

	_, hasLang := result.State["detected_language"]
	_, hasConf := result.State["confidence"]
	_, hasAlt := result.State["alternatives"]
	require.True(t, hasLang, "state should contain detected_language")
	require.True(t, hasConf, "state should contain confidence")
	require.True(t, hasAlt, "state should contain alternatives")
	require.Equal(t, "fr", result.State["detected_language"])
	require.Equal(t, 0.88, result.State["confidence"])
	require.Equal(t, []string{"en"}, result.State["alternatives"])
}

func TestLanguageDetector_AllPhases(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"language":     "en",
		"confidence":   0.95,
		"alternatives": []string{},
	})

	mock := &MockLLMClient{Response: string(resp)}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	msgs := []Message{UserMessage("Hello")}

	// ProcessOutputStream is pass-through.
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: msgs,
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}
	result, err := d.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)

	// ProcessOutputResult is pass-through.
	pctx.Phase = OutputResultPhase
	result, err = d.ProcessOutputResult(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)

	// ProcessAPIError is pass-through.
	pctx.Phase = APIErrorPhase
	result, err = d.ProcessAPIError(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
}

func TestLanguageDetector_MultipleMessages(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"language":     "de",
		"confidence":   0.92,
		"alternatives": []string{"nl"},
	})

	mock := &MockLLMClient{Response: string(resp)}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			UserMessage("Hallo, wie geht es dir?"),
			AssistantMessage("Mir geht es gut."),
			UserMessage("Das ist wunderbar."),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "de", result.State["detected_language"])
	require.Len(t, result.Messages, 3)

	// All messages should be tagged, not just the user messages.
	for i, msg := range result.Messages {
		require.Equal(t, "de", msg.Meta["detected_language"],
			"message %d should have detected_language tag", i)
	}

	// Only one LLM call with the first user message content.
	require.Len(t, mock.Calls, 1)
	require.Equal(t, "Hallo, wie geht es dir?", mock.Calls[0].Input)
}

func TestLanguageDetector_RunAllPhases(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"language":     "en",
		"confidence":   0.99,
		"alternatives": []string{},
	})

	mock := &MockLLMClient{Response: string(resp)}
	d := NewLanguageDetector(mock)

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			UserMessage("Hello world"),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	final := RunAllPhases(t, d, pctx)
	require.Len(t, final.Messages, 1)
	require.Equal(t, "en", final.Messages[0].Meta["detected_language"])
	require.Equal(t, "en", final.State["detected_language"])
}

func TestLanguageDetector_PreservesExistingMeta(t *testing.T) {
	t.Parallel()

	resp, _ := json.Marshal(map[string]any{
		"language":     "en",
		"confidence":   0.95,
		"alternatives": []string{},
	})

	mock := &MockLLMClient{Response: string(resp)}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			{
				Role:    "user",
				Content: "Hello",
				Meta:    map[string]any{"existing_key": "existing_value"},
			},
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, "existing_value", result.Messages[0].Meta["existing_key"])
	require.Equal(t, "en", result.Messages[0].Meta["detected_language"])
}

func TestLanguageDetector_NoUserMessages(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			AssistantMessage("I can help with that."),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, "", result.State["detected_language"])
	require.Empty(t, mock.Calls, "LLM should not be called without user content")
}

func TestLanguageDetector_EmptyUserMessage(t *testing.T) {
	t.Parallel()

	mock := &MockLLMClient{}
	d := NewLanguageDetector(mock)
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			UserMessage(""),
			UserMessage("   "),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := d.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, "", result.State["detected_language"])
	require.Empty(t, mock.Calls, "LLM should not be called for whitespace-only input")
}
