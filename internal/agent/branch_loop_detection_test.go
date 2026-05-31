package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBranchLoopDetector_NoSteps(t *testing.T) {
	t.Parallel()

	det := NewBranchLoopDetector(BranchLoopDetectorConfig{BranchID: "b1"})
	event := det.Check()
	require.Equal(t, BranchLoopNone, event.Level)
	require.Equal(t, "b1", event.BranchID)
}

func TestBranchLoopDetector_FewerThanWindow(t *testing.T) {
	t.Parallel()

	det := NewBranchLoopDetector(BranchLoopDetectorConfig{BranchID: "b1"})
	for range 5 {
		event := det.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
		require.Equal(t, BranchLoopNone, event.Level)
	}
}

func TestBranchLoopDetector_SoftThreshold(t *testing.T) {
	t.Parallel()

	var events []BranchLoopEvent
	var mu sync.Mutex
	det := NewBranchLoopDetector(BranchLoopDetectorConfig{
		BranchID: "b1",
		OnEvent: func(e BranchLoopEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		},
	})

	for i := range 10 {
		if i < 3 {
			det.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
		} else {
			det.RecordStep(makeToolStep("edit", fmt.Sprintf(`{"file":"%d.go"}`, i), "ok"))
		}
	}

	mu.Lock()
	softEvents := filterBranchEvents(events, BranchLoopSoft)
	mu.Unlock()
	require.NotEmpty(t, softEvents, "expected at least one soft event")
	require.Equal(t, 3, softEvents[0].RepeatCount)
	require.Equal(t, "read", softEvents[0].ToolName)
}

func TestBranchLoopDetector_HardThreshold(t *testing.T) {
	t.Parallel()

	var events []BranchLoopEvent
	var mu sync.Mutex
	det := NewBranchLoopDetector(BranchLoopDetectorConfig{
		BranchID: "b1",
		OnEvent: func(e BranchLoopEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		},
	})

	for i := range 10 {
		if i < 5 {
			det.RecordStep(makeToolStep("write", `{"file":"x.go"}`, "done"))
		} else {
			det.RecordStep(makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "ok"))
		}
	}

	mu.Lock()
	hardEvents := filterBranchEvents(events, BranchLoopHard)
	mu.Unlock()
	require.NotEmpty(t, hardEvents, "expected at least one hard event")
	require.Equal(t, 5, hardEvents[0].RepeatCount)
	require.Equal(t, "write", hardEvents[0].ToolName)
}

func TestBranchLoopDetector_DifferentSignaturesNoLoop(t *testing.T) {
	t.Parallel()

	var eventCount atomic.Int32
	det := NewBranchLoopDetector(BranchLoopDetectorConfig{
		BranchID: "b1",
		OnEvent: func(e BranchLoopEvent) {
			eventCount.Add(1)
		},
	})

	for i := range 10 {
		det.RecordStep(makeToolStep("tool", fmt.Sprintf(`{"i":%d}`, i), fmt.Sprintf("result-%d", i)))
	}

	require.Equal(t, int32(0), eventCount.Load())
}

func TestBranchLoopDetector_IndependentBranches(t *testing.T) {
	t.Parallel()

	var events []BranchLoopEvent
	var mu sync.Mutex
	onEvent := func(e BranchLoopEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	det1 := NewBranchLoopDetector(BranchLoopDetectorConfig{BranchID: "branch-1", OnEvent: onEvent})
	det2 := NewBranchLoopDetector(BranchLoopDetectorConfig{BranchID: "branch-2", OnEvent: onEvent})

	for range 10 {
		det1.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
	}

	for i := range 10 {
		det2.RecordStep(makeToolStep("tool", fmt.Sprintf(`{"i":%d}`, i), fmt.Sprintf("result-%d", i)))
	}

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, events, "expected events from branch-1")
	for _, e := range events {
		require.Equal(t, "branch-1", e.BranchID, "only branch-1 should have events")
	}
}

func TestBranchLoopDetector_Reset(t *testing.T) {
	t.Parallel()

	det := NewBranchLoopDetector(BranchLoopDetectorConfig{BranchID: "b1"})

	for range 10 {
		det.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
	}

	event := det.Check()
	require.Equal(t, BranchLoopHard, event.Level)

	det.Reset()
	event = det.Check()
	require.Equal(t, BranchLoopNone, event.Level)
	require.Equal(t, 0, det.Steps())
}

func TestBranchLoopDetector_CustomThresholds(t *testing.T) {
	t.Parallel()

	var events []BranchLoopEvent
	var mu sync.Mutex
	det := NewBranchLoopDetector(BranchLoopDetectorConfig{
		BranchID:   "b1",
		WindowSize: 6,
		SoftLimit:  2,
		HardLimit:  4,
		OnEvent: func(e BranchLoopEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		},
	})

	for i := range 6 {
		if i < 4 {
			det.RecordStep(makeToolStep("bash", `{"cmd":"ls"}`, "output"))
		} else {
			det.RecordStep(makeToolStep("edit", fmt.Sprintf(`{"f":"%d"}`, i), "ok"))
		}
	}

	mu.Lock()
	hardEvents := filterBranchEvents(events, BranchLoopHard)
	mu.Unlock()
	require.NotEmpty(t, hardEvents)
	require.Equal(t, 4, hardEvents[0].RepeatCount)
}

func TestBranchLoopDetector_EmptyStepsSkipped(t *testing.T) {
	t.Parallel()

	det := NewBranchLoopDetector(BranchLoopDetectorConfig{BranchID: "b1"})

	for i := range 10 {
		if i < 2 {
			det.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
		} else {
			det.RecordStep(makeEmptyStep())
		}
	}

	event := det.Check()
	require.Equal(t, BranchLoopNone, event.Level)
}

func TestBranchLoopDetector_Defaults(t *testing.T) {
	t.Parallel()

	det := NewBranchLoopDetector(BranchLoopDetectorConfig{BranchID: "test"})
	require.Equal(t, "test", det.BranchID())
	require.Equal(t, branchLoopWindowSize, det.windowSize)
	require.Equal(t, branchLoopSoftThreshold, det.softLimit)
	require.Equal(t, branchLoopHardThreshold, det.hardLimit)
}

func TestBranchTracker_RegisterAndRemove(t *testing.T) {
	t.Parallel()

	tracker := NewBranchTracker(func(e BranchLoopEvent) {})
	require.Equal(t, 0, tracker.ActiveBranches())

	det1 := tracker.Register()
	require.Equal(t, "branch-1", det1.BranchID())
	require.Equal(t, 1, tracker.ActiveBranches())

	det2 := tracker.Register()
	require.Equal(t, "branch-2", det2.BranchID())
	require.Equal(t, 2, tracker.ActiveBranches())

	tracker.Remove("branch-1")
	require.Equal(t, 1, tracker.ActiveBranches())
	require.Nil(t, tracker.Get("branch-1"))
	require.NotNil(t, tracker.Get("branch-2"))
}

func TestBranchTracker_EventsFromMultipleBranches(t *testing.T) {
	t.Parallel()

	var collected []BranchLoopEvent
	var mu sync.Mutex
	tracker := NewBranchTracker(func(e BranchLoopEvent) {
		mu.Lock()
		collected = append(collected, e)
		mu.Unlock()
	})

	det1 := tracker.Register()
	det2 := tracker.Register()

	for range 10 {
		det1.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
	}

	for i := range 10 {
		det2.RecordStep(makeToolStep("tool", fmt.Sprintf(`{"i":%d}`, i), fmt.Sprintf("r-%d", i)))
	}

	mu.Lock()
	defer mu.Unlock()
	for _, e := range collected {
		require.Equal(t, "branch-1", e.BranchID)
		require.Equal(t, BranchLoopHard, e.Level)
	}
}

func TestContextWithBranchDetector(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	require.Nil(t, BranchDetectorFromContext(ctx))

	det := NewBranchLoopDetector(BranchLoopDetectorConfig{BranchID: "ctx-test"})
	ctx = ContextWithBranchDetector(ctx, det)
	retrieved := BranchDetectorFromContext(ctx)
	require.NotNil(t, retrieved)
	require.Equal(t, "ctx-test", retrieved.BranchID())
}

func TestParallelController_BranchTracking(t *testing.T) {
	t.Parallel()

	var events []BranchLoopEvent
	var mu sync.Mutex
	tracker := NewBranchTracker(func(e BranchLoopEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 2})
	pc.SetBranchTracker(tracker)

	_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
		det := BranchDetectorFromContext(ctx)
		if det == nil {
			return nil, fmt.Errorf("expected branch detector in context")
		}
		for range 10 {
			det.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
		}
		return "done", nil
	}, "")
	require.NoError(t, err)

	_, err = pc.WaitAll(context.Background())
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, events, "expected branch loop events")
	for _, e := range events {
		require.Equal(t, BranchLoopHard, e.Level)
	}
}

func TestParallelController_BranchTrackingCleanup(t *testing.T) {
	t.Parallel()

	tracker := NewBranchTracker(func(e BranchLoopEvent) {})
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 2})
	pc.SetBranchTracker(tracker)

	_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
		return "ok", nil
	}, "")
	require.NoError(t, err)

	_, err = pc.WaitAll(context.Background())
	require.NoError(t, err)

	require.Equal(t, 0, tracker.ActiveBranches())
}

func TestParallelController_NoBranchTracking(t *testing.T) {
	t.Parallel()

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 2})

	_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
		det := BranchDetectorFromContext(ctx)
		if det != nil {
			return nil, fmt.Errorf("expected nil branch detector without tracker")
		}
		return "ok", nil
	}, "")
	require.NoError(t, err)

	_, err = pc.WaitAll(context.Background())
	require.NoError(t, err)
}

func TestParallelController_BranchTrackingConcurrent(t *testing.T) {
	t.Parallel()

	var eventCount atomic.Int32
	tracker := NewBranchTracker(func(e BranchLoopEvent) {
		eventCount.Add(1)
	})

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 3})
	pc.SetBranchTracker(tracker)

	var wg sync.WaitGroup
	for i := range 3 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
				det := BranchDetectorFromContext(ctx)
				if det == nil {
					return nil, fmt.Errorf("branch %d: expected detector", idx)
				}
				for j := range 10 {
					det.RecordStep(makeToolStep("tool", fmt.Sprintf(`{"b":%d,"s":%d}`, idx, j), "ok"))
				}
				return idx, nil
			}, "")
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()

	_, err := pc.WaitAll(context.Background())
	require.NoError(t, err)

	require.Equal(t, int32(0), eventCount.Load(), "no loops expected with varied steps")
	require.Equal(t, 0, tracker.ActiveBranches(), "all branches should be cleaned up")
}

func TestBranchLoopLevel_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level    BranchLoopLevel
		expected string
	}{
		{BranchLoopNone, "none"},
		{BranchLoopSoft, "soft"},
		{BranchLoopHard, "hard"},
		{BranchLoopLevel(99), "unknown(99)"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.expected, tt.level.String())
	}
}

func TestBranchLoopDetector_RecordStepFiresCallback(t *testing.T) {
	t.Parallel()

	var fired atomic.Bool
	det := NewBranchLoopDetector(BranchLoopDetectorConfig{
		BranchID: "b1",
		OnEvent: func(e BranchLoopEvent) {
			fired.Store(true)
		},
	})

	for range 9 {
		det.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
	}
	require.False(t, fired.Load())

	det.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
	require.True(t, fired.Load())
}

func TestNewBranchTracker_NilCallback(t *testing.T) {
	t.Parallel()

	tracker := NewBranchTracker(nil)
	require.NotNil(t, tracker.onEvent, "nil callback should be replaced with no-op")

	det := tracker.Register()
	for range 10 {
		det.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
	}
}

func TestBranchLoopDetector_HardStopsAtHardNotBeyond(t *testing.T) {
	t.Parallel()

	var events []BranchLoopEvent
	var mu sync.Mutex
	det := NewBranchLoopDetector(BranchLoopDetectorConfig{
		BranchID: "b1",
		OnEvent: func(e BranchLoopEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		},
	})

	for range 10 {
		det.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
	}

	mu.Lock()
	defer mu.Unlock()
	hardEvents := filterBranchEvents(events, BranchLoopHard)
	require.NotEmpty(t, hardEvents)
	require.Equal(t, 10, hardEvents[len(hardEvents)-1].RepeatCount)
}

func TestParallelController_BranchTrackingCancelsOnHardLoop(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var hardEvent atomic.Bool
	tracker := NewBranchTracker(func(e BranchLoopEvent) {
		if e.Level == BranchLoopHard {
			hardEvent.Store(true)
		}
	})

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 2})
	pc.SetBranchTracker(tracker)

	_, err := pc.Submit(ctx, func(taskCtx context.Context) (any, error) {
		det := BranchDetectorFromContext(taskCtx)
		if det == nil {
			return nil, fmt.Errorf("expected detector")
		}
		for range 10 {
			event := det.RecordStep(makeToolStep("read", `{"file":"a.go"}`, "content"))
			if event.Level == BranchLoopHard {
				return nil, fmt.Errorf("hard loop: %s", event.Message)
			}
		}
		return "ok", nil
	}, "")
	require.NoError(t, err)

	_, _ = pc.WaitAll(ctx)
	require.True(t, hardEvent.Load(), "expected hard loop event")
}

func filterBranchEvents(events []BranchLoopEvent, level BranchLoopLevel) []BranchLoopEvent {
	var filtered []BranchLoopEvent
	for _, e := range events {
		if e.Level == level {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
