package repomap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConformanceSnapshot captures Aider-side parity sign-off inputs from one run.
type ConformanceSnapshot struct {
	Version                     string `json:"version"`
	AiderCommitSHA              string `json:"aider_commit_sha"`
	ComparatorPath              string `json:"comparator_path"`
	FixturesSHA256              string `json:"fixtures_sha256"`
	ComparatorConfigVersion     string `json:"comparator_config_version"`
	TokenizerSupportVersion     string `json:"tokenizer_support_version"`
	ExplorerFamilyMatrixVersion string `json:"explorer_family_matrix_version"`
	Profile                     string `json:"profile"`
	DeterministicMode           bool   `json:"deterministic_mode"`
	EnhancementTiersEnabled     string `json:"enhancement_tiers_enabled"`
	TokenCounterMode            string `json:"token_counter_mode"`
	FixedSeed                   int64  `json:"fixed_seed"`
	GateAPassed                 bool   `json:"gate_a_passed"`
}

// BuildConformanceSnapshot validates Aider parity prerequisites and returns a
// single-run snapshot that can be embedded into the sign-off bundle.
func BuildConformanceSnapshot(basePath string) (*ConformanceSnapshot, error) {
	if strings.TrimSpace(basePath) == "" {
		basePath = "."
	}

	fixtures, err := LoadParityAiderFixtures(basePath)
	if err != nil {
		return nil, fmt.Errorf("load parity fixtures: %w", err)
	}
	if len(fixtures) == 0 {
		return nil, fmt.Errorf("load parity fixtures: no fixtures")
	}

	fixture := fixtures[0]
	if err := fixture.Validate(true); err != nil {
		return nil, fmt.Errorf("validate fixture %q: %w", fixture.FixtureID, err)
	}

	if err := ValidateAllProtocolArtifacts(); err != nil {
		return nil, fmt.Errorf("validate protocol artifacts: %w", err)
	}

	ts, err := LoadTokenizerSupport()
	if err != nil {
		return nil, fmt.Errorf("load tokenizer support: %w", err)
	}
	matrix, err := LoadExplorerFamilyMatrix()
	if err != nil {
		return nil, fmt.Errorf("load explorer family matrix: %w", err)
	}
	if err := ValidateProtocolArtifact(ts); err != nil {
		return nil, fmt.Errorf("validate tokenizer support: %w", err)
	}
	if err := ValidateProtocolArtifact(matrix); err != nil {
		return nil, fmt.Errorf("validate explorer family matrix: %w", err)
	}

	for _, fx := range fixtures {
		if err := VerifyFixturesIntegrity(fx, basePath); err != nil {
			return nil, fmt.Errorf("fixture integrity check failed for %q: %w", fx.FixtureID, err)
		}
		if fx.Provenance.AiderCommitSHA != fixture.Provenance.AiderCommitSHA {
			return nil, fmt.Errorf("fixture provenance mismatch: aider_commit_sha differs between fixtures")
		}
		if fx.Provenance.ComparatorPath != fixture.Provenance.ComparatorPath {
			return nil, fmt.Errorf("fixture provenance mismatch: comparator_path differs between fixtures")
		}
		if fx.Provenance.FixturesSHA256 != fixture.Provenance.FixturesSHA256 {
			return nil, fmt.Errorf("fixture provenance mismatch: fixtures_sha256 differs between fixtures")
		}
	}

	if !strings.Contains(fixture.Provenance.ComparatorPath, fixture.Provenance.AiderCommitSHA) {
		return nil, fmt.Errorf("comparator_path does not include aider_commit_sha")
	}

	comparatorCfg, err := loadComparatorConfig(basePath)
	if err != nil {
		return nil, fmt.Errorf("load comparator config: %w", err)
	}
	if !gitSHAPattern.MatchString(strings.TrimSpace(comparatorCfg.AiderReference.TargetCommitSHA)) {
		return nil, fmt.Errorf("invalid comparator target_commit_sha: expected 40-char hex commit SHA")
	}
	if isPlaceholderCommitSHA(comparatorCfg.AiderReference.TargetCommitSHA) {
		return nil, fmt.Errorf("invalid comparator target_commit_sha: placeholder value is not allowed")
	}
	if comparatorCfg.AiderReference.TargetCommitSHA != fixture.Provenance.AiderCommitSHA {
		return nil, fmt.Errorf("comparator target_commit_sha does not match fixture aider_commit_sha")
	}

	profile := parityProfileFromFixture(fixture)
	if profile == nil {
		return nil, fmt.Errorf("fixture %q has no parity profile", fixture.FixtureID)
	}

	if err := RunParityHarnessPreflight(fixture.Provenance, ParityPreflightOptions{
		RequireComparatorTuple: true,
		CorpusBasePath:         basePath,
		Profile: &ParityPreflightProfile{
			ID:                      profile.ProfileID,
			TokenBudget:             profile.TokenBudget,
			RepeatRuns:              profile.RepeatRuns,
			DeterministicMode:       profile.DeterministicMode,
			EnhancementTiersEnabled: profile.EnhancementTiersEnabled,
			TokenCounterMode:        profile.TokenCounterMode,
			FixedSeed:               profile.FixedSeed,
			ParityMode:              profile.ParityMode,
		},
	}); err != nil {
		return nil, fmt.Errorf("repomap parity preflight: %w", err)
	}

	comparatorVersion := strings.TrimSpace(comparatorCfg.ParityProtocol.Version)
	if comparatorVersion == "" {
		comparatorVersion = strings.TrimSpace(comparatorCfg.Version)
	}
	if comparatorVersion == "" {
		return nil, fmt.Errorf("missing comparator protocol version")
	}

	return &ConformanceSnapshot{
		Version:                     "1",
		AiderCommitSHA:              fixture.Provenance.AiderCommitSHA,
		ComparatorPath:              fixture.Provenance.ComparatorPath,
		FixturesSHA256:              fixture.Provenance.FixturesSHA256,
		ComparatorConfigVersion:     comparatorVersion,
		TokenizerSupportVersion:     ts.Version,
		ExplorerFamilyMatrixVersion: matrix.Version,
		Profile:                     profile.ProfileID,
		DeterministicMode:           profile.DeterministicMode,
		EnhancementTiersEnabled:     strings.ToLower(strings.TrimSpace(profile.EnhancementTiersEnabled)),
		TokenCounterMode:            strings.ToLower(strings.TrimSpace(profile.TokenCounterMode)),
		FixedSeed:                   profile.FixedSeed,
		GateAPassed:                 true,
	}, nil
}

func parityProfileFromFixture(fx ParityAiderFixture) *ParityProfile {
	for i := range fx.Profiles {
		if fx.Profiles[i].ParityMode {
			return &fx.Profiles[i]
		}
	}
	return nil
}

type comparatorConfigArtifact struct {
	Version        string `json:"version"`
	AiderReference struct {
		TargetCommitSHA string `json:"target_commit_sha"`
	} `json:"aider_reference"`
	ParityProtocol struct {
		Version string `json:"version"`
	} `json:"parity_protocol"`
}

func loadComparatorConfig(basePath string) (*comparatorConfigArtifact, error) {
	path := filepath.Join(basePath, "testdata", "parity_aider", "comparator_config.v1.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg comparatorConfigArtifact
	if err := json.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
