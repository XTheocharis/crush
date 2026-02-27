package explorer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// RuntimeInventoryPath is the path to the runtime ingestion paths artifact.
	RuntimeInventoryPath = "testdata/parity_volt/runtime_ingestion_paths.v1.json"
)

// RuntimeIngestionPath describes a single runtime ingestion/retrieval path.
type RuntimeIngestionPath struct {
	ID                          string         `json:"id"`
	PathKind                    string         `json:"path_kind"`
	EntryPoint                  string         `json:"entrypoint"`
	Trigger                     string         `json:"trigger"`
	InScope                     bool           `json:"in_scope"`
	PersistsExplorationParity   bool           `json:"persists_exploration_parity"`
	PersistsExplorationEnhanced bool           `json:"persists_exploration_enhanced"`
	ConfigGates                 []string       `json:"config_gates"`
	Description                 string         `json:"description,omitempty"`
	Explorer                    string         `json:"explorer,omitempty"`
	Preconditions               map[string]any `json:"preconditions,omitempty"`
	FallbackChainPosition       any            `json:"fallback_chain_position,omitempty"`
	LLMEnhancement              bool           `json:"llm_enhancement,omitempty"`
	AgentEligible               bool           `json:"agent_eligible,omitempty"`
	ParserRequired              bool           `json:"parser_required,omitempty"`
	Tier                        int            `json:"tier,omitempty"`
}

// UnmarshalJSON supports both B3 `entrypoint` and legacy `entry_point` naming.
func (p *RuntimeIngestionPath) UnmarshalJSON(data []byte) error {
	type runtimeIngestionPathAlias RuntimeIngestionPath
	type runtimeIngestionPathPayload struct {
		runtimeIngestionPathAlias
		EntryPointLegacy string `json:"entry_point"`
	}
	var payload runtimeIngestionPathPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	*p = RuntimeIngestionPath(payload.runtimeIngestionPathAlias)
	if strings.TrimSpace(p.EntryPoint) == "" {
		p.EntryPoint = strings.TrimSpace(payload.EntryPointLegacy)
	}
	return nil
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

// RuntimePersistencePolicy is the persistence contract for a resolved runtime path.
type RuntimePersistencePolicy struct {
	PathID   string
	PathKind string
	Explorer string
	Persist  bool
}

// RuntimePersistenceMatrix resolves path-level persistence behavior by profile.
type RuntimePersistenceMatrix struct {
	version           string
	profile           OutputProfile
	policyByExplorer  map[string]RuntimePersistencePolicy
	defaultNonPersist RuntimePersistencePolicy
}

// LoadRuntimeInventory loads the runtime inventory from disk.
func LoadRuntimeInventory() (*RuntimeInventory, error) {
	content, err := os.ReadFile(RuntimeInventoryPath)
	if err != nil {
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			return nil, fmt.Errorf("failed to read runtime inventory: %w", err)
		}
		altPath := filepath.Join(filepath.Dir(thisFile), RuntimeInventoryPath)
		content, err = os.ReadFile(altPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read runtime inventory: %w", err)
		}
	}

	var inventory RuntimeInventory
	if err := json.Unmarshal(content, &inventory); err != nil {
		return nil, fmt.Errorf("failed to unmarshal runtime inventory: %w", err)
	}

	return &inventory, nil
}

// LoadRuntimePersistenceMatrix builds a versioned runtime persistence matrix
// from the runtime inventory.
func LoadRuntimePersistenceMatrix(profile OutputProfile) (*RuntimePersistenceMatrix, error) {
	inventory, err := LoadRuntimeInventory()
	if err != nil {
		return nil, err
	}
	if err := ValidateInventory(inventory); err != nil {
		return nil, fmt.Errorf("invalid runtime inventory for persistence matrix: %w", err)
	}

	resolvedProfile := strings.ToLower(strings.TrimSpace(string(profile)))
	if resolvedProfile == "" {
		resolvedProfile = strings.ToLower(strings.TrimSpace(inventory.Profile))
	}

	matrix := &RuntimePersistenceMatrix{
		version:          strings.TrimSpace(inventory.Version),
		profile:          OutputProfile(resolvedProfile),
		policyByExplorer: make(map[string]RuntimePersistencePolicy, len(inventory.Paths)),
		defaultNonPersist: RuntimePersistencePolicy{
			Persist: false,
		},
	}

	for _, path := range inventory.Paths {
		explorer := strings.TrimSpace(path.Explorer)
		persist := path.PersistsExplorationEnhanced
		if matrix.profile == OutputProfileParity {
			persist = path.PersistsExplorationParity
		}
		policy := RuntimePersistencePolicy{
			PathID:   strings.TrimSpace(path.ID),
			PathKind: strings.TrimSpace(path.PathKind),
			Explorer: explorer,
			Persist:  persist,
		}
		if explorer != "" {
			matrix.policyByExplorer[explorer] = policy
			if runtimeKey := runtimeExplorerKeyFromInventoryExplorer(explorer); runtimeKey != "" {
				matrix.policyByExplorer[runtimeKey] = policy
			}
		}
		if path.ID == "lcm.tool_output.create" {
			for _, runtimeKey := range []string{
				"binary", "json", "csv", "yaml", "toml", "ini", "xml", "html",
				"markdown", "latex", "sqlite", "logs", "go", "python", "javascript",
				"typescript", "rust", "java", "cpp", "c", "ruby", "treesitter",
				"shell", "text", "fallback",
			} {
				matrix.policyByExplorer[runtimeKey] = policy
			}
		}
	}

	return matrix, nil
}

// Version returns the version string tied to the matrix artifact.
func (m *RuntimePersistenceMatrix) Version() string {
	if m == nil {
		return ""
	}
	return m.version
}

// PolicyForExplorer resolves deterministic persistence policy for explorerUsed.
// Unknown explorers are treated as non-persisted.
func (m *RuntimePersistenceMatrix) PolicyForExplorer(explorerUsed string) RuntimePersistencePolicy {
	if m == nil {
		return RuntimePersistencePolicy{Persist: false}
	}

	normalized := strings.TrimSpace(explorerUsed)
	if normalized == "" {
		return m.defaultNonPersist
	}
	if idx := strings.Index(normalized, "+"); idx >= 0 {
		normalized = strings.TrimSpace(normalized[:idx])
	}
	if policy, ok := m.policyByExplorer[normalized]; ok {
		return policy
	}
	if inventoryName := runtimeExplorerKeyToInventoryExplorer(normalized); inventoryName != "" {
		if policy, ok := m.policyByExplorer[inventoryName]; ok {
			return policy
		}
	}
	return m.defaultNonPersist
}

func runtimeExplorerKeyFromInventoryExplorer(explorer string) string {
	switch strings.TrimSpace(explorer) {
	case "BinaryExplorer":
		return "binary"
	case "JSONExplorer":
		return "json"
	case "CSVExplorer":
		return "csv"
	case "YAMLExplorer":
		return "yaml"
	case "TOMLExplorer":
		return "toml"
	case "INIExplorer":
		return "ini"
	case "XMLExplorer":
		return "xml"
	case "HTMLExplorer":
		return "html"
	case "MarkdownExplorer":
		return "markdown"
	case "LatexExplorer":
		return "latex"
	case "SQLiteExplorer":
		return "sqlite"
	case "LogsExplorer":
		return "logs"
	case "GoExplorer":
		return "go"
	case "PythonExplorer":
		return "python"
	case "JavaScriptExplorer":
		return "javascript"
	case "TypeScriptExplorer":
		return "typescript"
	case "RustExplorer":
		return "rust"
	case "JavaExplorer":
		return "java"
	case "CppExplorer":
		return "cpp"
	case "CExplorer":
		return "c"
	case "RubyExplorer":
		return "ruby"
	case "TreeSitterExplorer":
		return "treesitter"
	case "ShellExplorer":
		return "shell"
	case "TextExplorer":
		return "text"
	case "FallbackExplorer":
		return "fallback"
	default:
		return ""
	}
}

func runtimeExplorerKeyToInventoryExplorer(explorerUsed string) string {
	switch strings.TrimSpace(explorerUsed) {
	case "binary":
		return "BinaryExplorer"
	case "json":
		return "JSONExplorer"
	case "csv":
		return "CSVExplorer"
	case "yaml":
		return "YAMLExplorer"
	case "toml":
		return "TOMLExplorer"
	case "ini":
		return "INIExplorer"
	case "xml":
		return "XMLExplorer"
	case "html":
		return "HTMLExplorer"
	case "markdown":
		return "MarkdownExplorer"
	case "latex":
		return "LatexExplorer"
	case "sqlite":
		return "SQLiteExplorer"
	case "logs":
		return "LogsExplorer"
	case "go":
		return "GoExplorer"
	case "python":
		return "PythonExplorer"
	case "javascript":
		return "JavaScriptExplorer"
	case "typescript":
		return "TypeScriptExplorer"
	case "rust":
		return "RustExplorer"
	case "java":
		return "JavaExplorer"
	case "cpp":
		return "CppExplorer"
	case "c":
		return "CExplorer"
	case "ruby":
		return "RubyExplorer"
	case "treesitter":
		return "TreeSitterExplorer"
	case "shell":
		return "ShellExplorer"
	case "text":
		return "TextExplorer"
	case "fallback":
		return "FallbackExplorer"
	default:
		return ""
	}
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
func GenerateRuntimeInventory(_ *Registry, profile OutputProfile) (*RuntimeInventory, error) {
	resolvedProfile := strings.ToLower(strings.TrimSpace(string(profile)))
	if resolvedProfile == "" {
		resolvedProfile = string(OutputProfileEnhancement)
	}

	paths := []RuntimeIngestionPath{
		{
			ID:                          "lcm.tool_output.create",
			PathKind:                    "ingestion",
			EntryPoint:                  "messageDecorator.Create",
			Trigger:                     "tool_output_over_threshold",
			InScope:                     true,
			PersistsExplorationParity:   false,
			PersistsExplorationEnhanced: true,
			ConfigGates:                 []string{"DisableLargeToolOutput", "LargeToolOutputTokenThreshold"},
		},
		{
			ID:                          "lcm.describe.readback",
			PathKind:                    "retrieval",
			EntryPoint:                  "lcm_describe",
			Trigger:                     "describe_by_file_id",
			InScope:                     true,
			PersistsExplorationParity:   false,
			PersistsExplorationEnhanced: true,
			ConfigGates:                 []string{"session_lineage_scope"},
		},
		{
			ID:                          "lcm.expand.readback",
			PathKind:                    "retrieval",
			EntryPoint:                  "lcm_expand",
			Trigger:                     "expand_by_file_id",
			InScope:                     true,
			PersistsExplorationParity:   false,
			PersistsExplorationEnhanced: true,
			ConfigGates:                 []string{"session_lineage_scope", "sub_agent_only"},
		},
	}

	return &RuntimeInventory{
		Version:                 "1",
		GeneratedAt:             time.Now().UTC().Format(time.RFC3339),
		DiscoveryMethod:         "deterministic_static_plus_runtime",
		Profile:                 resolvedProfile,
		DeterministicMode:       true,
		EnhancementTiersEnabled: "none",
		TokenCounterMode:        "tokenizer_backed",
		FixedSeed:               1337,
		Paths:                   paths,
	}, nil
}

// KindValue returns a path kind value for an explorer.
func KindValue(explorer Explorer) string {
	switch e := explorer.(type) {
	case *BinaryExplorer:
		return "native_binary"
	case *JSONExplorer, *CSVExplorer, *YAMLExplorer, *TOMLExplorer, *INIExplorer, *XMLExplorer, *HTMLExplorer, *MarkdownExplorer, *LatexExplorer, *SQLiteExplorer, *LogsExplorer:
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
	if inventory == nil {
		return fmt.Errorf("runtime inventory is nil")
	}
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

	profile := strings.ToLower(strings.TrimSpace(inventory.Profile))
	if profile == string(OutputProfileParity) {
		if !inventory.DeterministicMode {
			return fmt.Errorf("parity profile requires deterministic_mode=true")
		}
		if strings.ToLower(strings.TrimSpace(inventory.EnhancementTiersEnabled)) != "none" {
			return fmt.Errorf("parity profile requires enhancement_tiers_enabled=none")
		}
		if strings.ToLower(strings.TrimSpace(inventory.TokenCounterMode)) != "tokenizer_backed" {
			return fmt.Errorf("parity profile requires token_counter_mode tokenizer_backed")
		}
		if inventory.FixedSeed <= 0 {
			return fmt.Errorf("parity profile requires positive fixed_seed")
		}
	}

	if len(inventory.Paths) == 0 {
		return fmt.Errorf("paths array must not be empty")
	}

	if err := validateB3RuntimeInventoryPaths(inventory.Paths, profile); err != nil {
		return err
	}

	return nil
}

func validateB3RuntimeInventoryPaths(paths []RuntimeIngestionPath, profile string) error {
	requiredIDs := map[string]bool{
		"lcm.tool_output.create": false,
		"lcm.describe.readback":  false,
		"lcm.expand.readback":    false,
	}
	allowedKinds := map[string]struct{}{
		"ingestion": {},
		"retrieval": {},
	}

	for i, path := range paths {
		if strings.TrimSpace(path.ID) == "" {
			return fmt.Errorf("path[%d]: missing required field: id", i)
		}
		if _, ok := requiredIDs[path.ID]; ok {
			requiredIDs[path.ID] = true
		}

		kind := strings.ToLower(strings.TrimSpace(path.PathKind))
		if _, ok := allowedKinds[kind]; !ok {
			return fmt.Errorf("path[%d]: invalid path_kind %q (expected ingestion or retrieval)", i, path.PathKind)
		}
		if strings.TrimSpace(path.EntryPoint) == "" {
			return fmt.Errorf("path[%d]: missing required field: entrypoint", i)
		}
		if strings.TrimSpace(path.Trigger) == "" {
			return fmt.Errorf("path[%d]: missing required field: trigger", i)
		}
		if len(path.ConfigGates) == 0 {
			return fmt.Errorf("path[%d]: config_gates must not be empty", i)
		}
	}

	for id, present := range requiredIDs {
		if !present {
			return fmt.Errorf("runtime inventory missing required path id: %s", id)
		}
	}
	return nil
}
