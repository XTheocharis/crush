package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/openai"
	"charm.land/fantasy/providers/openaicompat"
	"charm.land/fantasy/providers/openrouter"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/eval"
	"github.com/charmbracelet/crush/internal/eval/scorers/judge"
	"github.com/charmbracelet/crush/internal/eval/scorers/mastra"
	"github.com/charmbracelet/crush/internal/eval/scorers/metric"
	"github.com/charmbracelet/crush/internal/message"
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
	capture string
}

func init() {
	evalCmd.Flags().StringVar(&evalFlags.dataset, "dataset", "", "Path to JSON dataset file")
	evalCmd.Flags().StringVar(&evalFlags.scorer, "scorer", "", "Scorer to run (e.g., build_success, code_quality, correctness)")
	evalCmd.Flags().StringVar(&evalFlags.input, "input", ".", "Input file or directory")
	evalCmd.Flags().StringVar(&evalFlags.output, "output", "", "Output report file (JSON)")
	evalCmd.Flags().StringVar(&evalFlags.capture, "capture", "", "Capture a session into eval dataset format (session ID)")
}

func runEval(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	out := cmd.OutOrStdout()

	if evalFlags.capture != "" {
		return runCapture(ctx, cmd, out)
	}

	if evalFlags.scorer == "" {
		fmt.Fprintln(out, "Available scorers:")
		fmt.Fprintln(out, "  Judge:   code_quality, correctness, completeness, clarity, safety, performance,")
		fmt.Fprintln(out, "           maintainability, error_handling, documentation, conventions, testing_quality, edge_cases")
		fmt.Fprintln(out, "  Metric:  build_success, test_pass_rate, syntax_validity, lint_score, edit_distance,")
		fmt.Fprintln(out, "           coverage_score, typecheck_score")
		return nil
	}

	if evalFlags.dataset == "" {
		return fmt.Errorf("--dataset is required when running a scorer")
	}

	dataset, err := eval.LoadDataset(evalFlags.dataset)
	if err != nil {
		return fmt.Errorf("failed to load dataset: %w", err)
	}

	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	dataDir, _ := cmd.Flags().GetString("data-dir")
	cfgStore, err := config.Init(cwd, dataDir, false)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}
	if dataDir == "" {
		dataDir = cfgStore.Config().Options.DataDirectory
	}

	llmClient := resolveJudgeLLMClient(cwd, dataDir)
	harness := eval.NewEvalHarness()
	registerScorers(harness, llmClient)

	var storage *eval.ScorerStorage
	conn, connErr := db.Connect(ctx, dataDir)
	if connErr != nil {
		slog.Warn("Failed to connect eval storage, results will not be persisted", "error", connErr)
	} else {
		defer db.Release(dataDir)
		storage = eval.NewScorerStorage(conn)
	}

	criteria := eval.NewScoringCriteria()
	reportGen := eval.NewReportGenerator(criteria)
	runner := eval.NewEvalRunner(harness, storage)

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

// fantasyJudgeClient adapts a fantasy.LanguageModel to the judge.LLMClient
// interface used by eval judge scorers. It sends the evaluation prompt as a
// single user message and returns the raw text response.
type fantasyJudgeClient struct {
	lm fantasy.LanguageModel
}

func (c *fantasyJudgeClient) Complete(ctx context.Context, prompt string) (string, error) {
	resp, err := c.lm.Generate(ctx, fantasy.Call{
		Prompt: []fantasy.Message{
			fantasy.NewUserMessage(prompt),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Content.Text(), nil
}

func buildEvalProvider(cfg *config.ProviderConfig, apiKey, baseURL string) (fantasy.Provider, error) {
	switch cfg.Type {
	case openai.Name:
		opts := []openai.Option{openai.WithAPIKey(apiKey)}
		if baseURL != "" {
			opts = append(opts, openai.WithBaseURL(baseURL))
		}
		return openai.New(opts...)
	case anthropic.Name:
		var opts []anthropic.Option
		if apiKey != "" {
			opts = append(opts, anthropic.WithAPIKey(apiKey))
		}
		if baseURL != "" {
			opts = append(opts, anthropic.WithBaseURL(baseURL))
		}
		return anthropic.New(opts...)
	case openrouter.Name:
		return openrouter.New(openrouter.WithAPIKey(apiKey))
	case openaicompat.Name:
		opts := []openaicompat.Option{
			openaicompat.WithAPIKey(apiKey),
		}
		if baseURL != "" {
			opts = append(opts, openaicompat.WithBaseURL(baseURL))
		}
		return openaicompat.New(opts...)
	default:
		return nil, fmt.Errorf("unsupported eval provider type: %q", cfg.Type)
	}
}

// resolveJudgeLLMClient attempts to build a real LLM client from the project
// configuration. It uses the small model if available (judge prompts are short)
// and falls back to the large model. Returns noopLLMClient when no provider is
// configured.
func resolveJudgeLLMClient(cwd, dataDir string) judge.LLMClient {
	store, err := config.Init(cwd, dataDir, false)
	if err != nil {
		slog.Debug("Eval judge: config init failed, using noop client", "error", err)
		return noopLLMClient{}
	}

	cfg := store.Config()

	// Prefer the small model for judge calls (prompts are short), fall back to
	// the large model.
	modelType := config.SelectedModelTypeSmall
	providerCfg := cfg.GetProviderForModel(modelType)
	if providerCfg == nil {
		modelType = config.SelectedModelTypeLarge
		providerCfg = cfg.GetProviderForModel(modelType)
	}
	if providerCfg == nil {
		slog.Debug("Eval judge: no provider configured, using noop client")
		return noopLLMClient{}
	}

	selected, ok := cfg.Models[modelType]
	if !ok {
		slog.Debug("Eval judge: no model selected, using noop client")
		return noopLLMClient{}
	}

	apiKey, _ := store.Resolve(providerCfg.APIKey)
	baseURL, _ := store.Resolve(providerCfg.BaseURL)

	provider, err := buildEvalProvider(providerCfg, apiKey, baseURL)
	if err != nil {
		slog.Debug("Eval judge: provider build failed, using noop client", "error", err)
		return noopLLMClient{}
	}

	lm, err := provider.LanguageModel(context.Background(), selected.Model)
	if err != nil {
		slog.Debug("Eval judge: language model resolution failed, using noop client", "error", err)
		return noopLLMClient{}
	}

	slog.Debug("Eval judge: using real LLM client",
		"provider", selected.Provider,
		"model", selected.Model,
	)
	return &fantasyJudgeClient{lm: lm}
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

	harness.Register(mastra.NewMastraScorer("MastraAnswerRelevancy", llmClient, threshold, "answer_relevancy",
		"Evaluate how relevant the response is to the user's query.\nConsider:\n- Does the response directly address the question?\n- Is there unnecessary or tangential information?\n- Would the answer satisfy the user's intent?"))
	harness.Register(mastra.NewMastraScorer("MastraFaithfulness", llmClient, threshold, "faithfulness",
		"Evaluate whether the response is faithful to the provided context.\nConsider:\n- Does the response contain information not supported by the context?\n- Are claims backed by evidence?\n- Is there any fabrication or unsupported inference?"))

	return harness
}

func runCapture(ctx context.Context, cmd *cobra.Command, out io.Writer) error {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	dataDir, _ := cmd.Flags().GetString("data-dir")
	cfgStore, err := config.Init(cwd, dataDir, false)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}
	if dataDir == "" {
		dataDir = cfgStore.Config().Options.DataDirectory
	}

	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}
	defer db.Release(dataDir)

	queries := db.New(conn)
	msgService := message.NewService(queries)

	sessionID := evalFlags.capture
	msgs, err := msgService.List(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to list messages for session %s: %w", sessionID, err)
	}

	dataset, err := eval.CaptureSession(ctx, sessionID, msgs)
	if err != nil {
		return fmt.Errorf("failed to capture session: %w", err)
	}

	outputPath := evalFlags.output
	if outputPath == "" {
		outputPath = fmt.Sprintf("capture_%s.json", sessionID)
	}

	if err := eval.WriteCaptureDataset(dataset, outputPath); err != nil {
		return fmt.Errorf("failed to write dataset: %w", err)
	}

	fmt.Fprintf(out, "Captured %d examples from session %s to %s\n", len(dataset.Examples), sessionID, outputPath)
	return nil
}
