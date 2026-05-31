package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopologicalSort_EmptyList(t *testing.T) {
	t.Parallel()
	result, err := topologicalSort(nil)
	require.NoError(t, err)
	require.Nil(t, result)

	result, err = topologicalSort([]Subtask{})
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestTopologicalSort_NoDeps(t *testing.T) {
	t.Parallel()
	tasks := []Subtask{
		{ID: "a", Task: "task a"},
		{ID: "b", Task: "task b"},
		{ID: "c", Task: "task c"},
	}
	result, err := topologicalSort(tasks)
	require.NoError(t, err)
	require.Len(t, result, 3)
	require.Equal(t, tasks, result, "no deps should return tasks in original order")
}

func TestTopologicalSort_LinearChain(t *testing.T) {
	t.Parallel()
	tasks := []Subtask{
		{ID: "c", Task: "task c", DependsOn: []string{"b"}},
		{ID: "a", Task: "task a"},
		{ID: "b", Task: "task b", DependsOn: []string{"a"}},
	}
	result, err := topologicalSort(tasks)
	require.NoError(t, err)
	require.Len(t, result, 3)

	ids := make([]string, len(result))
	for i, t := range result {
		ids[i] = t.ID
	}
	require.Equal(t, []string{"a", "b", "c"}, ids)
}

func TestTopologicalSort_DiamondDependency(t *testing.T) {
	t.Parallel()
	tasks := []Subtask{
		{ID: "d", Task: "task d", DependsOn: []string{"b", "c"}},
		{ID: "a", Task: "task a"},
		{ID: "b", Task: "task b", DependsOn: []string{"a"}},
		{ID: "c", Task: "task c", DependsOn: []string{"a"}},
	}
	result, err := topologicalSort(tasks)
	require.NoError(t, err)
	require.Len(t, result, 4)

	ids := make([]string, len(result))
	for i, t := range result {
		ids[i] = t.ID
	}

	require.Equal(t, "a", ids[0])
	require.ElementsMatch(t, []string{"b", "c"}, ids[1:3])
	require.Equal(t, "d", ids[3])
}

func TestTopologicalSort_CircularDependency(t *testing.T) {
	t.Parallel()
	tasks := []Subtask{
		{ID: "a", Task: "task a", DependsOn: []string{"c"}},
		{ID: "b", Task: "task b", DependsOn: []string{"a"}},
		{ID: "c", Task: "task c", DependsOn: []string{"b"}},
	}
	_, err := topologicalSort(tasks)
	require.Error(t, err)
	require.Contains(t, err.Error(), "circular dependency")
}

func TestTopologicalSort_SelfDependency(t *testing.T) {
	t.Parallel()
	tasks := []Subtask{
		{ID: "a", Task: "task a", DependsOn: []string{"a"}},
	}
	_, err := topologicalSort(tasks)
	require.Error(t, err)
	require.Contains(t, err.Error(), "circular dependency")
}

func TestTopologicalSort_UnknownDependency(t *testing.T) {
	t.Parallel()
	tasks := []Subtask{
		{ID: "a", Task: "task a", DependsOn: []string{"nonexistent"}},
	}
	_, err := topologicalSort(tasks)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown task")
	require.Contains(t, err.Error(), "nonexistent")
}

func TestTopologicalSort_DuplicateID(t *testing.T) {
	t.Parallel()
	tasks := []Subtask{
		{ID: "a", Task: "first a", DependsOn: []string{}},
		{ID: "a", Task: "second a", DependsOn: []string{}},
	}
	_, err := topologicalSort(tasks)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate subtask ID")
}

func TestTopologicalSort_BackwardCompatibleEmptyDependsOn(t *testing.T) {
	t.Parallel()
	tasks := []Subtask{
		{ID: "x", Task: "task x", DependsOn: nil},
		{ID: "y", Task: "task y", DependsOn: []string{}},
	}
	result, err := topologicalSort(tasks)
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.Equal(t, tasks, result, "nil/empty DependsOn should preserve order")
}

func TestTopologicalSort_ComplexDAG(t *testing.T) {
	t.Parallel()
	tasks := []Subtask{
		{ID: "f", Task: "f", DependsOn: []string{"d", "e"}},
		{ID: "d", Task: "d", DependsOn: []string{"b"}},
		{ID: "e", Task: "e", DependsOn: []string{"b", "c"}},
		{ID: "a", Task: "a"},
		{ID: "b", Task: "b", DependsOn: []string{"a"}},
		{ID: "c", Task: "c", DependsOn: []string{"a"}},
	}
	result, err := topologicalSort(tasks)
	require.NoError(t, err)
	require.Len(t, result, 6)

	positions := make(map[string]int)
	for i, t := range result {
		positions[t.ID] = i
	}

	require.Less(t, positions["a"], positions["b"], "a must come before b")
	require.Less(t, positions["a"], positions["c"], "a must come before c")
	require.Less(t, positions["b"], positions["d"], "b must come before d")
	require.Less(t, positions["b"], positions["e"], "b must come before e")
	require.Less(t, positions["c"], positions["e"], "c must come before e")
	require.Less(t, positions["d"], positions["f"], "d must come before f")
	require.Less(t, positions["e"], positions["f"], "e must come before f")
}

func TestOperator_DependsOnIntegrationWithOperator(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "c", Task: "task c depends on b", DependsOn: []string{"b"}},
			{ID: "a", Task: "task a has no deps"},
			{ID: "b", Task: "task b depends on a", DependsOn: []string{"a"}},
		},
	}

	rec := &xrushRecordingExecutor{
		responses: []StructuredResponse{
			{Success: true, Result: "done-a"},
			{Success: true, Result: "done-b"},
			{Success: true, Result: "done-c"},
		},
	}

	op := NewOperator(
		OperatorConfig{Strategy: StrategySequential, AutoSelect: new(false)},
		rec.exec,
		decomp,
	)
	result := op.Run(t.Context(), "run tasks with dependencies using sequential strategy", nil)

	require.True(t, result.Success)

	rec.callsMu.Lock()
	defer rec.callsMu.Unlock()
	require.Len(t, rec.calls, 3)
	require.Equal(t, "task a has no deps", rec.calls[0].Task)
	require.Equal(t, "task b depends on a", rec.calls[1].Task)
	require.Equal(t, "task c depends on b", rec.calls[2].Task)
}

func TestOperator_CircularDependencyReturnsError(t *testing.T) {
	t.Parallel()

	decomp := &xrushFixedDecomposer{
		subtasks: []Subtask{
			{ID: "a", Task: "task a circular", DependsOn: []string{"b"}},
			{ID: "b", Task: "task b circular", DependsOn: []string{"a"}},
		},
	}

	exec := xrushStubExecutor(StructuredResponse{Success: true, Result: "ok"})
	op := NewOperator(
		OperatorConfig{Strategy: StrategyLLMMap, AutoSelect: new(false)},
		exec,
		decomp,
	)
	result := op.Run(t.Context(), "run tasks with circular dependencies that should fail", nil)

	require.False(t, result.Success)
	require.Contains(t, result.Error, "dependency resolution failed")
	require.Contains(t, result.Error, "circular dependency")
}

func TestOperator_NoDependsOnBackwardCompat(t *testing.T) {
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

	op := NewOperator(
		OperatorConfig{Strategy: StrategyLLMMap, AutoSelect: new(false)},
		rec.exec,
		decomp,
	)
	result := op.Run(t.Context(), "run tasks without any dependencies for backward compatibility", nil)

	require.True(t, result.Success)
	require.Len(t, result.SubResults, 2)
}
