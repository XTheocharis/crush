package scorers_test

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/eval"
	"github.com/charmbracelet/crush/internal/eval/scorers"
	"github.com/charmbracelet/crush/internal/eval/scorers/judge"
	"github.com/stretchr/testify/require"
)

type testLLMClient struct{}

func (testLLMClient) Complete(_ context.Context, _ string) (string, error) {
	return `{"score": 0.5, "explanation": "test"}`, nil
}

func TestSpecNamedScorers(t *testing.T) {
	t.Parallel()
	configs := scorers.SpecScorerConfigs()

	expectedNames := []string{
		"AnswerRelevancy", "AnswerSimilarity", "Faithfulness",
		"Bias", "Hallucination", "Toxicity",
		"ToolCallAccuracy", "ContextRelevance", "ContextPrecision",
		"NoiseSensitivity", "PromptAlignment", "TrajectoryScorer",
		"Completeness", "TextualDifference",
		"KeywordCoverage", "ContentSimilarity",
		"Tone", "ToolCallAccuracyCode", "TrajectoryCodeScorer",
		"MastraAnswerRelevancy", "MastraFaithfulness",
	}

	require.Len(t, configs, 21, "expected exactly 21 spec scorer configs")
	for _, name := range expectedNames {
		factory, ok := configs[name]
		require.True(t, ok, "missing spec scorer config: %s", name)
		require.NotNil(t, factory, "nil factory for %s", name)
	}
}

func TestAllSpecScorersRegister(t *testing.T) {
	t.Parallel()
	configs := scorers.SpecScorerConfigs()
	h := eval.NewEvalHarness()
	client := testLLMClient{}
	for _, factory := range configs {
		factory(h, client, 0.6)
	}
	registered := h.Scorers()
	require.Len(t, registered, 21, "expected 21 registered scorers")
}

var _ judge.LLMClient = testLLMClient{}
