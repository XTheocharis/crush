package lcm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Ghost-cue types identify the kind of context reference a cue carries.
const (
	CueTypeSummaryID      = "summary_id"
	CueTypeLineagePointer = "lineage_pointer"
	CueTypeArchiveStub    = "archive_stub"
)

// GhostCue is a transparent context hint injected into system prompts or tool
// results. Cues are never shown to the user; they provide the LLM with
// lightweight pointers into LCM-managed context so it can retrieve detailed
// information on demand.
type GhostCue struct {
	ID       string // Unique cue identifier (cue_ prefix).
	Type     string // One of CueTypeSummaryID, CueTypeLineagePointer, CueTypeArchiveStub.
	Priority int    // Higher values are injected first when budget is constrained.
	Content  string // Resolved cue text after template substitution.
}

// CueTemplate maps variable names to values for template substitution.
type CueTemplate struct {
	// Template is a string containing {{.VarName}} placeholders.
	Template string
	// Vars supplies the values that replace the placeholders.
	Vars map[string]string
}

// CueInjector manages ghost-cue templates and injects cues into prompt text
// at designated boundaries. Injection respects priority ordering and a token
// budget so that the most important cues survive compaction.
type CueInjector struct {
	templates map[string]CueTemplate
}

// NewCueInjector creates a CueInjector with built-in default templates for all
// three cue types. Callers may add or override templates with RegisterTemplate.
func NewCueInjector() *CueInjector {
	ci := &CueInjector{
		templates: map[string]CueTemplate{
			CueTypeSummaryID: {
				Template: "[{{.SummaryID}}] {{.Snippet}}",
				Vars:     nil,
			},
			CueTypeLineagePointer: {
				Template: "[Lineage: {{.ParentIDs}}, depth={{.Depth}}]",
				Vars:     nil,
			},
			CueTypeArchiveStub: {
				Template: "[Archived: {{.SummaryID}}, tokens={{.TokenCount}}]",
				Vars:     nil,
			},
		},
	}
	return ci
}

// RegisterTemplate adds or replaces the template for a given cue type.
func (ci *CueInjector) RegisterTemplate(cueType string, tmpl CueTemplate) {
	ci.templates[cueType] = tmpl
}

// Render applies variable substitution to the template registered for cueType.
// It returns the resolved content string. If the cue type has no registered
// template the raw vars are concatenated as a fallback.
func (ci *CueInjector) Render(cueType string, vars map[string]string) string {
	tmpl, ok := ci.templates[cueType]
	if !ok {
		// Fallback: join variable values with spaces.
		vals := make([]string, 0, len(vars))
		for _, v := range vars {
			vals = append(vals, v)
		}
		return strings.Join(vals, " ")
	}
	result := tmpl.Template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{."+k+"}}", v)
	}
	return result
}

// NewCue creates a GhostCue with a unique ID and rendered content.
func (ci *CueInjector) NewCue(cueType string, priority int, vars map[string]string) GhostCue {
	id := generateCueID(cueType)
	return GhostCue{
		ID:       id,
		Type:     cueType,
		Priority: priority,
		Content:  ci.Render(cueType, vars),
	}
}

// InjectIntoPrompt inserts cues into the prompt text at the system prompt
// boundary (after the closing tag). Cues are sorted by descending priority.
// The tokenBudget limits how many cues are included; each cue's token cost is
// estimated via EstimateTokens. Cues that exceed the remaining budget are
// silently dropped.
func (ci *CueInjector) InjectIntoPrompt(prompt string, cues []GhostCue, tokenBudget int64) string {
	if len(cues) == 0 {
		return prompt
	}

	sorted := make([]GhostCue, len(cues))
	copy(sorted, cues)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	var injected []GhostCue
	remaining := tokenBudget
	for _, cue := range sorted {
		cost := EstimateTokens(cue.Content)
		if cost <= remaining {
			injected = append(injected, cue)
			remaining -= cost
		}
	}

	if len(injected) == 0 {
		return prompt
	}

	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n")
	for _, cue := range injected {
		b.WriteString(cue.Content)
		b.WriteString("\n")
	}
	return b.String()
}

// InjectIntoToolResult appends cues to a tool result string. This is the
// second injection boundary. The same priority ordering and budget logic
// applies as InjectIntoPrompt.
func (ci *CueInjector) InjectIntoToolResult(result string, cues []GhostCue, tokenBudget int64) string {
	if len(cues) == 0 {
		return result
	}

	sorted := make([]GhostCue, len(cues))
	copy(sorted, cues)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	var injected []GhostCue
	remaining := tokenBudget
	for _, cue := range sorted {
		cost := EstimateTokens(cue.Content)
		if cost <= remaining {
			injected = append(injected, cue)
			remaining -= cost
		}
	}

	if len(injected) == 0 {
		return result
	}

	var b strings.Builder
	b.WriteString(result)
	b.WriteString("\n")
	for _, cue := range injected {
		b.WriteString(cue.Content)
		b.WriteString("\n")
	}
	return b.String()
}

// generateCueID creates a unique cue identifier using the cue type and
// timestamp.
func generateCueID(cueType string) string {
	ts := time.Now().UnixNano()
	input := fmt.Sprintf("%s:%d", cueType, ts)
	h := sha256.Sum256([]byte(input))
	return "cue_" + hex.EncodeToString(h[:])[:16]
}
