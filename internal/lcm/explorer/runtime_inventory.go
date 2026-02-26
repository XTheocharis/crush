package explorer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	// RuntimeInventoryPath is the path to the runtime ingestion paths artifact.
	RuntimeInventoryPath = "testdata/parity_volt/runtime_ingestion_paths.v1.json"
)

// RuntimeIngestionPath describes a single runtime ingestion path.
type RuntimeIngestionPath struct {
	ID                    string         `json:"id"`
	PathKind              string         `json:"path_kind"`
	Description           string         `json:"description"`
	EntryPoint            string         `json:"entry_point"`
	Explorer              string         `json:"explorer"`
	Preconditions         map[string]any `json:"preconditions"`
	FallbackChainPosition any            `json:"fallback_chain_position"`
	LLMEnhancement        bool           `json:"llm_enhancement"`
	AgentEligible         bool           `json:"agent_eligible,omitempty"`
	ParserRequired        bool           `json:"parser_required,omitempty"`
	Tier                  int            `json:"tier,omitempty"`
}

// RuntimeInventory represents the runtime ingestion paths artifact.
type RuntimeInventory struct {
	Version                 string                 `json:"version"`
	GeneratedAt             string                 `json:"generated_at"`
	DiscoveryMethod         string                 `json:"discovery_method"`
	Profile                 string                 `json:"profile"`
	DeterministicMode       bool                   `json:"deterministic_mode,omitempty"`
	EnhancementTiersEnabled string                 `json:"enhancement_tiers_enabled,omitempty"`
	TokenCounterMode        string                 `json:"token_counter_mode,omitempty"`
	FixedSeed               int64                  `json:"fixed_seed,omitempty"`
	Paths                   []RuntimeIngestionPath `json:"paths"`
}

// LoadRuntimeInventory loads the runtime inventory from disk.
func LoadRuntimeInventory() (*RuntimeInventory, error) {
	artifactPath := RuntimeInventoryPath
	content, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read runtime inventory: %w", err)
	}

	var inventory RuntimeInventory
	if err := json.Unmarshal(content, &inventory); err != nil {
		return nil, fmt.Errorf("failed to unmarshal runtime inventory: %w", err)
	}

	return &inventory, nil
}

// DriftReport describes drift between discovered and artifact paths.
type DriftReport struct {
	MissingPaths  []RuntimeIngestionPath
	ExtraPaths    []DiscoveredPath
	OrderingDrift []OrderingDriftEntry
}

// DiscoveredPath represents a path discovered at runtime.
type DiscoveredPath struct {
	ExplorerName string
	Kind         string
	Position     int
}

// OrderingDriftEntry represents a position ordering difference.
type OrderingDriftEntry struct {
	ArtifactPath   RuntimeIngestionPath
	DiscoveredPath DiscoveredPath
	Delta          int
}

// DiscoverRuntimePaths enumerates runtime ingestion paths from a registry.
func DiscoverRuntimePaths(registry *Registry, profile OutputProfile) []DiscoveredPath {
	paths := make([]DiscoveredPath, 0, len(registry.explorers))
	for i, explorer := range registry.explorers {
		path := DiscoveredPath{
			ExplorerName: explorerName(explorer),
			Kind:         KindValue(explorer),
			Position:     i + 1,
		}
		paths = append(paths, path)
	}

	// Add tier 2 enhancement path if LLM is available.
	if registry.llm != nil {
		paths = append(paths, DiscoveredPath{
			ExplorerName: "LLMClient",
			Kind:         "enhancement_tier2",
			Position:     len(paths) + 1,
		})
	}

	// Add tier 3 agent path if agent function is available.
	if registry.agentFn != nil {
		paths = append(paths, DiscoveredPath{
			ExplorerName: "AgentFunc",
			Kind:         "enhancement_tier3",
			Position:     len(paths) + 1,
		})
	}

	// Add tree-sitter path if parser is present.
	if registry.tsParser != nil {
		paths = append(paths, DiscoveredPath{
			ExplorerName: "TreeSitterExplorer",
			Kind:         "code_format_enhanced",
			Position:     9, // Position after HTMLExplorer
		})
	}

	return paths
}

// explorerIdent returns the canonical identifier for an explorer.
func explorerIdent(explorer Explorer) string {
	switch e := explorer.(type) {
	case *BinaryExplorer:
		return "BinaryExplorer"
	case *JSONExplorer:
		return "JSONExplorer"
	case *CSVExplorer:
		return "CSVExplorer"
	case *YAMLExplorer:
		return "YAMLExplorer"
	case *TOMLExplorer:
		return "TOMLExplorer"
	case *INIExplorer:
		return "INIExplorer"
	case *XMLExplorer:
		return "XMLExplorer"
	case *HTMLExplorer:
		return "HTMLExplorer"
	case *GoExplorer:
		return "GoExplorer"
	case *PythonExplorer:
		return "PythonExplorer"
	case *JavaScriptExplorer:
		return "JavaScriptExplorer"
	case *TypeScriptExplorer:
		return "TypeScriptExplorer"
	case *RustExplorer:
		return "RustExplorer"
	case *JavaExplorer:
		return "JavaExplorer"
	case *CppExplorer:
		return "CppExplorer"
	case *CExplorer:
		return "CExplorer"
	case *RubyExplorer:
		return "RubyExplorer"
	case *TreeSitterExplorer:
		return "TreeSitterExplorer"
	case *ShellExplorer:
		return "ShellExplorer"
	case *TextExplorer:
		return "TextExplorer"
	case *FallbackExplorer:
		return "FallbackExplorer"
	default:
		return fmt.Sprintf("UnknownExplorer_%T", e)
	}
}

// explorerName returns the canonical name for an explorer.
func explorerName(explorer Explorer) string {
	return explorerIdent(explorer)
}

// CheckDrift compares discovered paths against the artifact and reports differences.
func CheckDrift(registry *Registry, profile OutputProfile) (*DriftReport, error) {
	inventory, err := LoadRuntimeInventory()
	if err != nil {
		return nil, err
	}

	discovered := DiscoverRuntimePaths(registry, profile)

	report := &DriftReport{
		MissingPaths:  make([]RuntimeIngestionPath, 0),
		ExtraPaths:    make([]DiscoveredPath, 0),
		OrderingDrift: make([]OrderingDriftEntry, 0),
	}

	// Build map of discovered paths for lookups.
	discoveredMap := make(map[string]DiscoveredPath)
	for _, path := range discovered {
		key := fmt.Sprintf("%s:%s", path.ExplorerName, path.Kind)
		discoveredMap[key] = path
	}

	// Check for missing paths.
	for _, artifactPath := range inventory.Paths {
		key := fmt.Sprintf("%s:%s", artifactPath.Explorer, artifactPath.PathKind)
		if _, exists := discoveredMap[key]; !exists {
			report.MissingPaths = append(report.MissingPaths, artifactPath)
		}
	}

	// Check for extra paths.
	artifactMap := make(map[string]bool)
	for _, artifactPath := range inventory.Paths {
		key := fmt.Sprintf("%s:%s", artifactPath.Explorer, artifactPath.PathKind)
		artifactMap[key] = true
	}

	for _, discPath := range discovered {
		key := fmt.Sprintf("%s:%s", discPath.ExplorerName, discPath.Kind)
		if !artifactMap[key] {
			report.ExtraPaths = append(report.ExtraPaths, discPath)
		}
	}

	// Check ordering for common paths.
	for _, discPath := range discovered {
		key := fmt.Sprintf("%s:%s", discPath.ExplorerName, discPath.Kind)
		for _, artifactPath := range inventory.Paths {
			artifactKey := fmt.Sprintf("%s:%s", artifactPath.Explorer, artifactPath.PathKind)
			if key == artifactKey {
				artifactPos, ok := artifactPath.FallbackChainPosition.(float64)
				if ok {
					delta := discPath.Position - int(artifactPos)
					if delta != 0 {
						report.OrderingDrift = append(report.OrderingDrift, OrderingDriftEntry{
							ArtifactPath:   artifactPath,
							DiscoveredPath: discPath,
							Delta:          delta,
						})
					}
				}
				break
			}
		}
	}

	return report, nil
}

// GenerateRuntimeInventory creates a new runtime inventory artifact.
func GenerateRuntimeInventory(registry *Registry, profile OutputProfile) (*RuntimeInventory, error) {
	discovered := DiscoverRuntimePaths(registry, profile)

	paths := make([]RuntimeIngestionPath, 0, len(discovered))
	for _, discPath := range discovered {
		path := RuntimeIngestionPath{
			ID:                    fmt.Sprintf("path_%s", discPath.ExplorerName),
			PathKind:              discPath.Kind,
			Description:           fmt.Sprintf("Discovered runtime path for %s", discPath.ExplorerName),
			EntryPoint:            "RuntimeAdapter.Explore",
			Explorer:              discPath.ExplorerName,
			Preconditions:         map[string]any{},
			FallbackChainPosition: discPath.Position,
			LLMEnhancement:        true,
		}
		paths = append(paths, path)
	}

	return &RuntimeInventory{
		Version:         "1",
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		DiscoveryMethod: "runtime_discovery",
		Profile:         string(profile),
		Paths:           paths,
	}, nil
}

// KindValue returns a path kind value for an explorer.
func KindValue(explorer Explorer) string {
	switch e := explorer.(type) {
	case *BinaryExplorer:
		return "native_binary"
	case *JSONExplorer, *CSVExplorer, *YAMLExplorer, *TOMLExplorer, *INIExplorer, *XMLExplorer, *HTMLExplorer:
		return "data_format_native"
	case *GoExplorer, *PythonExplorer, *JavaScriptExplorer, *TypeScriptExplorer, *RustExplorer, *JavaExplorer, *CppExplorer, *CExplorer, *RubyExplorer:
		return "code_format_native"
	case *TreeSitterExplorer:
		return "code_format_enhanced"
	case *ShellExplorer:
		return "shell_format_native"
	case *TextExplorer:
		return "text_format_generic"
	case *FallbackExplorer:
		return "fallback_final"
	default:
		return fmt.Sprintf("unknown_%T", e)
	}
}

// ValidateInventory ensures the inventory artifact is well-formed and complete.
func ValidateInventory(inventory *RuntimeInventory) error {
	if inventory.Version == "" {
		return fmt.Errorf("missing required field: version")
	}
	if inventory.GeneratedAt == "" {
		return fmt.Errorf("missing required field: generated_at")
	}
	if inventory.DiscoveryMethod == "" {
		return fmt.Errorf("missing required field: discovery_method")
	}
	if inventory.Profile == "" {
		return fmt.Errorf("missing required field: profile")
	}
	if strings.EqualFold(strings.TrimSpace(inventory.Profile), string(OutputProfileParity)) {
		if !inventory.DeterministicMode {
			return fmt.Errorf("parity profile requires deterministic_mode=true")
		}
		if strings.ToLower(strings.TrimSpace(inventory.EnhancementTiersEnabled)) != "none" {
			return fmt.Errorf("parity profile requires enhancement_tiers_enabled=none")
		}
		counterMode := strings.ToLower(strings.TrimSpace(inventory.TokenCounterMode))
		if counterMode != "tokenizer_backed" && counterMode != "heuristic" {
			return fmt.Errorf("parity profile requires token_counter_mode tokenizer_backed or heuristic")
		}
		if inventory.FixedSeed <= 0 {
			return fmt.Errorf("parity profile requires positive fixed_seed")
		}
	}
	if len(inventory.Paths) == 0 {
		return fmt.Errorf("paths array must not be empty")
	}

	for i, path := range inventory.Paths {
		if path.ID == "" {
			return fmt.Errorf("path[%d]: missing required field: id", i)
		}
		if path.PathKind == "" {
			return fmt.Errorf("path[%d]: missing required field: path_kind", i)
		}
		if path.Description == "" {
			return fmt.Errorf("path[%d]: missing required field: description", i)
		}
		if path.EntryPoint == "" {
			return fmt.Errorf("path[%d]: missing required field: entry_point", i)
		}
		if path.Explorer == "" {
			return fmt.Errorf("path[%d]: missing required field: explorer", i)
		}
		if strings.EqualFold(strings.TrimSpace(inventory.Profile), string(OutputProfileParity)) {
			if path.PathKind == "enhancement_tier2" || path.PathKind == "enhancement_tier3" || path.LLMEnhancement {
				return fmt.Errorf("parity profile must not include enhancement paths: %s (%s)", path.ID, path.PathKind)
			}
		}
	}

	return nil
}
