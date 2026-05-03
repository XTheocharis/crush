package repomap

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func validRepomapBundle() ParityProvenanceBundle {
	return ParityProvenanceBundle{
		AiderCommitSHA:    strings.Repeat("a", 40),
		ComparatorPath:    "../aider",
		FixturesSHA256:    strings.Repeat("b", 64),
		GrepASTProvenance: "grep-ast@v1.2.3",
		TokenizerID:       "cl100k_base",
		TokenizerVersion:  "v0.1.0",
	}
}

func TestParityProvenanceBundleValidateRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*ParityProvenanceBundle)
		want   string
	}{
		{
			name: "missing aider commit sha",
			mutate: func(b *ParityProvenanceBundle) {
				b.AiderCommitSHA = ""
			},
			want: "aider_commit_sha",
		},
		{
			name: "missing comparator path",
			mutate: func(b *ParityProvenanceBundle) {
				b.ComparatorPath = ""
			},
			want: "comparator_path",
		},
		{
			name: "missing fixtures sha",
			mutate: func(b *ParityProvenanceBundle) {
				b.FixturesSHA256 = ""
			},
			want: "fixtures_sha256",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bundle := validRepomapBundle()
			tt.mutate(&bundle)

			err := bundle.Validate(false)
			require.Error(t, err, "expected error")
			require.Contains(t, err.Error(), tt.want, "expected error to contain %q", tt.want)
		})
	}
}

func TestParityProvenanceBundleValidateComparatorTupleRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*ParityProvenanceBundle)
		want   string
	}{
		{
			name: "missing grep ast provenance",
			mutate: func(b *ParityProvenanceBundle) {
				b.GrepASTProvenance = ""
			},
			want: "grep_ast_provenance",
		},
		{
			name: "missing tokenizer id",
			mutate: func(b *ParityProvenanceBundle) {
				b.TokenizerID = ""
			},
			want: "tokenizer_id",
		},
		{
			name: "missing tokenizer version",
			mutate: func(b *ParityProvenanceBundle) {
				b.TokenizerVersion = ""
			},
			want: "tokenizer_version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bundle := validRepomapBundle()
			tt.mutate(&bundle)

			err := bundle.Validate(true)
			require.Error(t, err, "expected error")
			require.Contains(t, err.Error(), tt.want, "expected error to contain %q", tt.want)
		})
	}
}

func TestParityProvenanceBundleValidateComparatorTupleOptional(t *testing.T) {
	t.Parallel()

	bundle := validRepomapBundle()
	bundle.GrepASTProvenance = ""
	bundle.TokenizerID = ""
	bundle.TokenizerVersion = ""

	require.NoError(t, bundle.Validate(false), "expected no error")
}

func TestParityProvenanceBundleValidateInvalidDigests(t *testing.T) {
	t.Parallel()

	bundle := validRepomapBundle()
	bundle.AiderCommitSHA = "short"
	require.Error(t, bundle.Validate(false), "expected invalid aider_commit_sha error")

	bundle = validRepomapBundle()
	bundle.FixturesSHA256 = "notsha"
	require.Error(t, bundle.Validate(false), "expected invalid fixtures_sha256 error")
}

func TestRunParityHarnessPreflight(t *testing.T) {
	t.Parallel()

	bundle := validRepomapBundle()
	require.NoError(t, RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile: &ParityPreflightProfile{
			ID:                      "parity-preflight",
			TokenBudget:             1024,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "tokenizer_backed",
			FixedSeed:               1337,
		},
	}), "expected no error")

	bundle.TokenizerID = ""
	err := RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile: &ParityPreflightProfile{
			ID:                      "parity-preflight",
			TokenBudget:             1024,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "tokenizer_backed",
			FixedSeed:               1337,
		},
	})
	require.Error(t, err, "expected error")
	require.Contains(t, err.Error(), "tokenizer_id", "expected tokenizer_id error")
}

func TestRunParityHarnessPreflightProfileValidation(t *testing.T) {
	t.Parallel()

	bundle := validRepomapBundle()

	err := RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile: &ParityPreflightProfile{
			ID:                      "",
			TokenBudget:             1024,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "tokenizer_backed",
			FixedSeed:               1337,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "profile")

	err = RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile: &ParityPreflightProfile{
			ID:                      "default",
			TokenBudget:             0,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "tokenizer_backed",
			FixedSeed:               1337,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "token budget")

	err = RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile: &ParityPreflightProfile{
			ID:                      "default",
			TokenBudget:             1024,
			RepeatRuns:              0,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "tokenizer_backed",
			FixedSeed:               1337,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "repeat runs")
}

func TestRunParityHarnessPreflightComparatorTupleArtifactMatch(t *testing.T) {
	t.Parallel()

	bundle := validRepomapBundle()
	bundle.TokenizerID = "nonexistent-tokenizer"
	bundle.TokenizerVersion = "v9.9.9"

	err := RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile: &ParityPreflightProfile{
			ID:                      "parity-preflight",
			TokenBudget:             1024,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "tokenizer_backed",
			FixedSeed:               1337,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "tokenizer tuple")
}

func TestRunParityHarnessPreflightCorpusReadiness(t *testing.T) {
	t.Parallel()

	bundle := validRepomapBundle()

	err := RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: false,
		CorpusBasePath:         t.TempDir(),
		Profile: &ParityPreflightProfile{
			ID:                      "parity-preflight",
			TokenBudget:             1024,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "tokenizer_backed",
			FixedSeed:               1337,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "corpus readiness")
}

func TestRunParityHarnessPreflightDeterministicScoringEnforcement(t *testing.T) {
	t.Parallel()

	bundle := validRepomapBundle()

	err := RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile: &ParityPreflightProfile{
			ID:                      "parity-preflight",
			TokenBudget:             1024,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       false,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "tokenizer_backed",
			FixedSeed:               1337,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "deterministic_mode")

	err = RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile: &ParityPreflightProfile{
			ID:                      "parity-preflight",
			TokenBudget:             1024,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "all",
			TokenCounterMode:        "tokenizer_backed",
			FixedSeed:               1337,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "enhancement_tiers_enabled")

	err = RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile: &ParityPreflightProfile{
			ID:                      "parity-preflight",
			TokenBudget:             1024,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "",
			FixedSeed:               1337,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "token_counter_mode")

	err = RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile: &ParityPreflightProfile{
			ID:                      "parity-preflight",
			TokenBudget:             1024,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "heuristic",
			FixedSeed:               1337,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "token_counter_mode")

	err = RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile: &ParityPreflightProfile{
			ID:                      "parity-preflight",
			TokenBudget:             1024,
			RepeatRuns:              2,
			ParityMode:              true,
			DeterministicMode:       true,
			EnhancementTiersEnabled: "none",
			TokenCounterMode:        "tokenizer_backed",
			FixedSeed:               0,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "fixed_seed")
}

// Protocol artifact tests for task #220

func TestProtocolArtifact_LoadTokenizerSupport(t *testing.T) {
	t.Parallel()

	ts, err := LoadTokenizerSupport()
	require.NoError(t, err)
	require.NotNil(t, ts)

	require.NoError(t, ValidateProtocolArtifact(ts), "expected artifact to be valid")
}

func TestProtocolArtifact_ValidateTokenizerSupport(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateTokenizerSupportArtifact(), "expected valid tokenizer support artifact")
}

func TestProtocolArtifact_TokenizerSupportVersion(t *testing.T) {
	t.Parallel()

	ts, err := LoadTokenizerSupport()
	require.NoError(t, err)
	require.Equal(t, "1", ts.Version, "version must be 1")

	require.NoError(t, ts.ValidateVersion(), "expected valid version")
}

func TestProtocolArtifact_TokenizerSupportRequiredFields(t *testing.T) {
	t.Parallel()

	ts, err := LoadTokenizerSupport()
	require.NoError(t, err)

	require.NotEmpty(t, ts.Description, "description is required")
	require.NotEmpty(t, ts.GeneratedAt, "generated_at is required")
	require.Greater(t, len(ts.SupportedFamilies), 0, "supported_families must not be empty")

	require.NoError(t, ts.ValidateRequiredFields(), "expected all required fields present")
}

func TestProtocolArtifact_TokenizerSupportInvalidVersion(t *testing.T) {
	t.Parallel()

	ts := &TokenizerSupport{Version: "2"}
	err := ts.ValidateVersion()
	require.Error(t, err, "expected error for unsupported version")
	require.Contains(t, err.Error(), "unsupported version", "expected version error")
}

func TestProtocolArtifact_TokenizerSupportMissingFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*TokenizerSupport)
		want   string
	}{
		{
			name: "missing version",
			mutate: func(ts *TokenizerSupport) {
				ts.Version = ""
			},
			want: "missing version",
		},
		{
			name: "missing description",
			mutate: func(ts *TokenizerSupport) {
				ts.Description = ""
			},
			want: "missing description",
		},
		{
			name: "missing generated_at",
			mutate: func(ts *TokenizerSupport) {
				ts.GeneratedAt = ""
			},
			want: "missing generated_at",
		},
		{
			name: "empty supported_families",
			mutate: func(ts *TokenizerSupport) {
				ts.SupportedFamilies = []TokenizerFamily{}
			},
			want: "must not be empty",
		},
		{
			name: "family missing model_family",
			mutate: func(ts *TokenizerSupport) {
				ts.SupportedFamilies = []TokenizerFamily{
					{ModelFamily: "", TokenizerID: "test", TokenizerVersion: "1.0"},
				}
			},
			want: "missing model_family",
		},
		{
			name: "family missing tokenizer_id",
			mutate: func(ts *TokenizerSupport) {
				ts.SupportedFamilies = []TokenizerFamily{
					{ModelFamily: "test", TokenizerID: "", TokenizerVersion: "1.0"},
				}
			},
			want: "missing tokenizer_id",
		},
		{
			name: "family missing tokenizer_version",
			mutate: func(ts *TokenizerSupport) {
				ts.SupportedFamilies = []TokenizerFamily{
					{ModelFamily: "test", TokenizerID: "test", TokenizerVersion: ""},
				}
			},
			want: "missing tokenizer_version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ts := &TokenizerSupport{
				Version:     "1",
				Description: "test",
				GeneratedAt: "2024-01-01T00:00:00Z",
				SupportedFamilies: []TokenizerFamily{
					{ModelFamily: "test", TokenizerID: "test", TokenizerVersion: "1.0"},
				},
			}
			tt.mutate(ts)

			err := ts.ValidateRequiredFields()
			require.Error(t, err, "expected error")
			require.Contains(t, err.Error(), tt.want, "expected error to contain %q", tt.want)
		})
	}
}

func TestProtocolArtifact_LoadExplorerFamilyMatrix(t *testing.T) {
	t.Parallel()

	efm, err := LoadExplorerFamilyMatrix()
	require.NoError(t, err)
	require.NotNil(t, efm)

	require.NoError(t, ValidateProtocolArtifact(efm), "expected artifact to be valid")
}

func TestProtocolArtifact_ValidateExplorerFamilyMatrix(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateExplorerFamilyMatrixArtifact(), "expected valid explorer family matrix artifact")
}

func TestProtocolArtifact_ExplorerFamilyMatrixVersion(t *testing.T) {
	t.Parallel()

	efm, err := LoadExplorerFamilyMatrix()
	require.NoError(t, err)
	require.Equal(t, "1", efm.Version, "version must be 1")

	require.NoError(t, efm.ValidateVersion(), "expected valid version")
}

func TestProtocolArtifact_ExplorerFamilyMatrixRequiredFields(t *testing.T) {
	t.Parallel()

	efm, err := LoadExplorerFamilyMatrix()
	require.NoError(t, err)

	require.NotEmpty(t, efm.Description, "description is required")
	require.NotEmpty(t, efm.GeneratedAt, "generated_at is required")
	require.Greater(t, len(efm.Explorers), 0, "explorers must not be empty")

	require.NoError(t, efm.ValidateRequiredFields(), "expected all required fields present")
}

func TestProtocolArtifact_ExplorerFamilyMatrixInvalidVersion(t *testing.T) {
	t.Parallel()

	efm := &ExplorerFamilyMatrix{Version: "2"}
	err := efm.ValidateVersion()
	require.Error(t, err, "expected error for unsupported version")
	require.Contains(t, err.Error(), "unsupported version", "expected version error")
}

func TestProtocolArtifact_ExplorerFamilyMatrixMissingFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*ExplorerFamilyMatrix)
		want   string
	}{
		{
			name: "missing version",
			mutate: func(efm *ExplorerFamilyMatrix) {
				efm.Version = ""
			},
			want: "missing version",
		},
		{
			name: "missing description",
			mutate: func(efm *ExplorerFamilyMatrix) {
				efm.Description = ""
			},
			want: "missing description",
		},
		{
			name: "missing generated_at",
			mutate: func(efm *ExplorerFamilyMatrix) {
				efm.GeneratedAt = ""
			},
			want: "missing generated_at",
		},
		{
			name: "empty explorers",
			mutate: func(efm *ExplorerFamilyMatrix) {
				efm.Explorers = []ExplorerInfo{}
			},
			want: "must not be empty",
		},
		{
			name: "explorer missing explorer_id",
			mutate: func(efm *ExplorerFamilyMatrix) {
				efm.Explorers = []ExplorerInfo{
					{ExplorerID: "", ExplorerType: "test", Description: "test"},
				}
			},
			want: "missing explorer_id",
		},
		{
			name: "explorer missing explorer_type",
			mutate: func(efm *ExplorerFamilyMatrix) {
				efm.Explorers = []ExplorerInfo{
					{ExplorerID: "test", ExplorerType: "", Description: "test"},
				}
			},
			want: "missing explorer_type",
		},
		{
			name: "explorer missing description",
			mutate: func(efm *ExplorerFamilyMatrix) {
				efm.Explorers = []ExplorerInfo{
					{ExplorerID: "test", ExplorerType: "test", Description: ""},
				}
			},
			want: "missing description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			efm := &ExplorerFamilyMatrix{
				Version:     "1",
				Description: "test",
				GeneratedAt: "2024-01-01T00:00:00Z",
				Explorers: []ExplorerInfo{
					{ExplorerID: "test", ExplorerType: "test", Description: "test"},
				},
			}
			tt.mutate(efm)

			err := efm.ValidateRequiredFields()
			require.Error(t, err, "expected error")
			require.Contains(t, err.Error(), tt.want, "expected error to contain %q", tt.want)
		})
	}
}

func TestProtocolArtifact_ValidateAllProtocolArtifacts(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateAllProtocolArtifacts(), "expected all protocol artifacts to be valid")
}

func TestProtocolArtifact_TokenizerSupportMethods(t *testing.T) {
	t.Parallel()

	ts, err := LoadTokenizerSupport()
	require.NoError(t, err)

	tokenizers := ts.GetSupportedTokenizers()
	require.NotEmpty(t, tokenizers, "should have supported tokenizers")

	require.Contains(t, tokenizers, "anthropic", "should support anthropic family")
	require.Contains(t, tokenizers, "openai", "should support openai family")

	models := ts.GetModelsForFamily("anthropic")
	require.Greater(t, len(models), 0, "should have models for anthropic family")
	require.Contains(t, models, "claude-3-opus-20240229")
}

func TestProtocolArtifact_ExplorerFamilyMatrixMethods(t *testing.T) {
	t.Parallel()

	efm, err := LoadExplorerFamilyMatrix()
	require.NoError(t, err)

	exp := efm.GetExplorerByID("TreeSitterExplorer")
	require.NotNil(t, exp, "should find TreeSitterExplorer")
	require.Equal(t, "TreeSitterExplorer", exp.ExplorerID)

	enhancedExplorers := efm.GetExplorersByType("code_format_enhanced")
	require.Greater(t, len(enhancedExplorers), 0, "should have code format explorers")

	found := false
	for _, e := range enhancedExplorers {
		if e.ExplorerID == "TreeSitterExplorer" {
			found = true
			break
		}
	}
	require.True(t, found, "should find TreeSitterExplorer in code_format_enhanced type")
}

func TestProtocolArtifact_ValidateGeneratedAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{
			name:  "valid RFC3339 with Z",
			value: "2024-01-01T00:00:00Z",
			valid: true,
		},
		{
			name:  "valid RFC3339 with milliseconds",
			value: "2024-01-01T00:00:00.123Z",
			valid: true,
		},
		{
			name:  "valid RFC3339 with offset",
			value: "2024-01-01T00:00:00+00:00",
			valid: true,
		},
		{
			name:  "empty string",
			value: "",
			valid: false,
		},
		{
			name:  "invalid format",
			value: "2024-01-01",
			valid: false,
		},
		{
			name:  "random string",
			value: "not-a-date",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateGeneratedAt(tt.value)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
