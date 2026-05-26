package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// xrushFixedDecomposer returns a fixed set of subtasks or an error.
type xrushFixedDecomposer struct {
	subtasks []Subtask
	err      error
	called   atomic.Int32
}

func (d *xrushFixedDecomposer) Decompose(_ context.Context, _ string, _ map[string]string) ([]Subtask, error) {
	d.called.Add(1)
	return d.subtasks, d.err
}

type xrushRecordingExecutor struct {
	responses []StructuredResponse
	calls     []StructuredRequest
	callsMu   sync.Mutex
	index     atomic.Int32
}

func (e *xrushRecordingExecutor) exec(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
	e.callsMu.Lock()
	e.calls = append(e.calls, req)
	e.callsMu.Unlock()
	idx := int(e.index.Add(1)) - 1
	if idx < len(e.responses) {
		return e.responses[idx], nil
	}
	return StructuredResponse{Success: true, Result: "default"}, nil
}

func xrushStubExecutor(responses ...StructuredResponse) SubagentExecutor {
	rec := &xrushRecordingExecutor{responses: responses}
	return rec.exec
}

func xrushPtrBool(b bool) *bool { return &b }

// Tests from operator_test.go

func TestXrushOperatorLLMMapRunsInParallel(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "subtask A"},
			{ID: "b", Task: "subtask B"},
			{ID: "c", Task: "subtask C"},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "result-A"},
			{Success: true, Result: "result-B"},
			{Success: true, Result: "result-C"},
		},
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyLLMMap, AutoSelect: xrushPtrBool(false)}, rec.exec, decomp)
	result := op.Run(t.Context(), "decompose this task into three parts", nil)

	require.True(t, result.Success)
	require.Contains(t, result.Result, "result-A")
	require.Contains(t, result.Result, "result-B")
	require.Contains(t, result.Result, "result-C")
	require.Len(t, result.SubResults, 3)
	require.Equal(t, int32(1), decomp.called.Load())

	for _, call := range rec.calls {
		require.Nil(t, call.Context, "LLM-Map should not pass context to subtasks")
	}
}

func TestXrushOperatorAgenticMapPassesContext(t *testing.T) {
	t.Parallel()

	ctx := map[string]string{"language": "go", "module": "auth"}
	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "refactor auth", Context: ctx},
			{ID: "b", Task: "write tests", Context: ctx},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "refactored"},
			{Success: true, Result: "tested"},
		},
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyAgenticMap}, rec.exec, decomp)
	result := op.Run(t.Context(), "refactor and test the auth module", ctx)

	require.True(t, result.Success)
	require.Len(t, result.SubResults, 2)

	for _, call := range rec.calls {
		require.Equal(t, "go", call.Context["language"])
		require.Equal(t, "auth", call.Context["module"])
	}
}

func TestXrushOperatorBatchSharesContext(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "step one", Context: map[string]string{"env": "test"}},
			{ID: "b", Task: "step two", Context: map[string]string{}},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "step one done"},
			{Success: true, Result: "step two done"},
		},
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyBatch}, rec.exec, decomp)
	result := op.Run(t.Context(), "run two batch steps", nil)

	require.True(t, result.Success)
	require.Len(t, result.SubResults, 2)

	require.Len(t, rec.calls, 2)
	secondCall := rec.calls[1]
	require.Equal(t, "test", secondCall.Context["env"], "batch should propagate shared context")
}

func TestXrushOperatorSequentialPipesOutput(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "generate list"},
			{ID: "b", Task: "process list"},
		},
	}

	callCount := 0
	exec := func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
		callCount++
		if callCount == 1 {
			return StructuredResponse{Success: true, Result: "item1,item2"}, nil
		}
		require.Contains(t, req.Context, "previous_output",
			"sequential strategy must pipe previous output")
		require.Equal(t, "item1,item2", req.Context["previous_output"])
		return StructuredResponse{Success: true, Result: "processed all"}, nil
	}

	op := NewOperator(OperatorConfig{Strategy: StrategySequential}, exec, decomp)
	result := op.Run(t.Context(), "generate then process a list", nil)

	require.True(t, result.Success)
	require.Len(t, result.SubResults, 2)
}

func TestXrushOperatorSequentialStopsOnFailure(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "step one"},
			{ID: "b", Task: "step two"},
			{ID: "c", Task: "step three"},
		},
	}

	callCount := 0
	exec := func(_ context.Context, _ StructuredRequest) (StructuredResponse, error) {
		callCount++
		if callCount == 2 {
			return StructuredResponse{Success: false, Error: "step two failed"}, nil
		}
		return StructuredResponse{Success: true, Result: "ok"}, nil
	}

	op := NewOperator(OperatorConfig{Strategy: StrategySequential}, exec, decomp)
	result := op.Run(t.Context(), "run three sequential steps that fail", nil)

	require.False(t, result.Success)
	require.Contains(t, result.Error, "subtask 1 failed")
	require.Len(t, result.SubResults, 2, "should stop after failure")
}

func TestXrushOperatorMaxDepthEnforcement(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{{ID: "a", Task: "recursive subtask"}},
	}

	exec := func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
		return StructuredResponse{Success: true, Result: "done: " + req.Task}, nil
	}

	op := NewOperator(OperatorConfig{MaxDepth: 1, Strategy: StrategyLLMMap}, exec, decomp)
	result := op.Run(t.Context(), "decompose this task into subtasks", nil)

	require.True(t, result.Success, "depth=1 should succeed at root level")
}

func TestXrushOperatorRecursionDepthExceeded(t *testing.T) {
	t.Parallel()

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "ok"})

	op := NewOperator(OperatorConfig{MaxDepth: 2, Strategy: StrategyLLMMap}, exec,
		&xrushFixedDecomposer{subtasks: []Subtask{{ID: "a", Task: "subtask"}}},
	)

	result := op.run(t.Context(), "decompose recursively at depth 3", nil, 3)
	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "max recursion depth")
}

func TestXrushOperatorCycleDetection(t *testing.T) {
	t.Parallel()

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "ok"})
	op := NewOperator(OperatorConfig{Strategy: StrategyLLMMap}, exec, nil)

	sig := taskSignature("same task", nil)
	op.visited.Store(sig, true)

	result := op.run(t.Context(), "same task", nil, 0)
	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "cycle detected")
}

func TestXrushOperatorEmptyTask(t *testing.T) {
	t.Parallel()

	exec := xrushStubExecutor()
	op := NewOperator(OperatorConfig{Strategy: StrategyLLMMap}, exec, nil)

	result := op.Run(t.Context(), "   ", nil)
	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "empty task")
}

func TestXrushOperatorSmallTaskExecutesDirectly(t *testing.T) {
	t.Parallel()

	called := false
	exec := func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
		called = true
		return StructuredResponse{Success: true, Result: "tiny result"}, nil
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyLLMMap}, exec, &xrushFixedDecomposer{})
	result := op.Run(t.Context(), "short", nil)

	require.True(t, result.Success)
	require.True(t, called, "small task should execute directly without decomposition")
	require.Equal(t, 0, result.Depth)
}

func TestXrushOperatorDecomposeReturnsEmpty(t *testing.T) {
	t.Parallel()

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "direct result"})
	decomp := &xrushFixedDecomposer{subtasks: nil}

	op := NewOperator(OperatorConfig{Strategy: StrategyLLMMap}, exec, decomp)
	result := op.Run(t.Context(), "decompose this task but get nothing", nil)

	require.True(t, result.Success)
	require.Equal(t, "direct result", result.Result)
}

func TestXrushOperatorDecomposeError(t *testing.T) {
	t.Parallel()

	exec := xrushStubExecutor()
	decomp := &xrushFixedDecomposer{err: fmt.Errorf("LLM unavailable")}

	op := NewOperator(OperatorConfig{Strategy: StrategyLLMMap}, exec, decomp)
	result := op.Run(t.Context(), "decompose this task but LLM fails", nil)

	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "decompose failed")
	require.Contains(t, result.Error, "LLM unavailable")
}

func TestXrushOperatorExecutorError(t *testing.T) {
	t.Parallel()

	exec := func(_ context.Context, _ StructuredRequest) (StructuredResponse, error) {
		return StructuredResponse{}, fmt.Errorf("executor crashed")
	}
	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{{ID: "a", Task: "subtask A"}},
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyLLMMap}, exec, decomp)
	result := op.Run(t.Context(), "decompose this task then executor fails", nil)

	require.NotEmpty(t, result.Error)
}

func TestXrushOperatorConfigDefaults(t *testing.T) {
	t.Parallel()

	cfg := OperatorConfig{}.withDefaults()
	require.Equal(t, defaultMaxDepth, cfg.MaxDepth)
	require.Equal(t, defaultMaxWorkers, cfg.MaxWorkers)
	require.NotNil(t, cfg.AutoSelect)
	require.True(t, *cfg.AutoSelect)
}

func TestXrushOperatorConfigPreservesCustomValues(t *testing.T) {
	t.Parallel()

	cfg := OperatorConfig{MaxDepth: 5, MaxWorkers: 8}.withDefaults()
	require.Equal(t, 5, cfg.MaxDepth)
	require.Equal(t, 8, cfg.MaxWorkers)
}

func TestXrushDecomposeStrategyString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "llm-map", StrategyLLMMap.String())
	require.Equal(t, "agentic-map", StrategyAgenticMap.String())
	require.Equal(t, "batch", StrategyBatch.String())
	require.Equal(t, "sequential", StrategySequential.String())
	require.Contains(t, DecomposeStrategy(99).String(), "unknown")
}

func TestXrushOperatorBatchStoresResultsInContext(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "step1", Task: "first step", Context: map[string]string{"k": "v"}},
			{ID: "step2", Task: "second step", Context: map[string]string{}},
		},
	}

	var calls []StructuredRequest
	exec := func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
		calls = append(calls, req)
		return StructuredResponse{Success: true, Result: fmt.Sprintf("result-%s", req.Task)}, nil
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyBatch}, exec, decomp)
	result := op.Run(t.Context(), "run two batch steps that share context", nil)

	require.True(t, result.Success)
	require.Len(t, calls, 2)

	secondCall := calls[1]
	require.Equal(t, "v", secondCall.Context["k"], "shared context should propagate")
	require.Contains(t, secondCall.Context["subtask_step1_result"], "result-first step",
		"batch should store previous results in shared context")
}

func TestXrushOperatorLLMMapPartialFailure(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "subtask A"},
			{ID: "b", Task: "subtask B"},
		},
	}

	exec := xrushStubExecutor(
		StructuredResponse{Success: true, Result: "A ok"},
		StructuredResponse{Success: false, Error: "B failed", Result: "B partial"},
	)

	op := NewOperator(OperatorConfig{Strategy: StrategyLLMMap}, exec, decomp)
	result := op.Run(t.Context(), "decompose with partial failure expected", nil)

	require.False(t, result.Success, "partial failure should mark overall result as failed")
	require.Contains(t, result.Result, "A ok")
}

func TestXrushTruncate(t *testing.T) {
	t.Parallel()
	require.Equal(t, "hello", truncate("hello", 10))
	require.Equal(t, "hello world ...", truncate("hello world this is long", 12))
}

func TestXrushTaskSignature(t *testing.T) {
	t.Parallel()

	sig1 := taskSignature("same task", map[string]string{"a": "1"})
	sig2 := taskSignature("same task", map[string]string{"a": "1"})
	require.Equal(t, sig1, sig2)

	sig3 := taskSignature("different task", map[string]string{"a": "1"})
	require.NotEqual(t, sig1, sig3)
}

func TestXrushOperatorVisitedIsResetPerRun(t *testing.T) {
	t.Parallel()

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "ok"})
	decomp := &xrushFixedDecomposer{}

	op1 := NewOperator(OperatorConfig{Strategy: StrategyLLMMap}, exec, decomp)
	r1 := op1.Run(t.Context(), "do the thing that is decomposable", nil)
	require.True(t, r1.Success)

	op2 := NewOperator(OperatorConfig{Strategy: StrategyLLMMap}, exec, decomp)
	r2 := op2.Run(t.Context(), "do the thing that is decomposable", nil)
	require.True(t, r2.Success, "fresh operator should not have stale visited state")
}

func TestXrushOperatorMaxWorkersLimitsConcurrency(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "subtask A"},
			{ID: "b", Task: "subtask B"},
			{ID: "c", Task: "subtask C"},
			{ID: "d", Task: "subtask D"},
		},
	}

	exec := xrushStubExecutor(
		StructuredResponse{Success: true, Result: "A"},
		StructuredResponse{Success: true, Result: "B"},
		StructuredResponse{Success: true, Result: "C"},
		StructuredResponse{Success: true, Result: "D"},
	)

	op := NewOperator(OperatorConfig{Strategy: StrategyLLMMap, MaxWorkers: 2}, exec, decomp)
	result := op.Run(t.Context(), "decompose into four subtasks with limited workers", nil)

	require.True(t, result.Success)
	require.Len(t, result.SubResults, 4)
}

func TestXrushOperatorDepth0RunsAtRoot(t *testing.T) {
	t.Parallel()

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "root result"})
	op := NewOperator(OperatorConfig{Strategy: StrategyLLMMap}, exec, nil)

	result := op.run(t.Context(), "short", nil, 0)
	require.True(t, result.Success)
	require.Equal(t, 0, result.Depth)
}

func TestXrushOperatorSequentialPreservesContext(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "first step", Context: map[string]string{"key": "val"}},
			{ID: "b", Task: "second step", Context: map[string]string{"other": "data"}},
		},
	}

	var calls []StructuredRequest
	exec := func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
		calls = append(calls, req)
		return StructuredResponse{Success: true, Result: fmt.Sprintf("done-%s", req.Task)}, nil
	}

	op := NewOperator(OperatorConfig{Strategy: StrategySequential}, exec, decomp)
	result := op.Run(t.Context(), "run two sequential steps with context", nil)

	require.True(t, result.Success)
	require.Len(t, calls, 2)

	require.Equal(t, "val", calls[0].Context["key"])
	require.Equal(t, "data", calls[1].Context["other"])
	require.Equal(t, "done-first step", calls[1].Context["previous_output"])
}

func TestXrushOperatorResultAggregationFormatsWell(t *testing.T) {
	t.Parallel()

	parts := []string{"alpha", "beta", "gamma"}
	joined := strings.Join(parts, "\n")
	require.Contains(t, joined, "alpha")
	require.Contains(t, joined, "beta")
	require.Contains(t, joined, "gamma")
	require.Equal(t, 3, strings.Count(joined, "\n")+1)
}

func TestXrushSelectStrategy_EmptySubtasks(t *testing.T) {
	t.Parallel()
	require.Equal(t, StrategyLLMMap, SelectStrategy(nil))
	require.Equal(t, StrategyLLMMap, SelectStrategy([]Subtask{}))
}

func TestXrushSelectStrategy_AllReadOnly(t *testing.T) {
	t.Parallel()
	subtasks := []Subtask{
		{ID: "1", Task: "read file", Tools: []string{"view"}},
		{ID: "2", Task: "search code", Tools: []string{"grep"}},
	}
	require.Equal(t, StrategyLLMMap, SelectStrategy(subtasks))
}

func TestXrushSelectStrategy_MixedReadWrite(t *testing.T) {
	t.Parallel()
	subtasks := []Subtask{
		{ID: "1", Task: "read file", Tools: []string{"view"}},
		{ID: "2", Task: "edit file", Tools: []string{"edit"}},
	}
	require.Equal(t, StrategyAgenticMap, SelectStrategy(subtasks))
}

func TestXrushSelectStrategy_SimilarBatch(t *testing.T) {
	t.Parallel()
	subtasks := []Subtask{
		{ID: "1", Task: "edit auth", Tools: []string{"edit", "view"}},
		{ID: "2", Task: "edit user", Tools: []string{"edit", "view"}},
		{ID: "3", Task: "edit session", Tools: []string{"edit", "view"}},
	}
	require.Equal(t, StrategyBatch, SelectStrategy(subtasks))
}

func TestXrushSelectStrategy_Sequential(t *testing.T) {
	t.Parallel()
	subtasks := []Subtask{
		{ID: "1", Task: "read config", Tools: []string{"view"}},
		{ID: "2", Task: "update config", Tools: []string{"edit"}, Context: map[string]string{"subtask_1_result": "data"}},
	}
	require.Equal(t, StrategySequential, SelectStrategy(subtasks))
}

func TestXrushSelectStrategy_PreviousOutputTriggersSequential(t *testing.T) {
	t.Parallel()
	subtasks := []Subtask{
		{ID: "1", Task: "step one", Tools: []string{"view"}},
		{ID: "2", Task: "step two", Tools: []string{"edit"}, Context: map[string]string{"previous_output": "step one result"}},
	}
	require.Equal(t, StrategySequential, SelectStrategy(subtasks))
}

func TestXrushSelectStrategy_UnknownTools(t *testing.T) {
	t.Parallel()
	subtasks := []Subtask{
		{ID: "1", Task: "do thing", Tools: []string{"unknown_tool"}},
	}
	require.Equal(t, StrategyAgenticMap, SelectStrategy(subtasks))
}

func TestXrushSelectStrategy_NoTools(t *testing.T) {
	t.Parallel()
	subtasks := []Subtask{
		{ID: "1", Task: "do thing", Tools: nil},
	}
	require.Equal(t, StrategyAgenticMap, SelectStrategy(subtasks))
}

func TestXrushSelectStrategy_SingleSubtask(t *testing.T) {
	t.Parallel()
	subtasks := []Subtask{
		{ID: "1", Task: "read config", Tools: []string{"view"}},
	}
	require.Equal(t, StrategyLLMMap, SelectStrategy(subtasks))
}

func TestXrushSelectStrategy_DiverseToolsAgenticMap(t *testing.T) {
	t.Parallel()
	subtasks := []Subtask{
		{ID: "1", Task: "edit auth", Tools: []string{"edit", "view"}},
		{ID: "2", Task: "run tests", Tools: []string{"bash"}},
	}
	require.Equal(t, StrategyAgenticMap, SelectStrategy(subtasks))
}

func TestXrushOperatorAutoSelectStrategy(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "read auth file", Tools: []string{"view"}},
			{ID: "b", Task: "search for patterns", Tools: []string{"grep"}},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "read-A"},
			{Success: true, Result: "read-B"},
		},
	}

	op := NewOperator(OperatorConfig{}, rec.exec, decomp)
	result := op.Run(t.Context(), "decompose this task with auto strategy selection", nil)

	require.True(t, result.Success)
	require.Equal(t, StrategyLLMMap, result.Strategy,
		"auto-select should pick LLMMap for read-only subtasks")
	for _, call := range rec.calls {
		require.Nil(t, call.Context, "LLMMap should not pass context to subtasks")
	}
}

func TestXrushOperatorAutoSelectAgenticMap(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "read auth file", Tools: []string{"view"}},
			{ID: "b", Task: "edit auth file", Tools: []string{"edit"}},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "read-A"},
			{Success: true, Result: "edited-B"},
		},
	}

	op := NewOperator(OperatorConfig{}, rec.exec, decomp)
	result := op.Run(t.Context(), "decompose this task with mixed read-write subtasks", nil)

	require.True(t, result.Success)
	require.Equal(t, StrategyAgenticMap, result.Strategy,
		"auto-select should pick AgenticMap for mixed read/write subtasks")
}

func TestXrushOperatorExplicitStrategyNotOverridden(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "read auth file", Tools: []string{"view"}},
			{ID: "b", Task: "search for patterns", Tools: []string{"grep"}},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "result-A"},
			{Success: true, Result: "result-B"},
		},
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyAgenticMap}, rec.exec, decomp)
	result := op.Run(t.Context(), "decompose this task with explicit agentic-map strategy", nil)

	require.True(t, result.Success)
	require.Equal(t, StrategyAgenticMap, result.Strategy,
		"explicitly set strategy must not be overridden by auto-select")
}

// ===========================================================================
// Tests from operator_integration_test.go
// (adapted: test operator directly instead of through coordinator options)
// ===========================================================================

func TestXrushOperatorIntegration_DecomposeUsingStrategy(t *testing.T) {
	t.Parallel()

	strategies := []DecomposeStrategy{
		StrategyLLMMap,
		StrategyAgenticMap,
		StrategyBatch,
		StrategySequential,
	}

	for _, strategy := range strategies {
		t.Run(strategy.String(), func(t *testing.T) {
			t.Parallel()

			decomp := &xrushFixedDecomposer{
				subtasks: []Subtask{
					{ID: "a", Task: "subtask alpha"},
					{ID: "b", Task: "subtask beta"},
				},
			}
			rec := &xrushRecordingExecutor{
				responses: []StructuredResponse{
					{Success: true, Result: "alpha-done"},
					{Success: true, Result: "beta-done"},
				},
			}

			cfg := OperatorConfig{Strategy: strategy}
			if strategy == StrategyLLMMap {
				cfg.AutoSelect = xrushPtrBool(false)
			}

			op := NewOperator(cfg, rec.exec, decomp)

			result := op.Run(t.Context(), "decompose this task into two parts for testing", nil)

			require.True(t, result.Success, "strategy %s should succeed", strategy)
			require.Len(t, result.SubResults, 2, "strategy %s should produce 2 sub-results", strategy)
			require.Equal(t, strategy, result.Strategy)

			rec.callsMu.Lock()
			require.Len(t, rec.calls, 2, "executor should be called once per subtask")
			rec.callsMu.Unlock()
		})
	}
}

func TestXrushOperatorIntegration_CycleDetection(t *testing.T) {
	t.Parallel()

	task := "repeated task that should trigger cycle detection mechanism"

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "ok"})
	op := NewOperator(OperatorConfig{Strategy: StrategyLLMMap}, exec, nil)

	sig := taskSignature(task, nil)
	op.visited.Store(sig, true)

	result := op.Run(t.Context(), task, nil)

	require.False(t, result.Success, "cycle detection should mark result as unsuccessful")
	require.Contains(t, result.Error, "cycle detected", "error should mention cycle detection")
	require.Contains(t, result.Error, "already visited", "error should mention already visited")
}

func TestXrushOperatorIntegration_MaxDepthRespected(t *testing.T) {
	t.Parallel()

	defaultCfg := OperatorConfig{}.withDefaults()
	require.Equal(t, defaultMaxDepth, defaultCfg.MaxDepth)
	require.Equal(t, 3, defaultCfg.MaxDepth, "default MaxDepth should be 3")

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "ok"})
	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{{ID: "a", Task: "recursive subtask"}},
	}

	op := NewOperator(
		OperatorConfig{MaxDepth: 1, Strategy: StrategyLLMMap},
		exec,
		decomp,
	)

	rootResult := op.Run(t.Context(), "root level task that needs decomposition", nil)
	require.True(t, rootResult.Success, "root level (depth 0) should succeed with MaxDepth=1")

	deepResult := op.run(t.Context(), "deep task", nil, 2)
	require.NotEmpty(t, deepResult.Error, "exceeding max depth should produce an error")
	require.Contains(t, deepResult.Error, "max recursion depth", "error should mention max recursion depth")
}

func TestXrushOperatorIntegration_ResultAggregation(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "research the codebase"},
			{ID: "b", Task: "implement the feature"},
			{ID: "c", Task: "write tests for the feature"},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "research-complete"},
			{Success: true, Result: "implementation-complete"},
			{Success: true, Result: "tests-complete"},
		},
	}

	op := NewOperator(
		OperatorConfig{Strategy: StrategyLLMMap, AutoSelect: xrushPtrBool(false)},
		rec.exec,
		decomp,
	)

	result := op.Run(t.Context(), "research implement and test a new feature please", nil)

	require.True(t, result.Success, "all subtasks succeeded, overall result should be successful")
	require.Len(t, result.SubResults, 3)

	require.Contains(t, result.Result, "research-complete")
	require.Contains(t, result.Result, "implementation-complete")
	require.Contains(t, result.Result, "tests-complete")

	rec.callsMu.Lock()
	require.Len(t, rec.calls, 3)
	rec.callsMu.Unlock()
}

func TestXrushOperatorIntegration_PartialFailureAggregation(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "succeeding subtask"},
			{ID: "b", Task: "failing subtask"},
			{ID: "c", Task: "another succeeding subtask"},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "ok-a"},
			{Success: false, Error: "subtask B exploded", Result: "partial-b"},
			{Success: true, Result: "ok-c"},
		},
	}

	op := NewOperator(
		OperatorConfig{Strategy: StrategyLLMMap, AutoSelect: xrushPtrBool(false)},
		rec.exec,
		decomp,
	)

	result := op.Run(t.Context(), "run three subtasks where one fails", nil)

	require.False(t, result.Success, "partial failure should mark overall result as unsuccessful")
	require.Contains(t, result.Result, "ok-a", "successful results should still appear in aggregation")
	require.Contains(t, result.Result, "ok-c", "successful results should still appear in aggregation")
	require.Contains(t, result.Error, "subtask B exploded", "failure message should be in error field")
}

func TestXrushE2EDecomposition_SequentialStrategy(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "s1", Task: "first step in the sequential pipeline"},
			{ID: "s2", Task: "second step in the sequential pipeline"},
			{ID: "s3", Task: "third step in the sequential pipeline"},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "step-1-output"},
			{Success: true, Result: "step-2-output"},
			{Success: true, Result: "step-3-final"},
		},
	}

	op := NewOperator(
		OperatorConfig{Strategy: StrategySequential},
		rec.exec,
		decomp,
	)

	result := op.Run(t.Context(), "run a three-step sequential pipeline from start to finish", nil)

	require.True(t, result.Success, "sequential pipeline should succeed")
	require.Len(t, result.SubResults, 3, "all three steps should produce sub-results")
	require.Contains(t, result.Result, "step-1-output")
	require.Contains(t, result.Result, "step-2-output")
	require.Contains(t, result.Result, "step-3-final")

	rec.callsMu.Lock()
	require.Len(t, rec.calls, 3)
	require.Equal(t, "first step in the sequential pipeline", rec.calls[0].Task)
	require.Equal(t, "second step in the sequential pipeline", rec.calls[1].Task)
	require.Equal(t, "third step in the sequential pipeline", rec.calls[2].Task)
	require.Equal(t, "step-1-output", rec.calls[1].Context["previous_output"])
	require.Equal(t, "step-2-output", rec.calls[2].Context["previous_output"])
	rec.callsMu.Unlock()
}

func TestXrushE2EDecomposition_ParallelStrategy(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "p1", Task: "parallel subtask one for concurrency test"},
			{ID: "p2", Task: "parallel subtask two for concurrency test"},
			{ID: "p3", Task: "parallel subtask three for concurrency test"},
			{ID: "p4", Task: "parallel subtask four for concurrency test"},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "parallel-result-1"},
			{Success: true, Result: "parallel-result-2"},
			{Success: true, Result: "parallel-result-3"},
			{Success: true, Result: "parallel-result-4"},
		},
	}

	op := NewOperator(
		OperatorConfig{Strategy: StrategyLLMMap, AutoSelect: xrushPtrBool(false)},
		rec.exec,
		decomp,
	)

	result := op.Run(t.Context(), "decompose into four parallel subtasks that run concurrently", nil)

	require.True(t, result.Success, "parallel strategy should succeed")
	require.Len(t, result.SubResults, 4, "all four subtask results should be collected")
	require.Contains(t, result.Result, "parallel-result-1")
	require.Contains(t, result.Result, "parallel-result-2")
	require.Contains(t, result.Result, "parallel-result-3")
	require.Contains(t, result.Result, "parallel-result-4")
	require.Equal(t, StrategyLLMMap, result.Strategy)

	rec.callsMu.Lock()
	for _, call := range rec.calls {
		require.Nil(t, call.Context, "LLM-Map should not pass context to subtasks")
	}
	rec.callsMu.Unlock()
}

func TestXrushE2EDecomposition_BatchStrategy(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "b1", Task: "first batch step", Context: map[string]string{"env": "production"}},
			{ID: "b2", Task: "second batch step", Context: map[string]string{}},
			{ID: "b3", Task: "third batch step", Context: map[string]string{}},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "batch-1-done"},
			{Success: true, Result: "batch-2-done"},
			{Success: true, Result: "batch-3-done"},
		},
	}

	op := NewOperator(
		OperatorConfig{Strategy: StrategyBatch},
		rec.exec,
		decomp,
	)

	result := op.Run(t.Context(), "run three batch steps sharing context between them", nil)

	require.True(t, result.Success, "batch strategy should succeed")
	require.Len(t, result.SubResults, 3)
	require.Contains(t, result.Result, "batch-1-done")
	require.Contains(t, result.Result, "batch-2-done")
	require.Contains(t, result.Result, "batch-3-done")
	require.Equal(t, StrategyBatch, result.Strategy)

	rec.callsMu.Lock()
	require.Len(t, rec.calls, 3)
	require.Equal(t, "production", rec.calls[1].Context["env"])
	require.Contains(t, rec.calls[1].Context["subtask_b1_result"], "batch-1-done")
	require.Contains(t, rec.calls[2].Context["subtask_b2_result"], "batch-2-done")
	rec.callsMu.Unlock()
}

func TestXrushE2EDecomposition_LLMMapStrategy(t *testing.T) {
	t.Parallel()

	readOnlySubtasks := []Subtask{
		{ID: "r1", Task: "search the codebase for auth patterns", Tools: []string{"grep"}},
		{ID: "r2", Task: "list all files in the project structure", Tools: []string{"glob"}},
	}

	selected := SelectStrategy(readOnlySubtasks)
	require.Equal(t, StrategyLLMMap, selected, "read-only subtasks should select LLM-Map strategy")

	decomp := &xrushFixedDecomposer{subtasks: readOnlySubtasks}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "found-3-patterns"},
			{Success: true, Result: "listed-42-files"},
		},
	}

	op := NewOperator(
		OperatorConfig{Strategy: selected},
		rec.exec,
		decomp,
	)

	result := op.Run(t.Context(), "search the codebase and list all project files for review", nil)

	require.True(t, result.Success)
	require.Len(t, result.SubResults, 2)
	require.Contains(t, result.Result, "found-3-patterns")
	require.Contains(t, result.Result, "listed-42-files")
	require.Equal(t, StrategyLLMMap, result.Strategy)
}

func TestXrushE2EDecomposition_DepthLimiting(t *testing.T) {
	t.Parallel()

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "executed"})

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{{ID: "d1", Task: "a recursive subtask at root depth"}},
	}

	op := NewOperator(
		OperatorConfig{MaxDepth: DefaultMaxRecursionDepth, Strategy: StrategyLLMMap},
		exec,
		decomp,
	)

	rootResult := op.run(t.Context(), "root level task needing decomposition", nil, 0)
	require.True(t, rootResult.Success, "depth 0 should succeed")

	atLimit := op.run(t.Context(), "task at the recursion depth limit boundary", nil, DefaultMaxRecursionDepth)
	require.True(t, atLimit.Success, "depth == maxDepth should still succeed")

	overLimit := op.run(t.Context(), "task exceeding the recursion depth limit", nil, DefaultMaxRecursionDepth+1)
	require.False(t, overLimit.Success, "depth > maxDepth should fail")
	require.Contains(t, overLimit.Error, "max recursion depth")
}

func TestXrushE2EDecomposition_CycleDetection(t *testing.T) {
	t.Parallel()

	taskText := "a task that would cause infinite recursion without cycle detection"

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "executed"})

	op := NewOperator(
		OperatorConfig{Strategy: StrategyLLMMap, MaxDepth: 5},
		exec,
		&xrushFixedDecomposer{},
	)

	sig := taskSignature(taskText, nil)
	op.visited.Store(sig, true)

	result := op.Run(t.Context(), taskText, nil)

	require.False(t, result.Success, "cycle-detected task should fail")
	require.Contains(t, result.Error, "cycle detected")
	require.Contains(t, result.Error, "already visited")
	require.Equal(t, StrategyLLMMap, result.Strategy)
}
