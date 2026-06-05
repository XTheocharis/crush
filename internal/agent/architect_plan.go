package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
// strips markdown code fences and leading/trailing whitespace before parsing.
func ParseArchitectPlan(data string) (ArchitectPlan, error) {
	cleaned := extractJSON(data)
	var plan ArchitectPlan
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		return ArchitectPlan{}, fmt.Errorf("parse architect plan: %w", err)
	}
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now()
	}
	return plan, nil
}

// extractJSON strips markdown code fences and surrounding text, returning
// only the JSON payload. If the LLM wraps its output in ```json ... ``` or
// adds commentary before/after, this recovers the JSON object.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Strip opening code fence.
	if after, ok := strings.CutPrefix(s, "```"); ok {
		after = strings.TrimLeft(after, "\r\n")
		// Skip optional language tag line (e.g. "json\n") only if
		// the remaining text does not start with '{'.
		if len(after) > 0 && after[0] != '{' {
			if idx := strings.Index(after, "\n"); idx >= 0 {
				after = after[idx+1:]
			}
		}
		// Strip closing code fence.
		if idx := strings.LastIndex(after, "```"); idx >= 0 {
			after = after[:idx]
		}
		s = strings.TrimSpace(after)
	}

	// Find the first '{'.
	start := strings.Index(s, "{")
	if start < 0 {
		return s
	}

	// Walk to the matching '}' using brace depth.
	depth := 0
	end := -1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
				goto done
			}
		}
	}

done:

	if end >= 0 {
		return s[start:end]
	}
	return s[start:]
}

// String returns a human-readable summary of the plan.
func (p ArchitectPlan) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Plan (%d steps)", len(p.Steps))
	if p.Rationale != "" {
		fmt.Fprintf(&b, ": %s", p.Rationale)
	}
	for i, step := range p.Steps {
		fmt.Fprintf(&b, "\n  %d. [%s] %s", i+1, step.Status, step.Description)
		if len(step.TargetFiles) > 0 {
			fmt.Fprintf(&b, " (files: %s)", strings.Join(step.TargetFiles, ", "))
		}
	}
	return b.String()
}

// TaskCategory represents the classified intent of a user prompt.
type TaskCategory string

const (
	CategoryUnknown  TaskCategory = "unknown"
	CategoryBug      TaskCategory = "bug"
	CategoryFeature  TaskCategory = "feature"
	CategoryRefactor TaskCategory = "refactor"
)

// categoryKeywords maps each TaskCategory to the lowercase keywords that
// signal that intent.
var categoryKeywords = map[TaskCategory][]string{
	CategoryBug: {
		"fix", "bug", "error", "crash", "broken", "regression", "issue",
	},
	CategoryFeature: {
		"add", "implement", "create", "build", "new", "support",
	},
	CategoryRefactor: {
		"refactor", "restructure", "clean up", "simplify", "optimize",
	},
}

// IsPlanningCategory classifies a user prompt into a TaskCategory using
// keyword heuristics. The category with the highest keyword count wins; ties
// are broken in declaration order (bug, feature, refactor). Returns
// CategoryUnknown when no keywords match.
func IsPlanningCategory(prompt string) TaskCategory {
	lower := strings.ToLower(prompt)

	best := CategoryUnknown
	bestCount := 0

	for _, cat := range []TaskCategory{CategoryBug, CategoryFeature, CategoryRefactor} {
		count := 0
		for _, kw := range categoryKeywords[cat] {
			if strings.Contains(lower, kw) {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			best = cat
		}
	}

	return best
}

// IsPlanningTask classifies a user prompt as planning-worthy (complex) or
// simple (direct execution). It uses category heuristics, explicit planning
// indicators, and prompt length to decide whether the two-phase
// architect→editor flow should kick in.
func IsPlanningTask(prompt string) bool {
	// Fast path: any recognised category triggers planning.
	if IsPlanningCategory(prompt) != CategoryUnknown {
		return true
	}

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
	for segment := range strings.FieldsSeq(prompt) {
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

// MarkStepSkipped sets the step at the given 0-based index to skipped.
func (p *ArchitectPlan) MarkStepSkipped(idx int) {
	if idx >= 0 && idx < len(p.Steps) {
		p.Steps[idx].Status = PlanStepSkipped
	}
}

// MarkAllRunning sets all pending steps to running and returns the count of
// steps transitioned.
func (p *ArchitectPlan) MarkAllRunning() int {
	count := 0
	for i := range p.Steps {
		if p.Steps[i].Status == PlanStepPending {
			p.Steps[i].Status = PlanStepRunning
			count++
		}
	}
	return count
}

// MarkAllCompleted sets all running steps to completed and returns the count
// of steps transitioned.
func (p *ArchitectPlan) MarkAllCompleted() int {
	count := 0
	for i := range p.Steps {
		if p.Steps[i].Status == PlanStepRunning {
			p.Steps[i].Status = PlanStepCompleted
			count++
		}
	}
	return count
}

// MarkAllFailed sets all running steps to failed and returns the count of
// steps transitioned.
func (p *ArchitectPlan) MarkAllFailed() int {
	count := 0
	for i := range p.Steps {
		if p.Steps[i].Status == PlanStepRunning {
			p.Steps[i].Status = PlanStepFailed
			count++
		}
	}
	return count
}

// ErrApprovalRequired is returned when the approval gate blocks plan
// execution because user approval has not been granted.
var ErrApprovalRequired = fmt.Errorf("architect plan requires user approval before execution")

// ApprovalGate blocks plan execution when both the config flag
// (RequireApproval) and the plan's ApprovalRequired field are true.
type ApprovalGate struct {
	RequireApproval bool
}

// NewApprovalGate creates an ApprovalGate from the given config flag.
func NewApprovalGate(configRequiresApproval bool) *ApprovalGate {
	return &ApprovalGate{RequireApproval: configRequiresApproval}
}

// Check returns ErrApprovalRequired when both the config and plan require
// approval. Returns nil when execution should continue.
func (g *ApprovalGate) Check(plan ArchitectPlan) error {
	if g.RequireApproval && plan.ApprovalRequired {
		slog.Info("Approval gate blocking plan execution",
			"steps", len(plan.Steps),
			"rationale", plan.Rationale,
		)
		return ErrApprovalRequired
	}
	return nil
}
