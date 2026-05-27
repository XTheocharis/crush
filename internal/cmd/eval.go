package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/crush/internal/eval"
	"github.com/charmbracelet/crush/internal/eval/scorers/judge"
	"github.com/charmbracelet/crush/internal/eval/scorers/metric"
	"github.com/spf13/cobra"
)

// evalCmd represents the eval command.
var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Run evaluation harness for scoring agent performance",
	Long: `Run evaluation scorers to measure agent performance across
multiple dimensions including code quality, test coverage, and build success.`,
	RunE: runEval,
}

var evalFlags struct {
	dataset string
	scorer  string
	input   string
	output  string
}

func init() {
	evalCmd.Flags().StringVar(&evalFlags.dataset, "dataset", "", "Path to JSON dataset file")
	evalCmd.Flags().StringVar(&evalFlags.scorer, "scorer", "", "Scorer to run (e.g., build, test, quality)")
	evalCmd.Flags().StringVar(&evalFlags.input, "input", ".", "Input file or directory")
	evalCmd.Flags().StringVar(&evalFlags.output, "output", "", "Output report file (JSON)")
}

func runEval(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	if evalFlags.scorer == "" {
		fmt.Fprintln(out, "Available scorers: build, test, quality")
		return nil
	}

	if evalFlags.dataset == "" {
		return fmt.Errorf("--dataset is required when running a scorer")
	}

	dataset, err := eval.LoadDataset(evalFlags.dataset)
	if err != nil {
		return fmt.Errorf("failed to load dataset: %w", err)
	}

	harness := eval.NewEvalHarness()
	registerScorers(harness, nil)

	criteria := eval.NewScoringCriteria()
	reportGen := eval.NewReportGenerator(criteria)
	runner := eval.NewEvalRunner(harness, nil)

	outcome, err := runner.Run(ctx, dataset, evalFlags.dataset, evalFlags.scorer)
	if err != nil {
		return fmt.Errorf("evaluation failed: %w", err)
	}

	if evalFlags.output != "" {
		var report *eval.ScoredReport
		for _, res := range outcome.Results {
			if res.Report != nil {
				report = reportGen.ApplyCriteria(res.Report)
				break
			}
		}
		if report == nil {
			return fmt.Errorf("no results to report")
		}
		output, err := reportGen.ToJSON(report)
		if err != nil {
			return fmt.Errorf("failed to generate JSON report: %w", err)
		}
		if err := writeReportFile(evalFlags.output, output); err != nil {
			return err
		}
		fmt.Fprintf(out, "Report written to %s\n", evalFlags.output)
	} else {
		for _, res := range outcome.Results {
			if res.Report != nil {
				report := reportGen.ApplyCriteria(res.Report)
				fmt.Fprintln(out, reportGen.ToMarkdown(report))
				fmt.Fprintln(out, reportGen.ExecutiveSummary(report))
			}
		}
	}

	return nil
}

func writeReportFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write report file: %w", err)
	}
	return nil
}

type noopLLMClient struct{}

func (noopLLMClient) Complete(_ context.Context, prompt string) (string, error) {
	return `{"score": 0.5, "explanation": "noop"}`, nil
}

func registerScorers(harness *eval.EvalHarness, llmClient judge.LLMClient) *eval.EvalHarness {
	if llmClient == nil {
		llmClient = noopLLMClient{}
	}
	threshold := 0.6

	harness.Register(judge.NewCodeQualityScorer(llmClient, threshold))
	harness.Register(judge.NewCorrectnessScorer(llmClient, threshold))
	harness.Register(judge.NewCompletenessScorer(llmClient, threshold))
	harness.Register(judge.NewClarityScorer(llmClient, threshold))
	harness.Register(judge.NewSafetyScorer(llmClient, threshold))
	harness.Register(judge.NewPerformanceScorer(llmClient, threshold))
	harness.Register(judge.NewMaintainabilityScorer(llmClient, threshold))
	harness.Register(judge.NewErrorHandlingScorer(llmClient, threshold))
	harness.Register(judge.NewDocumentationScorer(llmClient, threshold))
	harness.Register(judge.NewConventionsScorer(llmClient, threshold))
	harness.Register(judge.NewTestingQualityScorer(llmClient, threshold))
	harness.Register(judge.NewEdgeCasesScorer(llmClient, threshold))

	harness.Register(metric.NewBuildSuccessScorer())
	harness.Register(metric.NewTestPassRateScorer(threshold))
	harness.Register(metric.NewSyntaxValidityScorer(threshold))
	harness.Register(metric.NewLintScoreScorer(50, threshold))
	harness.Register(metric.NewEditDistanceScorer(threshold))
	harness.Register(metric.NewCoverageScoreScorer(threshold))
	harness.Register(metric.NewTypeCheckScoreScorer(20, threshold))

	return harness
}
