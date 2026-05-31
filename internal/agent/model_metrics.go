package agent

import (
	"sync"
	"time"
)

// ModelMetrics tracks per-model performance statistics for LLM requests.
type ModelMetrics struct {
	ModelName string

	mu             sync.RWMutex
	requestCount   int64
	successCount   int64
	failureCount   int64
	totalLatency   time.Duration
	totalInputTok  int64
	totalOutputTok int64
	totalCost      float64
}

// RequestCount returns the total number of recorded requests.
func (m *ModelMetrics) RequestCount() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.requestCount
}

// SuccessCount returns the number of successful requests.
func (m *ModelMetrics) SuccessCount() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.successCount
}

// FailureCount returns the number of failed requests.
func (m *ModelMetrics) FailureCount() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.failureCount
}

// TotalLatency returns the cumulative request latency.
func (m *ModelMetrics) TotalLatency() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.totalLatency
}

// TotalInputTokens returns the cumulative input token count.
func (m *ModelMetrics) TotalInputTokens() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.totalInputTok
}

// TotalOutputTokens returns the cumulative output token count.
func (m *ModelMetrics) TotalOutputTokens() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.totalOutputTok
}

// TotalCost returns the cumulative cost in USD.
func (m *ModelMetrics) TotalCost() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.totalCost
}

// SuccessRate returns the ratio of successful requests to total requests.
// Returns 0 when no requests have been recorded.
func (m *ModelMetrics) SuccessRate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.requestCount == 0 {
		return 0
	}
	return float64(m.successCount) / float64(m.requestCount)
}

// AvgLatency returns the average request latency.
// Returns 0 when no requests have been recorded.
func (m *ModelMetrics) AvgLatency() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.requestCount == 0 {
		return 0
	}
	return m.totalLatency / time.Duration(m.requestCount)
}

// TokenEfficiency returns the ratio of output tokens to total tokens
// (input + output). Returns 0 when no tokens have been consumed.
func (m *ModelMetrics) TokenEfficiency() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	total := m.totalInputTok + m.totalOutputTok
	if total == 0 {
		return 0
	}
	return float64(m.totalOutputTok) / float64(total)
}

// MetricsStore is a thread-safe, session-scoped store for per-model metrics.
// It is not persisted across sessions.
type MetricsStore struct {
	mu      sync.RWMutex
	metrics map[string]*ModelMetrics
}

// NewMetricsStore creates a new empty MetricsStore.
func NewMetricsStore() *MetricsStore {
	return &MetricsStore{
		metrics: make(map[string]*ModelMetrics),
	}
}

// Record records the outcome of a single LLM request for the given model.
// Empty model names are ignored.
func (s *MetricsStore) Record(model string, latency time.Duration, success bool, inputTokens, outputTokens int, cost float64) {
	if model == "" {
		return
	}
	s.mu.Lock()
	m, ok := s.metrics[model]
	if !ok {
		m = &ModelMetrics{ModelName: model}
		s.metrics[model] = m
	}
	s.mu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestCount++
	if success {
		m.successCount++
	} else {
		m.failureCount++
	}
	m.totalLatency += latency
	m.totalInputTok += int64(inputTokens)
	m.totalOutputTok += int64(outputTokens)
	m.totalCost += cost
}

// Get returns the metrics for the given model, or nil if none exist.
func (s *MetricsStore) Get(model string) *ModelMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics[model]
}

// GetAll returns a shallow copy of the internal metrics map. The returned map
// is safe to iterate without holding the lock.
func (s *MetricsStore) GetAll() map[string]*ModelMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*ModelMetrics, len(s.metrics))
	for k, v := range s.metrics {
		out[k] = v
	}
	return out
}
