package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultRepoMapMaxTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ctxWin   int
		expected int
	}{
		{"small window", 4096, 1024},
		{"medium window", 32768, 4096},
		{"large window", 200000, 4096},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, DefaultRepoMapMaxTokens(tt.ctxWin))
		})
	}
}

func TestDefaultRepoMapMaxTokensLCM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ctxWin   int
		expected int
	}{
		{"very small window", 4096, 1024},
		{"minimum boundary", 8192, 1024},
		{"medium window", 32768, 4096},
		{"large window 128k", 128000, 8192},
		{"large window 200k", 200000, 8192},
		{"exact boundary", 65536, 8192},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DefaultRepoMapMaxTokensLCM(tt.ctxWin)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestDefaultRepoMapMaxTokensLCM_AlwaysGreaterOrEqualToBase(t *testing.T) {
	t.Parallel()

	// For all reasonable context windows, LCM budget should be >= base budget.
	for _, ctxWin := range []int{4096, 8192, 16384, 32768, 65536, 128000, 200000} {
		base := DefaultRepoMapMaxTokens(ctxWin)
		lcmBudget := DefaultRepoMapMaxTokensLCM(ctxWin)
		require.GreaterOrEqual(t, lcmBudget, base,
			"LCM budget should be >= base for ctxWin=%d", ctxWin)
	}
}
