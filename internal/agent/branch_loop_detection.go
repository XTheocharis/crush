package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"charm.land/fantasy"
)

const (
	// branchLoopSoftThreshold is the number of repeated tool-call signatures
	// within the sliding window that triggers a soft (warning) escalation per
	// branch.
	branchLoopSoftThreshold = 3

	// branchLoopHardThreshold is the number of repeated tool-call signatures
	// that triggers a hard (stop) escalation per branch.
	branchLoopHardThreshold = 5

	// branchLoopWindowSize is the number of recent steps examined per branch.
	branchLoopWindowSize = 10
)

// BranchLoopLevel represents the severity of a detected branch loop.
type BranchLoopLevel int

const (
	// BranchLoopNone means no loop detected on this branch.
	BranchLoopNone BranchLoopLevel = iota
	// BranchLoopSoft means a likely loop — the branch should be warned.
	BranchLoopSoft
	// BranchLoopHard means a severe loop — the branch should be stopped.
	BranchLoopHard
)

func (l BranchLoopLevel) String() string {
	switch l {
	case BranchLoopNone:
		return "none"
	case BranchLoopSoft:
		return "soft"
	case BranchLoopHard:
		return "hard"
	default:
		return fmt.Sprintf("unknown(%d)", l)
	}
}

// BranchLoopEvent describes a loop detection event on a specific branch.
type BranchLoopEvent struct {
	BranchID    string
	Level       BranchLoopLevel
	RepeatCount int
	ToolName    string
	Message     string
}

// BranchLoopDetector tracks tool call patterns for a single parallel branch.
// Each parallel subagent gets its own instance. Thresholds: soft warning at 3
// repetitions, hard stop at 5.
type BranchLoopDetector struct {
	branchID   string
	windowSize int
	softLimit  int
	hardLimit  int
	mu         sync.Mutex
	steps      []fantasy.StepResult

	onEvent func(event BranchLoopEvent)
}

// BranchLoopDetectorConfig configures a per-branch loop detector.
type BranchLoopDetectorConfig struct {
	BranchID   string
	WindowSize int
	SoftLimit  int
	HardLimit  int
	OnEvent    func(event BranchLoopEvent)
}

// NewBranchLoopDetector creates a new per-branch loop detector. Defaults are
// used for any zero-valued config fields: window=10, soft=3, hard=5.
func NewBranchLoopDetector(cfg BranchLoopDetectorConfig) *BranchLoopDetector {
	if cfg.SoftLimit <= 0 {
		cfg.SoftLimit = branchLoopSoftThreshold
	}
	if cfg.HardLimit <= 0 {
		cfg.HardLimit = branchLoopHardThreshold
	}
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = branchLoopWindowSize
	}
	return &BranchLoopDetector{
		branchID:   cfg.BranchID,
		windowSize: cfg.WindowSize,
		softLimit:  cfg.SoftLimit,
		hardLimit:  cfg.HardLimit,
		onEvent:    cfg.OnEvent,
	}
}

// RecordStep records a step and checks for loops. If a loop is detected, the
// configured OnEvent callback is invoked and the event is returned. Otherwise
// a zero-level event is returned.
func (d *BranchLoopDetector) RecordStep(step fantasy.StepResult) BranchLoopEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.steps = append(d.steps, step)
	event := d.check()
	if event.Level != BranchLoopNone && d.onEvent != nil {
		d.onEvent(event)
	}
	return event
}

// Check checks for loops without recording a new step.
func (d *BranchLoopDetector) Check() BranchLoopEvent {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.check()
}

// Reset clears all recorded steps.
func (d *BranchLoopDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.steps = d.steps[:0]
}

// BranchID returns the branch identifier.
func (d *BranchLoopDetector) BranchID() string {
	return d.branchID
}

// Steps returns the number of recorded steps.
func (d *BranchLoopDetector) Steps() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.steps)
}

func (d *BranchLoopDetector) check() BranchLoopEvent {
	if len(d.steps) < d.windowSize {
		return BranchLoopEvent{BranchID: d.branchID, Level: BranchLoopNone}
	}

	window := d.steps[len(d.steps)-d.windowSize:]
	counts := make(map[string]*branchPatternInfo)

	for _, step := range window {
		sig := getToolInteractionSignature(step.Content)
		if sig == "" {
			continue
		}
		info, ok := counts[sig]
		if !ok {
			info = &branchPatternInfo{
				tool: firstToolName(step.Content),
			}
			counts[sig] = info
		}
		info.count++
	}

	// Find the signature with the highest repeat count.
	var best *branchPatternInfo
	for _, info := range counts {
		if best == nil || info.count > best.count {
			best = info
		}
	}

	if best == nil {
		return BranchLoopEvent{BranchID: d.branchID, Level: BranchLoopNone}
	}

	switch {
	case best.count >= d.hardLimit:
		return BranchLoopEvent{
			BranchID:    d.branchID,
			Level:       BranchLoopHard,
			RepeatCount: best.count,
			ToolName:    best.tool,
			Message: fmt.Sprintf(
				"Branch %q HARD LOOP: tool %q repeated %d times. Halting branch.",
				d.branchID, best.tool, best.count,
			),
		}
	case best.count >= d.softLimit:
		return BranchLoopEvent{
			BranchID:    d.branchID,
			Level:       BranchLoopSoft,
			RepeatCount: best.count,
			ToolName:    best.tool,
			Message: fmt.Sprintf(
				"Branch %q SOFT LOOP: tool %q repeated %d times. Consider changing approach.",
				d.branchID, best.tool, best.count,
			),
		}
	default:
		return BranchLoopEvent{BranchID: d.branchID, Level: BranchLoopNone}
	}
}

type branchPatternInfo struct {
	count int
	tool  string
}

// BranchTracker manages per-branch loop detectors for a ParallelController.
// It creates a BranchLoopDetector for each submitted task and routes events
// back to a central callback.
type BranchTracker struct {
	mu       sync.Mutex
	branches map[string]*BranchLoopDetector
	seq      atomic.Uint64
	onEvent  func(event BranchLoopEvent)
}

// NewBranchTracker creates a new branch tracker. The onEvent callback is
// invoked whenever any branch detects a loop.
func NewBranchTracker(onEvent func(event BranchLoopEvent)) *BranchTracker {
	if onEvent == nil {
		onEvent = func(BranchLoopEvent) {}
	}
	return &BranchTracker{
		branches: make(map[string]*BranchLoopDetector),
		onEvent:  onEvent,
	}
}

// Register creates and registers a new branch loop detector with a unique ID.
func (bt *BranchTracker) Register() *BranchLoopDetector {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	id := fmt.Sprintf("branch-%d", bt.seq.Add(1))
	det := NewBranchLoopDetector(BranchLoopDetectorConfig{
		BranchID: id,
		OnEvent:  bt.onEvent,
	})
	bt.branches[id] = det
	return det
}

// Get returns the branch detector for the given branch ID, or nil.
func (bt *BranchTracker) Get(branchID string) *BranchLoopDetector {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return bt.branches[branchID]
}

// Remove removes a branch detector. This should be called when the branch task
// completes.
func (bt *BranchTracker) Remove(branchID string) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	delete(bt.branches, branchID)
}

// ActiveBranches returns the number of currently tracked branches.
func (bt *BranchTracker) ActiveBranches() int {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return len(bt.branches)
}

// contextKey is an unexported type for branch detector context keys.
type contextKey struct{}

// ContextWithBranchDetector returns a context that carries the given branch
// loop detector.
func ContextWithBranchDetector(ctx context.Context, det *BranchLoopDetector) context.Context {
	return context.WithValue(ctx, contextKey{}, det)
}

// BranchDetectorFromContext extracts a BranchLoopDetector from the context, or
// returns nil if none is present.
func BranchDetectorFromContext(ctx context.Context) *BranchLoopDetector {
	det, _ := ctx.Value(contextKey{}).(*BranchLoopDetector)
	return det
}
