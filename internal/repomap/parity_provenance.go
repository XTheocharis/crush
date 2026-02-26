package repomap

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	gitSHAPattern    = regexp.MustCompile(`\A[0-9a-fA-F]{40}\z`)
	sha256HexPattern = regexp.MustCompile(`\A[0-9a-fA-F]{64}\z`)
)

// ParityProvenanceBundle contains provenance required by parity adjudication.
type ParityProvenanceBundle struct {
	AiderCommitSHA    string `json:"aider_commit_sha"`
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
	if !gitSHAPattern.MatchString(strings.TrimSpace(b.AiderCommitSHA)) {
		return fmt.Errorf("invalid aider_commit_sha: expected 40-char hex commit SHA")
	}
	if strings.TrimSpace(b.ComparatorPath) == "" {
		return fmt.Errorf("missing comparator_path")
	}
	if !sha256HexPattern.MatchString(strings.TrimSpace(b.FixturesSHA256)) {
		return fmt.Errorf("invalid fixtures_sha256: expected 64-char hex SHA-256")
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
		if counterMode != "tokenizer_backed" && counterMode != "heuristic" {
			return fmt.Errorf("profile validation failed: token_counter_mode must be tokenizer_backed or heuristic")
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
	fixtures, err := LoadParityAiderFixtures(basePath)
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
