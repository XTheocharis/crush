package lcm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculatePressure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		currentTokens int64
		contextWindow int64
		want          float64
	}{
		{name: "zero window", currentTokens: 500, contextWindow: 0, want: 0},
		{name: "negative window", currentTokens: 500, contextWindow: -100, want: 0},
		{name: "50 percent", currentTokens: 50000, contextWindow: 100000, want: 50.0},
		{name: "70 percent", currentTokens: 70000, contextWindow: 100000, want: 70.0},
		{name: "85 percent", currentTokens: 85000, contextWindow: 100000, want: 85.0},
		{name: "95 percent", currentTokens: 95000, contextWindow: 100000, want: 95.0},
		{name: "100 percent", currentTokens: 100000, contextWindow: 100000, want: 100.0},
		{name: "over 100 percent", currentTokens: 120000, contextWindow: 100000, want: 120.0},
		{name: "zero tokens", currentTokens: 0, contextWindow: 100000, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CalculatePressure(tt.currentTokens, tt.contextWindow)
			require.InDelta(t, tt.want, got, 0.01)
		})
	}
}

func TestCalculatePressureTier(t *testing.T) {
	t.Parallel()

	cfg := DefaultPressureConfig()

	tests := []struct {
		name          string
		currentTokens int64
		contextWindow int64
		wantTier      PressureTier
		wantPressure  float64
	}{
		{
			name: "below low threshold", currentTokens: 50000,
			contextWindow: 100000, wantTier: PressureLow, wantPressure: 50.0,
		},
		{
			name: "at low threshold", currentTokens: 70000,
			contextWindow: 100000, wantTier: PressureLow, wantPressure: 70.0,
		},
		{
			name: "between low and medium", currentTokens: 80000,
			contextWindow: 100000, wantTier: PressureLow, wantPressure: 80.0,
		},
		{
			name: "at medium threshold", currentTokens: 85000,
			contextWindow: 100000, wantTier: PressureMedium, wantPressure: 85.0,
		},
		{
			name: "between medium and high", currentTokens: 90000,
			contextWindow: 100000, wantTier: PressureMedium, wantPressure: 90.0,
		},
		{
			name: "at high threshold", currentTokens: 95000,
			contextWindow: 100000, wantTier: PressureHigh, wantPressure: 95.0,
		},
		{
			name: "above high threshold", currentTokens: 98000,
			contextWindow: 100000, wantTier: PressureHigh, wantPressure: 98.0,
		},
		{
			name: "zero context window", currentTokens: 5000,
			contextWindow: 0, wantTier: PressureLow, wantPressure: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pressure, tier := CalculatePressureTier(tt.currentTokens, tt.contextWindow, cfg)
			require.Equal(t, tt.wantTier, tier)
			require.InDelta(t, tt.wantPressure, pressure, 0.01)
		})
	}
}

func TestCalculatePressureTierCustomThresholds(t *testing.T) {
	t.Parallel()

	cfg := PressureConfig{
		LowThreshold:    50.0,
		MediumThreshold: 75.0,
		HighThreshold:   90.0,
	}

	tests := []struct {
		name          string
		currentTokens int64
		contextWindow int64
		wantTier      PressureTier
	}{
		{name: "below 50%", currentTokens: 40, contextWindow: 100, wantTier: PressureLow},
		{name: "at 50%", currentTokens: 50, contextWindow: 100, wantTier: PressureLow},
		{name: "at 60%", currentTokens: 60, contextWindow: 100, wantTier: PressureLow},
		{name: "at 75%", currentTokens: 75, contextWindow: 100, wantTier: PressureMedium},
		{name: "at 80%", currentTokens: 80, contextWindow: 100, wantTier: PressureMedium},
		{name: "at 90%", currentTokens: 90, contextWindow: 100, wantTier: PressureHigh},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, tier := CalculatePressureTier(tt.currentTokens, tt.contextWindow, cfg)
			require.Equal(t, tt.wantTier, tier)
		})
	}
}

func TestPressureTierString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "low", PressureLow.String())
	require.Equal(t, "medium", PressureMedium.String())
	require.Equal(t, "high", PressureHigh.String())
	require.Equal(t, "unknown(42)", PressureTier(42).String())
}

func TestDefaultPressureConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultPressureConfig()
	require.InDelta(t, 70.0, cfg.LowThreshold, 0.01)
	require.InDelta(t, 85.0, cfg.MediumThreshold, 0.01)
	require.InDelta(t, 95.0, cfg.HighThreshold, 0.01)
}

func TestNewPressureCompactionSelectorFillsDefaults(t *testing.T) {
	t.Parallel()

	cfg := PressureConfig{}
	s := NewPressureCompactionSelector(cfg, func(_ context.Context) (int64, int64, error) {
		return 0, 0, nil
	}, nil)

	require.InDelta(t, 70.0, s.cfg.LowThreshold, 0.01)
	require.InDelta(t, 85.0, s.cfg.MediumThreshold, 0.01)
	require.InDelta(t, 95.0, s.cfg.HighThreshold, 0.01)
}

type pressureStubLayer struct {
	name             string
	priority         int
	shouldCompact    bool
	compactResult    *CompactionLayerResult
	compactErr       error
	compactCalled    bool
	shouldCheckCalls int
}

func (l *pressureStubLayer) Name() string { return l.name }

func (l *pressureStubLayer) Priority() int { return l.priority }

func (l *pressureStubLayer) ShouldCompact(_ context.Context, _ Budget) bool {
	l.shouldCheckCalls++
	return l.shouldCompact
}

func (l *pressureStubLayer) Compact(_ context.Context, _ Budget) (*CompactionLayerResult, error) {
	l.compactCalled = true
	return l.compactResult, l.compactErr
}

func TestPressureCompactionSelectorName(t *testing.T) {
	t.Parallel()

	s := NewPressureCompactionSelector(DefaultPressureConfig(), nil, nil)
	require.Equal(t, "pressure-selector", s.Name())
}

func TestPressureCompactionSelectorPriority(t *testing.T) {
	t.Parallel()

	s := NewPressureCompactionSelector(DefaultPressureConfig(), nil, nil)
	require.Equal(t, 5, s.Priority())
}

func TestPressureCompactionSelectorShouldCompact(t *testing.T) {
	t.Parallel()

	cfg := DefaultPressureConfig()
	ctx := context.Background()
	budget := Budget{SoftThreshold: 70000, HardLimit: 90000, ContextWindow: 100000}

	t.Run("above low threshold returns true", func(t *testing.T) {
		t.Parallel()
		s := NewPressureCompactionSelector(cfg, func(_ context.Context) (int64, int64, error) {
			return 75000, 100000, nil
		}, nil)
		require.True(t, s.ShouldCompact(ctx, budget))
	})

	t.Run("below low threshold returns false", func(t *testing.T) {
		t.Parallel()
		s := NewPressureCompactionSelector(cfg, func(_ context.Context) (int64, int64, error) {
			return 50000, 100000, nil
		}, nil)
		require.False(t, s.ShouldCompact(ctx, budget))
	})

	t.Run("usage error returns false", func(t *testing.T) {
		t.Parallel()
		s := NewPressureCompactionSelector(cfg, func(_ context.Context) (int64, int64, error) {
			return 0, 0, errTestUsage
		}, nil)
		require.False(t, s.ShouldCompact(ctx, budget))
	})
}

func TestPressureCompactionSelectorCompact(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	budget := Budget{SoftThreshold: 70000, HardLimit: 90000, ContextWindow: 100000}

	t.Run("low tier runs low layers only", func(t *testing.T) {
		t.Parallel()

		lowLayer := &pressureStubLayer{
			name:          "micro-compactor",
			priority:      1,
			shouldCompact: true,
			compactResult: &CompactionLayerResult{
				LayerName: "micro-compactor", TokensFreed: 500, ItemsAffected: 3, ActionTaken: true,
			},
		}
		mediumLayer := &pressureStubLayer{
			name:          "session-memory",
			priority:      2,
			shouldCompact: true,
			compactResult: &CompactionLayerResult{
				LayerName: "session-memory", TokensFreed: 1000, ItemsAffected: 5, ActionTaken: true,
			},
		}

		cfg := DefaultPressureConfig()
		s := NewPressureCompactionSelector(cfg, func(_ context.Context) (int64, int64, error) {
			return 72000, 100000, nil // 72% → Low tier
		}, map[PressureTier][]CompactionLayer{
			PressureLow:    {lowLayer},
			PressureMedium: {mediumLayer},
		})

		result, err := s.Compact(ctx, budget)
		require.NoError(t, err)
		require.True(t, lowLayer.compactCalled)
		require.False(t, mediumLayer.compactCalled)
		require.True(t, result.ActionTaken)
		require.Equal(t, int64(500), result.TokensFreed)
		require.Equal(t, 3, result.ItemsAffected)
	})

	t.Run("medium tier runs medium layers", func(t *testing.T) {
		t.Parallel()

		lowLayer := &pressureStubLayer{
			name:          "micro-compactor",
			priority:      1,
			shouldCompact: true,
			compactResult: &CompactionLayerResult{
				LayerName: "micro-compactor", TokensFreed: 500, ItemsAffected: 2, ActionTaken: true,
			},
		}
		mediumLayer := &pressureStubLayer{
			name:          "tool-output-compress",
			priority:      2,
			shouldCompact: true,
			compactResult: &CompactionLayerResult{
				LayerName: "tool-output-compress", TokensFreed: 2000, ItemsAffected: 4, ActionTaken: true,
			},
		}

		s := NewPressureCompactionSelector(DefaultPressureConfig(), func(_ context.Context) (int64, int64, error) {
			return 88000, 100000, nil
		}, map[PressureTier][]CompactionLayer{
			PressureLow:    {lowLayer},
			PressureMedium: {mediumLayer},
		})

		result, err := s.Compact(ctx, budget)
		require.NoError(t, err)
		require.False(t, lowLayer.compactCalled)
		require.True(t, mediumLayer.compactCalled)
		require.True(t, result.ActionTaken)
		require.Equal(t, int64(2000), result.TokensFreed)
		require.Equal(t, 4, result.ItemsAffected)
	})

	t.Run("high tier runs high layers", func(t *testing.T) {
		t.Parallel()

		highLayer := &pressureStubLayer{
			name:          "aggressive-summarizer",
			priority:      3,
			shouldCompact: true,
			compactResult: &CompactionLayerResult{
				LayerName: "aggressive-summarizer", TokensFreed: 5000, ItemsAffected: 10, ActionTaken: true,
			},
		}

		s := NewPressureCompactionSelector(DefaultPressureConfig(), func(_ context.Context) (int64, int64, error) {
			return 96000, 100000, nil
		}, map[PressureTier][]CompactionLayer{
			PressureHigh: {highLayer},
		})

		result, err := s.Compact(ctx, budget)
		require.NoError(t, err)
		require.True(t, highLayer.compactCalled)
		require.True(t, result.ActionTaken)
		require.Equal(t, int64(5000), result.TokensFreed)
		require.Equal(t, 10, result.ItemsAffected)
	})

	t.Run("no layers registered returns empty result", func(t *testing.T) {
		t.Parallel()

		s := NewPressureCompactionSelector(DefaultPressureConfig(), func(_ context.Context) (int64, int64, error) {
			return 96000, 100000, nil
		}, nil)

		result, err := s.Compact(ctx, budget)
		require.NoError(t, err)
		require.False(t, result.ActionTaken)
		require.Equal(t, "pressure-selector", result.LayerName)
	})

	t.Run("sub-layer error propagates", func(t *testing.T) {
		t.Parallel()

		failLayer := &pressureStubLayer{
			name:          "failing-layer",
			priority:      1,
			shouldCompact: true,
			compactErr:    errTestCompact,
		}

		s := NewPressureCompactionSelector(DefaultPressureConfig(), func(_ context.Context) (int64, int64, error) {
			return 96000, 100000, nil
		}, map[PressureTier][]CompactionLayer{
			PressureHigh: {failLayer},
		})

		_, err := s.Compact(ctx, budget)
		require.ErrorIs(t, err, errTestCompact)
	})

	t.Run("sub-layer not eligible is skipped", func(t *testing.T) {
		t.Parallel()

		ineligible := &pressureStubLayer{
			name:          "ineligible",
			priority:      1,
			shouldCompact: false,
		}
		eligible := &pressureStubLayer{
			name:          "eligible",
			priority:      2,
			shouldCompact: true,
			compactResult: &CompactionLayerResult{
				LayerName: "eligible", TokensFreed: 100, ItemsAffected: 1, ActionTaken: true,
			},
		}

		s := NewPressureCompactionSelector(DefaultPressureConfig(), func(_ context.Context) (int64, int64, error) {
			return 96000, 100000, nil
		}, map[PressureTier][]CompactionLayer{
			PressureHigh: {ineligible, eligible},
		})

		result, err := s.Compact(ctx, budget)
		require.NoError(t, err)
		require.False(t, ineligible.compactCalled)
		require.True(t, eligible.compactCalled)
		require.True(t, result.ActionTaken)
	})
}

func TestPressureCompactionSelectorSelectLayers(t *testing.T) {
	t.Parallel()

	lowLayer := &pressureStubLayer{name: "low-layer", priority: 1}
	medLayer := &pressureStubLayer{name: "med-layer", priority: 2}
	highLayer := &pressureStubLayer{name: "high-layer", priority: 3}

	s := NewPressureCompactionSelector(DefaultPressureConfig(), nil, map[PressureTier][]CompactionLayer{
		PressureLow:    {lowLayer},
		PressureMedium: {medLayer},
		PressureHigh:   {highLayer},
	})

	require.Len(t, s.SelectLayers(PressureLow), 1)
	require.Equal(t, "low-layer", s.SelectLayers(PressureLow)[0].Name())
	require.Len(t, s.SelectLayers(PressureMedium), 1)
	require.Equal(t, "med-layer", s.SelectLayers(PressureMedium)[0].Name())
	require.Len(t, s.SelectLayers(PressureHigh), 1)
	require.Equal(t, "high-layer", s.SelectLayers(PressureHigh)[0].Name())
}

func TestPressureCompactionSelectorTier(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("returns correct tier", func(t *testing.T) {
		t.Parallel()

		s := NewPressureCompactionSelector(DefaultPressureConfig(), func(_ context.Context) (int64, int64, error) {
			return 88000, 100000, nil
		}, nil)

		tier, err := s.Tier(ctx)
		require.NoError(t, err)
		require.Equal(t, PressureMedium, tier)
	})

	t.Run("usage error returns low tier with error", func(t *testing.T) {
		t.Parallel()

		s := NewPressureCompactionSelector(DefaultPressureConfig(), func(_ context.Context) (int64, int64, error) {
			return 0, 0, errTestUsage
		}, nil)

		tier, err := s.Tier(ctx)
		require.ErrorIs(t, err, errTestUsage)
		require.Equal(t, PressureLow, tier)
	})
}

func TestPressureCompactionSelectorCompactUsageError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	budget := Budget{ContextWindow: 100000}

	s := NewPressureCompactionSelector(DefaultPressureConfig(), func(_ context.Context) (int64, int64, error) {
		return 0, 0, errTestUsage
	}, nil)

	_, err := s.Compact(ctx, budget)
	require.ErrorIs(t, err, errTestUsage)
}

// Sentinel errors for tests.
var (
	errTestUsage   = errTest("usage error")
	errTestCompact = errTest("compact error")
)

type errTest string

func (e errTest) Error() string { return string(e) }
