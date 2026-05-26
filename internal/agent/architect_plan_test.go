package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestArchitectPlanSerialization(t *testing.T) {
	t.Parallel()

	original := ArchitectPlan{
		Steps: []PlanStep{
			{
				Description: "Read existing coordinator.go",
				TargetFiles: []string{"internal/agent/coordinator.go"},
				Status:      PlanStepPending,
			},
			{
				Description:  "Add two-phase flow",
				TargetFiles:  []string{"internal/agent/coordinator.go", "internal/agent/architect_plan.go"},
				Dependencies: []int{1},
				Status:       PlanStepPending,
			},
		},
		Rationale:        "Two-phase approach separates planning from execution",
		ApprovalRequired: false,
		CreatedAt:        time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		ModelID:          "test-model",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	parsed, err := ParseArchitectPlan(string(data))
	require.NoError(t, err)

	require.Equal(t, original.Steps, parsed.Steps)
	require.Equal(t, original.Rationale, parsed.Rationale)
	require.Equal(t, original.ApprovalRequired, parsed.ApprovalRequired)
	require.Equal(t, original.ModelID, parsed.ModelID)
	require.Equal(t, original.CreatedAt, parsed.CreatedAt)
}

func TestArchitectPlanParsingDefaults(t *testing.T) {
	t.Parallel()

	input := `{"steps":[{"description":"Do something","status":"pending"}],"rationale":"test"}`

	plan, err := ParseArchitectPlan(input)
	require.NoError(t, err)
	require.Len(t, plan.Steps, 1)
	require.Equal(t, "Do something", plan.Steps[0].Description)
	require.False(t, plan.ApprovalRequired)
	require.WithinDuration(t, time.Now(), plan.CreatedAt, 2*time.Second)
}

func TestArchitectPlanParsingInvalid(t *testing.T) {
	t.Parallel()

	_, err := ParseArchitectPlan("not json")
	require.Error(t, err)
}

func TestArchitectPlanStepStatusTransitions(t *testing.T) {
	t.Parallel()

	plan := ArchitectPlan{
		Steps: []PlanStep{
			{Description: "Step 1", Status: PlanStepPending},
			{Description: "Step 2", Status: PlanStepPending},
		},
	}

	plan.MarkStepRunning(0)
	require.Equal(t, PlanStepRunning, plan.Steps[0].Status)
	require.Equal(t, PlanStepPending, plan.Steps[1].Status)

	plan.MarkStepCompleted(0)
	require.Equal(t, PlanStepCompleted, plan.Steps[0].Status)

	plan.MarkStepFailed(1)
	require.Equal(t, PlanStepFailed, plan.Steps[1].Status)

	plan.MarkStepRunning(-1)
	plan.MarkStepRunning(99)
}

func TestArchitectPlanString(t *testing.T) {
	t.Parallel()

	plan := ArchitectPlan{
		Steps: []PlanStep{
			{Description: "First step", Status: PlanStepCompleted, TargetFiles: []string{"a.go"}},
			{Description: "Second step", Status: PlanStepPending},
		},
		Rationale: "test rationale",
	}

	s := plan.String()
	require.Contains(t, s, "Plan (2 steps)")
	require.Contains(t, s, "test rationale")
	require.Contains(t, s, "First step")
	require.Contains(t, s, "a.go")
	require.Contains(t, s, "[completed]")
	require.Contains(t, s, "[pending]")
}

func TestIsPlanningTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prompt   string
		expected bool
	}{
		{
			name:     "simple edit",
			prompt:   "fix the typo in main.go",
			expected: false,
		},
		{
			name:     "plan and implement",
			prompt:   "plan and implement a new auth system",
			expected: true,
		},
		{
			name:     "design and implement",
			prompt:   "design and implement a caching layer",
			expected: true,
		},
		{
			name:     "architect keyword",
			prompt:   "architect a solution for the database migration",
			expected: true,
		},
		{
			name:     "multi-step keyword",
			prompt:   "this is a multi-step task",
			expected: true,
		},
		{
			name:     "refactor the",
			prompt:   "refactor the entire module structure",
			expected: true,
		},
		{
			name:     "implement a new feature",
			prompt:   "implement a new feature for user management",
			expected: true,
		},
		{
			name:     "implement a complete",
			prompt:   "implement a complete REST API",
			expected: true,
		},
		{
			name:     "short simple prompt",
			prompt:   "what is 2+2?",
			expected: false,
		},
		{
			name:     "long prompt without file refs",
			prompt:   "this is a very long prompt that exceeds the character limit but does not contain any file references or path separators or file extensions so it should not be classified as a planning task even though it is long enough to potentially trigger the heuristic",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsPlanningTask(tt.prompt)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestSimpleTaskSkipsPlanning(t *testing.T) {
	t.Parallel()

	simplePrompts := []string{
		"fix the typo",
		"change the variable name",
		"add a comment",
		"update the readme",
		"what does this function do?",
	}

	for _, p := range simplePrompts {
		require.False(t, IsPlanningTask(p), "expected simple prompt to skip planning: %q", p)
	}
}
