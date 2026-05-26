package lsp

import (
	"math"
	"math/rand/v2"
	"time"
)

// DefaultBackoff returns an ExponentialBackoff with sensible defaults:
// 1s initial, 60s max, 2x multiplier, 5 max retries.
func DefaultBackoff() ExponentialBackoff {
	return ExponentialBackoff{
		InitialInterval: 1 * time.Second,
		MaxInterval:     60 * time.Second,
		Multiplier:      2.0,
		MaxRetries:      5,
	}
}

// ExponentialBackoff computes retry intervals with optional jitter.
type ExponentialBackoff struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxRetries      int
}

// NextInterval returns the backoff duration for the given attempt number
// (0-indexed). The result includes ±25% jitter to avoid thundering-herd
// effects. If the attempt exceeds MaxRetries, MaxInterval is returned.
func (b ExponentialBackoff) NextInterval(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	if b.MaxRetries > 0 && attempt > b.MaxRetries {
		attempt = b.MaxRetries
	}

	// Compute raw interval: InitialInterval * Multiplier^attempt.
	raw := float64(b.InitialInterval) * math.Pow(b.Multiplier, float64(attempt))
	maxF := float64(b.MaxInterval)
	if raw > maxF {
		raw = maxF
	}

	// Apply ±25% jitter.
	jitter := raw * 0.25
	raw += (rand.Float64()*2 - 1) * jitter

	if raw < 0 {
		raw = 0
	}
	return time.Duration(raw)
}

// RetryDelay returns the backoff interval for the given attempt, capped to
// [InitialInterval, MaxInterval].
func (b ExponentialBackoff) RetryDelay(attempt int) time.Duration {
	return b.NextInterval(attempt)
}

// [XRUSH: begin: exponential backoff retry state]
// serverRetryState tracks per-server retry state for exponential backoff.
type serverRetryState struct {
	LastAttempt  time.Time
	AttemptCount int
}

// [XRUSH: end]
