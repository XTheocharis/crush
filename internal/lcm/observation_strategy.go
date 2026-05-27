package lcm

import (
	"context"
	"encoding/json"
	"runtime"
)

// ObservationStrategy controls how observations are filtered and formatted.
// Implementations decide whether a given event should produce an observation
// and how the observation is serialized for storage.
type ObservationStrategy interface {
	// ShouldObserve returns true when the strategy allows an observation to be
	// created for the given event. The event string is a human-readable label
	// describing the trigger (e.g. "token_threshold_crossed").
	ShouldObserve(ctx context.Context, event string) bool

	// FormatObservation serializes an observation into bytes for persistent
	// storage. The default strategy uses JSON encoding.
	FormatObservation(obs Observation) []byte

	// CompressionLevel returns a hint for how aggressively observations should
	// be compressed. Higher values mean more aggressive compression. A value of
	// 0 means no compression (the default).
	CompressionLevel() int
}

// Compile-time interface satisfaction checks.
var (
	_ ObservationStrategy = (*DefaultStrategy)(nil)
	_ ObservationStrategy = (*ResourceScopedStrategy)(nil)
)

// DefaultStrategy implements ObservationStrategy with the original observation
// behavior: every event is observed and observations are encoded as JSON.
type DefaultStrategy struct{}

// ShouldObserve always returns true — the default strategy never skips
// observations.
func (DefaultStrategy) ShouldObserve(_ context.Context, _ string) bool {
	return true
}

// FormatObservation encodes the observation as JSON, matching the original
// storage format used by insertObservationBuffer.
func (DefaultStrategy) FormatObservation(obs Observation) []byte {
	data, err := json.Marshal(obs)
	if err != nil {
		// Fallback: return a minimal JSON object with the error.
		return []byte(`{"event":"marshal_error","context":"","implication":""}`)
	}
	return data
}

// CompressionLevel returns 0 (no compression) for the default strategy.
func (DefaultStrategy) CompressionLevel() int {
	return 0
}

// ResourceScopedStrategy implements ObservationStrategy with memory-pressure
// awareness. It checks runtime.MemStats before allowing an observation to
// proceed, skipping observations when Alloc exceeds the configured threshold
// fraction of Sys. Observations are still encoded as JSON when allowed.
type ResourceScopedStrategy struct {
	// AllocFraction is the fraction of Sys (0.0–1.0) above which observations
	// are skipped. The default of 0.8 means observations are skipped when
	// Alloc > 80% of Sys. A value of 0 or less disables the check (always
	// observe), matching DefaultStrategy behavior.
	AllocFraction float64
}

// ShouldObserve checks runtime memory stats before deciding. When
// Alloc exceeds AllocFraction * Sys, the observation is skipped to avoid
// adding memory pressure.
func (s ResourceScopedStrategy) ShouldObserve(_ context.Context, _ string) bool {
	if s.AllocFraction <= 0 {
		return true
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	threshold := float64(m.Sys) * s.AllocFraction
	return float64(m.Alloc) <= threshold
}

// FormatObservation encodes the observation as JSON, identical to
// DefaultStrategy.
func (ResourceScopedStrategy) FormatObservation(obs Observation) []byte {
	data, err := json.Marshal(obs)
	if err != nil {
		return []byte(`{"event":"marshal_error","context":"","implication":""}`)
	}
	return data
}

// CompressionLevel returns 1 for the resource-scoped strategy, indicating
// mild compression preference to keep stored observations compact under
// memory pressure.
func (ResourceScopedStrategy) CompressionLevel() int {
	return 1
}

// NewObservationStrategyFromConfig returns the appropriate ObservationStrategy
// based on the strategy name string. Accepted values are "default" and
// "resource-scoped". An empty string defaults to "default".
func NewObservationStrategyFromConfig(name string) ObservationStrategy {
	switch name {
	case "resource-scoped":
		return ResourceScopedStrategy{AllocFraction: 0.8}
	default:
		return DefaultStrategy{}
	}
}
