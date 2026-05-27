package eval

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	defaultMetricThreshold   = 0.7
	defaultLLMJudgeThreshold = 0.6
	defaultWeight            = 1.0
)

// Criterion defines the passing threshold and weight for a single scorer.
type Criterion struct {
	Threshold  float64    `json:"threshold"`
	Weight     float64    `json:"weight"`
	ScorerType ScorerType `json:"scorer_type"`
}

// ScoringCriteria maps scorer names to their evaluation criteria.
type ScoringCriteria struct {
	entries map[string]Criterion
}

// NewScoringCriteria creates an empty criteria set.
func NewScoringCriteria() *ScoringCriteria {
	return &ScoringCriteria{entries: make(map[string]Criterion)}
}

// DefaultCriteria returns criteria with default thresholds: metric scorers
// require ≥0.7 and LLM-judge scorers require ≥0.6, both with weight 1.0.
func DefaultCriteria(scorers []Scorer) *ScoringCriteria {
	c := NewScoringCriteria()
	for _, s := range scorers {
		threshold := defaultMetricThreshold
		if s.Type() == ScorerLLMJudge {
			threshold = defaultLLMJudgeThreshold
		}
		c.entries[s.Name()] = Criterion{
			Threshold:  threshold,
			Weight:     defaultWeight,
			ScorerType: s.Type(),
		}
	}
	return c
}

// Set configures the criterion for a named scorer.
func (c *ScoringCriteria) Set(name string, criterion Criterion) {
	c.entries[name] = criterion
}

// Get returns the criterion for a named scorer. If none is configured, it
// returns a default based on the scorer type.
func (c *ScoringCriteria) Get(name string, scorerType ScorerType) Criterion {
	if cr, ok := c.entries[name]; ok {
		return cr
	}
	threshold := defaultMetricThreshold
	if scorerType == ScorerLLMJudge {
		threshold = defaultLLMJudgeThreshold
	}
	return Criterion{Threshold: threshold, Weight: defaultWeight, ScorerType: scorerType}
}

// ScorerScore holds the evaluated result for a single scorer within a
// ScoredReport.
type ScorerScore struct {
	Name      string  `json:"name"`
	Score     float64 `json:"score"`
	Threshold float64 `json:"threshold"`
	Weight    float64 `json:"weight"`
	Passed    bool    `json:"passed"`
	Error     string  `json:"error,omitempty"`
}

// ScoredReport is the output of applying ScoringCriteria to an EvalReport.
type ScoredReport struct {
	SessionID     string        `json:"session_id"`
	Timestamp     string        `json:"timestamp"`
	ScorerScores  []ScorerScore `json:"scorer_scores"`
	WeightedScore float64       `json:"weighted_score"`
	OverallPassed bool          `json:"overall_passed"`
	PassedCount   int           `json:"passed_count"`
	FailedCount   int           `json:"failed_count"`
	TotalScorers  int           `json:"total_scorers"`
}

// ReportGenerator applies ScoringCriteria to EvalReports and produces
// ScoredReports with weighted aggregation.
type ReportGenerator struct {
	criteria *ScoringCriteria
}

// NewReportGenerator creates a generator with the given criteria.
func NewReportGenerator(criteria *ScoringCriteria) *ReportGenerator {
	if criteria == nil {
		criteria = NewScoringCriteria()
	}
	return &ReportGenerator{criteria: criteria}
}

// ApplyCriteria applies scoring criteria to an EvalReport and returns a
// ScoredReport with per-scorer pass/fail and weighted aggregation.
// Weighted aggregation: sum(score * weight) / sum(weight). Scorers with
// errors contribute a score of 0.0 with their configured weight.
func (g *ReportGenerator) ApplyCriteria(report *EvalReport) *ScoredReport {
	if report == nil {
		return &ScoredReport{}
	}

	scores := make([]ScorerScore, 0, len(report.Results))
	var weightedSum, totalWeight float64
	passedCount := 0

	for _, entry := range report.Results {
		cr := g.criteria.Get(entry.Name, entry.Type)

		ss := ScorerScore{
			Name:      entry.Name,
			Threshold: cr.Threshold,
			Weight:    cr.Weight,
		}

		if entry.Result == nil {
			ss.Score = 0
			ss.Passed = false
			ss.Error = "no result"
		} else if entry.Result.Error != "" {
			ss.Score = 0
			ss.Passed = false
			ss.Error = entry.Result.Error
		} else {
			ss.Score = round3(entry.Result.Score)
			ss.Passed = ss.Score >= cr.Threshold
		}

		if ss.Passed {
			passedCount++
		}

		weightedSum += ss.Score * cr.Weight
		totalWeight += cr.Weight
		scores = append(scores, ss)
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Name < scores[j].Name
	})

	weightedAvg := 0.0
	if totalWeight > 0 {
		weightedAvg = round3(weightedSum / totalWeight)
	}

	return &ScoredReport{
		SessionID:     report.SessionID,
		Timestamp:     report.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		ScorerScores:  scores,
		WeightedScore: weightedAvg,
		OverallPassed: passedCount == len(report.Results) && len(report.Results) > 0,
		PassedCount:   passedCount,
		FailedCount:   len(report.Results) - passedCount,
		TotalScorers:  len(report.Results),
	}
}

// ToJSON serializes a ScoredReport to indented JSON.
func (g *ReportGenerator) ToJSON(report *ScoredReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// ToMarkdown renders a ScoredReport as a markdown table with per-scorer
// results and an overall summary.
func (g *ReportGenerator) ToMarkdown(report *ScoredReport) string {
	if report == nil || len(report.ScorerScores) == 0 {
		return "## Evaluation Report\n\nNo scorer results.\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Evaluation Report: %s\n\n", report.SessionID)
	fmt.Fprintf(&b, "| Scorer | Score | Threshold | Status |\n")
	fmt.Fprintf(&b, "|--------|-------|-----------|--------|\n")

	for _, ss := range report.ScorerScores {
		status := "✅ PASS"
		if !ss.Passed {
			status = "❌ FAIL"
		}
		fmt.Fprintf(&b, "| %s | %.3f | %.3f | %s |\n", ss.Name, ss.Score, ss.Threshold, status)
	}

	statusStr := "PASS"
	if !report.OverallPassed {
		statusStr = "FAIL"
	}
	fmt.Fprintf(&b, "\n**Overall: %s** (weighted score: %.3f, %d/%d passed)\n",
		statusStr, report.WeightedScore, report.PassedCount, report.TotalScorers)

	return b.String()
}

// ExecutiveSummary returns a brief one-line summary of the scored report.
func (g *ReportGenerator) ExecutiveSummary(report *ScoredReport) string {
	if report == nil || report.TotalScorers == 0 {
		return "No scorers evaluated."
	}
	status := "PASSED"
	if !report.OverallPassed {
		status = "FAILED"
	}
	return fmt.Sprintf("%s: weighted score %.3f, %d/%d scorers passed",
		status, report.WeightedScore, report.PassedCount, report.TotalScorers)
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}
