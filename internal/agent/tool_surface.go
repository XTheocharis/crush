package agent

import (
	"sort"
	"strings"
	"sync"
)

// Capability is a 6-bit bitmask identifying a tool's functional category.
type Capability uint8

const (
	CapabilityFS               Capability = 1 << iota // 0x01
	CapabilityNetwork                                 // 0x02
	CapabilityCodeIntelligence                        // 0x04
	CapabilityExecution                               // 0x08
	CapabilityMemory                                  // 0x10
	CapabilityObservation                             // 0x20
)

func (c Capability) String() string {
	switch c {
	case CapabilityFS:
		return "filesystem"
	case CapabilityNetwork:
		return "network"
	case CapabilityCodeIntelligence:
		return "code-intelligence"
	case CapabilityExecution:
		return "execution"
	case CapabilityMemory:
		return "memory"
	case CapabilityObservation:
		return "observation"
	default:
		return "unknown"
	}
}

// AllCapabilities returns all defined capability bits.
func AllCapabilities() []Capability {
	return []Capability{
		CapabilityFS,
		CapabilityNetwork,
		CapabilityCodeIntelligence,
		CapabilityExecution,
		CapabilityMemory,
		CapabilityObservation,
	}
}

// SurfaceContext describes the current runtime context that determines which
// tools are visible. Fields mirror the conditions checked in buildTools.
type SurfaceContext struct {
	HasLSP     bool
	HasMCP     bool
	HasLCM     bool
	HasRepoMap bool
}

// toolMeta holds the bitmask and visibility state for a registered tool.
type toolMeta struct {
	Capabilities Capability
	Visible      bool
}

// ToolSurface manages which tools are currently visible based on the runtime
// context. It maintains a registry of known tools with capability bitmasks
// and updates visibility dynamically without modifying tool definitions.
type ToolSurface struct {
	mu    sync.RWMutex
	tools map[string]toolMeta
}

// NewToolSurface creates a surface with the default set of known tools
// pre-registered with their capability bitmasks.
func NewToolSurface() *ToolSurface {
	s := &ToolSurface{
		tools: make(map[string]toolMeta),
	}
	s.registerDefaults()
	return s
}

func (s *ToolSurface) registerDefaults() {
	s.Register("bash", CapabilityFS|CapabilityExecution)
	s.Register("edit", CapabilityFS)
	s.Register("multiedit", CapabilityFS)
	s.Register("write", CapabilityFS)
	s.Register("view", CapabilityFS|CapabilityObservation)
	s.Register("ls", CapabilityFS|CapabilityObservation)
	s.Register("glob", CapabilityFS|CapabilityObservation)
	s.Register("grep", CapabilityFS|CapabilityObservation)

	s.Register("fetch", CapabilityNetwork)
	s.Register("web_fetch", CapabilityNetwork)
	s.Register("web_search", CapabilityNetwork)
	s.Register("download", CapabilityNetwork)
	s.Register("sourcegraph", CapabilityNetwork)
	s.Register("agentic_fetch", CapabilityNetwork)

	s.Register("lsp_diagnostics", CapabilityCodeIntelligence)
	s.Register("lsp_references", CapabilityCodeIntelligence)
	s.Register("lsp_restart", CapabilityCodeIntelligence)
	s.Register("lsp_definition", CapabilityCodeIntelligence)
	s.Register("lsp_rename", CapabilityCodeIntelligence)
	s.Register("lsp_code_action", CapabilityCodeIntelligence)
	s.Register("lsp_safe_delete", CapabilityCodeIntelligence)
	s.Register("lsp_replace_symbol", CapabilityCodeIntelligence)
	s.Register("lsp_insert_before", CapabilityCodeIntelligence)
	s.Register("lsp_insert_after", CapabilityCodeIntelligence)

	s.Register("job_output", CapabilityExecution|CapabilityObservation)
	s.Register("job_kill", CapabilityExecution)

	s.Register("lcm_grep", CapabilityMemory)
	s.Register("lcm_describe", CapabilityMemory)
	s.Register("lcm_expand", CapabilityMemory)
	s.Register("llm_map", CapabilityMemory)
	s.Register("agentic_map", CapabilityMemory)
	s.Register("map_refresh", CapabilityMemory)

	s.Register("crush_info", CapabilityObservation)
	s.Register("crush_logs", CapabilityObservation)
	s.Register("todos", CapabilityObservation)
	s.Register("list_mcp_resources", CapabilityNetwork|CapabilityObservation)
	s.Register("read_mcp_resource", CapabilityNetwork|CapabilityObservation)

	s.Register("agent", CapabilityExecution)

	s.Register("batch_edit", CapabilityFS)
	s.Register("synthetic_output", CapabilityObservation)
}

// Register adds a tool to the surface with the given capability bitmask.
// If the tool already exists, it is updated. New tools default to visible.
func (s *ToolSurface) Register(name string, caps Capability) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[name] = toolMeta{Capabilities: caps, Visible: true}
}

// Unregister removes a tool from the surface entirely.
func (s *ToolSurface) Unregister(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tools, name)
}

// UpdateCapabilities scans the given context and adjusts tool visibility.
// Tools whose dependencies are satisfied become visible; tools whose
// dependencies are missing become hidden. Tools can reappear on subsequent
// calls if the context changes.
func (s *ToolSurface) UpdateCapabilities(ctx SurfaceContext) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name, meta := range s.tools {
		meta.Visible = s.isVisible(name, meta.Capabilities, ctx)
		s.tools[name] = meta
	}
}

func (s *ToolSurface) isVisible(name string, caps Capability, ctx SurfaceContext) bool {
	if caps&CapabilityCodeIntelligence != 0 && !ctx.HasLSP {
		if isOnlyCodeIntelligence(caps) {
			return false
		}
	}

	if caps&CapabilityMemory != 0 && !ctx.HasLCM {
		if name == "lcm_grep" || name == "lcm_describe" || name == "lcm_expand" {
			return false
		}
		if name == "llm_map" || name == "agentic_map" || name == "map_refresh" {
			if !ctx.HasRepoMap {
				return false
			}
		}
	}

	if (name == "list_mcp_resources" || name == "read_mcp_resource") && !ctx.HasMCP {
		return false
	}

	return true
}

func isOnlyCodeIntelligence(caps Capability) bool {
	return caps == CapabilityCodeIntelligence
}

// GetVisibleTools returns the names of all currently visible tools, sorted
// alphabetically.
func (s *ToolSurface) GetVisibleTools() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var visible []string
	for name, meta := range s.tools {
		if meta.Visible {
			visible = append(visible, name)
		}
	}
	sort.Strings(visible)
	return visible
}

// GetHiddenTools returns the names of all currently hidden tools, sorted
// alphabetically.
func (s *ToolSurface) GetHiddenTools() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var hidden []string
	for name, meta := range s.tools {
		if !meta.Visible {
			hidden = append(hidden, name)
		}
	}
	sort.Strings(hidden)
	return hidden
}

// GetToolCapabilities returns the capability bitmask for the named tool.
// Returns 0 if the tool is not registered.
func (s *ToolSurface) GetToolCapabilities(name string) Capability {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if meta, ok := s.tools[name]; ok {
		return meta.Capabilities
	}
	return 0
}

// IsVisible returns whether a specific tool is currently visible.
func (s *ToolSurface) IsVisible(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if meta, ok := s.tools[name]; ok {
		return meta.Visible
	}
	return false
}

// HasCapability checks whether a tool has a specific capability bit set.
func (s *ToolSurface) HasCapability(name string, cap Capability) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if meta, ok := s.tools[name]; ok {
		return meta.Capabilities&cap != 0
	}
	return false
}

// ToolCount returns the total number of registered tools.
func (s *ToolSurface) ToolCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tools)
}

// AgentPhase represents the current phase of the agent's work.
type AgentPhase int

const (
	// PhasePlanning means the agent is planning/architecting.
	PhasePlanning AgentPhase = iota
	// PhaseEditing means the agent is editing code.
	PhaseEditing
	// PhaseReviewing means the agent is reviewing changes.
	PhaseReviewing
)

var phaseHiddenTools = map[string]bool{
	"edit":      true,
	"multiedit": true,
	"write":     true,
}

// PhaseFilteredTools returns a filtered list of tool names based on the
// current phase. In Planning phase, write tools are hidden. In Editing and
// Reviewing phases, all tools are visible.
func PhaseFilteredTools(allTools []string, phase AgentPhase) []string {
	if phase != PhasePlanning {
		return allTools
	}
	filtered := make([]string, 0, len(allTools))
	for _, name := range allTools {
		if !phaseHiddenTools[name] {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

// ClassifyPhase determines the current agent phase from the prompt content.
// It reuses the prompt classification logic: plan keywords → PhasePlanning,
// edit keywords → PhaseEditing, else PhaseReviewing.
func ClassifyPhase(prompt string) AgentPhase {
	lower := strings.ToLower(prompt)

	editCount := 0
	for _, kw := range surfaceEditKeywords {
		if strings.Contains(lower, kw) {
			editCount++
		}
	}

	planCount := 0
	for _, kw := range surfacePlanKeywords {
		if strings.Contains(lower, kw) {
			planCount++
		}
	}

	total := editCount + planCount
	if total == 0 {
		return PhaseReviewing
	}

	if editCount > planCount {
		return PhaseEditing
	}
	return PhasePlanning
}

// surfaceEditKeywords are keywords that suggest an editing task for surface
// classification. Defined locally to avoid cross-file coupling during
// incremental porting.
var surfaceEditKeywords = []string{
	"edit", "fix", "change", "modify", "implement", "write", "update",
	"refactor", "delete", "remove", "add", "create", "insert", "replace",
	"rename",
}

// surfacePlanKeywords are keywords that suggest a planning task for surface
// classification. Defined locally to avoid cross-file coupling during
// incremental porting.
var surfacePlanKeywords = []string{
	"plan", "design", "architect", "review", "analyze", "think",
	"consider", "evaluate", "assess", "investigate", "explore", "understand",
}
