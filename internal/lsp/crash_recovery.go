package lsp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ErrServerCrashed indicates the LSP server process exited unexpectedly.
var ErrServerCrashed = errors.New("lsp: server crashed")

// ErrMaxRestartsExceeded is returned when the server has been restarted more
// times than the backoff's MaxRetries allows.
var ErrMaxRestartsExceeded = errors.New("lsp: max restart attempts exceeded")

// StartFunc is the function called to start (or restart) an LSP server.
type StartFunc func(ctx context.Context) error

// CrashRecovery wraps an LSP server start function with automatic crash
// detection and restart using exponential backoff. It uses the same
// ExponentialBackoff parameters as the rest of the LSP manager (1s→60s, 5
// retries by default).
type CrashRecovery struct {
	serverName string
	backoff    ExponentialBackoff
	start      StartFunc
	attempts   int
	crashed    bool
}

// NewCrashRecovery creates a CrashRecovery for the named server.
func NewCrashRecovery(serverName string, backoff ExponentialBackoff, start StartFunc) *CrashRecovery {
	return &CrashRecovery{
		serverName: serverName,
		backoff:    backoff,
		start:      start,
	}
}

// Run executes the start function with crash recovery. If the start function
// returns ErrServerCrashed, Run waits for the backoff interval and retries.
// Non-crash errors are returned immediately. The retry count is bounded by
// backoff.MaxRetries.
func (cr *CrashRecovery) Run(ctx context.Context) error {
	cr.attempts = 0
	cr.crashed = false

	for {
		cr.attempts++
		err := cr.start(ctx)
		if err == nil {
			return nil
		}

		if !errors.Is(err, ErrServerCrashed) {
			return err
		}

		cr.crashed = true
		retryIndex := cr.attempts - 1

		if cr.backoff.MaxRetries > 0 && retryIndex >= cr.backoff.MaxRetries {
			slog.Error("LSP server exceeded max restart attempts",
				"name", cr.serverName, "attempts", cr.attempts)
			return fmt.Errorf("%w: %s after %d attempts",
				ErrMaxRestartsExceeded, cr.serverName, cr.attempts)
		}

		delay := cr.backoff.RetryDelay(retryIndex)
		slog.Warn("LSP server crashed, restarting after backoff",
			"name", cr.serverName, "attempt", cr.attempts, "delay", delay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// Attempts returns the total number of start attempts made so far.
func (cr *CrashRecovery) Attempts() int {
	return cr.attempts
}

// LastCrashed reports whether the most recent start attempt resulted in a crash.
func (cr *CrashRecovery) LastCrashed() bool {
	return cr.crashed
}
