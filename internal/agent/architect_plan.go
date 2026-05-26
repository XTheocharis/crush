package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PlanStepStatus represents the execution status of a single plan step.
type PlanStepStatus string

const (
	PlanStepPending   PlanStepStatus = "pending"
	PlanStepRunning   PlanStepStatus = "running"
	PlanStepCompleted PlanStepStatus = "completed"
	PlanStepFailed    PlanStepStatus = "failed"
	PlanStepSkipped   PlanStepStatus = "skipped"
)

// PlanStep describes a single actionable step in an ArchitectPlan.
type PlanStep struct {
	// Description is a human-readable summary of what this step does.
	Description string `json:"description"`
	// TargetFiles lists the files this step intends to modify or create.
	TargetFiles []string `json:"target_files,omitempty"`
	// Dependencies lists the 1-based step indices that must complete
	// before this step can run.
	Dependencies []int `json:"dependencies,omitempty"`
	// Status tracks execution progress.
	Status PlanStepStatus `json:"status"`
}

// ArchitectPlan is the structured output of the architect phase. It breaks a
// complex task into ordered steps with dependency information so the editor
// phase can execute them methodically.
type ArchitectPlan struct {
	// Steps is the ordered list of actions the plan prescribes.
	Steps []PlanStep `json:"steps"`
	// Rationale explains why this plan was chosen over alternatives.
	Rationale string `json:"rationale"`
	// ApprovalRequired indicates whether the plan needs explicit user
	// approval before execution proceeds.
	ApprovalRequired bool `json:"approval_required"`
	// CreatedAt is the timestamp when the plan was generated.
	CreatedAt time.Time `json:"created_at"`
	// ModelID records which model produced the plan.
	ModelID string `json:"model_id"`
}

// ParseArchitectPlan deserialises a JSON string into an ArchitectPlan. It
// returns an error if the input cannot be unmarshalled.
func ParseArchitectPlan(data string) (ArchitectPlan, error) {
	var plan ArchitectPlan
	if err := json.Unmarshal([]byte(data), &plan); err != nil {
		return ArchitectPlan{}, fmt.Errorf("parse architect plan: %w", err)
	}
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now()
	}
	return plan, nil
}

// String returns a human-readable summary of the plan.
func (p ArchitectPlan) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Plan (%d steps)", len(p.Steps)))
	if p.Rationale != "" {
		b.WriteString(fmt.Sprintf(": %s", p.Rationale))
	}
	for i, step := range p.Steps {
		b.WriteString(fmt.Sprintf("\n  %d. [%s] %s", i+1, step.Status, step.Description))
		if len(step.TargetFiles) > 0 {
			b.WriteString(fmt.Sprintf(" (files: %s)", strings.Join(step.TargetFiles, ", ")))
		}
	}
	return b.String()
}

// IsPlanningTask classifies a user prompt as planning-worthy (complex) or
// simple (direct execution). It uses keyword heuristics and prompt length to
// decide whether the two-phase architect→editor flow should kick in.
func IsPlanningTask(prompt string) bool {
	lower := strings.ToLower(prompt)

	planningIndicators := []string{
		"plan and implement", "design and implement",
		"architect", "blueprint", "step-by-step plan",
		"multi-step", "multi-step task",
		"refactor the", "restructure",
		"implement the following", "implement a new feature",
		"implement a complete",
	}
	for _, indicator := range planningIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}

	fileRefCount := 0
	for _, segment := range strings.Fields(prompt) {
		if strings.Contains(segment, "/") || strings.Contains(segment, ".go") ||
			strings.Contains(segment, ".ts") || strings.Contains(segment, ".py") ||
			strings.Contains(segment, ".rs") || strings.Contains(segment, ".js") {
			fileRefCount++
		}
	}

	return len(prompt) > 500 && fileRefCount >= 3
}

// MarkStepRunning sets the step at the given 0-based index to running.
func (p *ArchitectPlan) MarkStepRunning(idx int) {
	if idx >= 0 && idx < len(p.Steps) {
		p.Steps[idx].Status = PlanStepRunning
	}
}

// MarkStepCompleted sets the step at the given 0-based index to completed.
func (p *ArchitectPlan) MarkStepCompleted(idx int) {
	if idx >= 0 && idx < len(p.Steps) {
		p.Steps[idx].Status = PlanStepCompleted
	}
}

// MarkStepFailed sets the step at the given 0-based index to failed.
func (p *ArchitectPlan) MarkStepFailed(idx int) {
	if idx >= 0 && idx < len(p.Steps) {
		p.Steps[idx].Status = PlanStepFailed
	}
}
