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

func TestSummarizer_SetLLM(t *testing.T) {
	t.Parallel()
	s := NewSummarizer(nil)
	ctx := context.Background()

	input := SummaryInput{
		SessionID: "sess-set-llm",
		Messages: []MessageForSummary{
			{ID: "m1", Role: "user", Content: "Hello world"},
		},
	}

	text, _, err := s.Summarize(ctx, input)
	require.NoError(t, err)
	require.Contains(t, text, "Hello world")

	mock := &mockLLMClient{response: "Configured summary"}
	s.SetLLM(mock)

	text, _, err = s.Summarize(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "Configured summary", text)
	require.Equal(t, 1, mock.callCount)
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

// promptTrackingMock records system prompts used during LLM calls.
type promptTrackingMock struct {
	responses     []string
	systemPrompts []string
	callNum       *int
}

func (m *promptTrackingMock) Complete(_ context.Context, systemPrompt, _ string) (string, error) {
	m.systemPrompts = append(m.systemPrompts, systemPrompt)
	idx := *m.callNum
	*m.callNum++
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return m.responses[len(m.responses)-1], nil
}

// TestFiveLevelEscalation verifies the 5-level escalation path:
//
//	Level 0 (Normal) → Level 1 (Extractive) → Level 2 (Aggressive)
//	  → Level 3 (Skeleton) → Level 4 (Deterministic/truncation)
//
// Each LLM level must use the correct system prompt, and Level 4 must not
// call the LLM at all (it uses deterministic truncation).
func TestFiveLevelEscalation(t *testing.T) {
	t.Parallel()

	input := SummaryInput{
		SessionID: "sess-5level",
		Messages: []MessageForSummary{
			{ID: "m1", Role: "user", Content: "Short message"},
		},
	}

	longResponse := strings.Repeat("verbose summary content. ", 200)
	callNum := 0

	mock := &promptTrackingMock{
		responses: []string{longResponse, longResponse, longResponse, longResponse},
		callNum:   &callNum,
	}

	s := NewSummarizer(mock)
	ctx := context.Background()

	text, tokens, err := s.Summarize(ctx, input)
	require.NoError(t, err)

	require.Equal(t, 4, *mock.callNum,
		"should call LLM exactly 4 times before deterministic fallback")
	require.LessOrEqual(t, len(text), FallbackMaxChars)
	require.Greater(t, tokens, int64(0))
	require.Len(t, mock.systemPrompts, 4)
	require.Equal(t, normalSummarizeSystemPrompt, mock.systemPrompts[0],
		"Level 0 should use normal prompt")
	require.Equal(t, extractiveSummarizeSystemPrompt, mock.systemPrompts[1],
		"Level 1 should use extractive prompt")
	require.Equal(t, aggressiveSummarizeSystemPrompt, mock.systemPrompts[2],
		"Level 2 should use aggressive prompt")
	require.Equal(t, skeletonSummarizeSystemPrompt, mock.systemPrompts[3],
		"Level 3 should use skeleton prompt")
}

// TestFiveLevelEscalation_Condense verifies the 5-level path for Condense.
func TestFiveLevelEscalation_Condense(t *testing.T) {
	t.Parallel()

	summaries := []ContextEntry{
		{SummaryID: "sum_1", SummaryContent: "Summary one content", SummaryKind: KindLeaf},
	}

	longResponse := strings.Repeat("verbose condensation. ", 200)
	callNum := 0

	mock := &promptTrackingMock{
		responses: []string{longResponse, longResponse, longResponse, longResponse},
		callNum:   &callNum,
	}

	s := NewSummarizer(mock)
	ctx := context.Background()

	text, tokens, err := s.Condense(ctx, summaries)
	require.NoError(t, err)

	require.Equal(t, 4, *mock.callNum,
		"Condense should call LLM exactly 4 times before deterministic fallback")
	require.LessOrEqual(t, len(text), FallbackMaxChars)
	require.Greater(t, tokens, int64(0))

	require.Len(t, mock.systemPrompts, 4)
	require.Equal(t, normalCondenseSystemPrompt, mock.systemPrompts[0])
	require.Equal(t, extractiveCondenseSystemPrompt, mock.systemPrompts[1])
	require.Equal(t, aggressiveCondenseSystemPrompt, mock.systemPrompts[2])
	require.Equal(t, skeletonCondenseSystemPrompt, mock.systemPrompts[3])
}

// TestCompressionLevelOrdering verifies the CompressionLevel enum values.
func TestCompressionLevelOrdering(t *testing.T) {
	t.Parallel()

	levels := []CompressionLevel{
		LevelNormal,
		LevelExtractive,
		LevelAggressive,
		LevelSkeleton,
		LevelDeterministic,
	}

	// Verify sequential ordering.
	for i, lvl := range levels {
		require.Equal(t, CompressionLevel(i), lvl,
			"CompressionLevel(%d) mismatch", i)
	}
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
