package explorer

import (
	"context"
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

	// Validate each path has required B3 fields
	for i, path := range inventory.Paths {
		require.NotEmptyf(t, path.ID, "path[%d] must have id", i)
		require.NotEmptyf(t, path.PathKind, "path[%d] must have path_kind", i)
		require.NotEmptyf(t, path.EntryPoint, "path[%d] must have entrypoint", i)
		require.NotEmptyf(t, path.Trigger, "path[%d] must have trigger", i)
		require.NotEmptyf(t, path.ConfigGates, "path[%d] must have config_gates", i)
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
			name: "path missing entrypoint",
			mutate: func(r *RuntimeInventory) {
				r.Paths[0].EntryPoint = ""
			},
			wantError: "entrypoint",
		},
		{
			name: "path missing trigger",
			mutate: func(r *RuntimeInventory) {
				r.Paths[0].Trigger = ""
			},
			wantError: "trigger",
		},
		{
			name: "path missing config_gates",
			mutate: func(r *RuntimeInventory) {
				r.Paths[0].ConfigGates = nil
			},
			wantError: "config_gates",
		},
	}

	for _, tt := range tests {
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
		"MarkdownExplorer",
		"LatexExplorer",
		"SQLiteExplorer",
		"LogsExplorer",
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
	require.NotEmpty(t, paths)
}

func TestDiscoverRuntimePaths_WithLLMTier3(t *testing.T) {
	t.Parallel()

	registry := NewRegistryWithLLM(&mockLLM{response: "test"}, func(ctx context.Context, path, systemPrompt, userPrompt string) (string, error) {
		return "agent result", nil
	})
	paths := DiscoverRuntimePaths(registry, OutputProfileEnhancement)
	require.NotEmpty(t, paths)
}

func TestCheckDrift_NoDrift_DefaultRegistry(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	report, err := CheckDrift(registry, OutputProfileEnhancement)
	require.NoError(t, err)
	require.NotNil(t, report)

	// Verify the report structure.
	require.NotNil(t, report.MissingPaths)
	require.NotNil(t, report.ExtraPaths)
	require.NotNil(t, report.OrderingDrift)

	// No artifact paths should be missing from discovery.
	require.Empty(t, report.MissingPaths)

	// No ordering drift expected for common paths.
	require.Empty(t, report.OrderingDrift)

	// TextExplorer and FallbackExplorer are in the artifact,
	// so they must NOT appear in ExtraPaths.
	for _, extra := range report.ExtraPaths {
		require.NotEqual(t, "TextExplorer", extra.ExplorerName,
			"TextExplorer must not be in ExtraPaths")
		require.NotEqual(t, "FallbackExplorer", extra.ExplorerName,
			"FallbackExplorer must not be in ExtraPaths")
	}
}

func TestCheckDrift_DetectsMissingPath(t *testing.T) {
	t.Parallel()

	// Build a synthetic inventory with an extra in-scope path that has no
	// corresponding explorer in the default registry.
	inventory := &RuntimeInventory{
		Version:         "1",
		GeneratedAt:     "2026-02-26T00:00:00Z",
		DiscoveryMethod: "deterministic_static_plus_runtime",
		Profile:         "parity",
		Paths: []RuntimeIngestionPath{
			{
				ID:       "lcm.tool_output.create",
				PathKind: "ingestion",
				InScope:  true,
				Explorer: "TextExplorer",
			},
			{
				ID:       "lcm.phantom.path",
				PathKind: "ingestion",
				InScope:  true,
				Explorer: "PhantomExplorer",
			},
			{
				ID:       "lcm.oob.ignored",
				PathKind: "ingestion",
				InScope:  false,
				Explorer: "",
			},
		},
	}

	registry := NewRegistry()
	discovered := DiscoverRuntimePaths(registry, OutputProfileParity)
	report := checkDriftAgainst(discovered, inventory)

	// PhantomExplorer is in-scope in the artifact but not in the registry,
	// so it must appear in MissingPaths.
	require.NotEmpty(t, report.MissingPaths, "expected MissingPaths to be non-empty")
	found := false
	for _, mp := range report.MissingPaths {
		if mp.Explorer == "PhantomExplorer" {
			found = true
			break
		}
	}
	require.True(t, found, "PhantomExplorer should be in MissingPaths")

	// The out-of-scope entry (empty explorer) must NOT appear in MissingPaths.
	for _, mp := range report.MissingPaths {
		require.NotEqual(t, "lcm.oob.ignored", mp.ID,
			"out-of-scope entry must be excluded from MissingPaths")
	}
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

	// No artifact paths should be missing.
	require.Empty(t, report.MissingPaths)
	require.Empty(t, report.OrderingDrift)

	// TreeSitterExplorer is parser-specific and must appear in ExtraPaths.
	var foundTS bool
	for _, extra := range report.ExtraPaths {
		if extra.ExplorerName == "TreeSitterExplorer" {
			foundTS = true
		}
	}
	require.True(t, foundTS, "TreeSitterExplorer must be in ExtraPaths for parser-backed registry")
}

func TestCheckDrift_WithLLM(t *testing.T) {
	t.Parallel()

	registry := NewRegistryWithLLM(&mockLLM{response: "test"}, nil)
	report, err := CheckDrift(registry, OutputProfileEnhancement)
	require.NoError(t, err)
	require.NotNil(t, report)

	// No artifact paths should be missing.
	require.Empty(t, report.MissingPaths)
	require.Empty(t, report.OrderingDrift)

	// LLMClient is LLM-specific and must appear in ExtraPaths.
	var foundLLM bool
	for _, extra := range report.ExtraPaths {
		if extra.ExplorerName == "LLMClient" {
			foundLLM = true
		}
	}
	require.True(t, foundLLM, "LLMClient must be in ExtraPaths for LLM-backed registry")
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
	require.Equal(t, "deterministic_static_plus_runtime", inventory.DiscoveryMethod)
	require.Equal(t, "enhancement", inventory.Profile)

	for _, path := range inventory.Paths {
		require.NotEmpty(t, path.ID)
		require.NotEmpty(t, path.PathKind)
		require.NotEmpty(t, path.EntryPoint)
		require.NotEmpty(t, path.Trigger)
		require.NotEmpty(t, path.ConfigGates)
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

	expectedKinds := []string{"ingestion", "retrieval"}
	for _, kind := range expectedKinds {
		require.Truef(t, pathKinds[kind], "expected path_kind %s in inventory", kind)
	}
	require.Len(t, pathKinds, len(expectedKinds), "B3 path_kind taxonomy must be strict")
}

func TestRuntimeInventory_ParityProfileMatch(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	require.Equal(t, "parity", inventory.Profile)
	require.Equal(t, "deterministic_static_plus_runtime", inventory.DiscoveryMethod)

	parityMatrix, err := LoadRuntimePersistenceMatrix(OutputProfileParity)
	require.NoError(t, err)
	require.NotNil(t, parityMatrix)
	require.False(t, parityMatrix.PolicyForExplorer("text").Persist)
	require.False(t, parityMatrix.PolicyForExplorer("binary").Persist)
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
			ID:                          "lcm.tool_output.create",
			PathKind:                    "ingestion",
			EntryPoint:                  "messageDecorator.Create",
			Trigger:                     "tool_output_over_threshold",
			InScope:                     true,
			PersistsExplorationParity:   false,
			PersistsExplorationEnhanced: false,
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
	inventory.TokenCounterMode = "heuristic"
	err = ValidateInventory(inventory)
	require.Error(t, err)
	require.Contains(t, err.Error(), "token_counter_mode")
	inventory.TokenCounterMode = "tokenizer_backed"

	inventory.FixedSeed = 0
	err = ValidateInventory(inventory)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fixed_seed")
	inventory.FixedSeed = 1337

	inventory.Paths[0].PersistsExplorationEnhanced = false
	err = ValidateInventory(inventory)
	require.NoError(t, err)
	inventory.Paths[0].PersistsExplorationEnhanced = true

	inventory.Paths[0].PathKind = "native_binary"
	err = ValidateInventory(inventory)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid path_kind")
}

func TestRuntimeInventory_PersistenceMatrixContracts(t *testing.T) {
	t.Parallel()

	matrix, err := LoadRuntimePersistenceMatrix(OutputProfileEnhancement)
	require.NoError(t, err)
	require.NotNil(t, matrix)
	require.Equal(t, "1", matrix.Version())

	persisted := matrix.PolicyForExplorer("text")
	require.True(t, persisted.Persist, "text path should persist exploration in enhancement profile")
	require.Equal(t, "ingestion", persisted.PathKind)
	require.Equal(t, "lcm.tool_output.create", persisted.PathID)

	nonPersisted := matrix.PolicyForExplorer("binary")
	require.True(t, nonPersisted.Persist, "binary path should persist exploration in enhancement profile")
	require.Equal(t, "ingestion", nonPersisted.PathKind)
	require.Equal(t, "lcm.tool_output.create", nonPersisted.PathID)

	retrievalParity := matrix.PolicyForExplorer("lcm_expand")
	require.False(t, retrievalParity.Persist, "retrieval path should not persist exploration in parity profile")

	retrieval := matrix.PolicyForExplorer("FallbackExplorer")
	require.True(t, retrieval.Persist, "retrieval path should persist exploration in enhancement profile")
	require.Equal(t, "retrieval", retrieval.PathKind)
	require.Equal(t, "lcm.expand.readback", retrieval.PathID)

	unknown := matrix.PolicyForExplorer("definitely_unknown_explorer")
	require.False(t, unknown.Persist, "unknown explorers should default to non-persist")
}

func TestRuntimeInventory_AllExplorerIDsPresent(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	ids := make(map[string]bool)
	for _, path := range inventory.Paths {
		require.NotEmpty(t, path.ID)
		require.NotContains(t, ids, path.ID, "duplicate path id detected: %s", path.ID)
		ids[path.ID] = true
	}

	required := []string{
		"lcm.tool_output.create",
		"lcm.describe.readback",
		"lcm.expand.readback",
	}
	for _, id := range required {
		require.Containsf(t, ids, id, "required B3 path id %s missing", id)
	}
}

func TestRuntimeInventory_CorePathsMustExist(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	byID := make(map[string]RuntimeIngestionPath)
	for _, p := range inventory.Paths {
		byID[p.ID] = p
	}

	create, ok := byID["lcm.tool_output.create"]
	require.True(t, ok)
	require.Equal(t, "ingestion", create.PathKind)
	require.True(t, create.InScope)

	describe, ok := byID["lcm.describe.readback"]
	require.True(t, ok)
	require.Equal(t, "retrieval", describe.PathKind)
	require.True(t, describe.InScope)

	expand, ok := byID["lcm.expand.readback"]
	require.True(t, ok)
	require.Equal(t, "retrieval", expand.PathKind)
	require.True(t, expand.InScope)
}

func TestRuntimeInventory_TierPathMetadata(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	for _, p := range inventory.Paths {
		require.NotContains(t, p.PathKind, "enhancement_tier", "B3 inventory must not use enhancement_tier path kinds")
	}
}

func TestRuntimeInventory_TreeSitterMetadata(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	byID := make(map[string]RuntimeIngestionPath, len(inventory.Paths))
	for _, p := range inventory.Paths {
		byID[p.ID] = p
	}

	create, ok := byID["lcm.tool_output.create"]
	require.True(t, ok)
	require.Equal(t, "messageDecorator.Create", create.EntryPoint)
	require.Contains(t, create.ConfigGates, "DisableLargeToolOutput")
	require.Contains(t, create.ConfigGates, "LargeToolOutputTokenThreshold")
}

func TestRuntimeInventory_EntryPointConsistency(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	for _, path := range inventory.Paths {
		require.NotEmpty(t, path.EntryPoint, "all runtime paths should define entrypoint")
	}
}

func TestRuntimeInventory_PreconditionFields(t *testing.T) {
	t.Parallel()

	inventory, err := LoadRuntimeInventory()
	require.NoError(t, err)

	// All paths should have deterministic B3 gating fields.
	for i, path := range inventory.Paths {
		require.NotEmptyf(t, path.Trigger, "path[%d] trigger should not be empty", i)
		require.NotEmptyf(t, path.ConfigGates, "path[%d] config_gates should not be empty", i)
	}
}
