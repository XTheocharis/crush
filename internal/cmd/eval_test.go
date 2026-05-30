package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/eval"
	"github.com/charmbracelet/crush/internal/eval/scorers/judge"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestEvalCommand_Exists(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "eval" {
			found = true
			break
		}
	}
	require.True(t, found, "eval command should be registered on rootCmd")
}

func TestEvalCommand_HasFlags(t *testing.T) {
	scorer := evalCmd.Flags().Lookup("scorer")
	require.NotNil(t, scorer, "should have --scorer flag")

	dataset := evalCmd.Flags().Lookup("dataset")
	require.NotNil(t, dataset, "should have --dataset flag")

	input := evalCmd.Flags().Lookup("input")
	require.NotNil(t, input, "should have --input flag")
	require.Equal(t, ".", input.DefValue, "--input should default to '.'")

	output := evalCmd.Flags().Lookup("output")
	require.NotNil(t, output, "should have --output flag")
	require.Equal(t, "", output.DefValue, "--output should default to empty")
}

func TestEvalCommand_NoScorer_ListsAvailable(t *testing.T) {
	var b bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&b)

	evalFlags.scorer = ""
	evalFlags.input = "."
	evalFlags.output = ""

	err := runEval(cmd, nil)
	require.NoError(t, err)

	output := b.String()
	require.True(t, strings.Contains(output, "Available scorers"), "should list available scorers")
}

func TestEvalCommand_WithScorer_Runs(t *testing.T) {
	tmpDir := t.TempDir()
	dataset := `{"name":"test","examples":[{"id":"e1","name":"example","input":{"session_id":"s1"}}]}`
	datasetPath := filepath.Join(tmpDir, "dataset.json")
	require.NoError(t, os.WriteFile(datasetPath, []byte(dataset), 0o644))

	var b bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&b)
	cmd.SetContext(context.Background())

	evalFlags.dataset = datasetPath
	evalFlags.scorer = "build"
	evalFlags.input = tmpDir
	evalFlags.output = ""

	err := runEval(cmd, nil)
	require.NoError(t, err)

	output := b.String()
	require.True(t, strings.Contains(output, "Evaluation Report"), "should produce evaluation report: %s", output)

	evalFlags.dataset = ""
	evalFlags.scorer = ""
	evalFlags.input = "."
}

func TestEvalCommand_WithScorerAndOutput_Runs(t *testing.T) {
	tmpDir := t.TempDir()

	dataset := `{"name":"test","examples":[{"id":"e1","name":"example","input":{"session_id":"s1"}}]}`
	datasetPath := filepath.Join(tmpDir, "dataset.json")
	require.NoError(t, os.WriteFile(datasetPath, []byte(dataset), 0o644))

	reportPath := filepath.Join(tmpDir, "report.json")

	var b bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&b)
	cmd.SetContext(context.Background())

	evalFlags.dataset = datasetPath
	evalFlags.scorer = "test"
	evalFlags.input = tmpDir
	evalFlags.output = reportPath

	err := runEval(cmd, nil)
	require.NoError(t, err)

	output := b.String()
	require.True(t, strings.Contains(output, reportPath), "should mention output path: %s", output)

	evalFlags.dataset = ""
	evalFlags.scorer = ""
	evalFlags.input = "."
	evalFlags.output = ""
}

func TestEvalCLIRegistration(t *testing.T) {
	harness := registerScorers(eval.NewEvalHarness(), nil)

	names := harness.Scorers()
	require.Len(t, names, 19, "expected 19 scorers, got %d: %v", len(names), names)

	expected := []string{
		"code_quality",
		"correctness",
		"completeness",
		"clarity",
		"safety",
		"performance",
		"maintainability",
		"error_handling",
		"documentation",
		"conventions",
		"testing_quality",
		"edge_cases",
		"build_success",
		"test_pass_rate",
		"syntax_validity",
		"lint_score",
		"edit_distance",
		"coverage_score",
		"typecheck_score",
	}

	sort.Strings(expected)
	sort.Strings(names)
	require.Equal(t, expected, names)
}

func TestWriteReportFile(t *testing.T) {
	t.Parallel()

	data := []byte(`{"status":"ok"}`)

	t.Run("writes to file in existing directory", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "report.json")
		require.NoError(t, writeReportFile(path, data))
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, data, got)
	})

	t.Run("creates parent directories", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "nested", "dir", "report.json")
		require.NoError(t, writeReportFile(path, data))
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, data, got)
	})

	t.Run("returns error on invalid path", func(t *testing.T) {
		t.Parallel()
		err := writeReportFile("/proc/nonexistent/dir/report.json", data)
		require.Error(t, err)
	})
}

func TestJudgeScorerUsesRealClient(t *testing.T) {
	t.Run("fantasyJudgeClient implements judge.LLMClient", func(t *testing.T) {
		t.Parallel()

		lm := &stubLanguageModel{response: `{"score": 0.9, "explanation": "good"}`}
		client := &fantasyJudgeClient{lm: lm}

		var _ judge.LLMClient = client
		resp, err := client.Complete(context.Background(), "evaluate this")
		require.NoError(t, err)
		require.Contains(t, resp, "0.9")
		require.Equal(t, 1, lm.callCount(), "expected one Generate call")
	})

	t.Run("registerScorers uses provided client not noop", func(t *testing.T) {
		tracking := &trackingLLMClient{response: `{"score": 0.8, "explanation": "ok"}`}
		harness := registerScorers(eval.NewEvalHarness(), tracking)

		input := &eval.EvalInput{
			SessionID: "test",
			Conversation: []eval.Message{
				{Role: "user", Content: "hello"},
			},
		}

		report, err := harness.Run(context.Background(), input)
		require.NoError(t, err)

		judgeCallCount := 0
		for _, entry := range report.Results {
			if entry.Type == eval.ScorerLLMJudge && entry.Result != nil && entry.Result.Error == "" {
				judgeCallCount++
			}
		}
		require.Equal(t, 12, judgeCallCount, "all 12 judge scorers should have been called")
		require.Equal(t, int64(12), tracking.callCount(), "tracking client should have been called 12 times")
	})

	t.Run("registerScorers defaults to noop when client is nil", func(t *testing.T) {
		harness := registerScorers(eval.NewEvalHarness(), nil)
		names := harness.Scorers()
		require.Len(t, names, 19, "should register all scorers even with nil client")
	})
}

type trackingLLMClient struct {
	response string
	count    atomic.Int64
}

func (c *trackingLLMClient) Complete(_ context.Context, _ string) (string, error) {
	c.count.Add(1)
	return c.response, nil
}

func (c *trackingLLMClient) callCount() int64 { return c.count.Load() }

type stubLanguageModel struct {
	response string
	count    atomic.Int64
}

func (m *stubLanguageModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	m.count.Add(1)
	return &fantasy.Response{
		Content: fantasy.ResponseContent{
			fantasy.TextContent{Text: m.response},
		},
	}, nil
}

func (m *stubLanguageModel) callCount() int { return int(m.count.Load()) }

func (m *stubLanguageModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, nil
}

func (m *stubLanguageModel) GenerateObject(_ context.Context, _ fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, nil
}

func (m *stubLanguageModel) StreamObject(_ context.Context, _ fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, nil
}

func (m *stubLanguageModel) Provider() string { return "stub" }
func (m *stubLanguageModel) Model() string    { return "stub" }

func TestEvalRunnerStorage_NonNilWithDB(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmpDir := t.TempDir()

	conn, err := db.Connect(ctx, tmpDir)
	require.NoError(t, err, "db.Connect should succeed")
	t.Cleanup(func() { db.Release(tmpDir) })

	storage := eval.NewScorerStorage(conn)
	require.NotNil(t, storage, "ScorerStorage should be non-nil when backed by real DB")

	h := eval.NewEvalHarness()
	h.Register(&stubEvalScorer{name: "accuracy", sType: eval.ScorerMetric, score: 0.9, passed: true})

	runner := eval.NewEvalRunner(h, storage)
	require.NotNil(t, runner, "EvalRunner should be non-nil")

	dataset := &eval.Dataset{
		Name: "storage-test",
		Examples: []eval.DatasetExample{
			{ID: "ex_1", Name: "test", Input: &eval.EvalInput{SessionID: "s1"}},
		},
	}

	outcome, err := runner.Run(ctx, dataset, "/data/test.json", "")
	require.NoError(t, err)
	require.True(t, outcome.Passed)

	run, err := storage.GetRun(ctx, outcome.RunID)
	require.NoError(t, err)
	require.Equal(t, "/data/test.json", run.DatasetPath)
}

type stubEvalScorer struct {
	name   string
	sType  eval.ScorerType
	score  float64
	passed bool
}

func (s *stubEvalScorer) Name() string                { return s.name }
func (s *stubEvalScorer) Type() eval.ScorerType       { return s.sType }
func (s *stubEvalScorer) Score(_ context.Context, _ *eval.EvalInput) (*eval.ScoreResult, error) {
	return &eval.ScoreResult{Score: s.score, Passed: s.passed, Explanation: "stub"}, nil
}
