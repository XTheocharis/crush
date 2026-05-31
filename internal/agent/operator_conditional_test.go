package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOperator_Conditional_SimpleTaskExecutesDirectly(t *testing.T) {
	t.Parallel()

	execCalled := false
	exec := func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
		execCalled = true
		return StructuredResponse{Success: true, Result: "done: " + req.Task}, nil
	}

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{{ID: "a", Task: "should not be called"}},
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyConditional, AutoSelect: new(false)}, exec, decomp)
	result := op.Run(t.Context(), "fix the typo in the readme file", nil)

	require.True(t, result.Success)
	require.True(t, execCalled, "simple task should execute directly")
	require.Equal(t, StrategyConditional, result.Strategy)
	require.Equal(t, 0, result.Depth)
	require.Equal(t, int32(0), decomp.called.Load(), "decomposer should not be called for simple tasks")
}

func TestOperator_Conditional_ComplexTaskByKeywordsIsDecomposed(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "plan the refactoring"},
			{ID: "b", Task: "execute the refactoring"},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "planned"},
			{Success: true, Result: "executed"},
		},
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyConditional, AutoSelect: new(false)}, rec.exec, decomp)
	result := op.Run(t.Context(), "refactor the authentication module to use JWT tokens", nil)

	require.True(t, result.Success)
	require.Equal(t, StrategyConditional, result.Strategy)
	require.Equal(t, int32(1), decomp.called.Load(), "decomposer should be called for complex tasks")
	require.Len(t, result.SubResults, 2)
}

func TestOperator_Conditional_ComplexTaskByLengthIsDecomposed(t *testing.T) {
	t.Parallel()

	longTask := strings.Repeat("write a function that processes the data ", 60)
	require.GreaterOrEqual(t, len(longTask), conditionalCharThreshold, "task must exceed char threshold")

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "parse the data"},
			{ID: "b", Task: "transform the data"},
			{ID: "c", Task: "write the output"},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "parsed"},
			{Success: true, Result: "transformed"},
			{Success: true, Result: "written"},
		},
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyConditional, AutoSelect: new(false)}, rec.exec, decomp)
	result := op.Run(t.Context(), longTask, nil)

	require.True(t, result.Success)
	require.Equal(t, int32(1), decomp.called.Load(), "long task should be decomposed")
	require.Len(t, result.SubResults, 3)
}

func TestOperator_Conditional_ComplexTaskByContextFilesIsDecomposed(t *testing.T) {
	t.Parallel()

	task := "update the configuration values across these files"
	ctx := map[string]string{
		"file_1": "config.yaml",
		"file_2": "settings.json",
		"file_3": "defaults.toml",
	}

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "update yaml"},
			{ID: "b", Task: "update json"},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "yaml-updated"},
			{Success: true, Result: "json-updated"},
		},
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyConditional, AutoSelect: new(false)}, rec.exec, decomp)
	result := op.Run(t.Context(), task, ctx)

	require.True(t, result.Success)
	require.Equal(t, int32(1), decomp.called.Load(), "multi-file task should be decomposed")
}

func TestOperator_Conditional_SimpleWithSingleFileContext(t *testing.T) {
	t.Parallel()

	execCalled := false
	exec := func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
		execCalled = true
		return StructuredResponse{Success: true, Result: "fixed"}, nil
	}

	ctx := map[string]string{"file_1": "main.go"}

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{{ID: "a", Task: "should not be called"}},
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyConditional, AutoSelect: new(false)}, exec, decomp)
	result := op.Run(t.Context(), "fix the bug in the main function", ctx)

	require.True(t, result.Success)
	require.True(t, execCalled, "single-file task should execute directly")
	require.Equal(t, int32(0), decomp.called.Load())
}

func TestOperator_Conditional_AllArchitectureKeywords(t *testing.T) {
	t.Parallel()

	for _, kw := range architectureKeywords {
		t.Run(kw, func(t *testing.T) {
			t.Parallel()

			task := kw + " the entire authentication system to be more secure"

			decompCalled := int32(0)
			decomp := &testCallCountDecomposer{
				called:   &decompCalled,
				subtasks: []Subtask{{ID: "a", Task: "step one"}},
			}

			rec := &xrushRecordingExecutor{
				responses: []StructuredResponse{{Success: true, Result: "done"}},
			}

			op := NewOperator(OperatorConfig{Strategy: StrategyConditional, AutoSelect: new(false)}, rec.exec, decomp)
			result := op.Run(t.Context(), task, nil)

			require.True(t, result.Success, "keyword %q should trigger decomposition", kw)
			require.Equal(t, int32(1), decompCalled, "keyword %q should trigger decomposition", kw)
		})
	}
}

type testCallCountDecomposer struct {
	subtasks []Subtask
	err      error
	called   *int32
}

func (d *testCallCountDecomposer) Decompose(_ context.Context, _ string, _ map[string]string) ([]Subtask, error) {
	*d.called++
	return d.subtasks, d.err
}

func TestOperator_Conditional_MaxDepthRespected(t *testing.T) {
	t.Parallel()

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "ok"})
	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{{ID: "a", Task: "refactor the sub-module"}},
	}

	op := NewOperator(
		OperatorConfig{MaxDepth: 3, Strategy: StrategyConditional, AutoSelect: new(false)},
		exec,
		decomp,
	)

	result := op.run(t.Context(), "refactor the module at depth 4", nil, 4)
	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "max recursion depth")
}

func TestOperator_Conditional_ExecutorError(t *testing.T) {
	t.Parallel()

	exec := func(_ context.Context, _ StructuredRequest) (StructuredResponse, error) {
		return StructuredResponse{}, fmt.Errorf("executor crashed")
	}

	decomp := &xrushFixedDecomposer{}

	op := NewOperator(OperatorConfig{Strategy: StrategyConditional, AutoSelect: new(false)}, exec, decomp)
	result := op.Run(t.Context(), "fix the simple bug in the code", nil)

	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "executor crashed")
}

func TestOperator_Conditional_DecomposeError(t *testing.T) {
	t.Parallel()

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "ok"})
	decomp := &xrushFixedDecomposer{err: fmt.Errorf("decomposer unavailable")}

	op := NewOperator(OperatorConfig{Strategy: StrategyConditional, AutoSelect: new(false)}, exec, decomp)
	result := op.Run(t.Context(), "refactor the module to use new patterns", nil)

	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "decompose failed")
	require.Contains(t, result.Error, "decomposer unavailable")
}

func TestOperator_Conditional_DecomposeReturnsEmpty(t *testing.T) {
	t.Parallel()

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "direct result"})
	decomp := &xrushFixedDecomposer{subtasks: nil}

	op := NewOperator(OperatorConfig{Strategy: StrategyConditional, AutoSelect: new(false)}, exec, decomp)
	result := op.Run(t.Context(), "refactor the entire codebase from scratch", nil)

	require.True(t, result.Success)
	require.Equal(t, "direct result", result.Result)
}

func TestIsComplexTask(t *testing.T) {
	t.Parallel()

	t.Run("short task with no context is simple", func(t *testing.T) {
		t.Parallel()
		require.False(t, isComplexTask("fix typo", nil))
	})

	t.Run("long task is complex", func(t *testing.T) {
		t.Parallel()
		longTask := strings.Repeat("a", conditionalCharThreshold)
		require.True(t, isComplexTask(longTask, nil))
	})

	t.Run("task just under threshold is simple", func(t *testing.T) {
		t.Parallel()
		task := strings.Repeat("a", conditionalCharThreshold-1)
		require.False(t, isComplexTask(task, nil))
	})

	t.Run("task at threshold is complex", func(t *testing.T) {
		t.Parallel()
		task := strings.Repeat("a", conditionalCharThreshold)
		require.True(t, isComplexTask(task, nil))
	})

	t.Run("multi-file context is complex", func(t *testing.T) {
		t.Parallel()
		ctx := map[string]string{"f1": "a.go", "f2": "b.go"}
		require.True(t, isComplexTask("update files", ctx))
	})

	t.Run("single-file context is simple", func(t *testing.T) {
		t.Parallel()
		ctx := map[string]string{"f1": "a.go"}
		require.False(t, isComplexTask("update file", ctx))
	})

	t.Run("architecture keywords trigger complex", func(t *testing.T) {
		t.Parallel()
		for _, kw := range architectureKeywords {
			require.True(t, isComplexTask(kw+" the code", nil), "keyword %q should be complex", kw)
		}
	})

	t.Run("keywords are case-insensitive", func(t *testing.T) {
		t.Parallel()
		require.True(t, isComplexTask("REFACTOR the code", nil))
		require.True(t, isComplexTask("Refactor the code", nil))
	})
}

func TestDecomposeStrategy_ConditionalString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "conditional", StrategyConditional.String())
}

func TestOperator_Conditional_UsesSequentialForComplexSubtasks(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "step one of the migration"},
			{ID: "b", Task: "step two of the migration"},
		},
	}

	var calls []StructuredRequest
	exec := func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
		calls = append(calls, req)
		return StructuredResponse{Success: true, Result: fmt.Sprintf("done-%s", req.Task)}, nil
	}

	op := NewOperator(OperatorConfig{Strategy: StrategyConditional, AutoSelect: new(false)}, exec, decomp)
	result := op.Run(t.Context(), "migrate the database schema to the new format", nil)

	require.True(t, result.Success)
	require.Len(t, calls, 2)
	require.Equal(t, "done-step one of the migration", calls[1].Context["previous_output"],
		"conditional strategy should use sequential execution for decomposed subtasks")
}
