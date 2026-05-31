// Package eval provides a scorer interface and evaluation harness for
// assessing agent output quality. Scorers are registered with an EvalHarness
// and run in parallel to produce an EvalReport.
package eval

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// ScorerType classifies how a scorer produces its result.
type ScorerType int

const (
	// ScorerMetric produces deterministic scores from code analysis.
	ScorerMetric ScorerType = iota
	// ScorerLLMJudge delegates scoring to an LLM-based evaluator.
	ScorerLLMJudge
	// ScorerMastra uses a 4-step pipeline: preprocess, analyze,
	// generateScore, generateReason.
	ScorerMastra
)

// String returns a human-readable name for the scorer type.
func (t ScorerType) String() string {
	switch t {
	case ScorerMetric:
		return "metric"
	case ScorerLLMJudge:
		return "llm_judge"
	case ScorerMastra:
		return "mastra"
	default:
		return "unknown"
	}
}

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// FileEdit represents a single file change made during a session.
type FileEdit struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// TestResult captures the outcome of running tests.
type TestResult struct {
	Total  int    `json:"total"`
	Passed int    `json:"passed"`
	Failed int    `json:"failed"`
	Output string `json:"output"`
}

// EvalInput contains all data available to a scorer for evaluation.
type EvalInput struct {
	SessionID    string            `json:"session_id"`
	Conversation []Message         `json:"conversation"`
	Edits        []FileEdit        `json:"edits"`
	TestResults  *TestResult       `json:"test_results,omitempty"`
	Files        map[string]string `json:"files"`
}

// ScoreResult is the output of a single scorer.
type ScoreResult struct {
	Score       float64        `json:"score"`
	Explanation string         `json:"explanation"`
	Passed      bool           `json:"passed"`
	Details     map[string]any `json:"details,omitempty"`
	Duration    time.Duration  `json:"duration"`
	ScorerName  string         `json:"scorer_name"`
	Error       string         `json:"error,omitempty"`
}

// Scorer evaluates agent output and produces a score.
type Scorer interface {
	// Name returns a unique identifier for this scorer.
	Name() string
	// Type returns the scorer classification.
	Type() ScorerType
	// Score evaluates the given input and returns a result.
	Score(ctx context.Context, input *EvalInput) (*ScoreResult, error)
}

// ScorerResultEntry pairs a scorer name with its result.
type ScorerResultEntry struct {
	Name   string       `json:"name"`
	Type   ScorerType   `json:"type"`
	Result *ScoreResult `json:"result"`
}

// EvalReport aggregates results from all registered scorers.
type EvalReport struct {
	SessionID    string              `json:"session_id"`
	Timestamp    time.Time           `json:"timestamp"`
	Results      []ScorerResultEntry `json:"results"`
	OverallScore float64             `json:"overall_score"`
	Passed       bool                `json:"passed"`
	Duration     time.Duration       `json:"duration"`
}

// EvalHarness runs a collection of scorers against an EvalInput.
type EvalHarness struct {
	scorers []Scorer
	mu      sync.RWMutex
}

// NewEvalHarness creates a harness with no registered scorers.
func NewEvalHarness() *EvalHarness {
	return &EvalHarness{}
}

// Register adds a scorer to the harness. If the scorer implements
// ScorerPipeline directly (not already wrapped in a PipelineScorer), it
// is wrapped automatically. It panics if a scorer with the same name is
// already registered.
func (h *EvalHarness) Register(s Scorer) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Unwrap PipelineScorer to check the inner type; avoid double-wrapping.
	inner := s
	if ps, ok := s.(*PipelineScorer); ok {
		inner = ps.impl
	}

	// If the scorer implements ScorerPipeline natively and is not already
	// wrapped, wrap it so the harness runs it through the pipeline.
	if _, ok := inner.(ScorerPipeline); ok {
		if _, alreadyWrapped := s.(*PipelineScorer); !alreadyWrapped {
			s = NewPipelineScorer(inner.(ScorerPipeline))
		}
	}

	for _, existing := range h.scorers {
		if existing.Name() == s.Name() {
			panic(fmt.Sprintf("eval: scorer %q already registered", s.Name()))
		}
	}
	h.scorers = append(h.scorers, s)
}

// Scorers returns a copy of the registered scorer names.
func (h *EvalHarness) Scorers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	names := make([]string, len(h.scorers))
	for i, s := range h.scorers {
		names[i] = s.Name()
	}
	return names
}

// Run executes all registered scorers in parallel and aggregates results
// into an EvalReport. Each scorer receives the same EvalInput. If a scorer
// returns an error, it is captured in the ScoreResult and does not prevent
// other scorers from running.
func (h *EvalHarness) Run(ctx context.Context, input *EvalInput) (*EvalReport, error) {
	h.mu.RLock()
	scorers := make([]Scorer, len(h.scorers))
	copy(scorers, h.scorers)
	h.mu.RUnlock()

	if len(scorers) == 0 {
		return &EvalReport{
			SessionID:    input.SessionID,
			Timestamp:    time.Now(),
			Results:      nil,
			OverallScore: 0,
			Passed:       true,
		}, nil
	}

	start := time.Now()

	var wg sync.WaitGroup
	entries := make([]ScorerResultEntry, len(scorers))

	for i, s := range scorers {
		wg.Add(1)
		go func(idx int, scorer Scorer) {
			defer wg.Done()
			entries[idx] = h.runScorer(ctx, scorer, input)
		}(i, s)
	}
	wg.Wait()

	report := &EvalReport{
		SessionID: input.SessionID,
		Timestamp: time.Now(),
		Results:   entries,
		Duration:  time.Since(start),
	}

	totalScore := 0.0
	allPassed := true
	scoreCount := 0

	for _, entry := range entries {
		if entry.Result == nil {
			continue
		}
		if entry.Result.Error != "" {
			allPassed = false
			continue
		}
		totalScore += entry.Result.Score
		if !entry.Result.Passed {
			allPassed = false
		}
		scoreCount++
	}

	if scoreCount > 0 {
		report.OverallScore = math.Round(totalScore/float64(scoreCount)*1000) / 1000
	}
	report.Passed = allPassed

	return report, nil
}

// runScorer executes a single scorer with timing and error capture.
func (h *EvalHarness) runScorer(ctx context.Context, s Scorer, input *EvalInput) ScorerResultEntry {
	entry := ScorerResultEntry{
		Name: s.Name(),
		Type: s.Type(),
	}

	scorerStart := time.Now()
	result, err := s.Score(ctx, input)
	duration := time.Since(scorerStart)

	if err != nil {
		entry.Result = &ScoreResult{
			Score:      0,
			Passed:     false,
			Error:      err.Error(),
			Duration:   duration,
			ScorerName: s.Name(),
		}
		return entry
	}

	result.Duration = duration
	result.ScorerName = s.Name()
	entry.Result = result

	return entry
}

// AggregateScore computes the mean score from a report's results,
// skipping entries with errors.
func AggregateScore(report *EvalReport) float64 {
	if report == nil || len(report.Results) == 0 {
		return 0
	}

	total := 0.0
	count := 0
	for _, entry := range report.Results {
		if entry.Result == nil || entry.Result.Error != "" {
			continue
		}
		total += entry.Result.Score
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// PassedScorers returns the names of scorers that passed.
func PassedScorers(report *EvalReport) []string {
	if report == nil {
		return nil
	}

	var names []string
	for _, entry := range report.Results {
		if entry.Result != nil && entry.Result.Passed {
			names = append(names, entry.Name)
		}
	}
	sort.Strings(names)
	return names
}

// FailedScorers returns the names of scorers that failed (including errors).
func FailedScorers(report *EvalReport) []string {
	if report == nil {
		return nil
	}

	var names []string
	for _, entry := range report.Results {
		if entry.Result == nil || !entry.Result.Passed {
			names = append(names, entry.Name)
		}
	}
	sort.Strings(names)
	return names
}
