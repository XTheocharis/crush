package eval

import (
	"path/filepath"
	"sync"
)

const (
	// DefaultPriorityBudget is the non-zero token budget used when the
	// adapter is wired into a ReadCoordinator.
	DefaultPriorityBudget = 10000

	highPriorityThreshold   = 0.6
	mediumPriorityThreshold = 0.2
)

// PriorityLevel classifies a file's relative importance.
type PriorityLevel int

const (
	PriorityLow PriorityLevel = iota
	PriorityMedium
	PriorityHigh
)

// RepomapPriorityAdapter wraps repomap file ranking scores and implements the
// PrioritySource interface for ReadCoordinator. It maps PageRank-derived file
// scores to priority levels and exposes raw scores for proportional budget
// allocation.
//
// The adapter is safe for concurrent use. Scores can be updated at any time
// via UpdateScores.
type RepomapPriorityAdapter struct {
	mu     sync.RWMutex
	scores map[string]float64
}

// NewRepomapPriorityAdapter creates an adapter with an initial set of scores.
// The scores map path → PageRank score (any positive float64). An empty or nil
// map is valid; use UpdateScores to populate later.
func NewRepomapPriorityAdapter(scores map[string]float64) *RepomapPriorityAdapter {
	a := &RepomapPriorityAdapter{}
	if len(scores) > 0 {
		a.scores = make(map[string]float64, len(scores))
		for k, v := range scores {
			a.scores[normalizePath(k)] = v
		}
	} else {
		a.scores = make(map[string]float64)
	}
	return a
}

// Priority returns the raw priority score for the given file path.
func (a *RepomapPriorityAdapter) Priority(path string) float64 {
	key := normalizePath(path)
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.scores[key]
}

// UpdateScores replaces the current score map with new values.
func (a *RepomapPriorityAdapter) UpdateScores(scores map[string]float64) {
	normalized := make(map[string]float64, len(scores))
	for k, v := range scores {
		normalized[normalizePath(k)] = v
	}
	a.mu.Lock()
	a.scores = normalized
	a.mu.Unlock()
}

// PriorityLevel returns the classified priority level for the given file path
// based on score thresholds relative to the maximum observed score. Files with
// no score are classified as PriorityLow.
func (a *RepomapPriorityAdapter) PriorityLevel(path string) PriorityLevel {
	key := normalizePath(path)
	a.mu.RLock()
	defer a.mu.RUnlock()

	score, ok := a.scores[key]
	if !ok || score <= 0 {
		return PriorityLow
	}

	maxScore := maxScoreLocked(a.scores)
	if maxScore <= 0 {
		return PriorityLow
	}

	ratio := score / maxScore
	switch {
	case ratio >= highPriorityThreshold:
		return PriorityHigh
	case ratio >= mediumPriorityThreshold:
		return PriorityMedium
	default:
		return PriorityLow
	}
}

// Len returns the number of files with scores.
func (a *RepomapPriorityAdapter) Len() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.scores)
}

func maxScoreLocked(scores map[string]float64) float64 {
	var max float64
	for _, v := range scores {
		if v > max {
			max = v
		}
	}
	return max
}

func normalizePath(p string) string {
	return filepath.ToSlash(p)
}
