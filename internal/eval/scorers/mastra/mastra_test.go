package mastra

import (
	"context"
	"fmt"
	"testing"

	"github.com/charmbracelet/crush/internal/eval"
	"github.com/stretchr/testify/require"
)

// mockLLM implements judge.LLMClient for testing.
type mockLLM struct {
	response string
	err      error
	called   bool
	prompt   string
}

func (m *mockLLM) Complete(_ context.Context, prompt string) (string, error) {
	m.called = true
	m.prompt = prompt
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

// mockStepHandler records calls for pipeline order verification.
type mockStepHandler struct {
	name      string
	called    bool
	callOrder *[]string
	output    *StepOutput
	err       error
}

func (h *mockStepHandler) Handle(_ context.Context, _ *StepInput) (*StepOutput, error) {
	h.called = true
	*h.callOrder = append(*h.callOrder, h.name)
	if h.err != nil {
		return nil, h.err
	}
	return h.output, nil
}

// trackingHandler returns a handler that appends its name to order on call.
func trackingHandler(name string, order *[]string, result string, score float64) *mockStepHandler {
	return &mockStepHandler{
		name:      name,
		callOrder: order,
		output:    &StepOutput{Result: result, Score: score},
	}
}

func TestPipelineOrder(t *testing.T) {
	t.Parallel()

	order := &[]string{}
	s := &MastraScorer{
		scorerName: "test_pipeline",
		threshold:  0.5,
		steps: [4]StepHandler{
			trackingHandler("preprocess", order, "normalized", 0),
			trackingHandler("analyze", order, "analysis done", 0),
			trackingHandler("generateScore", order, "", 0.8),
			trackingHandler("generateReason", order, "explanation", 0.8),
		},
	}

	result, err := s.Score(context.Background(), &eval.EvalInput{
		SessionID: "test",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"preprocess", "analyze", "generateScore", "generateReason"}, *order)
	require.Equal(t, 0.8, result.Score)
}

func TestScoreResult(t *testing.T) {
	t.Parallel()

	s := NewMastraScorer("test_scorer", &mockLLM{
		response: `{"score": 0.85, "explanation": "Highly relevant answer."}`,
	}, 0.6, "answer_relevancy",
		"Evaluate how relevant the response is to the user's query.")

	result, err := s.Score(context.Background(), &eval.EvalInput{
		SessionID: "test",
		Conversation: []eval.Message{
			{Role: "user", Content: "What is Go?"},
			{Role: "assistant", Content: "Go is a statically typed compiled language."},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 0.85, result.Score)
	require.Equal(t, "Highly relevant answer.", result.Explanation)
	require.True(t, result.Passed)
	require.Equal(t, eval.ScorerMastra, s.Type())
	require.Equal(t, "test_scorer", s.Name())
}

func TestPreprocessStep(t *testing.T) {
	t.Parallel()

	var receivedInput *StepInput
	s := &MastraScorer{
		scorerName: "test_preprocess",
		threshold:  0.5,
		steps: [4]StepHandler{
			&defaultPreprocessHandler{},
			StepHandlerFunc(func(_ context.Context, input *StepInput) (*StepOutput, error) {
				receivedInput = input
				return &StepOutput{Result: "analyzed", Score: 0}, nil
			}),
			StepHandlerFunc(func(_ context.Context, input *StepInput) (*StepOutput, error) {
				return &StepOutput{Result: "", Score: 0.7}, nil
			}),
			StepHandlerFunc(func(_ context.Context, input *StepInput) (*StepOutput, error) {
				return &StepOutput{Result: "reason", Score: 0.7}, nil
			}),
		},
	}

	_, err := s.Score(context.Background(), &eval.EvalInput{
		SessionID: "test",
		Conversation: []eval.Message{
			{Role: "user", Content: "Hello"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, receivedInput)
	require.NotEmpty(t, receivedInput.PreviousOutput)
}

func TestPipelineErrorHandling(t *testing.T) {
	t.Parallel()

	s := &MastraScorer{
		scorerName: "test_error",
		threshold:  0.5,
		steps: [4]StepHandler{
			StepHandlerFunc(func(_ context.Context, _ *StepInput) (*StepOutput, error) {
				return nil, fmt.Errorf("preprocess failed")
			}),
			trackingHandler("analyze", &[]string{}, "", 0),
			trackingHandler("generateScore", &[]string{}, "", 0),
			trackingHandler("generateReason", &[]string{}, "", 0),
		},
	}

	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "preprocess failed")
}

func TestScoreBelowThreshold(t *testing.T) {
	t.Parallel()

	s := NewMastraScorer("test_low", &mockLLM{
		response: `{"score": 0.3, "explanation": "Poor answer."}`,
	}, 0.6, "answer_relevancy",
		"Evaluate relevance.")

	result, err := s.Score(context.Background(), &eval.EvalInput{
		SessionID: "test",
	})
	require.NoError(t, err)
	require.Equal(t, 0.3, result.Score)
	require.False(t, result.Passed)
}

func TestScoreClampedToRange(t *testing.T) {
	t.Parallel()

	s := &MastraScorer{
		scorerName: "test_clamp",
		threshold:  0.5,
		steps: [4]StepHandler{
			StepHandlerFunc(func(_ context.Context, _ *StepInput) (*StepOutput, error) {
				return &StepOutput{Result: "preprocessed"}, nil
			}),
			StepHandlerFunc(func(_ context.Context, _ *StepInput) (*StepOutput, error) {
				return &StepOutput{Result: "analyzed"}, nil
			}),
			StepHandlerFunc(func(_ context.Context, _ *StepInput) (*StepOutput, error) {
				return &StepOutput{Result: "", Score: 1.5}, nil
			}),
			StepHandlerFunc(func(_ context.Context, _ *StepInput) (*StepOutput, error) {
				return &StepOutput{Result: "over score"}, nil
			}),
		},
	}

	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
}

func TestScoreClampedNegative(t *testing.T) {
	t.Parallel()

	s := &MastraScorer{
		scorerName: "test_clamp_neg",
		threshold:  0.5,
		steps: [4]StepHandler{
			StepHandlerFunc(func(_ context.Context, _ *StepInput) (*StepOutput, error) {
				return &StepOutput{Result: "preprocessed"}, nil
			}),
			StepHandlerFunc(func(_ context.Context, _ *StepInput) (*StepOutput, error) {
				return &StepOutput{Result: "analyzed"}, nil
			}),
			StepHandlerFunc(func(_ context.Context, _ *StepInput) (*StepOutput, error) {
				return &StepOutput{Result: "", Score: -0.5}, nil
			}),
			StepHandlerFunc(func(_ context.Context, _ *StepInput) (*StepOutput, error) {
				return &StepOutput{Result: "under score"}, nil
			}),
		},
	}

	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
}

func TestDefaultPreprocessOutput(t *testing.T) {
	t.Parallel()

	handler := &defaultPreprocessHandler{}
	output, err := handler.Handle(context.Background(), &StepInput{
		EvalInput: &eval.EvalInput{
			Conversation: []eval.Message{
				{Role: "user", Content: "Hello"},
			},
			Edits: []eval.FileEdit{
				{Path: "main.go", Before: "old", After: "new"},
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, output.Result, "[user]: Hello")
	require.Contains(t, output.Result, "main.go")
}

func TestDefaultGenerateScoreFromAnalysis(t *testing.T) {
	t.Parallel()

	handler := &defaultGenerateScoreHandler{}
	output, err := handler.Handle(context.Background(), &StepInput{
		PreviousOutput: `{"score": 0.75, "explanation": "Good"}`,
	})
	require.NoError(t, err)
	require.Equal(t, 0.75, output.Score)
}

func TestDefaultGenerateReason(t *testing.T) {
	t.Parallel()

	handler := &defaultGenerateReasonHandler{}
	output, err := handler.Handle(context.Background(), &StepInput{
		PreviousOutput: `{"score": 0.75, "explanation": "Good analysis"}`,
		Metadata: map[string]any{
			"analysis": "The response is relevant.",
			"score":    0.75,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, output.Result)
}

func TestNewMastraScorerAnswerRelevancy(t *testing.T) {
	t.Parallel()

	s := NewMastraScorer("answer_relevancy", &mockLLM{
		response: `{"score": 0.9, "explanation": "Very relevant."}`,
	}, 0.6, "answer_relevancy",
		"Evaluate how relevant the response is to the user's query.")

	require.Equal(t, "answer_relevancy", s.Name())
	require.Equal(t, eval.ScorerMastra, s.Type())
}

func TestNewMastraScorerFaithfulness(t *testing.T) {
	t.Parallel()

	s := NewMastraScorer("faithfulness", &mockLLM{
		response: `{"score": 0.7, "explanation": "Mostly faithful."}`,
	}, 0.6, "faithfulness",
		"Evaluate whether the response is faithful to the provided context.")

	require.Equal(t, "faithfulness", s.Name())
	require.Equal(t, eval.ScorerMastra, s.Type())
}

func TestMastraInHarness(t *testing.T) {
	t.Parallel()

	h := eval.NewEvalHarness()
	s := NewMastraScorer("mastra_test", &mockLLM{
		response: `{"score": 0.8, "explanation": "Good."}`,
	}, 0.6, "answer_relevancy",
		"Evaluate answer relevance.")
	h.Register(s)

	report, err := h.Run(context.Background(), &eval.EvalInput{
		SessionID: "mastra_harness_test",
		Conversation: []eval.Message{
			{Role: "user", Content: "What is Go?"},
			{Role: "assistant", Content: "Go is a programming language."},
		},
	})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	require.Equal(t, 0.8, report.Results[0].Result.Score)
	require.True(t, report.Passed)
}
