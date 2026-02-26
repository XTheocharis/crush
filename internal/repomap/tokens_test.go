package repomap

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeCounter struct {
	out int
	err error
}

func (f fakeCounter) Count(_ context.Context, _ string, _ string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.out, nil
}

func TestEstimateTokensByLanguageRatio(t *testing.T) {
	t.Parallel()

	require.Equal(t, 4, EstimateTokens("123456789012", "go"))
	require.Equal(t, 4, EstimateTokens("123456789012", "json"))
	require.Equal(t, 4, EstimateTokens("123456789012", "unknown"))
	require.Equal(t, 0, EstimateTokens("", "go"))
}

func TestCountParityAndSafetyTokensWithCounter(t *testing.T) {
	t.Parallel()

	metrics, err := CountParityAndSafetyTokens(context.Background(), fakeCounter{out: 100}, "m", stringsOfLen(100), "go")
	require.NoError(t, err)
	require.InDelta(t, 100, metrics.ParityTokens, 1e-9)

	// heuristic estimate for 100 chars in go: ceil(100/3.2)=32; safety=max(100, ceil(32*1.15)=37)=100
	require.Equal(t, 100, metrics.SafetyTokens)
}

func TestCountParityAndSafetyTokensWithoutCounterUsesHeuristic(t *testing.T) {
	t.Parallel()

	text := stringsOfLen(64)
	metrics, err := CountParityAndSafetyTokens(context.Background(), nil, "m", text, "json")
	require.NoError(t, err)

	est := EstimateTokens(text, "json")
	require.InDelta(t, est, metrics.ParityTokens, 1e-9)
	require.Equal(t, int(maxFloat64(float64(est), ceilFloat64(float64(est)*1.15))), metrics.SafetyTokens)
}

func TestCountParityAndSafetyTokensPropagatesCounterError(t *testing.T) {
	t.Parallel()

	_, err := CountParityAndSafetyTokens(context.Background(), fakeCounter{err: errors.New("boom")}, "m", "abc", "go")
	require.Error(t, err)
}

func stringsOfLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func ceilFloat64(v float64) float64 {
	if float64(int(v)) == v {
		return v
	}
	return float64(int(v) + 1)
}
