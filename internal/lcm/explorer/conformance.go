package explorer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConformanceSnapshot captures Volt-side parity sign-off inputs from one run.
type ConformanceSnapshot struct {
	Version                     string `json:"version"`
	VoltCommitSHA               string `json:"volt_commit_sha"`
	ComparatorPath              string `json:"comparator_path"`
	FixturesSHA256              string `json:"fixtures_sha256"`
	FixtureIndexVersion         string `json:"fixture_index_version"`
	RuntimeInventoryVersion     string `json:"runtime_inventory_version"`
	TokenizerSupportVersion     string `json:"tokenizer_support_version"`
	ExplorerFamilyMatrixVersion string `json:"explorer_family_matrix_version"`
	Profile                     string `json:"profile"`
	DeterministicMode           bool   `json:"deterministic_mode"`
	EnhancementTiersEnabled     string `json:"enhancement_tiers_enabled"`
	TokenCounterMode            string `json:"token_counter_mode"`
	FixedSeed                   int64  `json:"fixed_seed"`
	GateBPassed                 bool   `json:"gate_b_passed"`
}

// BuildConformanceSnapshot validates Volt parity prerequisites and returns a
// single-run snapshot that can be embedded into the sign-off bundle.
func resolveConformanceBasePath(basePath string) string {
	candidates := []string{
		basePath,
		filepath.Join(basePath, "internal", "lcm", "explorer"),
		filepath.Join(basePath, "..", "lcm", "explorer"),
		filepath.Join(basePath, "..", "..", "internal", "lcm", "explorer"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "testdata", "parity_volt", "fixture_index.v1.json")); err == nil {
			return candidate
		}
	}
	return basePath
}

func loadTokenizerSupportAtBase(basePath string) (*TokenizerSupport, error) {
	path := filepath.Join(basePath, "testdata", "parity_volt", "tokenizer_support.v1.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ts TokenizerSupport
	if err := json.Unmarshal(content, &ts); err != nil {
		return nil, err
	}
	return &ts, nil
}

func loadExplorerFamilyMatrixAtBase(basePath string) (*ExplorerFamilyMatrix, error) {
	path := filepath.Join(basePath, "testdata", "parity_volt", "explorer_family_matrix.v1.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var matrix ExplorerFamilyMatrix
	if err := json.Unmarshal(content, &matrix); err != nil {
		return nil, err
	}
	return &matrix, nil
}

func loadRuntimeInventoryAtBase(basePath string) (*RuntimeInventory, error) {
	path := filepath.Join(basePath, "testdata", "parity_volt", "runtime_ingestion_paths.v1.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var inventory RuntimeInventory
	if err := json.Unmarshal(content, &inventory); err != nil {
		return nil, err
	}
	return &inventory, nil
}

type explorerParityProvenanceFile struct {
	VoltCommitSHA  string `json:"volt_commit_sha"`
	ComparatorPath string `json:"comparator_path"`
	FixturesSHA256 string `json:"fixtures_sha256"`
}

func loadParityProvenanceBundle(path string) (*explorerParityProvenanceFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bundle explorerParityProvenanceFile
	if err := json.Unmarshal(content, &bundle); err != nil {
		return nil, err
	}
	return &bundle, nil
}

func validateProvenanceBundleConsistency(path string, expected ParityProvenanceBundle) error {
	bundle, err := loadParityProvenanceBundle(path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(bundle.VoltCommitSHA) != strings.TrimSpace(expected.VoltCommitSHA) {
		return fmt.Errorf("volt_commit_sha mismatch between fixture index and provenance bundle")
	}
	if strings.TrimSpace(bundle.ComparatorPath) != strings.TrimSpace(expected.ComparatorPath) {
		return fmt.Errorf("comparator_path mismatch between fixture index and provenance bundle")
	}
	if strings.TrimSpace(bundle.FixturesSHA256) != strings.TrimSpace(expected.FixturesSHA256) {
		return fmt.Errorf("fixtures_sha256 mismatch between fixture index and provenance bundle")
	}
	return nil
}

func tokenizerTupleSupported(ts *TokenizerSupport, tokenizerID, tokenizerVersion string) bool {
	for _, fam := range ts.SupportedFamilies {
		if fam.TokenizerID == tokenizerID && fam.TokenizerVersion == tokenizerVersion {
			return true
		}
	}
	return false
}

func BuildConformanceSnapshot(basePath string) (*ConformanceSnapshot, error) {
	if strings.TrimSpace(basePath) == "" {
		basePath = "."
	}
	basePath = resolveConformanceBasePath(basePath)

	cfg := NewDefaultParityFixtureConfig(basePath)
	loader := NewParityFixtureLoader(cfg)
	index, err := loader.LoadIndex()
	if err != nil {
		return nil, fmt.Errorf("load parity fixture index: %w", err)
	}

	ts, err := loadTokenizerSupportAtBase(basePath)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer support: %w", err)
	}
	if err := ValidateProtocolArtifact(ts); err != nil {
		return nil, fmt.Errorf("validate tokenizer support: %w", err)
	}

	matrix, err := loadExplorerFamilyMatrixAtBase(basePath)
	if err != nil {
		return nil, fmt.Errorf("load explorer family matrix: %w", err)
	}
	if err := ValidateProtocolArtifact(matrix); err != nil {
		return nil, fmt.Errorf("validate explorer family matrix: %w", err)
	}

	inventory, err := loadRuntimeInventoryAtBase(basePath)
	if err != nil {
		return nil, fmt.Errorf("load runtime inventory: %w", err)
	}
	if err := ValidateInventory(inventory); err != nil {
		return nil, fmt.Errorf("validate runtime inventory: %w", err)
	}

	bundle := ParityProvenanceBundle{
		VoltCommitSHA:     index.Metadata.VoltCommitSHA,
		ComparatorPath:    index.Metadata.ComparatorPath,
		FixturesSHA256:    index.Metadata.FixturesSHA256,
		GrepASTProvenance: "grep-ast@v1.2.3",
		TokenizerID:       "cl100k_base",
		TokenizerVersion:  "v0.1.0",
	}
	if !strings.Contains(index.Metadata.ComparatorPath, index.Metadata.VoltCommitSHA) {
		return nil, fmt.Errorf("volt parity comparator_path does not include volt_commit_sha")
	}
	provenanceBundlePath := filepath.Join(basePath, "testdata", "parity_volt", "provenance_bundle.v1.json")
	if err := validateProvenanceBundleConsistency(provenanceBundlePath, bundle); err != nil {
		return nil, fmt.Errorf("volt parity provenance bundle consistency failed: %w", err)
	}

	if err := VerifyFixturesIntegrity(index, cfg); err != nil {
		return nil, fmt.Errorf("volt fixture integrity validation failed: %w", err)
	}

	profile := &ParityPreflightProfile{
		ID:                      "conformance-volt",
		TokenBudget:             4096,
		RepeatRuns:              2,
		DeterministicMode:       true,
		EnhancementTiersEnabled: "none",
		TokenCounterMode:        "tokenizer_backed",
		FixedSeed:               1337,
		ParityMode:              true,
	}
	if strings.EqualFold(strings.TrimSpace(index.Metadata.Profile), "parity") {
		profile.DeterministicMode = index.Metadata.DeterministicMode
		profile.EnhancementTiersEnabled = index.Metadata.EnhancementTiersEnabled
		if strings.TrimSpace(index.Metadata.TokenCounterMode) != "" {
			profile.TokenCounterMode = index.Metadata.TokenCounterMode
		}
		if index.Metadata.FixedSeed > 0 {
			profile.FixedSeed = index.Metadata.FixedSeed
		}
	}

	if err := bundle.Validate(true); err != nil {
		return nil, fmt.Errorf("volt parity provenance validation failed: %w", err)
	}
	if err := validateParityPreflightProfile(profile); err != nil {
		return nil, fmt.Errorf("volt parity profile validation failed: %w", err)
	}
	if !tokenizerTupleSupported(ts, bundle.TokenizerID, bundle.TokenizerVersion) {
		return nil, fmt.Errorf("volt parity tokenizer tuple validation failed: tokenizer tuple %q@%q is not present in tokenizer support artifact", bundle.TokenizerID, bundle.TokenizerVersion)
	}
	fixtures, err := loader.LoadAllFixtures()
	if err != nil {
		return nil, fmt.Errorf("volt parity corpus readiness failed: %w", err)
	}
	if len(fixtures) == 0 {
		return nil, fmt.Errorf("volt parity corpus readiness failed: no parity fixtures found")
	}

	return &ConformanceSnapshot{
		Version:                     "1",
		VoltCommitSHA:               index.Metadata.VoltCommitSHA,
		ComparatorPath:              index.Metadata.ComparatorPath,
		FixturesSHA256:              index.Metadata.FixturesSHA256,
		FixtureIndexVersion:         index.Metadata.Version,
		RuntimeInventoryVersion:     inventory.Version,
		TokenizerSupportVersion:     ts.Version,
		ExplorerFamilyMatrixVersion: matrix.Version,
		Profile:                     strings.TrimSpace(index.Metadata.Profile),
		DeterministicMode:           profile.DeterministicMode,
		EnhancementTiersEnabled:     strings.ToLower(strings.TrimSpace(profile.EnhancementTiersEnabled)),
		TokenCounterMode:            strings.ToLower(strings.TrimSpace(profile.TokenCounterMode)),
		FixedSeed:                   profile.FixedSeed,
		GateBPassed:                 true,
	}, nil
}
