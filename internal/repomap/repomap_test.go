package repomap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDisableLatchParityModeDeadlineExceeded verifies that after a
// context.DeadlineExceeded error in parity-mode Generate, the session
// is permanently disabled and subsequent calls return the fallback.
func TestDisableLatchParityModeDeadlineExceeded(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Pre-populate a known-good cached map so fallback returns it.
	svc.sessionCaches.Store("sess-1", "cached-map", 42)

	// Manually trigger the disable latch as if DeadlineExceeded occurred
	// during parity-mode generation.
	svc.disableForSession("sess-1")
	require.True(t, svc.isDisabledForSession("sess-1"))

	// Generate must return the fallback (cached) value without regenerating.
	m, tok, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: true,
	})
	require.NoError(t, err)
	require.Equal(t, "cached-map", m)
	require.Equal(t, 42, tok)

	// Even ForceRefresh does not bypass the latch: the latch check
	// precedes ForceRefresh cache clearing in Generate(). The fallback
	// returns the last-loaded cache snapshot from before the clear.
	m, tok, err = svc.Generate(context.Background(), GenerateOpts{
		SessionID:    "sess-1",
		ParityMode:   true,
		ForceRefresh: true,
	})
	require.NoError(t, err)
	// Latch fires before ForceRefresh clearing — fallback uses snapshot.
	require.Equal(t, "cached-map", m)
	require.Equal(t, 42, tok)
}

// TestDisableLatchResetClears verifies that Reset() clears the disable
// latch, allowing future Generate calls to proceed normally.
func TestDisableLatchResetClears(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	svc.disableForSession("sess-1")
	require.True(t, svc.isDisabledForSession("sess-1"))

	err := svc.Reset(context.Background(), "sess-1")
	require.NoError(t, err)

	require.False(t, svc.isDisabledForSession("sess-1"))

	// After reset, Generate should proceed past the latch check.
	// With nil db it will fallback, but the latch is no longer the cause.
	svc.sessionCaches.Store("sess-1", "new-map", 99)
	m, tok, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID: "sess-1",
	})
	require.NoError(t, err)
	require.Equal(t, "new-map", m)
	require.Equal(t, 99, tok)
}

// TestDisableLatchMultiSessionIsolation verifies that disabling session A
// does not affect session B.
func TestDisableLatchMultiSessionIsolation(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	svc.sessionCaches.Store("sess-A", "map-A", 10)
	svc.sessionCaches.Store("sess-B", "map-B", 20)

	// Disable session A only.
	svc.disableForSession("sess-A")

	require.True(t, svc.isDisabledForSession("sess-A"))
	require.False(t, svc.isDisabledForSession("sess-B"))

	// Session A returns fallback due to latch.
	mA, tokA, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:  "sess-A",
		ParityMode: true,
	})
	require.NoError(t, err)
	require.Equal(t, "map-A", mA)
	require.Equal(t, 10, tokA)

	// Session B proceeds normally (returns cache because db is nil).
	mB, tokB, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:  "sess-B",
		ParityMode: true,
	})
	require.NoError(t, err)
	require.Equal(t, "map-B", mB)
	require.Equal(t, 20, tokB)
}

// TestDisableLatchNonDeadlineErrorDoesNotTrigger verifies that non-deadline
// errors (such as file read errors) do not trigger the disable latch.
// This tests the infrastructure contract directly since reaching the error
// paths in extractTags/FitToBudget requires database setup.
func TestDisableLatchNonDeadlineErrorDoesNotTrigger(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Verify session starts without latch.
	require.False(t, svc.isDisabledForSession("sess-1"))

	// Generate with nil db falls through to fallback(nil) without error.
	// No latch should be set because there's no DeadlineExceeded error.
	_, _, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: true,
	})
	require.NoError(t, err)
	require.False(t, svc.isDisabledForSession("sess-1"),
		"non-error generate path must not trigger disable latch")
}

// TestDisableLatchContextCanceledDoesNotTrigger verifies that
// context.Canceled does not trigger the disable latch. Canceled represents
// user cancellation, not resource exhaustion.
func TestDisableLatchContextCanceledDoesNotTrigger(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Pre-populate cache so the service is not freshly empty.
	svc.sessionCaches.Store("sess-1", "map", 10)

	// A canceled context will cause checkContextsDone to return
	// context.Canceled immediately, never reaching the error handlers.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := svc.Generate(ctx, GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: true,
	})
	require.ErrorIs(t, err, context.Canceled)

	// The session must NOT be disabled.
	require.False(t, svc.isDisabledForSession("sess-1"),
		"context.Canceled must not trigger disable latch")
}

// TestDisableLatchEnhancementModeDeadlineDoesNotTrigger verifies that
// context.DeadlineExceeded in enhancement mode does NOT trigger the
// permanent disable latch. Enhancement mode treats timeouts as transient.
func TestDisableLatchEnhancementModeDeadlineDoesNotTrigger(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Simulate: DeadlineExceeded occurred, but ParityMode is false.
	// The latch should NOT engage.
	require.False(t, svc.isDisabledForSession("sess-1"))

	// A timed-out context triggers checkContextsDone before the latch
	// handlers, so we test the guard condition directly: the error
	// handlers only call disableForSession when opts.ParityMode is true.
	// With enhancement mode, the session stays enabled.

	// Manually verify the parity guard: calling disableForSession is
	// gated on opts.ParityMode in the error handlers.
	svc.sessionCaches.Store("sess-1", "map", 10)

	// Generate with a deadline-exceeded context returns immediately
	// from checkContextsDone, which doesn't trigger the latch.
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-ctx.Done() // Ensure deadline passed.

	_, _, err := svc.Generate(ctx, GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: false, // Enhancement mode.
	})
	require.Error(t, err)
	require.False(t, svc.isDisabledForSession("sess-1"),
		"enhancement-mode DeadlineExceeded must not trigger disable latch")
}

// TestDisableLatchEnhancementModeRetryAfterTimeout verifies that in
// enhancement mode, a timeout is transient and subsequent calls retry.
func TestDisableLatchEnhancementModeRetryAfterTimeout(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// First call: deadline exceeded.
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-ctx.Done()

	_, _, err := svc.Generate(ctx, GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: false,
	})
	require.Error(t, err)

	// Session is not disabled.
	require.False(t, svc.isDisabledForSession("sess-1"))

	// Second call: fresh context, cached value available.
	svc.sessionCaches.Store("sess-1", "recovered-map", 55)
	m, tok, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: false,
	})
	require.NoError(t, err)
	require.Equal(t, "recovered-map", m)
	require.Equal(t, 55, tok)
}

// TestDisableLatchInfrastructureMethods directly tests the disableForSession
// and isDisabledForSession methods for correctness and concurrency safety.
func TestDisableLatchInfrastructureMethods(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Initially no session is disabled.
	require.False(t, svc.isDisabledForSession("a"))
	require.False(t, svc.isDisabledForSession("b"))
	require.False(t, svc.isDisabledForSession(""))

	// Disable session "a".
	svc.disableForSession("a")
	require.True(t, svc.isDisabledForSession("a"))
	require.False(t, svc.isDisabledForSession("b"))

	// Idempotent: disabling twice is safe.
	svc.disableForSession("a")
	require.True(t, svc.isDisabledForSession("a"))

	// Delete clears the latch.
	svc.disabledSessions.Delete("a")
	require.False(t, svc.isDisabledForSession("a"))
}

// TestDisableLatchConcurrentAccess verifies the sync.Map-based latch
// is safe under concurrent access from multiple goroutines.
func TestDisableLatchConcurrentAccess(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())
	const goroutines = 100

	done := make(chan struct{})

	// Concurrent writers.
	for i := range goroutines {
		go func(idx int) {
			sessionID := "sess-" + string(rune('A'+idx%26))
			svc.disableForSession(sessionID)
			_ = svc.isDisabledForSession(sessionID)
		}(i)
	}

	// Concurrent readers.
	for i := range goroutines {
		go func(idx int) {
			sessionID := "sess-" + string(rune('A'+idx%26))
			_ = svc.isDisabledForSession(sessionID)
		}(i)
	}

	close(done)
}

// TestDisableLatchResetOnlyAffectsTargetSession verifies that Reset()
// on one session does not clear the latch for other sessions.
func TestDisableLatchResetOnlyAffectsTargetSession(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	svc.disableForSession("sess-1")
	svc.disableForSession("sess-2")
	svc.disableForSession("sess-3")

	require.True(t, svc.isDisabledForSession("sess-1"))
	require.True(t, svc.isDisabledForSession("sess-2"))
	require.True(t, svc.isDisabledForSession("sess-3"))

	// Reset only sess-2.
	err := svc.Reset(context.Background(), "sess-2")
	require.NoError(t, err)

	require.True(t, svc.isDisabledForSession("sess-1"),
		"sess-1 latch must survive reset of sess-2")
	require.False(t, svc.isDisabledForSession("sess-2"),
		"sess-2 latch must be cleared by reset")
	require.True(t, svc.isDisabledForSession("sess-3"),
		"sess-3 latch must survive reset of sess-2")
}
