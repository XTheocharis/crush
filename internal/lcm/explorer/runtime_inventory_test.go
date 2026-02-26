package explorer

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadRuntimeInventory_ValidArtifact(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)
	require.NotNil(t, inventory)

	// Validate required top-level fields
	require.Equal(t, "1", inventory.Version)
	require.NotEmpty(t, inventory.GeneratedAt)
	require.NotEmpty(t, inventory.DiscoveryMethod)
	require.NotEmpty(t, inventory.Profile)

	// Validate paths array
	require.NotEmpty(t, inventory.Paths)
	require.Greater(t, len(inventory.Paths), 10, "should have multiple paths")

	// Validate each path has required fields
	for i, path := range inventory.Paths {
		require.NotEmptyf(t, path.ID, "path[%d] must have id", i)
		require.NotEmptyf(t, path.PathKind, "path[%d] must have path_kind", i)
		require.NotEmptyf(t, path.Description, "path[%d] must have description", i)
		require.NotEmptyf(t, path.EntryPoint, "path[%d] must have entry_point", i)
		require.NotEmptyf(t, path.Explorer, "path[%d] must have explorer", i)
		require.NotNilf(t, path.Preconditions, "path[%d] must have preconditions", i)
	}
}

func TestValidateInventory_WellFormed(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)
	require.NoError(t, ValidateInventory(inventory))
}

func TestValidateInventory_MissingFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*RuntimeInventory)
		wantError string
	}{
		{
			name: "missing version",
			mutate: func(r *RuntimeInventory) {
				r.Version = ""
			},
			wantError: "version",
		},
		{
			name: "missing generated_at",
			mutate: func(r *RuntimeInventory) {
				r.GeneratedAt = ""
			},
			wantError: "generated_at",
		},
		{
			name: "missing discovery_method",
			mutate: func(r *RuntimeInventory) {
				r.DiscoveryMethod = ""
			},
			wantError: "discovery_method",
		},
		{
			name: "missing profile",
			mutate: func(r *RuntimeInventory) {
				r.Profile = ""
			},
			wantError: "profile",
		},
		{
			name: "empty paths",
			mutate: func(r *RuntimeInventory) {
				r.Paths = []RuntimeIngestionPath{}
			},
			wantError: "paths array must not be empty",
		},
		{
			name: "path missing id",
			mutate: func(r *RuntimeInventory) {
				r.Paths[0].ID = ""
			},
			wantError: "id",
		},
		{
			name: "path missing path_kind",
			mutate: func(r *RuntimeInventory) {
				r.Paths[0].PathKind = ""
			},
			wantError: "path_kind",
		},
		{
			name: "path missing description",
			mutate: func(r *RuntimeInventory) {
				r.Paths[0].Description = ""
			},
			wantError: "description",
		},
		{
			name: "path missing entry_point",
			mutate: func(r *RuntimeInventory) {
				r.Paths[0].EntryPoint = ""
			},
			wantError: "entry_point",
		},
		{
			name: "path missing explorer",
			mutate: func(r *RuntimeInventory) {
				r.Paths[0].Explorer = ""
			},
			wantError: "explorer",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inventory, _ := LoadRuntimeInventory()
			tt.mutate(inventory)
			err := ValidateInventory(inventory)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestDiscoverRuntimePaths_WithoutParser(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	paths := DiscoverRuntimePaths(registry, OutputProfileEnhancement)

	require.NotEmpty(t, paths)

	// Check for expected explorers
	explorerNames := make(map[string]bool)
	for _, path := range paths {
		explorerNames[path.ExplorerName] = true
	}

	// Core explorers should always be present
	expectedExplorers := []string{
		"BinaryExplorer",
		"JSONExplorer",
		"CSVExplorer",
		"YAMLExplorer",
		"TOMLExplorer",
		"INIExplorer",
		"XMLExplorer",
		"HTMLExplorer",
		"GoExplorer",
		"PythonExplorer",
		"JavaScriptExplorer",
		"TypeScriptExplorer",
		"RustExplorer",
		"JavaExplorer",
		"CppExplorer",
		"CExplorer",
		"RubyExplorer",
		"ShellExplorer",
		"TextExplorer",
		"FallbackExplorer",
	}

	for _, exp := range expectedExplorers {
		require.Truef(t, explorerNames[exp], "expected explorer %s in discovered paths", exp)
	}

	// Tree-sitter should NOT be present without parser
	require.False(t, explorerNames["TreeSitterExplorer"], "TreeSitterExplorer should not be present without parser")
}

func TestDiscoverRuntimePaths_WithParser(t *testing.T) {
	t.Parallel()

	mockParser := &mockTreeSitterParser{
		supports: map[string]bool{},
		hasTags:  map[string]bool{},
	}
	registry := NewRegistry(WithTreeSitter(mockParser))
	paths := DiscoverRuntimePaths(registry, OutputProfileEnhancement)

	require.NotEmpty(t, paths)

	// Tree-sitter should be present with parser
	explorerNames := make(map[string]bool)
	for _, path := range paths {
		explorerNames[path.ExplorerName] = true
	}
	require.True(t, explorerNames["TreeSitterExplorer"], "TreeSitterExplorer should be present with parser")
}

func TestDiscoverRuntimePaths_WithLLMTier2(t *testing.T) {
	t.Parallel()

	registry := NewRegistryWithLLM(&mockLLM{response: "test"}, nil)
	paths := DiscoverRuntimePaths(registry, OutputProfileEnhancement)

	// Check for tier 2 enhancement path
	hasTier2 := false
	for _, path := range paths {
		if path.Kind == "enhancement_tier2" && path.ExplorerName == "LLMClient" {
			hasTier2 = true
			break
		}
	}
	require.True(t, hasTier2, "tier 2 LLM enhancement path should be present with LLM client")
}

func TestDiscoverRuntimePaths_WithLLMTier3(t *testing.T) {
	t.Parallel()

	registry := NewRegistryWithLLM(&mockLLM{response: "test"}, func(ctx context.Context, path, systemPrompt, userPrompt string) (string, error) {
		return "agent result", nil
	})
	paths := DiscoverRuntimePaths(registry, OutputProfileEnhancement)

	// Check for tier 3 agent path
	hasTier3 := false
	for _, path := range paths {
		if path.Kind == "enhancement_tier3" && path.ExplorerName == "AgentFunc" {
			hasTier3 = true
			break
		}
	}
	require.True(t, hasTier3, "tier 3 agent path should be present with agent function")
}

func TestCheckDrift_NoDrift_DefaultRegistry(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	report, err := CheckDrift(registry, OutputProfileEnhancement)
	require.NoError(t, err)
	require.NotNil(t, report)

	// Verify the report structure
	require.NotNil(t, report.MissingPaths)
	require.NotNil(t, report.ExtraPaths)
	require.NotNil(t, report.OrderingDrift)
}

func TestCheckDrift_WithParser(t *testing.T) {
	t.Parallel()

	mockParser := &mockTreeSitterParser{
		supports: map[string]bool{},
		hasTags:  map[string]bool{},
	}
	registry := NewRegistry(WithTreeSitter(mockParser))
	report, err := CheckDrift(registry, OutputProfileEnhancement)
	require.NoError(t, err)
	require.NotNil(t, report)
}

func TestCheckDrift_WithLLM(t *testing.T) {
	t.Parallel()

	registry := NewRegistryWithLLM(&mockLLM{response: "test"}, nil)
	report, err := CheckDrift(registry, OutputProfileEnhancement)
	require.NoError(t, err)
	require.NotNil(t, report)
}

func TestGenerateRuntimeInventory(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	inventory, err := GenerateRuntimeInventory(registry, OutputProfileEnhancement)
	require.NoError(t, err)
	require.NotNil(t, inventory)

	// Validate generated inventory structure
	require.NoError(t, ValidateInventory(inventory))
	require.Equal(t, "1", inventory.Version)
	require.NotEmpty(t, inventory.GeneratedAt)
	require.Equal(t, "runtime_discovery", inventory.DiscoveryMethod)
	require.Equal(t, "enhancement", inventory.Profile)

	// All paths should have required fields
	for _, path := range inventory.Paths {
		require.NotEmpty(t, path.ID)
		require.NotEmpty(t, path.PathKind)
		require.NotEmpty(t, path.EntryPoint)
		require.NotEmpty(t, path.Explorer)
	}
}

func TestRuntimeInventory_AllPathKindsCovered(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	// Collect all path kinds
	pathKinds := make(map[string]bool)
	for _, path := range inventory.Paths {
		pathKinds[path.PathKind] = true
	}

	// Expected path kinds based on enhancement inventory requirements.
	expectedKinds := []string{
		"native_binary",
		"data_format_native",
		"code_format_native",
		"code_format_enhanced",
		"shell_format_native",
		"text_format_generic",
		"fallback_final",
		"enhancement_tier2",
		"enhancement_tier3",
	}

	for _, kind := range expectedKinds {
		require.Truef(t, pathKinds[kind], "expected path_kind %s in inventory", kind)
	}
}

func TestRuntimeInventory_ParityProfileMatch(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	// Artifact currently tracks enhancement profile for non-gate parity operations.
	require.Equal(t, "enhancement", inventory.Profile)
	require.Equal(t, "static_code_analysis", inventory.DiscoveryMethod)
}

func TestValidateInventory_ParityDeterministicScoringRules(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	inventory.Profile = string(OutputProfileParity)
	inventory.DeterministicMode = true
	inventory.EnhancementTiersEnabled = "none"
	inventory.TokenCounterMode = "tokenizer_backed"
	inventory.FixedSeed = 1337

	inventory.Paths = []RuntimeIngestionPath{
		{
			ID:                    "path_binary_direct",
			PathKind:              "native_binary",
			Description:           "binary",
			EntryPoint:            "RuntimeAdapter.Explore",
			Explorer:              "BinaryExplorer",
			Preconditions:         map[string]any{"ok": true},
			FallbackChainPosition: 1,
			LLMEnhancement:        false,
		},
	}
	require.NoError(t, ValidateInventory(inventory))

	inventory.DeterministicMode = false
	err = ValidateInventory(inventory)
	require.Error(t, err)
	require.Contains(t, err.Error(), "deterministic_mode")
	inventory.DeterministicMode = true

	inventory.EnhancementTiersEnabled = "llm"
	err = ValidateInventory(inventory)
	require.Error(t, err)
	require.Contains(t, err.Error(), "enhancement_tiers_enabled")
	inventory.EnhancementTiersEnabled = "none"

	inventory.TokenCounterMode = ""
	err = ValidateInventory(inventory)
	require.Error(t, err)
	require.Contains(t, err.Error(), "token_counter_mode")
	inventory.TokenCounterMode = "tokenizer_backed"

	inventory.FixedSeed = 0
	err = ValidateInventory(inventory)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fixed_seed")
	inventory.FixedSeed = 1337

	inventory.Paths[0].LLMEnhancement = true
	err = ValidateInventory(inventory)
	require.Error(t, err)
	require.Contains(t, err.Error(), "enhancement paths")
	inventory.Paths[0].LLMEnhancement = false

	inventory.Paths[0].PathKind = "enhancement_tier2"
	err = ValidateInventory(inventory)
	require.Error(t, err)
	require.Contains(t, err.Error(), "enhancement paths")
}

func TestRuntimeInventory_PersistenceMatrixContracts(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	byID := make(map[string]RuntimeIngestionPath, len(inventory.Paths))
	for _, p := range inventory.Paths {
		byID[p.ID] = p
	}

	persisted, ok := byID["path_text_generic"]
	require.True(t, ok, "path_text_generic must exist in runtime inventory")
	require.True(t, persisted.LLMEnhancement, "text path should persist exploration in enhancement profile")
	require.Equal(t, "text_format_generic", persisted.PathKind)

	nonPersisted, ok := byID["path_binary_direct"]
	require.True(t, ok, "path_binary_direct must exist in runtime inventory")
	require.False(t, nonPersisted.LLMEnhancement, "binary path should remain non-persisted in enhancement profile")
	require.Equal(t, "native_binary", nonPersisted.PathKind)
}

func TestRuntimeInventory_AllExplorerIDsPresent(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	// Collect all IDs
	ids := make(map[string]bool)
	for _, path := range inventory.Paths {
		require.NotEmpty(t, path.ID)
		require.NotContains(t, ids, path.ID, "duplicate path id detected: %s", path.ID)
		ids[path.ID] = true
	}

	// All IDs should start with "path_"
	for _, path := range inventory.Paths {
		require.Truef(t, strings.HasPrefix(path.ID, "path_"), "path id %s should start with 'path_'", path.ID)
	}
}

func TestRuntimeInventory_CorePathsMustExist(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	// Build a map of explorer name to kind
	explorerMap := make(map[string]string)
	for _, path := range inventory.Paths {
		explorerMap[path.Explorer] = path.PathKind
	}

	// Core explorers must exist in the artifact
	requiredExplorers := map[string]string{
		"BinaryExplorer":   "native_binary",
		"JSONExplorer":     "data_format_native",
		"CSVExplorer":      "data_format_native",
		"YAMLExplorer":     "data_format_native",
		"ShellExplorer":    "shell_format_native",
		"TextExplorer":     "text_format_generic",
		"FallbackExplorer": "fallback_final",
	}

	for explorer, expectedKind := range requiredExplorers {
		kind, exists := explorerMap[explorer]
		require.Truef(t, exists, "required explorer %s must exist in inventory", explorer)
		require.Equalf(t, expectedKind, kind, "explorer %s should have path_kind %s", explorer, expectedKind)
	}
}

func TestRuntimeInventory_TierPathMetadata(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	// Find tier 2 and tier 3 paths in enhancement profile inventory.
	var tier2Path, tier3Path *RuntimeIngestionPath
	for i := range inventory.Paths {
		if inventory.Paths[i].PathKind == "enhancement_tier2" {
			tier2Path = &inventory.Paths[i]
		}
		if inventory.Paths[i].PathKind == "enhancement_tier3" {
			tier3Path = &inventory.Paths[i]
		}
	}

	// Validate tier 2 path
	require.NotNil(t, tier2Path, "tier 2 path must exist")
	require.Equal(t, 2, tier2Path.Tier, "tier 2 path must have tier field set to 2")

	// Validate tier 3 path
	require.NotNil(t, tier3Path, "tier 3 path must exist")
	require.Equal(t, 3, tier3Path.Tier, "tier 3 path must have tier field set to 3")
}

func TestRuntimeInventory_TreeSitterMetadata(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	// Find TreeSitterExplorer path
	var tsPath *RuntimeIngestionPath
	for i := range inventory.Paths {
		if inventory.Paths[i].Explorer == "TreeSitterExplorer" {
			tsPath = &inventory.Paths[i]
			break
		}
	}

	// Validate TreeSitter path metadata
	require.NotNil(t, tsPath, "TreeSitterExplorer path must exist")
	require.True(t, tsPath.ParserRequired, "TreeSitterExplorer must have parser_required=true")
	require.Equal(t, "code_format_enhanced", tsPath.PathKind)
}

func TestRuntimeInventory_EntryPointConsistency(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	// All paths should have the same entry point
	for _, path := range inventory.Paths {
		require.Equal(t, "RuntimeAdapter.Explore", path.EntryPoint,
			"all runtime paths should use RuntimeAdapter.Explore as entry_point")
	}
}

func TestRuntimeInventory_PreconditionFields(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	// All paths should have non-empty preconditions
	for i, path := range inventory.Paths {
		require.NotNil(t, path.Preconditions, "path[%d] must have preconditions map", i)
		// Preconditions map should not be empty
		require.Greater(t, len(path.Preconditions), 0,
			"path[%d] preconditions should not be empty", i)
	}
}
