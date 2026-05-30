package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/config"
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

func TestIsPlanningCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prompt   string
		expected TaskCategory
	}{
		{
			name:     "bug fix",
			prompt:   "fix the nil pointer dereference in handler.go",
			expected: CategoryBug,
		},
		{
			name:     "bug crash",
			prompt:   "the app crashes on startup",
			expected: CategoryBug,
		},
		{
			name:     "bug regression",
			prompt:   "this is a regression from last week",
			expected: CategoryBug,
		},
		{
			name:     "bug broken",
			prompt:   "the login flow is broken",
			expected: CategoryBug,
		},
		{
			name:     "bug error",
			prompt:   "getting an error when parsing JSON",
			expected: CategoryBug,
		},
		{
			name:     "bug issue",
			prompt:   "resolve the issue with timeouts",
			expected: CategoryBug,
		},
		{
			name:     "feature add",
			prompt:   "add pagination to the user list endpoint",
			expected: CategoryFeature,
		},
		{
			name:     "feature implement",
			prompt:   "implement OAuth2 login",
			expected: CategoryFeature,
		},
		{
			name:     "feature create",
			prompt:   "create a dashboard component",
			expected: CategoryFeature,
		},
		{
			name:     "feature build",
			prompt:   "build a notification system",
			expected: CategoryFeature,
		},
		{
			name:     "feature new",
			prompt:   "new endpoint for exporting data",
			expected: CategoryFeature,
		},
		{
			name:     "feature support",
			prompt:   "support dark mode in the settings",
			expected: CategoryFeature,
		},
		{
			name:     "refactor keyword",
			prompt:   "refactor the database access layer",
			expected: CategoryRefactor,
		},
		{
			name:     "restructure",
			prompt:   "restructure the packages for clarity",
			expected: CategoryRefactor,
		},
		{
			name:     "clean up",
			prompt:   "clean up the unused imports",
			expected: CategoryRefactor,
		},
		{
			name:     "simplify",
			prompt:   "simplify the config loading logic",
			expected: CategoryRefactor,
		},
		{
			name:     "optimize",
			prompt:   "optimize the query performance",
			expected: CategoryRefactor,
		},
		{
			name:     "mixed bug and feature",
			prompt:   "fix the error and add logging",
			expected: CategoryBug,
		},
		{
			name:     "mixed feature and refactor",
			prompt:   "add caching and optimize the rendering pipeline",
			expected: CategoryFeature,
		},
		{
			name:     "unknown empty string",
			prompt:   "",
			expected: CategoryUnknown,
		},
		{
			name:     "unknown no keywords",
			prompt:   "what is 2+2?",
			expected: CategoryUnknown,
		},
		{
			name:     "unknown conversational",
			prompt:   "hello, how are you doing today?",
			expected: CategoryUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsPlanningCategory(tt.prompt)
			require.Equal(t, tt.expected, result, "IsPlanningCategory(%q) = %q, want %q", tt.prompt, result, tt.expected)
		})
	}
}

func TestIsPlanningTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prompt   string
		expected bool
	}{
		{
			name:     "bug fix triggers planning via category",
			prompt:   "fix the nil pointer dereference in handler.go",
			expected: true,
		},
		{
			name:     "feature add triggers planning via category",
			prompt:   "add pagination to the user list",
			expected: true,
		},
		{
			name:     "refactor triggers planning via category",
			prompt:   "refactor the database layer",
			expected: true,
		},
		{
			name:     "plan and implement indicator",
			prompt:   "plan and implement a new auth system",
			expected: true,
		},
		{
			name:     "design and implement indicator",
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
			name:     "implement a new feature indicator",
			prompt:   "implement a new feature for user management",
			expected: true,
		},
		{
			name:     "implement a complete indicator",
			prompt:   "implement a complete REST API",
			expected: true,
		},
		{
			name:     "short simple prompt is unknown",
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
		"what does this function do?",
	}

	for _, p := range simplePrompts {
		require.False(t, IsPlanningTask(p), "expected simple prompt to skip planning: %q", p)
	}
}

func TestArchitectEditorTwoPhase_Feature(t *testing.T) {
	t.Parallel()

	withArch := &coordinator{
		cfg: config.NewTestStore(&config.Config{
			Options: &config.Options{
				ArchitectModel: &config.SelectedModel{
					Provider: "anthropic",
					Model:    "claude-opus-4",
				},
			},
		}),
	}
	require.True(t, withArch.shouldUseTwoPhase("add a new authentication endpoint"),
		"feature prompt with ArchitectModel configured should use two-phase")

	withoutArch := &coordinator{
		cfg: config.NewTestStore(&config.Config{
			Options: &config.Options{},
		}),
	}
	require.False(t, withoutArch.shouldUseTwoPhase("add a new authentication endpoint"),
		"feature prompt without ArchitectModel should not use two-phase")
}

func TestArchitectEditorTwoPhase_Bug(t *testing.T) {
	t.Parallel()

	c := &coordinator{
		cfg: config.NewTestStore(&config.Config{
			Options: &config.Options{
				ArchitectModel: &config.SelectedModel{
					Provider: "anthropic",
					Model:    "claude-opus-4",
				},
			},
		}),
	}

	require.False(t, c.shouldUseTwoPhase("fix the nil pointer crash in handler"),
		"bug prompt should never use two-phase even with ArchitectModel")
}

func TestArchitectEditorTwoPhase_Fallback(t *testing.T) {
	t.Parallel()

	archModel := &config.SelectedModel{Provider: "anthropic", Model: "claude-opus-4"}

	withArch := &coordinator{
		cfg: config.NewTestStore(&config.Config{
			Options: &config.Options{ArchitectModel: archModel},
		}),
	}

	require.False(t, withArch.shouldUseTwoPhase("refactor the database layer"),
		"refactor prompt should never use two-phase even with ArchitectModel")
	require.False(t, withArch.shouldUseTwoPhase("what is 2+2?"),
		"unknown category should never use two-phase")

	nilOptions := &coordinator{
		cfg: config.NewTestStore(&config.Config{}),
	}
	require.False(t, nilOptions.shouldUseTwoPhase("add a new feature"),
		"nil options should not use two-phase")
}

func TestArchitectPlanExecutionPrompt(t *testing.T) {
	t.Parallel()

	plan := ArchitectPlan{
		Steps: []PlanStep{
			{Description: "Read coordinator.go", Status: PlanStepCompleted, TargetFiles: []string{"coordinator.go"}},
			{Description: "Add two-phase flow", Status: PlanStepPending, Dependencies: []int{1}},
		},
		Rationale: "Separate planning from execution",
	}

	s := plan.String()
	require.Contains(t, s, "Read coordinator.go")
	require.Contains(t, s, "Add two-phase flow")
	require.Contains(t, s, "Separate planning from execution")
}

func TestArchitectTemplateLoaded(t *testing.T) {
	t.Parallel()

	p, err := architectPrompt(
		prompt.WithTimeFunc(func() time.Time {
			return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
		}),
		prompt.WithPlatform("linux"),
		prompt.WithWorkingDir("/tmp/test-project"),
	)
	require.NoError(t, err, "architectPrompt() should load the embedded template without error")

	cfg := config.NewTestStore(&config.Config{
		Options: &config.Options{},
	})
	built, err := p.Build(context.Background(), "test-provider", "test-model", cfg)
	require.NoError(t, err, "Build() should execute the template without error")

	require.Contains(t, built, "You are the Architect agent", "built prompt should contain architect role definition")
	require.Contains(t, built, "output_schema", "built prompt should contain JSON output schema")
	require.Contains(t, built, `"step"`, "built prompt should reference step field")
	require.Contains(t, built, `/tmp/test-project`, "built prompt should contain working directory")
	require.Contains(t, built, "6/1/2025", "built prompt should contain the injected date")
}

func TestArchitectTemplateIsNonEmpty(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, architectPromptTmpl, "architectPromptTmpl should be embedded at compile time")
}

func TestMarkStepTracking(t *testing.T) {
	t.Parallel()

	plan := ArchitectPlan{
		Steps: []PlanStep{
			{Description: "Step 1", Status: PlanStepPending},
			{Description: "Step 2", Status: PlanStepPending},
			{Description: "Step 3", Status: PlanStepPending},
		},
	}

	// Transition: pending -> running for all steps.
	runningCount := plan.MarkAllRunning()
	require.Equal(t, 3, runningCount)
	for i, step := range plan.Steps {
		require.Equal(t, PlanStepRunning, step.Status, "step %d should be running", i)
	}

	// Calling MarkAllRunning again should not transition already-running steps.
	runningCount = plan.MarkAllRunning()
	require.Equal(t, 0, runningCount)

	// Transition: running -> completed for all steps.
	completedCount := plan.MarkAllCompleted()
	require.Equal(t, 3, completedCount)
	for i, step := range plan.Steps {
		require.Equal(t, PlanStepCompleted, step.Status, "step %d should be completed", i)
	}

	// Calling MarkAllCompleted again should not transition already-completed steps.
	completedCount = plan.MarkAllCompleted()
	require.Equal(t, 0, completedCount)
}

func TestMarkStepTrackingFailedPath(t *testing.T) {
	t.Parallel()

	plan := ArchitectPlan{
		Steps: []PlanStep{
			{Description: "Step 1", Status: PlanStepPending},
			{Description: "Step 2", Status: PlanStepPending},
		},
	}

	plan.MarkAllRunning()
	for i, step := range plan.Steps {
		require.Equal(t, PlanStepRunning, step.Status, "step %d should be running", i)
	}

	failedCount := plan.MarkAllFailed()
	require.Equal(t, 2, failedCount)
	for i, step := range plan.Steps {
		require.Equal(t, PlanStepFailed, step.Status, "step %d should be failed", i)
	}

	// MarkAllFailed is idempotent on non-running steps.
	failedCount = plan.MarkAllFailed()
	require.Equal(t, 0, failedCount)
}

func TestMarkStepSkipped(t *testing.T) {
	t.Parallel()

	plan := ArchitectPlan{
		Steps: []PlanStep{
			{Description: "Step 1", Status: PlanStepPending},
		},
	}

	plan.MarkStepSkipped(0)
	require.Equal(t, PlanStepSkipped, plan.Steps[0].Status)

	// Out-of-bounds indices are no-ops.
	plan.MarkStepSkipped(-1)
	plan.MarkStepSkipped(99)
}

func TestMarkAllMixedStates(t *testing.T) {
	t.Parallel()

	plan := ArchitectPlan{
		Steps: []PlanStep{
			{Description: "Step 1", Status: PlanStepCompleted},
			{Description: "Step 2", Status: PlanStepPending},
			{Description: "Step 3", Status: PlanStepRunning},
		},
	}

	runningCount := plan.MarkAllRunning()
	require.Equal(t, 1, runningCount)
	require.Equal(t, PlanStepCompleted, plan.Steps[0].Status)
	require.Equal(t, PlanStepRunning, plan.Steps[1].Status)
	require.Equal(t, PlanStepRunning, plan.Steps[2].Status)

	completedCount := plan.MarkAllCompleted()
	require.Equal(t, 2, completedCount)
	require.Equal(t, PlanStepCompleted, plan.Steps[0].Status)
	require.Equal(t, PlanStepCompleted, plan.Steps[1].Status)
	require.Equal(t, PlanStepCompleted, plan.Steps[2].Status)
}

func TestApprovalGate(t *testing.T) {
	t.Parallel()

	planRequiringApproval := ArchitectPlan{
		Steps:            []PlanStep{{Description: "Complex refactor", Status: PlanStepPending}},
		ApprovalRequired: true,
	}
	planNotRequiringApproval := ArchitectPlan{
		Steps:            []PlanStep{{Description: "Simple change", Status: PlanStepPending}},
		ApprovalRequired: false,
	}

	t.Run("blocks when config and plan both require approval", func(t *testing.T) {
		t.Parallel()
		gate := NewApprovalGate(true)
		err := gate.Check(planRequiringApproval)
		require.ErrorIs(t, err, ErrApprovalRequired)
	})

	t.Run("passes when config requires but plan does not", func(t *testing.T) {
		t.Parallel()
		gate := NewApprovalGate(true)
		err := gate.Check(planNotRequiringApproval)
		require.NoError(t, err)
	})

	t.Run("passes when config does not require approval", func(t *testing.T) {
		t.Parallel()
		gate := NewApprovalGate(false)
		err := gate.Check(planRequiringApproval)
		require.NoError(t, err)
	})

	t.Run("passes when neither requires approval", func(t *testing.T) {
		t.Parallel()
		gate := NewApprovalGate(false)
		err := gate.Check(planNotRequiringApproval)
		require.NoError(t, err)
	})
}
