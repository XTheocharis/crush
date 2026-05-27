package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/eval"
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
