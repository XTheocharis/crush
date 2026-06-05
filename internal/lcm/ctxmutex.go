package lcm

import (
	"context"
	"sync"
)

// ctxMutex wraps a sync.Mutex with context-aware locking.
// LockContext blocks until the mutex is acquired or the context is cancelled.
type ctxMutex struct {
	mu sync.Mutex
}

// LockContext acquires the mutex, returning an error if the context is
// cancelled before the lock is acquired.
func (m *ctxMutex) LockContext(ctx context.Context) error {
	// Fast path: try to acquire without goroutine overhead.
	if m.mu.TryLock() {
		return nil
	}

	// Slow path: race between context cancellation and lock acquisition.
	locked := make(chan struct{})
	go func() {
		m.mu.Lock()
		close(locked)
	}()

	select {
	case <-locked:
		return nil
	case <-ctx.Done():
		// The goroutine is still blocked on Lock(). We can't cancel it,
		// but we must not leak the goroutine. Wait for it to acquire the
		// lock, then unlock immediately.
		<-locked
		m.mu.Unlock()
		return ctx.Err()
	}
}

// Unlock releases the mutex.
func (m *ctxMutex) Unlock() {
	m.mu.Unlock()
}
