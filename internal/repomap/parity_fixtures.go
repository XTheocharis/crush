package repomap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func normalizeFixtureJSONForHash(data []byte) ([]byte, error) {
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	stripFixtureHashFields(payload)
	return json.Marshal(payload)
}

func stripFixtureHashFields(v any) {
	switch typed := v.(type) {
	case map[string]any:
		for k, child := range typed {
			if k == "fixtures_sha256" ||
				k == "aider_commit_sha" ||
				k == "comparator_path" {
				delete(typed, k)
				continue
			}
			stripFixtureHashFields(child)
		}
	case []any:
		for _, child := range typed {
			stripFixtureHashFields(child)
		}
	}
}

// ParityAiderFixture represents a complete Aider parity fixture with provenance metadata.
type ParityAiderFixture struct {
	FixtureID   string                  `json:"fixture_id"`
	Description string                  `json:"description"`
	Provenance  ParityProvenanceBundle  `json:"provenance"`
	Repository  ParityFixtureRepository `json:"repository"`
	Profiles    []ParityProfile         `json:"profiles"`
	Assertions  ParityAssertions        `json:"assertions"`
}

// ParityFixtureRepository describes the repository structure for a parity fixture.
type ParityFixtureRepository struct {
	Root  string   `json:"root"`
	Files []string `json:"files"`
}

// ParityProfile represents a parity test profile with budget and mode settings.
type ParityProfile struct {
	ProfileID               string       `json:"profile_id"`
	ParityMode              bool         `json:"parity_mode"`
	TokenBudget             int          `json:"token_budget"`
	RepeatRuns              int          `json:"repeat_runs"`
	DeterministicMode       bool         `json:"deterministic_mode"`
	EnhancementTiersEnabled string       `json:"enhancement_tiers_enabled"`
	TokenCounterMode        string       `json:"token_counter_mode"`
	FixedSeed               int64        `json:"fixed_seed"`
	ExpectedResults         Expectations `json:"expected_results"`
}

// Expectations holds expected outcomes for a parity profile.
type Expectations struct {
	RawHash        string     `json:"raw_hash,omitempty"`
	NormalizedHash string     `json:"normalized_hash,omitempty"`
	TopFiles       []string   `json:"top_files,omitempty"`
	StageCount     StageCount `json:"stage_count"`
}

// StageCount tracks expected stage entry counts.
type StageCount struct {
	Stage0 int `json:"stage0"`
	Stage1 int `json:"stage1"`
	Stage2 int `json:"stage2"`
	Stage3 int `json:"stage3"`
}

// ParityAssertions defines invariants that must hold for the fixture.
type ParityAssertions struct {
	RequireNonEmptyMap     bool    `json:"require_non_empty_map"`
	RequireRenderedEntries bool    `json:"require_rendered_entries"`
	RequireStageInvariants bool    `json:"require_stage_invariants"`
	RequireTrimOrder       []int   `json:"require_trim_order,omitempty"`
	ComparatorTolerancePct float64 `json:"comparator_tolerance_pct,omitempty"`
	ParityMode             bool    `json:"parity_mode"`
}

// LoadParityAiderFixtures loads all Aider parity fixtures from the testdata directory.
func LoadParityAiderFixtures(basePath string) ([]ParityAiderFixture, error) {
	fixtureDir := filepath.Join(basePath, "testdata", "parity_aider")
	paths, err := filepath.Glob(filepath.Join(fixtureDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob fixtures: %w", err)
	}

	sort.Strings(paths)
	fixtures := make([]ParityAiderFixture, 0, len(paths))

	for _, path := range paths {
		// Only fixture payloads participate in parity fixture loading.
		basename := filepath.Base(path)
		if !strings.HasSuffix(basename, "_fixture.json") {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read fixture %q: %w", path, err)
		}

		var fx ParityAiderFixture
		if err := json.Unmarshal(data, &fx); err != nil {
			return nil, fmt.Errorf("unmarshal fixture %q: %w", path, err)
		}

		if strings.TrimSpace(fx.FixtureID) == "" {
			return nil, fmt.Errorf("fixture %q missing fixture_id", path)
		}

		if err := fx.Validate(true); err != nil {
			return nil, fmt.Errorf("fixture %q validation failed: %w", path, err)
		}

		fixtures = append(fixtures, fx)
	}

	return fixtures, nil
}

// Validate ensures the fixture has all required fields and passes provenance checks.
func (fx ParityAiderFixture) Validate(requireComparatorTuple bool) error {
	if strings.TrimSpace(fx.FixtureID) == "" {
		return fmt.Errorf("missing fixture_id")
	}

	if err := fx.Provenance.Validate(requireComparatorTuple); err != nil {
		return fmt.Errorf("provenance validation failed: %w", err)
	}

	for i, profile := range fx.Profiles {
		if strings.TrimSpace(profile.ProfileID) == "" {
			return fmt.Errorf("profile %d missing profile_id", i)
		}
		if profile.TokenBudget <= 0 {
			return fmt.Errorf("profile %q token_budget must be positive", profile.ProfileID)
		}
		if profile.RepeatRuns <= 0 {
			return fmt.Errorf("profile %q repeat_runs must be positive", profile.ProfileID)
		}
		if profile.ParityMode {
			if !profile.DeterministicMode {
				return fmt.Errorf("profile %q deterministic_mode must be true in parity mode", profile.ProfileID)
			}
			if strings.ToLower(strings.TrimSpace(profile.EnhancementTiersEnabled)) != "none" {
				return fmt.Errorf("profile %q enhancement_tiers_enabled must be none in parity mode", profile.ProfileID)
			}
			counterMode := strings.ToLower(strings.TrimSpace(profile.TokenCounterMode))
			if counterMode != "tokenizer_backed" {
				return fmt.Errorf("profile %q token_counter_mode must be tokenizer_backed", profile.ProfileID)
			}
			if profile.FixedSeed <= 0 {
				return fmt.Errorf("profile %q fixed_seed must be positive in parity mode", profile.ProfileID)
			}
		}
	}

	return nil
}

// ComputeFixturesSHA256 computes the SHA-256 hash of all fixture files in the directory.
func ComputeFixturesSHA256(dirPath string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(dirPath, "*.json"))
	if err != nil {
		return "", fmt.Errorf("glob fixtures: %w", err)
	}

	sort.Strings(paths)

	combined := strings.Builder{}
	for _, path := range paths {
		basename := filepath.Base(path)
		if strings.HasPrefix(basename, "gate_a_evidence.") {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read fixture %q: %w", path, err)
		}

		normalized, err := normalizeFixtureJSONForHash(data)
		if err != nil {
			return "", fmt.Errorf("normalize fixture %q for hash: %w", path, err)
		}
		combined.Write(normalized)
	}

	sum := sha256.Sum256([]byte(combined.String()))
	return hex.EncodeToString(sum[:]), nil
}

// VerifyFixturesIntegrity checks that the fixtures_sha256 matches the computed value.
func VerifyFixturesIntegrity(fx ParityAiderFixture, basePath string) error {
	expected := strings.TrimSpace(fx.Provenance.FixturesSHA256)
	if expected == "" || isPlaceholderFixtureHash(expected) {
		return fmt.Errorf("fixture has empty or placeholder fixtures_sha256")
	}

	fixtureDir := filepath.Join(basePath, "testdata", "parity_aider")
	computed, err := ComputeFixturesSHA256(fixtureDir)
	if err != nil {
		return fmt.Errorf("compute fixtures sha256: %w", err)
	}

	if computed != expected {
		return fmt.Errorf("fixtures_sha256 mismatch: expected=%s computed=%s", expected, computed)
	}

	return nil
}

// ParityFixtureResult captures the result of running a parity fixture test.
type ParityFixtureResult struct {
	FixtureID          string
	ProfileID          string
	ParityMode         bool
	MapText            string
	Entries            []StageEntry
	ParityTokens       float64
	SafetyTokens       int
	RawHash            string
	NormalizedHash     string
	TrimmedStages      []int
	Stage0Preserved    bool
	ComparatorAccepted bool
	ComparatorDelta    float64
	Error              error
}
