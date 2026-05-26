package processor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnicodeNormalizer_ID(t *testing.T) {
	t.Parallel()
	p := UnicodeNormalizer{}
	require.Equal(t, "unicode_normalizer", p.ID())
}

func TestUnicodeNormalizer_NFKCNormalization(t *testing.T) {
	t.Parallel()
	p := UnicodeNormalizer{}
	ctx := context.Background()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"ligature", "\ufb01nd", "find"},
		{"circled_digit", "\u2463", "4"},
		{"fraction", "\u00bc", "1⁄4"},
		{"fullwidth_ascii", "\uff28ello", "Hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pctx := ProcessorContext{
				Phase:    InputPhase,
				Messages: []Message{UserMessage(tt.input)},
				State:    make(map[string]any),
				Metadata: make(map[string]any),
			}
			result, err := p.ProcessInput(ctx, pctx)
			require.NoError(t, err)
			AssertPhaseAction(t, result, ActionContinue)
			require.Len(t, result.Messages, 1)
			require.Equal(t, tt.expected, result.Messages[0].Content)
		})
	}
}

func TestUnicodeNormalizer_ZeroWidthStripping(t *testing.T) {
	t.Parallel()
	p := UnicodeNormalizer{}
	ctx := context.Background()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"zw_space", "hello\u200bworld", "helloworld"},
		{"zw_non_joiner", "hello\u200cworld", "helloworld"},
		{"zw_joiner", "hello\u200dworld", "helloworld"},
		{"bom", "\ufeffhello", "hello"},
		{"soft_hyphen", "hello\u00adworld", "helloworld"},
		{"multiple_zw", "a\u200bb\u200cc\u200dd", "abcd"},
		{"all_combined", "\ufeff\u200bhello\u200c\u00adworld\u200d", "helloworld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pctx := ProcessorContext{
				Phase:    InputPhase,
				Messages: []Message{UserMessage(tt.input)},
				State:    make(map[string]any),
				Metadata: make(map[string]any),
			}
			result, err := p.ProcessInput(ctx, pctx)
			require.NoError(t, err)
			AssertPhaseAction(t, result, ActionContinue)
			require.Len(t, result.Messages, 1)
			require.Equal(t, tt.expected, result.Messages[0].Content)
		})
	}
}

func TestUnicodeNormalizer_WhitespaceNormalization(t *testing.T) {
	t.Parallel()
	p := UnicodeNormalizer{}
	ctx := context.Background()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"multiple_spaces", "hello   world", "hello world"},
		{"tabs_and_spaces", "hello\t \tworld", "hello world"},
		{"leading_trailing", "  hello world  ", "hello world"},
		{"tabs_only", "hello\t\tworld", "hello world"},
		{"mixed_whitespace", "  hello  \t  world  ", "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pctx := ProcessorContext{
				Phase:    InputPhase,
				Messages: []Message{UserMessage(tt.input)},
				State:    make(map[string]any),
				Metadata: make(map[string]any),
			}
			result, err := p.ProcessInput(ctx, pctx)
			require.NoError(t, err)
			AssertPhaseAction(t, result, ActionContinue)
			require.Len(t, result.Messages, 1)
			require.Equal(t, tt.expected, result.Messages[0].Content)
		})
	}
}

func TestUnicodeNormalizer_AssistantAndToolMessages(t *testing.T) {
	t.Parallel()
	p := UnicodeNormalizer{}
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			UserMessage("hello\u200bworld"),
			AssistantMessage("hi  there\u200c"),
			ToolUseMessage("t1", "bash", "tool\t\u200dinput"),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	AssertPhaseAction(t, result, ActionContinue)
	require.Len(t, result.Messages, 3)
	require.Equal(t, "helloworld", result.Messages[0].Content)
	require.Equal(t, "hi there", result.Messages[1].Content)
	require.Equal(t, "tool input", result.Messages[2].Content)
}

func TestUnicodeNormalizer_EmptyInput(t *testing.T) {
	t.Parallel()
	p := UnicodeNormalizer{}
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	AssertPhaseAction(t, result, ActionContinue)
	require.Empty(t, result.Messages)
}

func TestUnicodeNormalizer_AlreadyNormalized(t *testing.T) {
	t.Parallel()
	p := UnicodeNormalizer{}
	ctx := context.Background()

	original := "Hello, world! This is already normalized."
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage(original)},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	AssertPhaseAction(t, result, ActionContinue)
	require.Len(t, result.Messages, 1)
	require.Equal(t, original, result.Messages[0].Content)
}

func TestUnicodeNormalizer_PreservesNewlines(t *testing.T) {
	t.Parallel()
	p := UnicodeNormalizer{}
	ctx := context.Background()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage("hello\nworld\nline3")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, "hello\nworld\nline3", result.Messages[0].Content)
}

func TestUnicodeNormalizer_RunAllPhases(t *testing.T) {
	t.Parallel()
	p := UnicodeNormalizer{}

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			UserMessage("hello\u200bworld"),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	final := RunAllPhases(t, p, pctx)
	require.Len(t, final.Messages, 1)
	require.Equal(t, "helloworld", final.Messages[0].Content)
}

func TestUnicodeNormalizer_CombinedNormalization(t *testing.T) {
	t.Parallel()
	p := UnicodeNormalizer{}
	ctx := context.Background()

	input := "\ufeff  \ufb01nd  \u2463\u200b  items  "
	expected := "find 4 items"

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Messages: []Message{UserMessage(input)},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	AssertPhaseAction(t, result, ActionContinue)
	require.Len(t, result.Messages, 1)
	require.Equal(t, expected, result.Messages[0].Content)
}
