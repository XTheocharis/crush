package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/charmbracelet/crush/internal/session"
)

// Section priority controls which sections are trimmed first when the token
// budget is exhausted. Lower priority sections are trimmed before higher ones.
const (
	PriorityMemories     = 10
	PriorityObservations = 20
	PriorityGhostCues    = 30
	PriorityContextFiles = 40
	PriorityToolSurface  = 50
	PriorityBase         = 100
)

// PromptSection is a named, token-counted section of the assembled prompt.
type PromptSection struct {
	Name     string
	Content  string
	Tokens   int64
	Priority int
}

// PromptAssemblyConfig configures the prompt assembler.
type PromptAssemblyConfig struct {
	TokenBudget int64
}

// PromptDataProvider provides token estimation for prompt assembly.
// The concrete implementation is provided by the LCM extension.
type PromptDataProvider interface {
	// EstimateTokens estimates the token count for the given string content.
	EstimateTokens(s string) int64
}

// defaultCharsPerToken is the fallback ratio when no PromptDataProvider is
// configured.
const defaultCharsPerToken = 4

// fallbackEstimateTokens provides a rough token estimate when no provider is
// available.
func fallbackEstimateTokens(s string) int64 {
	return int64(len(s) / defaultCharsPerToken)
}

// PromptAssembler dynamically assembles a system prompt from multiple sources,
// respecting a token budget. Sections are assembled in priority order; lower
// priority sections are trimmed first when the budget is exceeded.
type PromptAssembler struct {
	cfg      PromptAssemblyConfig
	provider PromptDataProvider
}

// NewPromptAssembler creates a new prompt assembler. If provider is nil, a
// simple character-based token estimator is used.
func NewPromptAssembler(cfg PromptAssemblyConfig, provider PromptDataProvider) *PromptAssembler {
	return &PromptAssembler{cfg: cfg, provider: provider}
}

func (pa *PromptAssembler) estimateTokens(s string) int64 {
	if pa.provider != nil {
		return pa.provider.EstimateTokens(s)
	}
	return fallbackEstimateTokens(s)
}

// Assemble builds the final prompt from the given sources. The assembly order
// is: base prompt -> context files -> tool descriptions -> ghost cues ->
// observations -> reflections -> memories -> om entries -> available tools.
// Sections that would exceed the token budget are dropped starting from lowest
// priority.
func (pa *PromptAssembler) Assemble(ctx context.Context, sources PromptSources) string {
	sections := pa.buildSections(ctx, sources)
	budget := pa.cfg.TokenBudget
	if budget <= 0 {
		budget = 200_000
	}

	var included []PromptSection
	var usedTokens int64

	sort.Slice(sections, func(i, j int) bool {
		return sections[i].Priority > sections[j].Priority
	})

	for _, sec := range sections {
		if usedTokens+sec.Tokens <= budget {
			included = append(included, sec)
			usedTokens += sec.Tokens
		}
	}

	sort.Slice(included, func(i, j int) bool {
		return indexOfSection(included[i].Name) < indexOfSection(included[j].Name)
	})

	var sb strings.Builder
	for i, sec := range included {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(sec.Content)
	}

	total := countTotalTokens(sections)
	if total > budget {
		slog.Debug("Prompt assembly trimmed sections",
			"total_tokens", total,
			"budget", budget,
			"used_tokens", usedTokens,
			"sections_included", len(included),
			"sections_total", len(sections),
		)
	}

	return sb.String()
}

func (pa *PromptAssembler) buildSections(ctx context.Context, sources PromptSources) []PromptSection {
	var sections []PromptSection

	if sources.BasePrompt != "" {
		sections = append(sections, PromptSection{
			Name:     "base",
			Content:  sources.BasePrompt,
			Tokens:   pa.estimateTokens(sources.BasePrompt),
			Priority: PriorityBase,
		})
	}

	if len(sources.ContextFiles) > 0 {
		sections = append(sections, pa.buildContextFilesSection(sources.ContextFiles))
	}

	if len(sources.ToolDescriptions) > 0 {
		sections = append(sections, pa.buildToolDescriptionsSection(sources.ToolDescriptions))
	}

	if len(sources.GhostCues) > 0 {
		sections = append(sections, pa.buildGhostCuesSection(sources.GhostCues))
	}

	observations, err := sources.Observations(ctx)
	if err == nil && len(observations) > 0 {
		sections = append(sections, pa.buildObservationsSection(observations))
	}

	reflections, err := sources.Reflections(ctx)
	if err == nil && len(reflections) > 0 {
		sections = append(sections, pa.buildReflectionsSection(reflections))
	}

	memories, err := sources.Memories(ctx)
	if err == nil && len(memories) > 0 {
		sections = append(sections, pa.buildMemoriesSection(memories))
	}

	omEntries, err := sources.OMEntries(ctx)
	if err == nil && len(omEntries) > 0 {
		sections = append(sections, pa.buildOMSection(omEntries))
	}

	if len(sources.VisibleTools) > 0 {
		sections = append(sections, pa.buildToolSurfaceSection(sources.VisibleTools))
	}

	return sections
}

func (pa *PromptAssembler) buildContextFilesSection(files []ContextFile) PromptSection {
	var sb strings.Builder
	for _, f := range files {
		fmt.Fprintf(&sb, "=== %s ===\n%s\n", f.Name, f.Content)
	}
	content := sb.String()
	return PromptSection{
		Name:     "context_files",
		Content:  content,
		Tokens:   pa.estimateTokens(content),
		Priority: PriorityContextFiles,
	}
}

func (pa *PromptAssembler) buildToolDescriptionsSection(descs []string) PromptSection {
	content := "Available tool descriptions:\n" + strings.Join(descs, "\n")
	return PromptSection{
		Name:     "tool_descriptions",
		Content:  content,
		Tokens:   pa.estimateTokens(content),
		Priority: PriorityToolSurface,
	}
}

func (pa *PromptAssembler) buildGhostCuesSection(cues []GhostCue) PromptSection {
	sorted := make([]GhostCue, len(cues))
	copy(sorted, cues)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	var parts []string
	for _, cue := range sorted {
		if cue.Content != "" {
			parts = append(parts, cue.Content)
		}
	}
	if len(parts) == 0 {
		return PromptSection{}
	}
	content := "<ghost-cues>\n" + strings.Join(parts, "\n") + "\n</ghost-cues>"
	return PromptSection{
		Name:     "ghost_cues",
		Content:  content,
		Tokens:   pa.estimateTokens(content),
		Priority: PriorityGhostCues,
	}
}

func (pa *PromptAssembler) buildObservationsSection(obs []Observation) PromptSection {
	var parts []string
	for _, o := range obs {
		parts = append(parts, fmt.Sprintf("- [%s] %s (implication: %s)", o.Event, o.Context, o.Implication))
	}
	content := "<observations>\n" + strings.Join(parts, "\n") + "\n</observations>"
	return PromptSection{
		Name:     "observations",
		Content:  content,
		Tokens:   pa.estimateTokens(content),
		Priority: PriorityObservations,
	}
}

func (pa *PromptAssembler) buildReflectionsSection(refs []Reflection) PromptSection {
	var parts []string
	for _, r := range refs {
		parts = append(parts, fmt.Sprintf("- %s (confidence: %.1f, suggestion: %s)", r.Insight, r.Confidence, r.ActionSuggestion))
	}
	content := "<reflections>\n" + strings.Join(parts, "\n") + "\n</reflections>"
	return PromptSection{
		Name:     "reflections",
		Content:  content,
		Tokens:   pa.estimateTokens(content),
		Priority: PriorityObservations,
	}
}

func (pa *PromptAssembler) buildMemoriesSection(memories []ExtractedMemory) PromptSection {
	var parts []string
	for _, m := range memories {
		parts = append(parts, fmt.Sprintf("- [%s] %s", m.Type, m.Content))
	}
	content := "<memories>\n" + strings.Join(parts, "\n") + "\n</memories>"
	return PromptSection{
		Name:     "memories",
		Content:  content,
		Tokens:   pa.estimateTokens(content),
		Priority: PriorityMemories,
	}
}

func (pa *PromptAssembler) buildOMSection(entries []session.OMEntry) PromptSection {
	sorted := make([]session.OMEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return omPriorityRank(sorted[i].Priority) > omPriorityRank(sorted[j].Priority)
	})

	var parts []string
	for _, e := range sorted {
		parts = append(parts, fmt.Sprintf("- [%s] %s: %s", e.Priority, e.Key, e.Value))
	}
	content := "<operational-memory>\n" + strings.Join(parts, "\n") + "\n</operational-memory>"
	return PromptSection{
		Name:     "om_entries",
		Content:  content,
		Tokens:   pa.estimateTokens(content),
		Priority: PriorityMemories,
	}
}

func (pa *PromptAssembler) buildToolSurfaceSection(tools []string) PromptSection {
	content := "Enabled tools: " + strings.Join(tools, ", ")
	return PromptSection{
		Name:     "tool_surface",
		Content:  content,
		Tokens:   pa.estimateTokens(content),
		Priority: PriorityToolSurface,
	}
}

func omPriorityRank(p string) int {
	switch p {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

var sectionOrder = []string{
	"base", "context_files", "tool_descriptions", "ghost_cues",
	"observations", "reflections", "memories", "om_entries", "tool_surface",
}

func indexOfSection(name string) int {
	for i, n := range sectionOrder {
		if n == name {
			return i
		}
	}
	return len(sectionOrder)
}

func countTotalTokens(sections []PromptSection) int64 {
	var total int64
	for _, s := range sections {
		total += s.Tokens
	}
	return total
}

// ContextFile represents a named context file to inject into the system prompt.
// Mirrors lcm.ContextFile for interface decoupling.
type ContextFile struct {
	Name    string
	Content string
}

// GhostCue is a transparent context hint injected into system prompts.
// Mirrors lcm.GhostCue for interface decoupling.
type GhostCue struct {
	ID       string
	Type     string
	Priority int
	Content  string
}

// Observation is a single (event, context, implication) tuple from the
// observer agent. Mirrors lcm.Observation for interface decoupling.
type Observation struct {
	Event       string
	Context     string
	Implication string
}

// Reflection is a single (insight, confidence, action_suggestion) tuple from
// the reflector agent. Mirrors lcm.Reflection for interface decoupling.
type Reflection struct {
	Insight          string
	Confidence       float64
	ActionSuggestion string
}

// ExtractedMemory is a structured memory extracted from conversation.
// Mirrors lcm.ExtractedMemory for interface decoupling.
type ExtractedMemory struct {
	Type       string
	Content    string
	Confidence float64
	Priority   float64
}

// PromptSources provides all the data sources for prompt assembly. Functions
// are used for expensive data (observations, reflections, memories) so they
// are only called when needed.
type PromptSources struct {
	BasePrompt       string
	ContextFiles     []ContextFile
	ToolDescriptions []string
	GhostCues        []GhostCue
	VisibleTools     []string
	Observations     func(ctx context.Context) ([]Observation, error)
	Reflections      func(ctx context.Context) ([]Reflection, error)
	Memories         func(ctx context.Context) ([]ExtractedMemory, error)
	OMEntries        func(ctx context.Context) ([]session.OMEntry, error)
}

// NoopSources returns a PromptSources with no-op functions for observations,
// reflections, memories, and OM entries. Useful for testing and when LCM is
// not active.
func NoopSources() PromptSources {
	return PromptSources{
		Observations: func(_ context.Context) ([]Observation, error) { return nil, nil },
		Reflections:  func(_ context.Context) ([]Reflection, error) { return nil, nil },
		Memories:     func(_ context.Context) ([]ExtractedMemory, error) { return nil, nil },
		OMEntries:    func(_ context.Context) ([]session.OMEntry, error) { return nil, nil },
	}
}
