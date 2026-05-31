package agent

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

func TestXrushParallelBasicSubmit(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 2})
	defer pc.Shutdown()

	fut, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
		return "hello", nil
	}, "")
	require.NoError(t, err)

	r, err := fut.Await(context.Background())
	require.NoError(t, err)
	require.NoError(t, r.Err)
	require.Equal(t, "hello", r.Value)
}

func TestXrushParallelTaskError(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 2})
	defer pc.Shutdown()

	fut, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
		return nil, errors.New("boom")
	}, "")
	require.NoError(t, err)

	r, err := fut.Await(context.Background())
	require.NoError(t, err)
	require.EqualError(t, r.Err, "boom")
}

func TestXrushParallelNilTask(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 2})
	defer pc.Shutdown()

	_, err := pc.Submit(context.Background(), nil, "")
	require.Error(t, err)
}

func TestXrushParallelCancelledContext(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 2})
	defer pc.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pc.Submit(ctx, func(ctx context.Context) (any, error) {
		return nil, nil
	}, "")
	require.Error(t, err)
}

func TestXrushParallelSubmitAfterShutdown(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 2})
	pc.Shutdown()

	_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
		return nil, nil
	}, "")
	require.Error(t, err)
}

func TestXrushParallelMaxConcurrentLimit(t *testing.T) {
	t.Parallel()

	max := 3
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: max})
	defer pc.Shutdown()

	var running atomic.Int32
	var peak atomic.Int32

	var wg sync.WaitGroup
	futures := make([]*Future, 10)

	for i := range 10 {
		wg.Go(func() {
		})
		fut, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
			cur := running.Add(1)
			for {
				old := peak.Load()
				if cur <= old || peak.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			running.Add(-1)
			return nil, nil
		}, fmt.Sprintf("area-%d", i))
		require.NoError(t, err)
		futures[i] = fut
	}
	wg.Wait()

	for _, f := range futures {
		r, err := f.Await(context.Background())
		require.NoError(t, err)
		require.NoError(t, r.Err)
	}

	require.LessOrEqual(t, int(peak.Load()), max)
}

func TestXrushParallelFocusChainSerialization(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	defer pc.Shutdown()

	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	for i := range 5 {
		_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
			cur := concurrent.Add(1)
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			concurrent.Add(-1)
			return i, nil
		}, "file-a")
		require.NoError(t, err)
	}

	_, err := pc.WaitAll(context.Background())
	require.NoError(t, err)

	require.Equal(t, int32(1), maxConcurrent.Load())
}

func TestXrushParallelDifferentFocusConcurrent(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	defer pc.Shutdown()

	var started sync.WaitGroup
	var canProceed atomic.Bool

	started.Add(3)

	submit := func(focus string) *Future {
		fut, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
			started.Done()
			for !canProceed.Load() {
				time.Sleep(time.Millisecond)
			}
			return focus, nil
		}, focus)
		require.NoError(t, err)
		return fut
	}

	f1 := submit("area-a")
	f2 := submit("area-b")
	f3 := submit("area-c")

	started.Wait()
	canProceed.Store(true)

	for _, f := range []*Future{f1, f2, f3} {
		r, err := f.Await(context.Background())
		require.NoError(t, err)
		require.NoError(t, r.Err)
	}
}

func TestXrushParallelEmptyFocusFullyConcurrent(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	defer pc.Shutdown()

	var started sync.WaitGroup
	var canProceed atomic.Bool

	started.Add(3)

	for i := range 3 {
		_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
			started.Done()
			for !canProceed.Load() {
				time.Sleep(time.Millisecond)
			}
			return i, nil
		}, "")
		require.NoError(t, err)
	}

	started.Wait()
	canProceed.Store(true)
}

func TestXrushParallelWaitAll(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 3})
	defer pc.Shutdown()

	for i := range 5 {
		_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
			time.Sleep(10 * time.Millisecond)
			return i, nil
		}, "")
		require.NoError(t, err)
	}

	results, err := pc.WaitAll(context.Background())
	require.NoError(t, err)
	require.Nil(t, results)
}

func TestXrushParallelWaitAllCancelled(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 1})
	defer pc.Shutdown()

	_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
		time.Sleep(2 * time.Second)
		return nil, nil
	}, "")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = pc.WaitAll(ctx)
	require.Error(t, err)
}

func TestXrushParallelFocusChainMixedAreas(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 10})
	defer pc.Shutdown()

	var areaAConcurrent atomic.Int32
	var areaBConcurrent atomic.Int32
	var areaAMax atomic.Int32
	var areaBMax atomic.Int32

	trackMax := func(cur *atomic.Int32, max *atomic.Int32) {
		for {
			old := max.Load()
			if cur.Load() <= old || max.CompareAndSwap(old, cur.Load()) {
				break
			}
		}
	}

	submit := func(focus string) *Future {
		fut, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
			var concurrent, maxVal *atomic.Int32
			if focus == "area-a" {
				concurrent = &areaAConcurrent
				maxVal = &areaAMax
			} else {
				concurrent = &areaBConcurrent
				maxVal = &areaBMax
			}
			concurrent.Add(1)
			trackMax(concurrent, maxVal)
			time.Sleep(20 * time.Millisecond)
			concurrent.Add(-1)
			return focus, nil
		}, focus)
		require.NoError(t, err)
		return fut
	}

	futures := []*Future{
		submit("area-a"),
		submit("area-b"),
		submit("area-a"),
		submit("area-b"),
		submit("area-a"),
	}

	for _, f := range futures {
		r, err := f.Await(context.Background())
		require.NoError(t, err)
		require.NoError(t, r.Err)
	}

	require.Equal(t, int32(1), areaAMax.Load(), "area-a tasks should never run concurrently")
	require.Equal(t, int32(1), areaBMax.Load(), "area-b tasks should never run concurrently")
}

func TestXrushParallelPoolUnderLoad(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 3})
	defer pc.Shutdown()

	var completed atomic.Int32

	futures := make([]*Future, 50)
	for i := range 50 {
		fut, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
			time.Sleep(time.Millisecond)
			completed.Add(1)
			return i, nil
		}, "")
		require.NoError(t, err)
		futures[i] = fut
	}

	for _, f := range futures {
		r, err := f.Await(context.Background())
		require.NoError(t, err)
		require.NoError(t, r.Err)
	}

	require.Equal(t, int32(50), completed.Load())
}

func TestXrushParallelContextCancellation(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 1})
	defer pc.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())

	fut, err := pc.Submit(ctx, func(ctx context.Context) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, "")
	require.NoError(t, err)

	cancel()

	r, err := fut.Await(context.Background())
	require.NoError(t, err)
	require.Error(t, r.Err)
}

func TestXrushParallelDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := ParallelControllerConfig{}
	require.Equal(t, defaultMaxConcurrent, cfg.maxConcurrent())
	require.Nil(t, cfg.FocusAreaFn)
}

func TestXrushDefaultFocusArea(t *testing.T) {
	t.Parallel()
	require.Equal(t, "some/path", DefaultFocusArea("some/path"))
	require.Equal(t, "", DefaultFocusArea(""))
}

// Tests from parallel_resource_test.go

func TestXrushParallel_NoLimits(t *testing.T) {
	pc := NewParallelController(ParallelControllerConfig{
		MaxConcurrent: 2,
	})
	defer pc.Shutdown()

	require.Nil(t, pc.Usage())
}

func TestXrushParallel_WithLimits(t *testing.T) {
	limits := &SubagentLimits{
		MaxSteps:    ResourceLimit{Soft: 8, Hard: 10},
		MaxTokens:   ResourceLimit{Soft: 800, Hard: 1000},
		MaxDuration: 5 * time.Second,
	}
	pc := NewParallelController(ParallelControllerConfig{
		MaxConcurrent: 2,
		Limits:        limits,
	})
	defer pc.Shutdown()

	require.NotNil(t, pc.Usage())
}

func TestXrushParallel_StepTracking(t *testing.T) {
	limits := &SubagentLimits{
		MaxSteps:    ResourceLimit{Soft: 8, Hard: 10},
		MaxTokens:   ResourceLimit{Soft: 80000, Hard: 100000},
		MaxDuration: 5 * time.Second,
	}
	pc := NewParallelController(ParallelControllerConfig{
		MaxConcurrent: 2,
		Limits:        limits,
	})
	defer pc.Shutdown()

	var ran atomic.Int32
	task := func(ctx context.Context) (any, error) {
		ran.Add(1)
		return "result", nil
	}

	fut, err := pc.Submit(context.Background(), task, "")
	require.NoError(t, err)
	res, err := fut.Await(context.Background())
	require.NoError(t, err)
	require.NoError(t, res.Err)

	require.Equal(t, int32(1), pc.Usage().StepsTaken.Load())
	require.Equal(t, int32(1), ran.Load())
}

func TestXrushParallel_TokenTracking(t *testing.T) {
	limits := &SubagentLimits{
		MaxSteps:    ResourceLimit{Soft: 8, Hard: 10},
		MaxTokens:   ResourceLimit{Soft: 80000, Hard: 100000},
		MaxDuration: 5 * time.Second,
	}
	pc := NewParallelController(ParallelControllerConfig{
		MaxConcurrent: 2,
		Limits:        limits,
	})
	defer pc.Shutdown()

	bigResult := make([]byte, 1000)
	for i := range bigResult {
		bigResult[i] = 'a'
	}
	task := func(ctx context.Context) (any, error) {
		return string(bigResult), nil
	}

	fut, err := pc.Submit(context.Background(), task, "")
	require.NoError(t, err)
	res, err := fut.Await(context.Background())
	require.NoError(t, err)
	require.NoError(t, res.Err)

	require.True(t, pc.Usage().TokensUsed.Load() > 0, "expected tokens to be tracked")
}

func TestXrushParallel_MultipleTasksAggregate(t *testing.T) {
	limits := &SubagentLimits{
		MaxSteps:    ResourceLimit{Soft: 8, Hard: 10},
		MaxTokens:   ResourceLimit{Soft: 80000, Hard: 100000},
		MaxDuration: 5 * time.Second,
	}
	pc := NewParallelController(ParallelControllerConfig{
		MaxConcurrent: 2,
		Limits:        limits,
	})
	defer pc.Shutdown()

	task := func(ctx context.Context) (any, error) {
		return "ok", nil
	}

	for range 3 {
		fut, err := pc.Submit(context.Background(), task, "")
		require.NoError(t, err)
		res, err := fut.Await(context.Background())
		require.NoError(t, err)
		require.NoError(t, res.Err)
	}

	require.Equal(t, int32(3), pc.Usage().StepsTaken.Load())
}

func TestErrgroupFirstErrorCancelsRemaining(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	defer pc.Shutdown()

	var started sync.WaitGroup
	var cancelled atomic.Int32

	started.Add(3)

	fastErr := func(ctx context.Context) (any, error) {
		started.Done()
		time.Sleep(10 * time.Millisecond)
		return nil, errors.New("fast error")
	}

	slow := func(ctx context.Context) (any, error) {
		started.Done()
		<-ctx.Done()
		cancelled.Add(1)
		return nil, ctx.Err()
	}

	_, err := pc.Submit(context.Background(), slow, "")
	_, err2 := pc.Submit(context.Background(), fastErr, "")
	_, err3 := pc.Submit(context.Background(), slow, "")
	require.NoError(t, err)
	require.NoError(t, err2)
	require.NoError(t, err3)

	started.Wait()

	_, waitErr := pc.WaitAll(context.Background())
	require.Error(t, waitErr)
	require.Equal(t, "fast error", waitErr.Error())
	require.True(t, cancelled.Load() >= 1, "expected at least one slow task to be cancelled")
}

func TestErrgroupWaitAllReturnsFirstError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		taskErr string
		wantErr string
	}{
		{name: "single error", taskErr: "task failed", wantErr: "task failed"},
		{name: "empty message", taskErr: "", wantErr: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 3})
			defer pc.Shutdown()

			_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
				return nil, errors.New(tt.taskErr)
			}, "")
			require.NoError(t, err)

			_, waitErr := pc.WaitAll(context.Background())
			require.Error(t, waitErr)
			require.Equal(t, tt.wantErr, waitErr.Error())
		})
	}
}

func TestErrgroupCancellationPreservesConcurrency(t *testing.T) {
	t.Parallel()
	max := 3
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: max})
	defer pc.Shutdown()

	var running atomic.Int32
	var peak atomic.Int32

	futures := make([]*Future, 10)
	for i := range 10 {
		taskErr := ""
		if i == 3 {
			taskErr = "early error"
		}
		fut, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
			cur := running.Add(1)
			for {
				old := peak.Load()
				if cur <= old || peak.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			running.Add(-1)
			if taskErr != "" {
				return nil, errors.New(taskErr)
			}
			return i, nil
		}, "")
		require.NoError(t, err)
		futures[i] = fut
	}

	for _, f := range futures {
		r, err := f.Await(context.Background())
		require.NoError(t, err)
		if r.Err != nil {
			require.Contains(t, []string{"early error", "context canceled"}, r.Err.Error())
		}
	}

	require.LessOrEqual(t, int(peak.Load()), max)
}

func TestErrgroupSuccessfulExecutionStillWorks(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 3})
	defer pc.Shutdown()

	futures := make([]*Future, 5)
	for i := range 5 {
		fut, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
			return i * 10, nil
		}, "")
		require.NoError(t, err)
		futures[i] = fut
	}

	_, waitErr := pc.WaitAll(context.Background())
	require.NoError(t, waitErr)

	for i, f := range futures {
		r, err := f.Await(context.Background())
		require.NoError(t, err)
		require.NoError(t, r.Err)
		require.Equal(t, i*10, r.Value)
	}
}

func TestErrgroupSubmitBlockedAfterError(t *testing.T) {
	t.Parallel()
	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 3})
	defer pc.Shutdown()

	_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
		return nil, errors.New("fail")
	}, "")
	require.NoError(t, err)

	_, waitErr := pc.WaitAll(context.Background())
	require.Error(t, waitErr)

	_, submitErr := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
		return "ok", nil
	}, "")
	require.Error(t, submitErr)
}
