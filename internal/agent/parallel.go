package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"golang.org/x/sync/errgroup"
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
// up to MaxConcurrent. On first task error, the errgroup context is
// cancelled so remaining in-flight goroutines shut down gracefully.
//
// When a Mailbox is configured via SetMailbox, branches that return errors
// broadcast a sibling_error notification to all other registered branches.
// Siblings receive the notification cooperatively — they are not forced to
// abort.
type ParallelController struct {
	cfg        ParallelControllerConfig
	sem        chan struct{}
	focusMu    sync.Mutex
	focusChans map[string]chan struct{}

	eg     *errgroup.Group
	ctx    context.Context
	cancel context.CancelFunc
	closed atomic.Bool
	usage  *ResourceUsage

	branchTracker *BranchTracker

	mailbox     *Mailbox
	branchNames sync.Map
}

func NewParallelController(cfg ParallelControllerConfig) *ParallelController {
	max := cfg.maxConcurrent()
	ctx, cancel := context.WithCancel(context.Background())
	eg, egCtx := errgroup.WithContext(ctx)
	pc := &ParallelController{
		cfg:        cfg,
		sem:        make(chan struct{}, max),
		focusChans: make(map[string]chan struct{}),
		eg:         eg,
		ctx:        egCtx,
		cancel:     cancel,
	}
	if cfg.Limits != nil {
		pc.usage = NewResourceUsage()
	}
	return pc
}

// SetBranchTracker enables per-branch loop detection. When set, each task
// submitted via Submit receives its own BranchLoopDetector injected into the
// task context.
func (pc *ParallelController) SetBranchTracker(tracker *BranchTracker) {
	pc.branchTracker = tracker
}

// SetMailbox enables sibling error propagation. When a branch fails, it
// broadcasts a sibling_error notification to all other registered branches.
// Siblings receive the notification but are not forced to abort.
func (pc *ParallelController) SetMailbox(m *Mailbox) {
	pc.mailbox = m
}

// Submit enqueues a task for execution. It returns a Future that resolves
// when the task completes. The focusArea parameter determines serialization:
// tasks with the same non-empty focus area run sequentially.
func (pc *ParallelController) Submit(ctx context.Context, task Task, focusArea string) (*Future, error) {
	return pc.SubmitWithName(ctx, task, focusArea, "")
}

// SubmitWithName enqueues a task for execution with an optional branch name.
// When a branch name is provided and a Mailbox is configured, a failing branch
// broadcasts a sibling_error notification to all other named branches.
func (pc *ParallelController) SubmitWithName(ctx context.Context, task Task, focusArea, branchName string) (*Future, error) {
	if pc.closed.Load() {
		return nil, fmt.Errorf("parallel controller is shut down")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("submit cancelled: %w", err)
	}
	if err := pc.ctx.Err(); err != nil {
		return nil, fmt.Errorf("submit cancelled: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task must not be nil")
	}

	fut := newFuture()
	taskCtx := mergeContexts(pc.ctx, ctx)

	var det *BranchLoopDetector
	if pc.branchTracker != nil {
		det = pc.branchTracker.Register()
		taskCtx = ContextWithBranchDetector(taskCtx, det)
	}

	if branchName != "" && pc.mailbox != nil {
		pc.branchNames.Store(branchName, struct{}{})
	}

	pc.eg.Go(func() error {
		if det != nil {
			defer pc.branchTracker.Remove(det.BranchID())
		}
		if branchName != "" {
			defer pc.branchNames.Delete(branchName)
		}
		result := pc.executeWithFocusChain(taskCtx, task, focusArea)
		fut.complete(result)
		if result.Err != nil {
			pc.notifySiblingErrors(branchName, result.Err)
			return result.Err
		}
		return nil
	})

	return fut, nil
}

// notifySiblingErrors broadcasts a sibling_error notification to all other
// registered branches when a branch fails. Errors during broadcast are logged
// but do not affect the failing branch's error propagation.
func (pc *ParallelController) notifySiblingErrors(failedBranch string, err error) {
	if pc.mailbox == nil || failedBranch == "" {
		return
	}
	msg := MailboxMessage{
		From:      failedBranch,
		Content:   fmt.Sprintf("sibling branch %q failed: %s", failedBranch, err.Error()),
		Type:      tools.MailboxMessageSiblingError,
		Timestamp: time.Now(),
	}
	_ = pc.mailbox.Broadcast(msg, failedBranch)
}

// WaitAll blocks until all submitted tasks have completed, the provided
// context is cancelled, or a task returns an error. When a task error
// triggers errgroup cancellation, WaitAll returns that error.
func (pc *ParallelController) WaitAll(ctx context.Context) ([]Result, error) {
	done := make(chan error, 1)
	go func() {
		done <- pc.eg.Wait()
	}()

	select {
	case err := <-done:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("wait cancelled: %w", ctx.Err())
	}
}

// Shutdown cancels all running tasks and prevents new submissions. It blocks
// until all in-flight tasks have finished.
func (pc *ParallelController) Shutdown() {
	pc.closed.CompareAndSwap(false, true)
	pc.cancel()
	_ = pc.eg.Wait()
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

// mergeContexts returns a context that is cancelled when either parent is
// cancelled. This lets tasks respect both the errgroup cancellation and
// the caller's individual context.
func mergeContexts(a, b context.Context) context.Context {
	ctx, cancel := context.WithCancel(a)
	go func() {
		select {
		case <-a.Done():
			cancel()
		case <-b.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx
}
