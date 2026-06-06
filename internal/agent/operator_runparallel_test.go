package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunParallelWithDependsOn(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "c", Task: "task c depends on b", DependsOn: []string{"b"}},
			{ID: "a", Task: "task a has no deps"},
			{ID: "b", Task: "task b depends on a", DependsOn: []string{"a"}},
		},
	}

	var mu sync.Mutex
	var order []string
	ready := make(chan struct{})

	exec := func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
		<-ready
		mu.Lock()
		order = append(order, req.Task)
		mu.Unlock()
		return StructuredResponse{Success: true, Result: "done: " + req.Task}, nil
	}

	op := NewOperator(
		OperatorConfig{Strategy: StrategyAgenticMap, AutoSelect: new(false)},
		exec,
		decomp,
	)

	done := make(chan OperatorResult, 1)
	go func() {
		done <- op.Run(t.Context(), "run a chain of dependent tasks in parallel mode", nil)
	}()
	time.Sleep(50 * time.Millisecond)
	close(ready)
	result := <-done

	require.True(t, result.Success)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, order, 3)
	require.Equal(t, "task a has no deps", order[0], "task A must complete first")
	require.Equal(t, "task b depends on a", order[1], "task B must complete after A")
	require.Equal(t, "task c depends on b", order[2], "task C must complete after B")
}

func TestRunParallelIndependentTasks(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "independent task alpha"},
			{ID: "b", Task: "independent task beta"},
			{ID: "c", Task: "independent task gamma"},
		},
	}

	var started atomic.Int32
	var maxConcurrent atomic.Int32
	var current atomic.Int32

	exec := func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
		started.Add(1)
		c := current.Add(1)
		for {
			old := maxConcurrent.Load()
			if c <= old || maxConcurrent.CompareAndSwap(old, c) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		current.Add(-1)
		return StructuredResponse{Success: true, Result: "done: " + req.Task}, nil
	}

	op := NewOperator(
		OperatorConfig{Strategy: StrategyAgenticMap, AutoSelect: new(false)},
		exec,
		decomp,
	)

	result := op.Run(t.Context(), "run three independent tasks concurrently", nil)

	require.True(t, result.Success)
	require.Equal(t, int32(3), started.Load(), "all three tasks should have started")
	require.GreaterOrEqual(t, maxConcurrent.Load(), int32(2),
		"independent tasks should run concurrently")
}

func TestRunParallelDiamondDependency(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "d", Task: "task d depends on b and c", DependsOn: []string{"b", "c"}},
			{ID: "a", Task: "task a root"},
			{ID: "b", Task: "task b depends on a", DependsOn: []string{"a"}},
			{ID: "c", Task: "task c depends on a", DependsOn: []string{"a"}},
		},
	}

	var mu sync.Mutex
	var order []string
	var levelTimes []time.Time
	startTime := time.Now()
	ready := make(chan struct{})

	exec := func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
		<-ready
		mu.Lock()
		order = append(order, req.Task)
		levelTimes = append(levelTimes, time.Now())
		mu.Unlock()
		return StructuredResponse{Success: true, Result: "done: " + req.Task}, nil
	}

	op := NewOperator(
		OperatorConfig{Strategy: StrategyAgenticMap, AutoSelect: new(false)},
		exec,
		decomp,
	)

	done := make(chan OperatorResult, 1)
	go func() {
		done <- op.Run(t.Context(), "run diamond dependency graph in parallel", nil)
	}()
	time.Sleep(50 * time.Millisecond)
	close(ready)
	result := <-done

	require.True(t, result.Success)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, order, 4)
	require.Equal(t, "task a root", order[0], "root task A must come first")

	bTime, cTime := levelTimes[1], levelTimes[2]
	dTime := levelTimes[3]
	require.True(t, bTime.Sub(startTime) >= 0)
	require.True(t, cTime.Sub(startTime) >= 0)
	require.True(t, dTime.After(bTime) || dTime.Equal(bTime),
		"D should start after or at same time as B and C")
	require.True(t, dTime.After(cTime) || dTime.Equal(cTime),
		"D should start after or at same time as B and C")
}

func TestComputeDepthLevels_NoDeps(t *testing.T) {
	t.Parallel()

	subtasks := []Subtask{
		{ID: "a", Task: "a"},
		{ID: "b", Task: "b"},
		{ID: "c", Task: "c"},
	}

	levels := computeDepthLevels(subtasks)
	require.Len(t, levels, 1, "all tasks with no deps should be at level 0")
	require.Len(t, levels[0], 3)
}

func TestComputeDepthLevels_LinearChain(t *testing.T) {
	t.Parallel()

	subtasks := []Subtask{
		{ID: "a", Task: "a"},
		{ID: "b", Task: "b", DependsOn: []string{"a"}},
		{ID: "c", Task: "c", DependsOn: []string{"b"}},
	}

	levels := computeDepthLevels(subtasks)
	require.Len(t, levels, 3, "A->B->C chain should have 3 depth levels")
	require.Len(t, levels[0], 1)
	require.Equal(t, 0, levels[0][0], "task A at index 0")
	require.Len(t, levels[1], 1)
	require.Equal(t, 1, levels[1][0], "task B at index 1")
	require.Len(t, levels[2], 1)
	require.Equal(t, 2, levels[2][0], "task C at index 2")
}

func TestComputeDepthLevels_Diamond(t *testing.T) {
	t.Parallel()

	subtasks := []Subtask{
		{ID: "a", Task: "a"},
		{ID: "b", Task: "b", DependsOn: []string{"a"}},
		{ID: "c", Task: "c", DependsOn: []string{"a"}},
		{ID: "d", Task: "d", DependsOn: []string{"b", "c"}},
	}

	levels := computeDepthLevels(subtasks)
	require.Len(t, levels, 3, "diamond should have 3 depth levels")
	require.Len(t, levels[0], 1, "level 0: task A only")
	require.Len(t, levels[1], 2, "level 1: tasks B and C in parallel")
	require.Len(t, levels[2], 1, "level 2: task D")
}
