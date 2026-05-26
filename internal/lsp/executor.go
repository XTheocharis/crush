package lsp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DefaultQueueDepth is the default buffer size for each server's task channel.
const DefaultQueueDepth = 100

// ErrQueueFull is returned when a task cannot be submitted because the
// server's queue is at capacity.
var ErrQueueFull = errors.New("lsp executor: queue full")

// ErrExecutorStopped is returned when a task is submitted after Stop has been
// called.
var ErrExecutorStopped = errors.New("lsp executor: stopped")

// ErrTimeout is returned when a task exceeds the configured per-task timeout.
var ErrTimeout = errors.New("lsp executor: task timed out")

// task wraps a function call with a channel for delivering the result back to
// the caller.
type task struct {
	fn  func() error
	res chan error
}

type executorOption struct {
	taskTimeout time.Duration
}

// Option is a functional option for configuring a TaskExecutor.
type Option func(*executorOption)

// WithTaskTimeout sets the maximum duration a single task may run before
// ErrTimeout is returned. A value of 0 disables per-task timeouts.
func WithTaskTimeout(d time.Duration) Option {
	return func(o *executorOption) {
		o.taskTimeout = d
	}
}

// serverQueue holds the per-server goroutine state.
type serverQueue struct {
	ch  chan task
	wg  sync.WaitGroup
	ctx context.Context // per-server cancel
}

// TaskExecutor serialises LSP operations per server so that each server
// processes at most one request at a time while different servers run
// concurrently. Each server gets its own goroutine reading from a buffered
// channel.
type TaskExecutor struct {
	queues sync.Map // map[string]*serverQueue

	mu   sync.Mutex
	done chan struct{} // closed when Stop completes

	depth       int // buffered channel size per server
	taskTimeout time.Duration
}

// NewTaskExecutor creates a TaskExecutor with the given per-server queue depth.
// If depth <= 0, DefaultQueueDepth is used. Optional functional options
// configure additional behaviour such as per-task timeouts.
func NewTaskExecutor(depth int, opts ...Option) *TaskExecutor {
	if depth <= 0 {
		depth = DefaultQueueDepth
	}
	cfg := executorOption{
		taskTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &TaskExecutor{
		done:        make(chan struct{}),
		depth:       depth,
		taskTimeout: cfg.taskTimeout,
	}
}

// Start initialises the executor's root context. It is safe to call Submit
// after Start returns.
func (e *TaskExecutor) Start() {
	// No-op: queues are created lazily on first Submit. Kept for API
	// forward-compatibility.
}

// Stop cancels the root context, waits for all per-server goroutines to drain,
// and then returns. After Stop, Submit returns ErrExecutorStopped.
func (e *TaskExecutor) Stop() {
	e.mu.Lock()
	select {
	case <-e.done:
		e.mu.Unlock()
		return
	default:
	}
	close(e.done)
	e.mu.Unlock()

	e.queues.Range(func(_, v any) bool {
		q := v.(*serverQueue)
		q.wg.Wait()
		return true
	})
}

// Submit enqueues fn for execution on the serialised queue for serverID.
// It blocks until fn has been executed (or the context / executor is
// cancelled), returning the error from fn.
//
// If the per-server queue is full, Submit returns ErrQueueFull immediately.
// If the executor has been stopped, Submit returns ErrExecutorStopped.
func (e *TaskExecutor) Submit(ctx context.Context, serverID string, fn func() error) error {
	// Fast-path: check if executor is stopped.
	select {
	case <-e.done:
		return ErrExecutorStopped
	default:
	}

	q := e.getOrCreateQueue(serverID)

	t := task{
		fn:  fn,
		res: make(chan error, 1),
	}

	// Non-blocking send to detect queue overflow.
	select {
	case q.ch <- t:
	case <-ctx.Done():
		return ctx.Err()
	case <-e.done:
		return ErrExecutorStopped
	default:
		return ErrQueueFull
	}

	// Wait for the result.
	select {
	case err := <-t.res:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-e.done:
		return ErrExecutorStopped
	}
}

// getOrCreateQueue returns the existing serverQueue for serverID, or lazily
// creates one.
func (e *TaskExecutor) getOrCreateQueue(serverID string) *serverQueue {
	if v, ok := e.queues.Load(serverID); ok {
		return v.(*serverQueue)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Double-check after acquiring the lock.
	if v, ok := e.queues.Load(serverID); ok {
		return v.(*serverQueue)
	}

	q := &serverQueue{
		ch:  make(chan task, e.depth),
		ctx: context.Background(),
	}
	q.wg.Add(1)
	go e.runQueue(q)

	e.queues.Store(serverID, q)
	return q
}

// runQueue drains tasks from a single server's channel.
func (e *TaskExecutor) runQueue(q *serverQueue) {
	defer q.wg.Done()
	for {
		select {
		case t := <-q.ch:
			if e.taskTimeout > 0 {
				resultCh := make(chan error, 1)
				go func() {
					resultCh <- t.fn()
				}()

				var err error
				select {
				case result := <-resultCh:
					err = result
				case <-time.After(e.taskTimeout):
					err = ErrTimeout
				}
				t.res <- err
			} else {
				t.res <- t.fn()
			}
		case <-e.done:
			return
		}
	}
}

// String implements fmt.Stringer for debugging.
func (e *TaskExecutor) String() string {
	count := 0
	e.queues.Range(func(_, _ any) bool {
		count++
		return true
	})
	return fmt.Sprintf("TaskExecutor(queues=%d, depth=%d)", count, e.depth)
}
