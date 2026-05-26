# Eval

Agent evaluation harness. Scorers assess agent output quality across
multiple dimensions. Invoked via `crush eval --dataset <path>
--scorer <name>`.

## Structure

- `harness.go`: `EvalHarness` (registers and runs `Scorer` instances in
  parallel), `Scorer` interface (`Name`, `Type`, `Score`), `ScorerType`
  enum (`ScorerMetric`, `ScorerLLMJudge`, `ScorerMastra`), `EvalInput`,
  `ScoreResult`, `EvalReport` types. `AggregateScore`, `PassedScorers`,
  `FailedScorers` helpers.
- `runner.go`: `EvalRunner` loads JSON datasets and runs them through a
  harness. `Dataset`, `DatasetExample`, `RunResult`, `EvalRunOutcome`
  types. `LoadDataset()` reads from disk. Supports scorer filtering by
  name.
- `storage.go`: `ScorerStorage` persists results and run metadata to
  SQLite. Tables: `scorer_results`, `eval_runs`. `HashInput()` for
  deduplication. `CompareRuns()` for side-by-side comparisons.
- `report.go`: `ReportGenerator` applies `ScoringCriteria` to produce
  `ScoredReport` with weighted aggregation. Output as JSON or markdown
  table. `Criterion` struct with threshold, weight, and scorer type.
- `readcoordinator.go`: `ReadCoordinator` reads files concurrently with
  deduplication, budget enforcement, and bounded worker pool. Supports
  `PrioritySource` for PageRank-proportional token allocation via
  `Allocate()`. `FileReader` interface for testability.
- `priority_adapter.go`: `RepomapPriorityAdapter` wraps repomap scores
  and implements `PrioritySource`. `PriorityLevel` enum (Low/Medium/High)
  with configurable thresholds.

### Scorers Sub-packages

- `scorers/config.go`: `SpecScorerConfigs()` maps spec scorer names to
  factory functions for all registered scorers.
- `scorers/judge/`: LLM-as-judge scorers (`NewLLMJudgeScorer`,
  `NewCodeQualityScorer`, etc.). `LLMClient` interface with
  `Complete(ctx, prompt)`. Formats domain-specific prompts, parses JSON
  responses.
- `scorers/mastra/`: 4-step pipeline scorer (preprocess, analyze,
  generateScore, generateReason). `MastraScorer` with configurable
  `StepHandler` per step.
- `scorers/metric/`: Deterministic code-analysis scorers:
  `BuildSuccessScorer`, `TestPassRateScorer`, `SyntaxValidityScorer`,
  `LintScoreScorer`, `EditDistanceScorer`, `CoverageScoreScorer`,
  `TypeCheckScoreScorer`, `KeywordCoverageScorer`,
  `ContentSimilarityScorer`, `TrajectoryCodeScorer`.

## Key Concepts

- **Scorer types**: `ScorerMetric` (deterministic), `ScorerLLMJudge`
  (LLM-based), `ScorerMastra` (4-step pipeline). Each produces a
  `ScoreResult` with score, explanation, pass/fail.
- **Parallel execution**: `EvalHarness.Run()` fires all scorers
  concurrently via goroutines and a `sync.WaitGroup`.
- **Scoring criteria**: Per-scorer thresholds and weights. Default:
  metric scorers need >= 0.7, LLM judges need >= 0.6. Weighted average
  across all scorers.
- **ReadCoordinator**: Bounded worker pool (default 8), token budget
  enforcement, priority-sorted file reading with per-file caps.

## Integration

- `internal/cmd/eval.go`: CLI entry point, registers scorers via
  `registerScorers()`.
- `internal/repomap`: PageRank scores fed via `RepomapPriorityAdapter`.
