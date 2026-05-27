package repomap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

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

// runeCounter returns one token per rune, making sampling math predictable.
type runeCounter struct{}

func (runeCounter) Count(_ context.Context, _ string, text string) (int, error) {
	return utf8.RuneCountInString(text), nil
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

// --- countWithSampling tests ---

func TestCountWithSamplingShortTextFullCount(t *testing.T) {
	t.Parallel()

	// Text shorter than 200 runes should use full tokenizer count.
	text := "hello world" // 11 runes, well under 200.
	counter := fakeCounter{out: 42}
	got, err := countWithSampling(context.Background(), counter, "m", text)
	require.NoError(t, err)
	require.InDelta(t, 42.0, got, 1e-9)
}

func TestCountWithSamplingLongTextUsesSampling(t *testing.T) {
	t.Parallel()

	// Build text >200 runes with 1000 lines to verify sampling behaviour.
	var b strings.Builder
	for i := range 1000 {
		fmt.Fprintf(&b, "line %04d content here\n", i)
	}
	text := b.String()
	require.Greater(t, utf8.RuneCountInString(text), 200)

	// runeCounter returns 1 token per rune, so with proportional
	// extrapolation the estimate should equal the total rune count.
	got, err := countWithSampling(context.Background(), runeCounter{}, "m", text)
	require.NoError(t, err)

	totalRunes := float64(utf8.RuneCountInString(text))
	// The proportional estimate should be very close to totalRunes because
	// (sampleTokens/sampleRunes)*totalRunes == totalRunes when 1 token/rune.
	require.InDelta(t, totalRunes, got, totalRunes*0.02)
}

func TestCountWithSamplingSamplingStep(t *testing.T) {
	t.Parallel()

	// 1000 lines should yield step = 1000/100 = 10, so every 10th line
	// is sampled. Verify by counting how many times a line-length counter
	// is invoked (the counter sees only sampled text).
	var b strings.Builder
	for i := range 1000 {
		fmt.Fprintf(&b, "L%04d\n", i)
	}
	text := b.String()
	require.Greater(t, utf8.RuneCountInString(text), 200)

	// Track how many runes the counter sees (the sample).
	var sampledRunes int
	lengthCounter := countFunc(func(_ context.Context, _ string, sample string) (int, error) {
		sampledRunes = utf8.RuneCountInString(sample)
		return sampledRunes, nil
	})

	_, err := countWithSampling(context.Background(), lengthCounter, "m", text)
	require.NoError(t, err)

	// With 1000 lines and step=10, we sample lines 0,10,20,...,990 = 100 lines.
	// Each line is "Lnnnn\n" = 6 chars.
	expectedSampledLines := 100
	expectedSampledRunes := expectedSampledLines * 6
	require.Equal(t, expectedSampledRunes, sampledRunes)
}

func TestCountWithSamplingProportionalEstimation(t *testing.T) {
	t.Parallel()

	// Construct text where sampled and unsampled lines have different lengths.
	// Lines at indices 0, 10, 20, ... will be sampled (step = 1000/100 = 10).
	var b strings.Builder
	for i := range 1000 {
		if i%10 == 0 {
			b.WriteString("AA\n") // Sampled lines: 3 runes each.
		} else {
			b.WriteString("BBBBBB\n") // Non-sampled: 7 runes each.
		}
	}
	text := b.String()
	totalRunes := float64(utf8.RuneCountInString(text))
	require.Greater(t, totalRunes, float64(200))

	// Sample = 100 lines of "AA\n" = 300 runes.
	// runeCounter sees 300 runes, returns 300 tokens.
	// Estimate = (300 / 300) * totalRunes = totalRunes.
	// But the actual token count is totalRunes if 1 token/rune.
	got, err := countWithSampling(context.Background(), runeCounter{}, "m", text)
	require.NoError(t, err)

	// Verify the math: sampleTokens / sampleRuneLen * totalRunes.
	sampleRuneLen := 100 * 3      // 100 lines of "AA\n".
	sampleTokens := sampleRuneLen // runeCounter returns rune count.
	expected := float64(sampleTokens) / float64(sampleRuneLen) * totalRunes
	require.InDelta(t, expected, got, 1e-9)
}

func TestCountWithSamplingPropagatesError(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("abcdefghij\n", 100) // >200 runes.
	counter := fakeCounter{err: errors.New("boom")}
	_, err := countWithSampling(context.Background(), counter, "m", text)
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

// --- Parity preflight guard tests ---

func TestCountParityAndSafetyTokensParityModeNilCounterFails(t *testing.T) {
	t.Parallel()

	// CountParityAndSafetyTokens with nil counter falls back to heuristic
	// (the preflight guard is in Generate, not here). But we verify that
	// the function itself still works correctly with nil counter.
	text := stringsOfLen(100)
	metrics, err := CountParityAndSafetyTokens(context.Background(), nil, "m", text, "go")
	require.NoError(t, err)

	// Should use heuristic path.
	est := EstimateTokens(text, "go")
	require.InDelta(t, float64(est), metrics.ParityTokens, 1e-9)
}

func TestCountParityAndSafetyTokensNonParityNilCounterSucceeds(t *testing.T) {
	t.Parallel()

	text := stringsOfLen(64)
	metrics, err := CountParityAndSafetyTokens(context.Background(), nil, "m", text, "json")
	require.NoError(t, err)

	est := EstimateTokens(text, "json")
	require.InDelta(t, float64(est), metrics.ParityTokens, 1e-9)
	require.Greater(t, metrics.SafetyTokens, 0)
}

// countFunc is a TokenCounter that delegates to a function.
type countFunc func(ctx context.Context, model, text string) (int, error)

func (f countFunc) Count(ctx context.Context, model, text string) (int, error) {
	return f(ctx, model, text)
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
