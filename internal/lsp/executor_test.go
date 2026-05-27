package lsp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newStartedExecutor(depth int) *TaskExecutor {
	e := NewTaskExecutor(depth)
	e.Start()
	return e
}

func TestTaskExecutorSerialization(t *testing.T) {
	t.Parallel()

	e := newStartedExecutor(100)
	defer e.Stop()

	var (
		counter atomic.Int64
		running atomic.Int64
		maxConc atomic.Int64
		wg      sync.WaitGroup
	)

	for range 50 {
		wg.Go(func() {
			err := e.Submit(context.Background(), "server-a", func() error {
				cur := running.Add(1)
				if cur > maxConc.Load() {
					maxConc.Store(cur)
				}
				counter.Add(1)
				running.Add(-1)
				return nil
			})
			require.NoError(t, err)
		})
	}
	wg.Wait()

	require.Equal(t, int64(50), counter.Load())
	require.Equal(t, int64(1), maxConc.Load(), "operations must be serialized")
}

func TestTaskExecutorConcurrentDifferentServers(t *testing.T) {
	t.Parallel()

	e := newStartedExecutor(50)
	defer e.Stop()

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := range 10 {
				serverID := fmt.Sprintf("server-%d", idx)
				err := e.Submit(context.Background(), serverID, func() error {
					return nil
				})
				require.NoError(t, err, "submit %d for %s", j, serverID)
			}
		}(i)
	}
	wg.Wait()
}

func TestTaskExecutorQueueOverflowReturnsError(t *testing.T) {
	t.Parallel()

	e := newStartedExecutor(2)
	defer e.Stop()

	blocker := make(chan struct{})

	// Submit a blocking task to "srv" — it occupies the executor goroutine.
	var wg sync.WaitGroup
	wg.Go(func() {
		_ = e.Submit(context.Background(), "srv", func() error {
			<-blocker
			return nil
		})
	})
	time.Sleep(50 * time.Millisecond)

	// Fill the queue to capacity with tasks that also block.
	drainBlockers := make([]chan struct{}, 2)
	for i := range drainBlockers {
		drainBlockers[i] = make(chan struct{})
		ch := drainBlockers[i]
		wg.Go(func() {
			_ = e.Submit(context.Background(), "srv", func() error {
				<-ch
				return nil
			})
		})
	}
	time.Sleep(50 * time.Millisecond)

	// Queue is now full (2 buffered + 1 executing). This should overflow.
	err := e.Submit(context.Background(), "srv", func() error { return nil })
	require.ErrorIs(t, err, ErrQueueFull, "submit to full queue should overflow")

	// Drain the queue and verify recovery.
	for _, ch := range drainBlockers {
		close(ch)
	}
	close(blocker)
	wg.Wait()

	err = e.Submit(context.Background(), "srv", func() error { return nil })
	require.NoError(t, err, "submit after drain should succeed")
}

func TestTaskExecutorStop(t *testing.T) {
	t.Parallel()

	e := newStartedExecutor(10)
	e.Stop()

	err := e.Submit(context.Background(), "srv", func() error { return nil })
	require.ErrorIs(t, err, ErrExecutorStopped)
}

func TestTaskExecutorStopWaitsForInFlight(t *testing.T) {
	t.Parallel()

	e := newStartedExecutor(10)

	var executed atomic.Bool
	go func() {
		_ = e.Submit(context.Background(), "srv", func() error {
			time.Sleep(200 * time.Millisecond)
			executed.Store(true)
			return nil
		})
	}()
	time.Sleep(50 * time.Millisecond)

	e.Stop()
	require.True(t, executed.Load(), "in-flight task should complete before Stop returns")
}

func TestTaskExecutorContextCancellation(t *testing.T) {
	t.Parallel()

	e := newStartedExecutor(10)
	defer e.Stop()

	ctx, cancel := context.WithCancel(context.Background())

	blocker := make(chan struct{})
	go func() {
		_ = e.Submit(ctx, "srv", func() error {
			<-blocker
			return nil
		})
	}()
	time.Sleep(50 * time.Millisecond)

	cancel()
	close(blocker)
	time.Sleep(50 * time.Millisecond)

	err := e.Submit(context.Background(), "srv", func() error { return nil })
	require.NoError(t, err)
}

func TestTaskExecutorPropagatesFunctionError(t *testing.T) {
	t.Parallel()

	e := newStartedExecutor(10)
	defer e.Stop()

	wantErr := errors.New("boom")
	err := e.Submit(context.Background(), "srv", func() error { return wantErr })
	require.ErrorIs(t, err, wantErr)
}

func TestTaskExecutorDefaultQueueDepth(t *testing.T) {
	t.Parallel()

	e := NewTaskExecutor(0)
	require.Equal(t, DefaultQueueDepth, e.depth)

	e = NewTaskExecutor(-5)
	require.Equal(t, DefaultQueueDepth, e.depth)
}

func TestTaskExecutorString(t *testing.T) {
	t.Parallel()

	e := newStartedExecutor(10)
	defer e.Stop()

	_ = e.Submit(context.Background(), "a", func() error { return nil })
	_ = e.Submit(context.Background(), "b", func() error { return nil })

	s := e.String()
	require.Contains(t, s, "TaskExecutor")
	require.Contains(t, s, "queues=2")
	require.Contains(t, s, "depth=10")
}

func TestTaskExecutorStopIdempotent(t *testing.T) {
	t.Parallel()

	e := newStartedExecutor(10)
	e.Stop()
	e.Stop()
}

func TestTaskTimeout(t *testing.T) {
	t.Parallel()

	e := NewTaskExecutor(1, WithTaskTimeout(100*time.Millisecond))
	e.Start()
	defer e.Stop()

	done := make(chan struct{})
	err := e.Submit(context.Background(), "slow-server", func() error {
		<-done
		return nil
	})

	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got: %v", err)
	}

	close(done)
}

func TestTaskTimeoutFastTaskSucceeds(t *testing.T) {
	t.Parallel()

	e := NewTaskExecutor(1, WithTaskTimeout(5*time.Second))
	e.Start()
	defer e.Stop()

	err := e.Submit(context.Background(), "fast-server", func() error {
		return nil
	})
	require.NoError(t, err)
}
