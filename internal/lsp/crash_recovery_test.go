package lsp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCrashRecoveryDetectsCrashAndRestarts(t *testing.T) {
	t.Parallel()

	var (
		attempts atomic.Int32
		backoff  = ExponentialBackoff{
			InitialInterval: 1 * time.Millisecond,
			MaxInterval:     10 * time.Millisecond,
			Multiplier:      2.0,
			MaxRetries:      5,
		}
	)

	cr := NewCrashRecovery("test-server", backoff, func(ctx context.Context) error {
		n := attempts.Add(1)
		if n <= 2 {
			return ErrServerCrashed
		}
		return nil
	})

	err := cr.Run(context.Background())
	require.NoError(t, err)
	require.Equal(t, int32(3), attempts.Load())
}

func TestCrashRecoveryExceedsMaxRetries(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	backoff := ExponentialBackoff{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     5 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      3,
	}

	cr := NewCrashRecovery("failing-server", backoff, func(ctx context.Context) error {
		attempts.Add(1)
		return ErrServerCrashed
	})

	err := cr.Run(context.Background())
	require.ErrorIs(t, err, ErrMaxRestartsExceeded)
	require.Equal(t, int32(4), attempts.Load(), "initial attempt + 3 retries")
}

func TestCrashRecoveryContextCancellation(t *testing.T) {
	t.Parallel()

	backoff := ExponentialBackoff{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     5 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      5,
	}

	cr := NewCrashRecovery("ctx-server", backoff, func(ctx context.Context) error {
		return ErrServerCrashed
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := cr.Run(ctx)
	require.Error(t, err)
}

func TestCrashRecoverySuccessOnFirstAttempt(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	backoff := ExponentialBackoff{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     10 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      5,
	}

	cr := NewCrashRecovery("good-server", backoff, func(ctx context.Context) error {
		attempts.Add(1)
		return nil
	})

	err := cr.Run(context.Background())
	require.NoError(t, err)
	require.Equal(t, int32(1), attempts.Load())
}

func TestCrashRecoveryNonCrashErrorReturns(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	backoff := ExponentialBackoff{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     10 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      5,
	}

	cr := NewCrashRecovery("err-server", backoff, func(ctx context.Context) error {
		attempts.Add(1)
		return ErrClientNotFound
	})

	err := cr.Run(context.Background())
	require.ErrorIs(t, err, ErrClientNotFound)
	require.Equal(t, int32(1), attempts.Load(), "non-crash error should not retry")
}

func TestCrashRecoveryStateTracking(t *testing.T) {
	t.Parallel()

	backoff := ExponentialBackoff{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     5 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      5,
	}

	cr := NewCrashRecovery("state-server", backoff, func(ctx context.Context) error {
		return nil
	})

	require.Equal(t, 0, cr.Attempts())
	require.False(t, cr.LastCrashed())
}
