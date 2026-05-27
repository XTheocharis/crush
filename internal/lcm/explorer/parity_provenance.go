package explorer

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	voltGitSHAPattern = regexp.MustCompile(`\A[0-9a-fA-F]{40}\z`)
	voltSHA256Pattern = regexp.MustCompile(`\A[0-9a-fA-F]{64}\z`)
)

func isPlaceholderCommitSHA(sha string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(sha))
	return trimmed == strings.Repeat("0", 40) ||
		trimmed == strings.Repeat("c", 40)
}

func isPlaceholderFixtureHash(hash string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(hash))
	return trimmed == strings.Repeat("d", 64) ||
		trimmed == "placeholder_compute_before_use"
}

func isPlaceholderComparatorPath(path string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(path))
	return strings.Contains(trimmed, "/tree/"+strings.Repeat("0", 40)) ||
		strings.Contains(trimmed, "placeholder")
}

// ParityProvenanceBundle contains provenance required by Volt parity adjudication.
type ParityProvenanceBundle struct {
	VoltCommitSHA     string `json:"volt_commit_sha"`
	ComparatorPath    string `json:"comparator_path"`
	FixturesSHA256    string `json:"fixtures_sha256"`
	GrepASTProvenance string `json:"grep_ast_provenance,omitempty"`
	TokenizerID       string `json:"tokenizer_id,omitempty"`
	TokenizerVersion  string `json:"tokenizer_version,omitempty"`
}

// ParityPreflightProfile captures profile fields required by the preflight gate.
type ParityPreflightProfile struct {
	ID                      string
	TokenBudget             int
	RepeatRuns              int
	DeterministicMode       bool
	EnhancementTiersEnabled string
	TokenCounterMode        string
	FixedSeed               int64
	ParityMode              bool
}

// ParityPreflightOptions controls strictness for preflight checks.
type ParityPreflightOptions struct {
	RequireComparatorTuple bool
	CorpusBasePath         string
	Profile                *ParityPreflightProfile
}

// Validate returns an error when required provenance fields are missing or malformed.
func (b ParityProvenanceBundle) Validate(requireComparatorTuple bool) error {
	if !voltGitSHAPattern.MatchString(strings.TrimSpace(b.VoltCommitSHA)) {
		return fmt.Errorf("invalid volt_commit_sha: expected 40-char hex commit SHA")
	}
	if isPlaceholderCommitSHA(b.VoltCommitSHA) {
		return fmt.Errorf("invalid volt_commit_sha: placeholder value is not allowed")
	}
	if strings.TrimSpace(b.ComparatorPath) == "" {
		return fmt.Errorf("missing comparator_path")
	}
	if isPlaceholderComparatorPath(b.ComparatorPath) {
		return fmt.Errorf("invalid comparator_path: placeholder value is not allowed")
	}
	if !voltSHA256Pattern.MatchString(strings.TrimSpace(b.FixturesSHA256)) {
		return fmt.Errorf("invalid fixtures_sha256: expected 64-char hex SHA-256")
	}
	if isPlaceholderFixtureHash(b.FixturesSHA256) {
		return fmt.Errorf("invalid fixtures_sha256: placeholder value is not allowed")
	}
	if requireComparatorTuple {
		if strings.TrimSpace(b.GrepASTProvenance) == "" {
			return fmt.Errorf("missing grep_ast_provenance")
		}
		if strings.TrimSpace(b.TokenizerID) == "" {
			return fmt.Errorf("missing tokenizer_id")
		}
		if strings.TrimSpace(b.TokenizerVersion) == "" {
			return fmt.Errorf("missing tokenizer_version")
		}
	}
	return nil
}

func validateParityPreflightProfile(profile *ParityPreflightProfile) error {
	if profile == nil {
		return fmt.Errorf("profile validation failed: missing profile")
	}
	if strings.TrimSpace(profile.ID) == "" {
		return fmt.Errorf("profile validation failed: missing profile id")
	}
	if profile.TokenBudget <= 0 {
		return fmt.Errorf("profile validation failed: token budget must be positive")
	}
	if profile.RepeatRuns <= 0 {
		return fmt.Errorf("profile validation failed: repeat runs must be positive")
	}
	if profile.ParityMode {
		if !profile.DeterministicMode {
			return fmt.Errorf("profile validation failed: deterministic_mode must be true in parity mode")
		}
		if strings.ToLower(strings.TrimSpace(profile.EnhancementTiersEnabled)) != "none" {
			return fmt.Errorf("profile validation failed: enhancement_tiers_enabled must be none in parity mode")
		}
		counterMode := strings.ToLower(strings.TrimSpace(profile.TokenCounterMode))
		if counterMode != "tokenizer_backed" {
			return fmt.Errorf("profile validation failed: token_counter_mode must be tokenizer_backed")
		}
		if profile.FixedSeed <= 0 {
			return fmt.Errorf("profile validation failed: fixed_seed must be positive in parity mode")
		}
	}
	return nil
}

func validateParityTokenizerTuple(bundle ParityProvenanceBundle, requireComparatorTuple bool) error {
	ts, err := LoadTokenizerSupport()
	if err != nil {
		return fmt.Errorf("tokenizer support load failed: %w", err)
	}
	if err := ValidateProtocolArtifact(ts); err != nil {
		return fmt.Errorf("tokenizer support validation failed: %w", err)
	}
	if !requireComparatorTuple {
		return nil
	}

	for _, fam := range ts.SupportedFamilies {
		if fam.TokenizerID == bundle.TokenizerID && fam.TokenizerVersion == bundle.TokenizerVersion {
			return nil
		}
	}
	return fmt.Errorf("tokenizer tuple %q@%q is not present in tokenizer support artifact", bundle.TokenizerID, bundle.TokenizerVersion)
}

func validateParityCorpus(basePath string) error {
	cfg := NewDefaultParityFixtureConfig(basePath)
	loader := NewParityFixtureLoader(cfg)
	index, err := loader.LoadIndex()
	if err != nil {
		return fmt.Errorf("corpus readiness failed: %w", err)
	}
	if err := VerifyFixturesIntegrity(index, cfg); err != nil {
		return fmt.Errorf("corpus readiness failed: fixture integrity check failed: %w", err)
	}
	if err := ValidateB1ScoringProtocolArtifact(); err != nil {
		return fmt.Errorf("corpus readiness failed: b1 scoring protocol artifact invalid: %w", err)
	}
	fixtures, err := loader.LoadAllFixtures()
	if err != nil {
		return fmt.Errorf("corpus readiness failed: %w", err)
	}
	if len(fixtures) == 0 {
		return fmt.Errorf("corpus readiness failed: no parity fixtures found")
	}
	return nil
}

// RunParityHarnessPreflight validates all parity prerequisites before execution.
func RunParityHarnessPreflight(bundle ParityProvenanceBundle, opts ParityPreflightOptions) error {
	if err := bundle.Validate(opts.RequireComparatorTuple); err != nil {
		return err
	}
	if err := validateParityTokenizerTuple(bundle, opts.RequireComparatorTuple); err != nil {
		return err
	}
	if err := validateParityPreflightProfile(opts.Profile); err != nil {
		return err
	}
	basePath := opts.CorpusBasePath
	if strings.TrimSpace(basePath) == "" {
		basePath = "."
	}
	if err := validateParityCorpus(basePath); err != nil {
		return err
	}
	return nil
}
