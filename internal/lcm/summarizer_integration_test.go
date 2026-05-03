package lcm

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestIntegration_Summarizer_Summarize(t *testing.T) {
	testutil.SkipIfNoIntegration(t)
	t.Parallel()

	llm := testutil.NewLLMClient(t)
	s := NewSummarizer(llm)

	messages := []MessageForSummary{
		{Seq: 1, Role: "user", Content: "Can you help me refactor the authentication module? It's getting too complex."},
		{Seq: 2, Role: "assistant", Content: "I'd be happy to help refactor the authentication module. Let me look at the current structure. The main issues I see are: 1) The AuthService class handles both JWT and session-based auth, violating SRP. 2) Token validation is duplicated across three files. 3) The middleware chain is hard to follow."},
		{Seq: 3, Role: "user", Content: "Yes, exactly. Can you split the AuthService into separate JWT and session handlers?"},
		{Seq: 4, Role: "assistant", Content: "I've split AuthService into JWTAuthHandler and SessionAuthHandler. Each now has a single responsibility. I also extracted the shared token validation into a TokenValidator utility. The middleware chain now uses a strategy pattern to select the appropriate handler. Files modified: internal/auth/jwt_handler.go, internal/auth/session_handler.go, internal/auth/token_validator.go, internal/middleware/auth.go."},
		{Seq: 5, Role: "user", Content: "That looks great. What about the tests?"},
	}

	// Compute the input token count the same way Summarize does internally:
	// it compares EstimateTokens(result) against EstimateTokens(userPrompt),
	// where userPrompt includes the XML framing from formatMessagesForSummary.
	inputTokens := EstimateTokens(formatMessagesForSummary(messages))

	result, tokens, err := s.Summarize(context.Background(), SummaryInput{
		SessionID: "test-session",
		Messages:  messages,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result)
	require.Greater(t, tokens, int64(0))

	// The three-level escalation guarantees the output is smaller than the
	// input (normal → aggressive → deterministic truncation). Verify that
	// the full pipeline actually achieves compression.
	require.Less(t, tokens, inputTokens,
		"summary tokens (%d) should be less than input tokens (%d) after three-level escalation",
		tokens, inputTokens)

	t.Logf("Summary (%d tokens, input was %d tokens):\n%s", tokens, inputTokens, result)
}

func TestIntegration_Summarizer_Condense(t *testing.T) {
	testutil.SkipIfNoIntegration(t)
	t.Parallel()

	llm := testutil.NewLLMClient(t)
	s := NewSummarizer(llm)

	summaries := []ContextEntry{
		{
			SummaryID:      "sum_000001",
			SummaryContent: "User requested refactoring of the authentication module. Assistant split AuthService into JWTAuthHandler and SessionAuthHandler, extracted TokenValidator utility, and applied strategy pattern to middleware. Files: internal/auth/jwt_handler.go, internal/auth/session_handler.go, internal/auth/token_validator.go, internal/middleware/auth.go.",
			SummaryKind:    KindLeaf,
			TokenCount:     80,
		},
		{
			SummaryID:      "sum_000002",
			SummaryContent: "User asked about test coverage. Assistant added unit tests for JWTAuthHandler and SessionAuthHandler with table-driven tests. Integration tests added for the middleware chain. Test coverage increased from 45% to 89%. Files: internal/auth/jwt_handler_test.go, internal/auth/session_handler_test.go, internal/middleware/auth_test.go.",
			SummaryKind:    KindLeaf,
			TokenCount:     70,
		},
	}

	result, tokens, err := s.Condense(context.Background(), summaries)
	require.NoError(t, err)
	require.NotEmpty(t, result)
	require.Greater(t, tokens, int64(0))

	// Condensed summary should be shorter than both inputs combined.
	combinedLen := len(summaries[0].SummaryContent) + len(summaries[1].SummaryContent)
	require.Less(t, len(result), combinedLen,
		"condensed summary should be shorter than combined input")

	t.Logf("Condensed:\n%s", result)
}
