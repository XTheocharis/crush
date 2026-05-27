package lcm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextLimits_AvailableTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		limits ContextLimits
		want   int
	}{
		{
			name:   "normal case",
			limits: ContextLimits{MaxTokens: 128000, ReserveTokens: 25600, SummaryBudget: 25600},
			want:   102400,
		},
		{
			name:   "zero reserve",
			limits: ContextLimits{MaxTokens: 100000, ReserveTokens: 0, SummaryBudget: 0},
			want:   100000,
		},
		{
			name:   "reserve exceeds max",
			limits: ContextLimits{MaxTokens: 50000, ReserveTokens: 60000, SummaryBudget: 0},
			want:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.limits.AvailableTokens())
		})
	}
}

func TestContextLimits_EffectiveBudget(t *testing.T) {
	t.Parallel()

	t.Run("prefers summary budget when set", func(t *testing.T) {
		t.Parallel()
		cl := ContextLimits{MaxTokens: 100000, ReserveTokens: 20000, SummaryBudget: 15000}
		require.Equal(t, 15000, cl.EffectiveBudget())
	})

	t.Run("falls back to available tokens when summary budget is zero", func(t *testing.T) {
		t.Parallel()
		cl := ContextLimits{MaxTokens: 100000, ReserveTokens: 20000, SummaryBudget: 0}
		require.Equal(t, 80000, cl.EffectiveBudget())
	})
}

func TestContextLimits_PressureThreshold(t *testing.T) {
	t.Parallel()

	cl := ContextLimits{MaxTokens: 100000, ReserveTokens: 0, SummaryBudget: 0}

	tests := []struct {
		name string
		pct  float64
		want int64
	}{
		{"70 percent", 70.0, 70000},
		{"85 percent", 85.0, 85000},
		{"95 percent", 95.0, 95000},
		{"zero", 0.0, 0},
		{"100 percent", 100.0, 100000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, cl.PressureThreshold(tc.pct))
		})
	}
}

func TestDefaultContextLimits(t *testing.T) {
	t.Parallel()

	t.Run("128k model", func(t *testing.T) {
		t.Parallel()
		cl := DefaultContextLimits(128000)
		require.Equal(t, 128000, cl.MaxTokens)
		require.Equal(t, 25600, cl.ReserveTokens)
		require.Equal(t, 25600, cl.SummaryBudget)
		require.Equal(t, 102400, cl.AvailableTokens())
	})

	t.Run("200k model", func(t *testing.T) {
		t.Parallel()
		cl := DefaultContextLimits(200000)
		require.Equal(t, 200000, cl.MaxTokens)
		require.Equal(t, 40000, cl.ReserveTokens)
		require.Equal(t, 40000, cl.SummaryBudget)
		require.Equal(t, 160000, cl.AvailableTokens())
	})

	t.Run("small model", func(t *testing.T) {
		t.Parallel()
		cl := DefaultContextLimits(8000)
		require.Equal(t, 8000, cl.MaxTokens)
		require.Equal(t, 1600, cl.ReserveTokens)
		require.Equal(t, 1600, cl.SummaryBudget)
		require.Equal(t, 6400, cl.AvailableTokens())
	})
}
