package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"
)

// DatasetExample is a single test case in an eval dataset.
type DatasetExample struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Input    *EvalInput `json:"input"`
	Expected *EvalInput `json:"expected,omitempty"`
}

// Dataset is a collection of examples to evaluate.
type Dataset struct {
	Name     string           `json:"name"`
	Version  string           `json:"version,omitempty"`
	Examples []DatasetExample `json:"examples"`
}

// RunResult holds the outcome of evaluating a single example.
type RunResult struct {
	ExampleID   string      `json:"example_id"`
	ExampleName string      `json:"example_name"`
	Report      *EvalReport `json:"report"`
	InputHash   string      `json:"input_hash"`
	Error       string      `json:"error,omitempty"`
}

// EvalRunOutcome holds the complete outcome of an eval run.
type EvalRunOutcome struct {
	RunID        string        `json:"run_id"`
	DatasetPath  string        `json:"dataset_path"`
	Results      []RunResult   `json:"results"`
	OverallScore float64       `json:"overall_score"`
	Passed       bool          `json:"passed"`
	Duration     time.Duration `json:"duration"`
}

// EvalRunner loads datasets, runs evaluations through an EvalHarness,
// and persists results via ScorerStorage.
type EvalRunner struct {
	harness *EvalHarness
	storage *ScorerStorage
}

// NewEvalRunner creates a new runner with the given harness and optional storage.
// If storage is nil, results are not persisted.
func NewEvalRunner(harness *EvalHarness, storage *ScorerStorage) *EvalRunner {
	return &EvalRunner{
		harness: harness,
		storage: storage,
	}
}

// LoadDataset reads a JSON dataset file from disk.
func LoadDataset(path string) (*Dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read dataset %s: %w", path, err)
	}

	var dataset Dataset
	if err := json.Unmarshal(data, &dataset); err != nil {
		return nil, fmt.Errorf("parse dataset %s: %w", path, err)
	}

	if len(dataset.Examples) == 0 {
		return nil, fmt.Errorf("dataset %s has no examples", path)
	}

	return &dataset, nil
}

// Run evaluates all examples in a dataset, optionally filtering by scorer name.
// It returns an EvalRunOutcome with aggregated results.
func (r *EvalRunner) Run(ctx context.Context, dataset *Dataset, datasetPath, scorerFilter string) (*EvalRunOutcome, error) {
	if dataset == nil {
		return nil, fmt.Errorf("dataset is nil")
	}

	start := time.Now()
	outcome := &EvalRunOutcome{
		DatasetPath: datasetPath,
	}

	harness := r.harness
	if scorerFilter != "" {
		filtered := NewEvalHarness()
		for _, s := range harness.RegisteredScorers() {
			if s.Name() == scorerFilter {
				filtered.Register(s)
				break
			}
		}
		harness = filtered
	}

	for _, example := range dataset.Examples {
		result := r.runExample(ctx, harness, example)
		outcome.Results = append(outcome.Results, result)
	}

	outcome.Duration = time.Since(start)
	outcome.RunID = generateRunID()

	totalScore := 0.0
	scoreCount := 0
	allPassed := true

	for _, res := range outcome.Results {
		if res.Error != "" {
			allPassed = false
			continue
		}
		if res.Report != nil {
			totalScore += res.Report.OverallScore
			if !res.Report.Passed {
				allPassed = false
			}
			scoreCount++
		}
	}

	if scoreCount > 0 {
		outcome.OverallScore = math.Round(totalScore/float64(scoreCount)*1000) / 1000
	}
	outcome.Passed = allPassed && len(outcome.Results) > 0

	if r.storage != nil {
		run := &StoredEvalRun{
			RunID:         outcome.RunID,
			DatasetPath:   datasetPath,
			ScorerFilter:  scorerFilter,
			TotalExamples: len(dataset.Examples),
			OverallScore:  outcome.OverallScore,
			OverallPassed: outcome.Passed,
			DurationMs:    outcome.Duration.Milliseconds(),
		}
		if err := r.storage.SaveRun(ctx, run); err != nil {
			return outcome, fmt.Errorf("save eval run: %w", err)
		}

		for _, res := range outcome.Results {
			if res.Report == nil {
				continue
			}
			if err := r.storage.SaveReport(ctx, res.Report, res.InputHash); err != nil {
				slog.Warn("Failed to save eval report", "error", err, "example", res.InputHash)
			}
		}
	}

	return outcome, nil
}

func (r *EvalRunner) runExample(ctx context.Context, harness *EvalHarness, example DatasetExample) RunResult {
	result := RunResult{
		ExampleID:   example.ID,
		ExampleName: example.Name,
	}

	if example.Input == nil {
		result.Error = "nil input"
		return result
	}

	result.InputHash = HashInput(example.Input)

	report, err := harness.Run(ctx, example.Input)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.Report = report
	return result
}

// RegisteredScorers returns the underlying harness scorers. This is needed
// for the EvalRunner to access the scorer list for filtering.
func (h *EvalHarness) RegisteredScorers() []Scorer {
	h.mu.RLock()
	defer h.mu.RUnlock()

	scorers := make([]Scorer, len(h.scorers))
	copy(scorers, h.scorers)
	return scorers
}

func generateRunID() string {
	return fmt.Sprintf("eval_%d", time.Now().UnixMilli())
}
