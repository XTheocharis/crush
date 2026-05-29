package lcm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewGraduatedPressureSystem(t *testing.T) {
	t.Parallel()

	mock := &mockLLMClient{}
	cfg := DefaultPressureConfig()
	limits := DefaultContextLimits(128000)

	gps := NewGraduatedPressureSystem(cfg, limits, mock)
	require.NotNil(t, gps)
	require.Equal(t, limits, gps.Limits())
}

func TestGraduatedPressureSystem_StrategiesForTier(t *testing.T) {
	t.Parallel()

	mock := &mockLLMClient{}
	gps := NewGraduatedPressureSystem(DefaultPressureConfig(), DefaultContextLimits(128000), mock)

	t.Run("low tier has 1 strategy", func(t *testing.T) {
		t.Parallel()
		strats := gps.StrategiesForTier(PressureLow)
		require.Len(t, strats, 1)
		require.Equal(t, "purge_errors", strats[0].Name())
	})

	t.Run("medium tier has 3 strategies", func(t *testing.T) {
		t.Parallel()
		strats := gps.StrategiesForTier(PressureMedium)
		require.Len(t, strats, 3)
		require.Equal(t, "purge_errors", strats[0].Name())
		require.Equal(t, "dedup", strats[1].Name())
		require.Equal(t, "message", strats[2].Name())
	})

	t.Run("high tier has 4 strategies", func(t *testing.T) {
		t.Parallel()
		strats := gps.StrategiesForTier(PressureHigh)
		require.Len(t, strats, 4)
		require.Equal(t, "purge_errors", strats[0].Name())
		require.Equal(t, "dedup", strats[1].Name())
		require.Equal(t, "message", strats[2].Name())
		require.Equal(t, "range", strats[3].Name())
	})
}

func TestGraduatedPressureSystem_TierForTokens(t *testing.T) {
	t.Parallel()

	gps := NewGraduatedPressureSystem(DefaultPressureConfig(), DefaultContextLimits(100000), &mockLLMClient{})

	tests := []struct {
		name          string
		currentTokens int64
		wantTier      PressureTier
	}{
		{"well below soft offset", 50000, PressureLow},
		{"below soft offset (80000)", 79000, PressureLow},
		{"at soft offset (80000)", 80000, PressureLow},
		{"above soft offset", 82000, PressureLow},
		{"at compact offset (87000)", 87000, PressureMedium},
		{"between compact and hard", 90000, PressureMedium},
		{"at hard offset (97000)", 97000, PressureHigh},
		{"above hard offset", 99000, PressureHigh},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := gps.TierForTokens(tc.currentTokens)
			require.Equal(t, tc.wantTier, got)
		})
	}
}

func TestGraduatedPressureSystem_TierForTokens_ZeroLimits(t *testing.T) {
	t.Parallel()

	gps := NewGraduatedPressureSystem(
		DefaultPressureConfig(),
		ContextLimits{MaxTokens: 0, ReserveTokens: 0, SummaryBudget: 0},
		&mockLLMClient{},
	)

	require.Equal(t, PressureLow, gps.TierForTokens(50000))
}

func TestGraduatedPressureSystem_StrategiesForTokens(t *testing.T) {
	t.Parallel()

	gps := NewGraduatedPressureSystem(DefaultPressureConfig(), DefaultContextLimits(100000), &mockLLMClient{})

	t.Run("50k tokens returns purge_errors only", func(t *testing.T) {
		t.Parallel()
		strats := gps.StrategiesForTokens(50000)
		require.Len(t, strats, 1)
		require.Equal(t, "purge_errors", strats[0].Name())
	})

	t.Run("88k tokens returns 3 strategies", func(t *testing.T) {
		t.Parallel()
		strats := gps.StrategiesForTokens(88000)
		require.Len(t, strats, 3)
	})

	t.Run("98k tokens returns 4 strategies", func(t *testing.T) {
		t.Parallel()
		strats := gps.StrategiesForTokens(98000)
		require.Len(t, strats, 4)
	})
}

func TestGraduatedPressureSystem_CustomThresholds(t *testing.T) {
	t.Parallel()

	cfg := PressureConfig{
		UseAbsoluteOffsets: false,
		LowThreshold:       50.0,
		MediumThreshold:    75.0,
		HighThreshold:      90.0,
	}
	gps := NewGraduatedPressureSystem(cfg, DefaultContextLimits(100000), &mockLLMClient{})

	tests := []struct {
		name          string
		currentTokens int64
		wantTier      PressureTier
	}{
		{"at 49%", 49000, PressureLow},
		{"at 50%", 50000, PressureLow},
		{"at 60%", 60000, PressureLow},
		{"at 75%", 75000, PressureMedium},
		{"at 80%", 80000, PressureMedium},
		{"at 90%", 90000, PressureHigh},
		{"at 95%", 95000, PressureHigh},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := gps.TierForTokens(tc.currentTokens)
			require.Equal(t, tc.wantTier, got)
		})
	}
}

func TestGraduatedPressureSystem_IntegratesWithExistingPressureSelector(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	budget := Budget{SoftThreshold: 70000, HardLimit: 90000, ContextWindow: 100000}

	gps := NewGraduatedPressureSystem(
		DefaultPressureConfig(),
		DefaultContextLimits(100000),
		&mockLLMClient{response: "compressed"},
	)

	usageFn := func(_ context.Context) (int64, int64, error) {
		return 90000, 100000, nil
	}

	pressureSelector := NewPressureCompactionSelector(
		DefaultPressureConfig(),
		usageFn,
		map[PressureTier][]CompactionLayer{},
	)

	tier, err := pressureSelector.Tier(ctx)
	require.NoError(t, err)

	strats := gps.StrategiesForTier(tier)
	require.NotEmpty(t, strats)

	_, selErr := pressureSelector.Compact(ctx, budget)
	require.NoError(t, selErr)
}
