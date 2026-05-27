package lsp

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExponentialBackoffDefaultValues(t *testing.T) {
	t.Parallel()

	b := DefaultBackoff()
	require.Equal(t, 1*time.Second, b.InitialInterval)
	require.Equal(t, 60*time.Second, b.MaxInterval)
	require.Equal(t, 2.0, b.Multiplier)
	require.Equal(t, 5, b.MaxRetries)
}

func TestExponentialBackoffIntervalGrowth(t *testing.T) {
	t.Parallel()

	b := DefaultBackoff()

	d0 := b.NextInterval(0)
	require.GreaterOrEqual(t, d0, 750*time.Millisecond)
	require.LessOrEqual(t, d0, 1250*time.Millisecond)

	d1 := b.NextInterval(1)
	require.GreaterOrEqual(t, d1, 1500*time.Millisecond)
	require.LessOrEqual(t, d1, 2500*time.Millisecond)

	d2 := b.NextInterval(2)
	require.GreaterOrEqual(t, d2, 3*time.Second)
	require.LessOrEqual(t, d2, 5*time.Second)

	d3 := b.NextInterval(3)
	require.GreaterOrEqual(t, d3, 6*time.Second)
	require.LessOrEqual(t, d3, 10*time.Second)

	d4 := b.NextInterval(4)
	require.GreaterOrEqual(t, d4, 12*time.Second)
	require.LessOrEqual(t, d4, 20*time.Second)

	d5 := b.NextInterval(5)
	require.GreaterOrEqual(t, d5, 24*time.Second)
	require.LessOrEqual(t, d5, 40*time.Second)
}

func TestExponentialBackoffMaxIntervalCap(t *testing.T) {
	t.Parallel()

	b := ExponentialBackoff{
		InitialInterval: 1 * time.Second,
		MaxInterval:     60 * time.Second,
		Multiplier:      2.0,
		MaxRetries:      0,
	}

	// Attempt 10: 1s * 2^10 = 1024s, capped at 60s ±25%.
	d := b.NextInterval(10)
	require.GreaterOrEqual(t, d, 45*time.Second)
	require.LessOrEqual(t, d, 75*time.Second)
}

func TestExponentialBackoffMaxRetriesCap(t *testing.T) {
	t.Parallel()

	b := DefaultBackoff()

	// MaxRetries=5, attempt 100 treated as attempt 5 → 32s ±25%.
	d := b.NextInterval(100)
	require.GreaterOrEqual(t, d, 24*time.Second)
	require.LessOrEqual(t, d, 40*time.Second)
}

func TestExponentialBackoffNegativeAttempt(t *testing.T) {
	t.Parallel()

	b := DefaultBackoff()

	// Negative attempt treated as 0 → 1s ±25%.
	d := b.NextInterval(-1)
	require.GreaterOrEqual(t, d, 750*time.Millisecond)
	require.LessOrEqual(t, d, 1250*time.Millisecond)
}

func TestExponentialBackoffJitterRange(t *testing.T) {
	t.Parallel()

	b := DefaultBackoff()

	// Run NextInterval many times for attempt 0 and verify all results
	// fall within [750ms, 1250ms] (1s ±25%).
	min := time.Duration(math.MaxInt64)
	max := time.Duration(0)
	for range 1000 {
		d := b.NextInterval(0)
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	require.GreaterOrEqual(t, min, 750*time.Millisecond)
	require.LessOrEqual(t, max, 1250*time.Millisecond)
}

func TestExponentialBackoffCustomConfig(t *testing.T) {
	t.Parallel()

	b := ExponentialBackoff{
		InitialInterval: 500 * time.Millisecond,
		MaxInterval:     10 * time.Second,
		Multiplier:      3.0,
		MaxRetries:      3,
	}

	d0 := b.NextInterval(0)
	require.GreaterOrEqual(t, d0, 375*time.Millisecond)
	require.LessOrEqual(t, d0, 625*time.Millisecond)

	d1 := b.NextInterval(1)
	require.GreaterOrEqual(t, d1, 1125*time.Millisecond)
	require.LessOrEqual(t, d1, 1875*time.Millisecond)

	d2 := b.NextInterval(2)
	require.GreaterOrEqual(t, d2, 3375*time.Millisecond)
	require.LessOrEqual(t, d2, 5625*time.Millisecond)

	// Attempt 3: 13.5s exceeds 10s cap → 10s ±25%.
	d3 := b.NextInterval(3)
	require.GreaterOrEqual(t, d3, 7500*time.Millisecond)
	require.LessOrEqual(t, d3, 12500*time.Millisecond)

	// Attempt 4: capped at MaxRetries=3, same as attempt 3.
	d4 := b.NextInterval(4)
	require.GreaterOrEqual(t, d4, 7500*time.Millisecond)
	require.LessOrEqual(t, d4, 12500*time.Millisecond)
}

func TestExponentialBackoffNoMaxRetries(t *testing.T) {
	t.Parallel()

	b := ExponentialBackoff{
		InitialInterval: 1 * time.Second,
		MaxInterval:     60 * time.Second,
		Multiplier:      2.0,
		MaxRetries:      0,
	}

	// MaxRetries=0: no attempt cap, only MaxInterval caps growth.
	d := b.NextInterval(10)
	require.GreaterOrEqual(t, d, 45*time.Second)
	require.LessOrEqual(t, d, 75*time.Second)
}

func TestRetryDelayMatchesNextInterval(t *testing.T) {
	t.Parallel()

	b := DefaultBackoff()
	for attempt := range 5 {
		d := b.RetryDelay(attempt)
		expected := b.InitialInterval * time.Duration(math.Pow(b.Multiplier, float64(attempt)))
		jitter := float64(expected) * 0.25
		require.GreaterOrEqual(t, d, time.Duration(float64(expected)-jitter))
		require.LessOrEqual(t, d, time.Duration(float64(expected)+jitter))
	}
}
