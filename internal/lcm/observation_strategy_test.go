package lcm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultStrategy_ShouldObserve_AlwaysTrue(t *testing.T) {
	t.Parallel()
	s := DefaultStrategy{}
	require.True(t, s.ShouldObserve(context.Background(), "any_event"))
}

func TestDefaultStrategy_FormatObservation_MatchesJSON(t *testing.T) {
	t.Parallel()
	s := DefaultStrategy{}
	obs := Observation{
		Event:       "test event",
		Context:     "test context",
		Implication: "test implication",
		TokenCount:  42,
	}

	got := s.FormatObservation(obs)

	// Must produce valid JSON that round-trips to the same observation.
	var parsed Observation
	require.NoError(t, json.Unmarshal(got, &parsed))
	require.Equal(t, obs.Event, parsed.Event)
	require.Equal(t, obs.Context, parsed.Context)
	require.Equal(t, obs.Implication, parsed.Implication)
	require.Equal(t, obs.TokenCount, parsed.TokenCount)

	// Must match the standard json.Marshal output (current hardcoded behavior).
	expected, err := json.Marshal(obs)
	require.NoError(t, err)
	require.Equal(t, string(expected), string(got))
}

func TestDefaultStrategy_CompressionLevel_Zero(t *testing.T) {
	t.Parallel()
	s := DefaultStrategy{}
	require.Equal(t, 0, s.CompressionLevel())
}

func TestResourceScopedStrategy_ShouldObserve_AllowedUnderThreshold(t *testing.T) {
	t.Parallel()
	// AllocFraction=1.0 means threshold = Sys, so Alloc <= Sys always.
	s := ResourceScopedStrategy{AllocFraction: 1.0}
	require.True(t, s.ShouldObserve(context.Background(), "any_event"))
}

func TestResourceScopedStrategy_ShouldObserve_SkipsWhenOverThreshold(t *testing.T) {
	t.Parallel()
	// AllocFraction near-zero means almost any Alloc exceeds the threshold.
	s := ResourceScopedStrategy{AllocFraction: 0.0000001}
	require.False(t, s.ShouldObserve(context.Background(), "any_event"))
}

func TestResourceScopedStrategy_ShouldObserve_ZeroFraction_AlwaysObserve(t *testing.T) {
	t.Parallel()
	// Zero or negative AllocFraction disables the check.
	s := ResourceScopedStrategy{AllocFraction: 0}
	require.True(t, s.ShouldObserve(context.Background(), "any_event"))
}

func TestResourceScopedStrategy_ShouldObserve_NegativeFraction_AlwaysObserve(t *testing.T) {
	t.Parallel()
	s := ResourceScopedStrategy{AllocFraction: -1}
	require.True(t, s.ShouldObserve(context.Background(), "any_event"))
}

func TestResourceScopedStrategy_FormatObservation_JSON(t *testing.T) {
	t.Parallel()
	s := ResourceScopedStrategy{AllocFraction: 0.8}
	obs := Observation{
		Event:       "resource event",
		Context:     "resource context",
		Implication: "resource implication",
		TokenCount:  100,
	}
	got := s.FormatObservation(obs)
	var parsed Observation
	require.NoError(t, json.Unmarshal(got, &parsed))
	require.Equal(t, obs, parsed)
}

func TestResourceScopedStrategy_CompressionLevel_One(t *testing.T) {
	t.Parallel()
	s := ResourceScopedStrategy{}
	require.Equal(t, 1, s.CompressionLevel())
}

func TestNewObservationStrategyFromConfig_Default(t *testing.T) {
	t.Parallel()
	s := NewObservationStrategyFromConfig("default")
	require.IsType(t, DefaultStrategy{}, s)
}

func TestNewObservationStrategyFromConfig_EmptyString(t *testing.T) {
	t.Parallel()
	s := NewObservationStrategyFromConfig("")
	require.IsType(t, DefaultStrategy{}, s)
}

func TestNewObservationStrategyFromConfig_ResourceScoped(t *testing.T) {
	t.Parallel()
	s := NewObservationStrategyFromConfig("resource-scoped")
	require.IsType(t, ResourceScopedStrategy{}, s)
	rs, ok := s.(ResourceScopedStrategy)
	require.True(t, ok)
	require.Equal(t, 0.8, rs.AllocFraction)
}

func TestNewObservationCoordinator_NilStrategy_Defaults(t *testing.T) {
	t.Parallel()
	_, store := setupObservationTestDB(t)
	oc := NewObservationCoordinator(store, &mockLLMClient{}, 0, nil)
	require.Equal(t, int64(DefaultObservationTokenThreshold), oc.Threshold())
	require.Equal(t, DefaultStrategy{}, oc.strategy)
}

func TestNewObservationCoordinator_ResourceScopedStrategy(t *testing.T) {
	t.Parallel()
	_, store := setupObservationTestDB(t)
	strategy := ResourceScopedStrategy{AllocFraction: 0.5}
	oc := NewObservationCoordinator(store, &mockLLMClient{}, 0, strategy)
	require.Equal(t, strategy, oc.strategy)
}
