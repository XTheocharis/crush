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
	// hardLimit = 40000 - 200 - 10000 = 29800
	require.Equal(t, int64(29800), budget.HardLimit)
}

func TestComputeBudget_LargeWindow(t *testing.T) {
	t.Parallel()
	// With a large window (200000), 25% = 50000 > 20000, so cap at 20000.
	budget := ComputeBudget(BudgetConfig{
		ContextWindow:   200000,
		CutoffThreshold: 0.6,
	})
	// hardLimit = 200000 - 200 - 20000 = 179800
	require.Equal(t, int64(179800), budget.HardLimit)
}

func TestComputeBudget_WithOverhead(t *testing.T) {
	t.Parallel()
	budget := ComputeBudget(BudgetConfig{
		ContextWindow:      128000,
		CutoffThreshold:    0.6,
		SystemPromptTokens: 5000,
		ToolTokens:         3000,
		RepoMapTokens:      2000,
	})

	// overhead = 5000 + 3000 + 2000 + 200 = 10200
	// outputReserve = min(20000, 128000*0.25=32000) = 20000
	// hardLimit = 128000 - 10200 - 20000 = 97800
	require.Equal(t, int64(97800), budget.HardLimit)

	// softRaw = 128000*0.6 - 10200 = 76800 - 10200 = 66600
	// softThreshold = min(66600, 97800) = 66600
	require.Equal(t, int64(66600), budget.SoftThreshold)
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
