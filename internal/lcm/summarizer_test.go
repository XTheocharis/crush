package lcm

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSummarizer_FallbackWhenNoLLM(t *testing.T) {
	t.Parallel()
	s := NewSummarizer(nil)
	ctx := context.Background()

	input := SummaryInput{
		SessionID: "sess-fb",
		Messages: []MessageForSummary{
			{ID: "m1", Role: "user", Content: "Hello world"},
			{ID: "m2", Role: "assistant", Content: "Hi there"},
		},
	}

	text, tokens, err := s.Summarize(ctx, input)
	require.NoError(t, err)
	require.Greater(t, tokens, int64(0))
	require.Contains(t, text, "Hello world")
	require.Contains(t, text, "Hi there")
}

func TestSummarizer_FallbackTruncation(t *testing.T) {
	t.Parallel()
	s := NewSummarizer(nil)
	ctx := context.Background()

	// Create messages that exceed FallbackMaxChars.
	longMsg := strings.Repeat("x", FallbackMaxChars+500)
	input := SummaryInput{
		SessionID: "sess-trunc",
		Messages: []MessageForSummary{
			{ID: "m1", Role: "user", Content: longMsg},
		},
	}

	text, tokens, err := s.Summarize(ctx, input)
	require.NoError(t, err)
	require.Greater(t, tokens, int64(0))
	require.LessOrEqual(t, len(text), FallbackMaxChars)
}

func TestSummarizer_WithLLM_NormalSuccess(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{
		response: "Short summary of the conversation",
	}
	s := NewSummarizer(mock)
	ctx := context.Background()

	input := SummaryInput{
		SessionID: "sess-llm",
		Messages: []MessageForSummary{
			{ID: "m1", Role: "user", Content: strings.Repeat("Long message content. ", 100)},
			{ID: "m2", Role: "assistant", Content: strings.Repeat("Long response. ", 100)},
			{ID: "m3", Role: "user", Content: strings.Repeat("Follow up. ", 100)},
		},
	}

	text, tokens, err := s.Summarize(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "Short summary of the conversation", text)
	require.Greater(t, tokens, int64(0))
	require.Equal(t, 1, mock.callCount, "should call LLM once for normal summarization")
}

func TestSummarizer_WithLLM_EscalationToAggressive(t *testing.T) {
	t.Parallel()
	// Return a response that is the same size as input to trigger escalation.
	callNum := 0
	longResponse := strings.Repeat("verbose summary content. ", 200)
	shortResponse := "brief summary"

	mock := &mockLLMClient{}
	// We'll use a custom approach: first call returns long, second returns short.
	s := NewSummarizer(&escalatingMockLLM{
		responses: []string{longResponse, shortResponse},
		callNum:   &callNum,
	})
	ctx := context.Background()

	input := SummaryInput{
		SessionID: "sess-escalate",
		Messages: []MessageForSummary{
			{ID: "m1", Role: "user", Content: "Short msg"},
		},
	}

	text, _, err := s.Summarize(ctx, input)
	require.NoError(t, err)
	require.Equal(t, shortResponse, text)
	_ = mock
}

func TestSummarizer_WithLLM_Error(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{
		err: fmt.Errorf("LLM unavailable"),
	}
	s := NewSummarizer(mock)
	ctx := context.Background()

	input := SummaryInput{
		SessionID: "sess-err",
		Messages: []MessageForSummary{
			{ID: "m1", Role: "user", Content: "test"},
		},
	}

	_, _, err := s.Summarize(ctx, input)
	require.Error(t, err)
	require.Contains(t, err.Error(), "LLM unavailable")
}

func TestSummarizer_Condense_NoLLM(t *testing.T) {
	t.Parallel()
	s := NewSummarizer(nil)
	ctx := context.Background()

	summaries := []ContextEntry{
		{SummaryID: "sum_1", SummaryContent: "Summary one content", SummaryKind: KindLeaf},
		{SummaryID: "sum_2", SummaryContent: "Summary two content", SummaryKind: KindLeaf},
	}

	text, tokens, err := s.Condense(ctx, summaries)
	require.NoError(t, err)
	require.Greater(t, tokens, int64(0))
	require.Greater(t, len(text), 0)
}

func TestSummarizer_Condense_WithLLM(t *testing.T) {
	t.Parallel()
	mock := &mockLLMClient{
		response: "Condensed summary",
	}
	s := NewSummarizer(mock)
	ctx := context.Background()

	summaries := []ContextEntry{
		{SummaryID: "sum_a", SummaryContent: strings.Repeat("content a. ", 50), SummaryKind: KindLeaf},
		{SummaryID: "sum_b", SummaryContent: strings.Repeat("content b. ", 50), SummaryKind: KindLeaf},
	}

	text, tokens, err := s.Condense(ctx, summaries)
	require.NoError(t, err)
	require.Equal(t, "Condensed summary", text)
	require.Greater(t, tokens, int64(0))
}

// escalatingMockLLM returns different responses for successive calls.
type escalatingMockLLM struct {
	responses []string
	callNum   *int
}

func (m *escalatingMockLLM) Complete(_ context.Context, _, _ string) (string, error) {
	idx := *m.callNum
	*m.callNum++
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return m.responses[len(m.responses)-1], nil
}

func BenchmarkSummarizer_Fallback(b *testing.B) {
	s := NewSummarizer(nil)
	ctx := context.Background()

	msgs := make([]MessageForSummary, 20)
	for i := range msgs {
		msgs[i] = MessageForSummary{
			ID:      fmt.Sprintf("m%d", i),
			Role:    "user",
			Content: strings.Repeat("bench message content. ", 50),
		}
	}

	input := SummaryInput{
		SessionID: "bench",
		Messages:  msgs,
	}

	for b.Loop() {
		_, _, _ = s.Summarize(ctx, input)
	}
}
