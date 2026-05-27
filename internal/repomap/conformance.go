package repomap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
	GateAEvidencePath           string `json:"gate_a_evidence_path"`
	RunID                       string `json:"run_id"`
}

// BuildConformanceSnapshot validates Aider parity prerequisites and returns a
// single-run snapshot that can be embedded into the sign-off bundle.
func BuildConformanceSnapshot(basePath string) (*ConformanceSnapshot, error) {
	runID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	return buildConformanceSnapshotWithRunID(basePath, runID)
}

func BuildConformanceSnapshotWithRunID(basePath, runID string) (*ConformanceSnapshot, error) {
	return buildConformanceSnapshotWithRunID(basePath, runID)
}

func buildConformanceSnapshotWithRunID(basePath, runID string) (*ConformanceSnapshot, error) {
	if strings.TrimSpace(basePath) == "" {
		basePath = "."
	}
	basePath = resolveRepomapConformanceBasePath(basePath)
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("missing run id")
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

	gateAEvidence, gateAErr := executeGateAForConformance(basePath, runID)

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
		GateAPassed:                 gateAErr == nil,
		GateAEvidencePath:           gateAEvidence,
		RunID:                       runID,
	}, nil
}

func executeGateAForConformance(basePath, runID string) (string, error) {
	workDir := resolveRepomapConformanceBasePath(basePath)
	evidenceDir := filepath.Join(os.TempDir(), "crush-parity-evidence", "repomap")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		return "", fmt.Errorf("create gate A evidence directory: %w", err)
	}
	evidencePath := filepath.Join(evidenceDir, "gate_a_evidence."+runID+".json")
	if absPath, err := filepath.Abs(evidencePath); err == nil {
		evidencePath = absPath
	}

	cmd := exec.CommandContext(context.Background(), "go", "test", "-run", "TestParityGateAAggregate", "-count=1", ".")
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	passed := err == nil

	sum := sha256.Sum256(out)
	payload := map[string]any{
		"run_id":         runID,
		"command":        "go test -run TestParityGateAAggregate -count=1 .",
		"passed":         passed,
		"output_sha256":  hex.EncodeToString(sum[:]),
		"output_excerpt": string(out),
	}
	content, mErr := json.MarshalIndent(payload, "", "  ")
	if mErr != nil {
		return "", fmt.Errorf("marshal gate A evidence: %w", mErr)
	}
	if wErr := os.WriteFile(evidencePath, content, 0o644); wErr != nil {
		return "", fmt.Errorf("write gate A evidence: %w", wErr)
	}
	if err != nil {
		return evidencePath, fmt.Errorf("gate A aggregate execution failed: %w", err)
	}
	return evidencePath, nil
}

func resolveRepomapConformanceBasePath(basePath string) string {
	candidates := []string{
		basePath,
		filepath.Join(basePath, "internal", "repomap"),
		filepath.Join(basePath, "..", "repomap"),
		filepath.Join(basePath, "..", "..", "internal", "repomap"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "testdata", "parity_aider", "comparator_config.v1.json")); err == nil {
			return candidate
		}
	}
	return basePath
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
