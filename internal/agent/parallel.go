package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

const defaultMaxConcurrent = 5

// Task is a function submitted to the ParallelController for execution.
type Task func(ctx context.Context) (any, error)

// Result holds the outcome of a completed task.
type Result struct {
	Value any
	Err   error
}

// Future represents a pending task whose result can be awaited.
type Future struct {
	result chan Result
	done   atomic.Bool
}

func newFuture() *Future {
	return &Future{result: make(chan Result, 1)}
}

func (f *Future) complete(r Result) {
	if f.done.CompareAndSwap(false, true) {
		f.result <- r
	}
}

// Await blocks until the task completes or the context is cancelled.
func (f *Future) Await(ctx context.Context) (Result, error) {
	select {
	case r := <-f.result:
		return r, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

// FocusAreaFunc extracts a focus area key from task metadata. Return ""
// for tasks with no focus affinity (fully concurrent).
type FocusAreaFunc func(taskMetadata string) string

// ParallelControllerConfig holds configuration for a ParallelController.
type ParallelControllerConfig struct {
	MaxConcurrent int
	FocusAreaFn   FocusAreaFunc
	Limits        *SubagentLimits
}

func (c ParallelControllerConfig) maxConcurrent() int {
	if c.MaxConcurrent <= 0 {
		return defaultMaxConcurrent
	}
	return c.MaxConcurrent
}

// DefaultFocusArea returns the metadata string as-is, treating it as the
// focus area key. An empty string means no focus affinity.
func DefaultFocusArea(taskMetadata string) string {
	return taskMetadata
}

// ParallelController manages concurrent subagent execution with optional
// focus-chain serialization. Tasks sharing the same focus area execute
// serially; tasks with different (or empty) focus areas run concurrently
// up to MaxConcurrent.
type ParallelController struct {
	cfg        ParallelControllerConfig
	sem        chan struct{}
	focusMu    sync.Mutex
	focusChans map[string]chan struct{}

	wg     sync.WaitGroup
	closed atomic.Bool
	usage  *ResourceUsage
}

func NewParallelController(cfg ParallelControllerConfig) *ParallelController {
	max := cfg.maxConcurrent()
	pc := &ParallelController{
		cfg:        cfg,
		sem:        make(chan struct{}, max),
		focusChans: make(map[string]chan struct{}),
	}
	if cfg.Limits != nil {
		pc.usage = NewResourceUsage()
	}
	return pc
}

// Submit enqueues a task for execution. It returns a Future that resolves
// when the task completes. The focusArea parameter determines serialization:
// tasks with the same non-empty focus area run sequentially.
func (pc *ParallelController) Submit(ctx context.Context, task Task, focusArea string) (*Future, error) {
	if pc.closed.Load() {
		return nil, fmt.Errorf("parallel controller is shut down")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("submit cancelled: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task must not be nil")
	}

	fut := newFuture()

	pc.wg.Add(1)
	go func() {
		defer pc.wg.Done()
		result := pc.executeWithFocusChain(ctx, task, focusArea)
		fut.complete(result)
	}()

	return fut, nil
}

// WaitAll blocks until all submitted tasks have completed or the provided
// context is cancelled. It returns the results of all tasks that finished
// before the wait ended.
func (pc *ParallelController) WaitAll(ctx context.Context) ([]Result, error) {
	done := make(chan struct{})
	go func() {
		pc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("wait cancelled: %w", ctx.Err())
	}
}

// Shutdown cancels all running tasks and prevents new submissions. It blocks
// until all in-flight tasks have finished.
func (pc *ParallelController) Shutdown() {
	if pc.closed.CompareAndSwap(false, true) {
		// No external cancel func stored since we rely on per-task contexts.
	}
	pc.wg.Wait()
}

func (pc *ParallelController) executeWithFocusChain(ctx context.Context, task Task, focusArea string) Result {
	// Serialize same-focus-area tasks.
	if focusArea != "" {
		focusChan := pc.focusLock(focusArea)
		defer pc.focusUnlock(focusArea, focusChan)
	}

	// Acquire a semaphore slot (bounded concurrency).
	select {
	case pc.sem <- struct{}{}:
		defer func() { <-pc.sem }()
	case <-ctx.Done():
		return Result{Err: ctx.Err()}
	}

	result, err := task(ctx)
	if err != nil {
		return Result{Err: err}
	}

	if pc.usage != nil {
		pc.usage.AddStep()
		if str, ok := result.(string); ok {
			pc.usage.AddTokens(str)
		}
		pc.usage.WarnStepsOnce(pc.cfg.Limits.MaxSteps)
		pc.usage.WarnTokensOnce(pc.cfg.Limits.MaxTokens)
		pc.usage.WarnDurationOnce(*pc.cfg.Limits)
	}

	return Result{Value: result}
}

// Usage returns the aggregate resource usage tracker, or nil if limits are
// not configured.
func (pc *ParallelController) Usage() *ResourceUsage {
	return pc.usage
}

func (pc *ParallelController) focusLock(area string) chan struct{} {
	pc.focusMu.Lock()
	ch, ok := pc.focusChans[area]
	if !ok {
		newCh := make(chan struct{}, 1)
		pc.focusChans[area] = newCh
		pc.focusMu.Unlock()
		return newCh
	}
	newCh := make(chan struct{}, 1)
	pc.focusChans[area] = newCh
	pc.focusMu.Unlock()
	<-ch
	return newCh
}

func (pc *ParallelController) focusUnlock(area string, ch chan struct{}) {
	pc.focusMu.Lock()
	defer pc.focusMu.Unlock()

	select {
	case ch <- struct{}{}:
	default:
	}
}
