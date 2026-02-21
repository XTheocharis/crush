package lcm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComputeBudget_DefaultValues(t *testing.T) {
	t.Parallel()
	budget := ComputeBudget(BudgetConfig{
		ContextWindow:   128000,
		CutoffThreshold: 0.6,
	})

	require.Greater(t, budget.SoftThreshold, int64(0), "soft threshold should be positive")
	require.Greater(t, budget.HardLimit, int64(0), "hard limit should be positive")
	require.Equal(t, int64(128000), budget.ContextWindow, "context window should be preserved")
}

func TestComputeBudget_SoftLessThanHard(t *testing.T) {
	t.Parallel()
	budget := ComputeBudget(BudgetConfig{
		ContextWindow:   128000,
		CutoffThreshold: 0.6,
	})

	require.LessOrEqual(t, budget.SoftThreshold, budget.HardLimit,
		"soft threshold (%d) should not exceed hard limit (%d)",
		budget.SoftThreshold, budget.HardLimit)
}

func TestComputeBudget_OutputReserveClamp(t *testing.T) {
	t.Parallel()
	// With a small window (40000), 25% = 10000 < 20000 default.
	budget := ComputeBudget(BudgetConfig{
		ContextWindow:   40000,
		CutoffThreshold: 0.6,
	})
	// hardLimit = 40000 - 0 - 10000 = 30000
	require.Equal(t, int64(30000), budget.HardLimit)
}

func TestComputeBudget_LargeWindow(t *testing.T) {
	t.Parallel()
	// With a large window (200000), 25% = 50000 > 20000, so cap at 20000.
	budget := ComputeBudget(BudgetConfig{
		ContextWindow:   200000,
		CutoffThreshold: 0.6,
	})
	// hardLimit = 200000 - 0 - 20000 = 180000
	require.Equal(t, int64(180000), budget.HardLimit)
}

func TestComputeBudget_WithOverhead(t *testing.T) {
	t.Parallel()
	budget := ComputeBudget(BudgetConfig{
		ContextWindow:      128000,
		CutoffThreshold:    0.6,
		SystemPromptTokens: 5000,
		ToolTokens:         3000,
	})

	// overhead = 5000 + 3000 = 8000
	// outputReserve = min(20000, 128000*0.25=32000) = 20000
	// hardLimit = 128000 - 8000 - 20000 = 100000
	require.Equal(t, int64(100000), budget.HardLimit)

	// softRaw = 128000*0.6 - 8000 = 76800 - 8000 = 68800
	// softThreshold = min(68800, 100000) = 68800
	require.Equal(t, int64(68800), budget.SoftThreshold)
}

func TestComputeBudget_ZeroContextWindow(t *testing.T) {
	t.Parallel()
	budget := ComputeBudget(BudgetConfig{
		ContextWindow:   0,
		CutoffThreshold: 0.6,
	})

	require.Equal(t, int64(0), budget.HardLimit, "hardLimit should be 0 for zero context window")
	require.Equal(t, int64(0), budget.SoftThreshold, "soft threshold should be 0 for zero context window")
}

func TestComputeBudget_HighCutoff(t *testing.T) {
	t.Parallel()
	budget := ComputeBudget(BudgetConfig{
		ContextWindow:   128000,
		CutoffThreshold: 0.95,
	})

	// softRaw = 128000 * 0.95 = 121600
	// hardLimit = 128000 - 20000 = 108000
	// softThreshold = min(121600, 108000) = 108000
	require.Equal(t, budget.SoftThreshold, budget.HardLimit,
		"with high cutoff, soft should clamp to hard limit")
}

func TestEstimateTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"empty string", "", 0},
		{"one char", "a", 1},
		{"four chars", "abcd", 1},
		{"five chars", "abcde", 2},
		{"eight chars", "abcdefgh", 2},
		{"nine chars", "abcdefghi", 3},
		{"unicode", "hello \u4e16\u754c", 2}, // 8 runes -> (8+3)/4 = 2
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := EstimateTokens(tt.input)
			require.Equal(t, tt.expected, got)
		})
	}
}

func BenchmarkEstimateTokens(b *testing.B) {
	s := strings.Repeat("Hello world, this is a benchmark string for token estimation. ", 100)
	for b.Loop() {
		EstimateTokens(s)
	}
}
