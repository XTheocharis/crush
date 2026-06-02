package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOperatorWiredInArchitectEditor(t *testing.T) {
	t.Parallel()

	var execCount atomic.Int32
	factory := &stubOperatorFactory{
		execFn: func(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
			execCount.Add(1)
			return StructuredResponse{Success: true, Result: "done: " + req.Task}, nil
		},
	}

	plan := ArchitectPlan{
		Steps: []PlanStep{
			{Description: "step one"},
			{Description: "step two"},
			{Description: "step three"},
			{Description: "step four"},
		},
		Rationale: "complex multi-step plan",
	}

	subtasks := planStepsToSubtasks(plan.Steps)
	require.Len(t, subtasks, 4)

	executor := makeSubagentExecutor(factory, "test-session")
	decomposer := &planDecomposer{steps: subtasks}
	op := NewOperator(OperatorConfig{
		MaxDepth:   defaultMaxDepth,
		MaxWorkers: defaultMaxWorkers,
		Strategy:   StrategyLLMMap,
	}, executor, decomposer)

	result := op.Run(t.Context(), "implement complex feature with multiple steps requiring careful decomposition of the architecture into subtasks", nil)
	require.True(t, result.Success)
	require.Equal(t, int32(4), execCount.Load(), "all 4 plan steps should execute via operator")
}

func TestOperatorFallbackOnFailure(t *testing.T) {
	t.Parallel()

	factory := &stubOperatorFactory{
		execFn: func(_ context.Context, _ StructuredRequest) (StructuredResponse, error) {
			return StructuredResponse{Success: false, Error: "subagent exploded"}, nil
		},
	}

	plan := ArchitectPlan{
		Steps: []PlanStep{
			{Description: "step one"},
			{Description: "step two"},
			{Description: "step three"},
			{Description: "step four"},
		},
		Rationale: "plan that will fail",
	}

	subtasks := planStepsToSubtasks(plan.Steps)
	executor := makeSubagentExecutor(factory, "test-session")
	decomposer := &planDecomposer{steps: subtasks}
	op := NewOperator(OperatorConfig{
		MaxDepth:   defaultMaxDepth,
		MaxWorkers: defaultMaxWorkers,
		Strategy:   StrategySequential,
	}, executor, decomposer)

	result := op.Run(t.Context(), "implement something that fails", nil)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "subtask 0 failed")
}

func TestOperatorSingleStepSkipped(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	factory := &stubOperatorFactory{
		execFn: func(_ context.Context, _ StructuredRequest) (StructuredResponse, error) {
			called.Store(true)
			return StructuredResponse{Success: true, Result: "should not be called"}, nil
		},
	}

	plan := ArchitectPlan{
		Steps: []PlanStep{
			{Description: "only one step"},
			{Description: "second step"},
			{Description: "third step"},
		},
		Rationale: "simple plan",
	}

	subtasks := planStepsToSubtasks(plan.Steps)
	require.Len(t, subtasks, 3)

	executor := makeSubagentExecutor(factory, "test-session")
	decomposer := &planDecomposer{steps: subtasks}
	op := NewOperator(OperatorConfig{
		MaxDepth:   defaultMaxDepth,
		MaxWorkers: defaultMaxWorkers,
		Strategy:   StrategyConditional,
	}, executor, decomposer)

	result := op.Run(t.Context(), "simple three-step task", nil)
	require.True(t, result.Success)
	require.True(t, called.Load(), "operator should execute subtasks for a 3-step plan")
}

func TestOperatorNoStateLeakBetweenInvocations(t *testing.T) {
	t.Parallel()

	task := "decompose this multi-step task for testing state isolation between operator instances"

	factory := &stubOperatorFactory{
		execFn: func(_ context.Context, _ StructuredRequest) (StructuredResponse, error) {
			return StructuredResponse{Success: true, Result: "ok"}, nil
		},
	}

	cfg := OperatorConfig{
		MaxDepth:   defaultMaxDepth,
		MaxWorkers: defaultMaxWorkers,
		Strategy:   StrategyLLMMap,
	}

	subtasks := []Subtask{
		{ID: "a", Task: "subtask alpha"},
		{ID: "b", Task: "subtask beta"},
	}

	executor := makeSubagentExecutor(factory, "session-1")
	op1 := NewOperator(cfg, executor, &planDecomposer{steps: subtasks})
	r1 := op1.Run(t.Context(), task, nil)
	require.True(t, r1.Success, "first invocation should succeed")

	executor2 := makeSubagentExecutor(factory, "session-2")
	op2 := NewOperator(cfg, executor2, &planDecomposer{steps: subtasks})
	r2 := op2.Run(t.Context(), task, nil)
	require.True(t, r2.Success, "second invocation with fresh operator should succeed without state leak")
}

type stubOperatorFactory struct {
	execFn func(ctx context.Context, req StructuredRequest) (StructuredResponse, error)
}

func (f *stubOperatorFactory) NewStructuredSubagent(_ context.Context, _ string) (StructuredSubagent, error) {
	return &stubOperatorSubagent{execFn: f.execFn}, nil
}

type stubOperatorSubagent struct {
	execFn func(ctx context.Context, req StructuredRequest) (StructuredResponse, error)
}

func (s *stubOperatorSubagent) Execute(ctx context.Context, req StructuredRequest) (StructuredResponse, error) {
	return s.execFn(ctx, req)
}

func (s *stubOperatorSubagent) Capabilities() []string { return nil }
