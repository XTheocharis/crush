package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadParityAiderFixtures(t *testing.T) {
	t.Parallel()

	fixtures, err := LoadParityAiderFixtures(".")
	require.NoError(t, err)
	require.NotEmpty(t, fixtures)

	for _, fx := range fixtures {
		t.Run(fx.FixtureID, func(t *testing.T) {
			t.Parallel()

			require.NotEmpty(t, fx.FixtureID)
			require.NotEmpty(t, fx.Profiles)

			for i, profile := range fx.Profiles {
				t.Run(profile.ProfileID, func(t *testing.T) {
					t.Parallel()
					require.NotEmpty(t, fx.Profiles[i].ProfileID)
					require.Greater(t, fx.Profiles[i].TokenBudget, 0)
					require.Greater(t, fx.Profiles[i].RepeatRuns, 0)
				})
			}
		})
	}
}

func TestParityAiderFixtureValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fx      ParityAiderFixture
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid fixture with comparator tuple",
			fx: ParityAiderFixture{
				FixtureID: "test_fixture",
				Provenance: ParityProvenanceBundle{
					AiderCommitSHA:    strings.Repeat("a", 40),
					ComparatorPath:    "../aider",
					FixturesSHA256:    strings.Repeat("b", 64),
					GrepASTProvenance: "grep-ast@v1.2.3",
					TokenizerID:       "cl100k_base",
					TokenizerVersion:  "v0.1.0",
				},
				Profiles: []ParityProfile{
					{
						ProfileID:               "test_profile",
						ParityMode:              true,
						TokenBudget:             1024,
						RepeatRuns:              10,
						DeterministicMode:       true,
						EnhancementTiersEnabled: "none",
						TokenCounterMode:        "tokenizer_backed",
						FixedSeed:               1337,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing fixture_id",
			fx: ParityAiderFixture{
				Provenance: ParityProvenanceBundle{
					AiderCommitSHA: strings.Repeat("a", 40),
					ComparatorPath: "../aider",
					FixturesSHA256: strings.Repeat("b", 64),
				},
			},
			wantErr: true,
			errMsg:  "fixture_id",
		},
		{
			name: "invalid provenance",
			fx: ParityAiderFixture{
				FixtureID: "test_fixture",
				Provenance: ParityProvenanceBundle{
					AiderCommitSHA: "short",
					ComparatorPath: "../aider",
					FixturesSHA256: strings.Repeat("b", 64),
				},
			},
			wantErr: true,
			errMsg:  "aider_commit_sha",
		},
		{
			name: "profile with zero token budget",
			fx: ParityAiderFixture{
				FixtureID: "test_fixture",
				Provenance: ParityProvenanceBundle{
					AiderCommitSHA:    strings.Repeat("a", 40),
					ComparatorPath:    "../aider",
					FixturesSHA256:    strings.Repeat("b", 64),
					GrepASTProvenance: "grep-ast@v1.2.3",
					TokenizerID:       "cl100k_base",
					TokenizerVersion:  "v0.1.0",
				},
				Profiles: []ParityProfile{
					{
						ProfileID:               "test_profile",
						ParityMode:              true,
						TokenBudget:             0,
						RepeatRuns:              10,
						DeterministicMode:       true,
						EnhancementTiersEnabled: "none",
						TokenCounterMode:        "tokenizer_backed",
						FixedSeed:               1337,
					},
				},
			},
			wantErr: true,
			errMsg:  "token_budget",
		},
		{
			name: "profile with zero repeat runs",
			fx: ParityAiderFixture{
				FixtureID: "test_fixture",
				Provenance: ParityProvenanceBundle{
					AiderCommitSHA:    strings.Repeat("a", 40),
					ComparatorPath:    "../aider",
					FixturesSHA256:    strings.Repeat("b", 64),
					GrepASTProvenance: "grep-ast@v1.2.3",
					TokenizerID:       "cl100k_base",
					TokenizerVersion:  "v0.1.0",
				},
				Profiles: []ParityProfile{
					{
						ProfileID:               "test_profile",
						ParityMode:              true,
						TokenBudget:             1024,
						RepeatRuns:              0,
						DeterministicMode:       true,
						EnhancementTiersEnabled: "none",
						TokenCounterMode:        "tokenizer_backed",
						FixedSeed:               1337,
					},
				},
			},
			wantErr: true,
			errMsg:  "repeat_runs",
		},
		{
			name: "parity profile requires deterministic_mode",
			fx: ParityAiderFixture{
				FixtureID: "test_fixture",
				Provenance: ParityProvenanceBundle{
					AiderCommitSHA:    strings.Repeat("a", 40),
					ComparatorPath:    "../aider",
					FixturesSHA256:    strings.Repeat("b", 64),
					GrepASTProvenance: "grep-ast@v1.2.3",
					TokenizerID:       "cl100k_base",
					TokenizerVersion:  "v0.1.0",
				},
				Profiles: []ParityProfile{
					{
						ProfileID:               "test_profile",
						ParityMode:              true,
						TokenBudget:             1024,
						RepeatRuns:              10,
						DeterministicMode:       false,
						EnhancementTiersEnabled: "none",
						TokenCounterMode:        "tokenizer_backed",
						FixedSeed:               1337,
					},
				},
			},
			wantErr: true,
			errMsg:  "deterministic_mode",
		},
		{
			name: "parity profile requires enhancement tiers none",
			fx: ParityAiderFixture{
				FixtureID: "test_fixture",
				Provenance: ParityProvenanceBundle{
					AiderCommitSHA:    strings.Repeat("a", 40),
					ComparatorPath:    "../aider",
					FixturesSHA256:    strings.Repeat("b", 64),
					GrepASTProvenance: "grep-ast@v1.2.3",
					TokenizerID:       "cl100k_base",
					TokenizerVersion:  "v0.1.0",
				},
				Profiles: []ParityProfile{
					{
						ProfileID:               "test_profile",
						ParityMode:              true,
						TokenBudget:             1024,
						RepeatRuns:              10,
						DeterministicMode:       true,
						EnhancementTiersEnabled: "all",
						TokenCounterMode:        "tokenizer_backed",
						FixedSeed:               1337,
					},
				},
			},
			wantErr: true,
			errMsg:  "enhancement_tiers_enabled",
		},
		{
			name: "parity profile requires token_counter_mode",
			fx: ParityAiderFixture{
				FixtureID: "test_fixture",
				Provenance: ParityProvenanceBundle{
					AiderCommitSHA:    strings.Repeat("a", 40),
					ComparatorPath:    "../aider",
					FixturesSHA256:    strings.Repeat("b", 64),
					GrepASTProvenance: "grep-ast@v1.2.3",
					TokenizerID:       "cl100k_base",
					TokenizerVersion:  "v0.1.0",
				},
				Profiles: []ParityProfile{
					{
						ProfileID:               "test_profile",
						ParityMode:              true,
						TokenBudget:             1024,
						RepeatRuns:              10,
						DeterministicMode:       true,
						EnhancementTiersEnabled: "none",
						TokenCounterMode:        "",
						FixedSeed:               1337,
					},
				},
			},
			wantErr: true,
			errMsg:  "token_counter_mode",
		},
		{
			name: "parity profile requires fixed_seed",
			fx: ParityAiderFixture{
				FixtureID: "test_fixture",
				Provenance: ParityProvenanceBundle{
					AiderCommitSHA:    strings.Repeat("a", 40),
					ComparatorPath:    "../aider",
					FixturesSHA256:    strings.Repeat("b", 64),
					GrepASTProvenance: "grep-ast@v1.2.3",
					TokenizerID:       "cl100k_base",
					TokenizerVersion:  "v0.1.0",
				},
				Profiles: []ParityProfile{
					{
						ProfileID:               "test_profile",
						ParityMode:              true,
						TokenBudget:             1024,
						RepeatRuns:              10,
						DeterministicMode:       true,
						EnhancementTiersEnabled: "none",
						TokenCounterMode:        "tokenizer_backed",
						FixedSeed:               0,
					},
				},
			},
			wantErr: true,
			errMsg:  "fixed_seed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.fx.Validate(true)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestComputeFixturesSHA256(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, "fixture1.json"),
		[]byte(`{"id": "fixture1"}`),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, "fixture2.json"),
		[]byte(`{"id": "fixture2"}`),
		0o644,
	))

	hash1, err := ComputeFixturesSHA256(tmpDir)
	require.NoError(t, err)
	require.Len(t, hash1, 64)

	hash2, err := ComputeFixturesSHA256(tmpDir)
	require.NoError(t, err)
	require.Equal(t, hash1, hash2)
}

func TestVerifyFixturesIntegrity(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "testdata", "parity_aider")
	require.NoError(t, os.MkdirAll(fixtureDir, 0o755))

	fixtureContent := `{"id": "test"}`
	require.NoError(t, os.WriteFile(
		filepath.Join(fixtureDir, "test.json"),
		[]byte(fixtureContent),
		0o644,
	))

	expectedHash, err := ComputeFixturesSHA256(fixtureDir)
	require.NoError(t, err)

	t.Run("valid Integrity", func(t *testing.T) {
		t.Parallel()

		fx := ParityAiderFixture{
			FixtureID: "test_fixture",
			Provenance: ParityProvenanceBundle{
				AiderCommitSHA:    strings.Repeat("a", 40),
				ComparatorPath:    "../aider/tree/" + strings.Repeat("a", 40),
				FixturesSHA256:    expectedHash,
				GrepASTProvenance: "grep-ast@v1.2.3",
				TokenizerID:       "cl100k_base",
				TokenizerVersion:  "v0.1.0",
			},
		}

		err := VerifyFixturesIntegrity(fx, tmpDir)
		require.NoError(t, err)
	})

	t.Run("invalid Integrity - mismatched hash", func(t *testing.T) {
		t.Parallel()

		fx := ParityAiderFixture{
			FixtureID: "test_fixture",
			Provenance: ParityProvenanceBundle{
				AiderCommitSHA:    strings.Repeat("a", 40),
				ComparatorPath:    "../aider/tree/" + strings.Repeat("a", 40),
				FixturesSHA256:    strings.Repeat("c", 64),
				GrepASTProvenance: "grep-ast@v1.2.3",
				TokenizerID:       "cl100k_base",
				TokenizerVersion:  "v0.1.0",
			},
		}

		err := VerifyFixturesIntegrity(fx, tmpDir)
		require.Error(t, err)
		require.Contains(t, err.Error(), "fixtures_sha256 mismatch")
	})

	t.Run("invalid integrity - placeholder hash", func(t *testing.T) {
		t.Parallel()

		fx := ParityAiderFixture{
			FixtureID: "test_fixture",
			Provenance: ParityProvenanceBundle{
				AiderCommitSHA:    strings.Repeat("a", 40),
				ComparatorPath:    "../aider/tree/" + strings.Repeat("a", 40),
				FixturesSHA256:    strings.Repeat("d", 64),
				GrepASTProvenance: "grep-ast@v1.2.3",
				TokenizerID:       "cl100k_base",
				TokenizerVersion:  "v0.1.0",
			},
		}

		err := VerifyFixturesIntegrity(fx, tmpDir)
		require.Error(t, err)
		require.Contains(t, err.Error(), "placeholder")
	})
}

func TestParityFixtureHarnessIntegration(t *testing.T) {
	t.Parallel()

	fixtures, err := LoadParityAiderFixtures(".")
	require.NoError(t, err)
	require.NotEmpty(t, fixtures)

	for _, fx := range fixtures {
		t.Run(fx.FixtureID, func(t *testing.T) {
			t.Parallel()

			for _, profile := range fx.Profiles {
				t.Run(profile.ProfileID, func(t *testing.T) {
					t.Parallel()

					t.Logf("fixture=%q profile=%q parity_mode=%v budget=%d",
						fx.FixtureID, profile.ProfileID, profile.ParityMode, profile.TokenBudget)

					if fx.Assertions.RequireNonEmptyMap {
						require.NotEmpty(t, fx.Description, "non-empty map requirement needs description")
					}

					if len(fx.Assertions.RequireTrimOrder) > 0 {
						require.Subset(t, []int{1, 2, 3}, fx.Assertions.RequireTrimOrder, "trim order must be subset of {1,2,3}")
					}
					if profile.ParityMode {
						require.True(t, profile.DeterministicMode, "parity profiles must enforce deterministic_mode")
						require.Equal(t, "none", strings.ToLower(strings.TrimSpace(profile.EnhancementTiersEnabled)), "parity profiles must disable enhancement tiers")
						require.Contains(t, []string{"tokenizer_backed", "heuristic"}, strings.ToLower(strings.TrimSpace(profile.TokenCounterMode)), "parity profiles must pin token counter mode")
						require.Greater(t, profile.FixedSeed, int64(0), "parity profiles must pin fixed seed")
					}
				})
			}
		})
	}
}
