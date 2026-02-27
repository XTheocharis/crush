package repomap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type verticalSliceFixture struct {
	Name       string            `json:"name"`
	Repository fixtureRepository `json:"repository"`
	Pipeline   fixturePipeline   `json:"pipeline"`
	Profiles   []fixtureProfile  `json:"profiles"`
	Assertions fixtureAssertions `json:"assertions"`
}

type fixtureRepository struct {
	Root  string   `json:"root"`
	Files []string `json:"files"`
}

type fixturePipeline struct {
	Stage0 []string           `json:"stage0_special_files"`
	Stage1 []fixtureRankedDef `json:"stage1_ranked_defs"`
	Stage2 []string           `json:"stage2_graph_nodes"`
	Stage3 []string           `json:"stage3_repo_files"`
}

type fixtureRankedDef struct {
	File  string  `json:"file"`
	Ident string  `json:"ident"`
	Rank  float64 `json:"rank"`
}

type fixtureProfile struct {
	Name                    string `json:"name"`
	ParityMode              bool   `json:"parity_mode"`
	TokenBudget             int    `json:"token_budget"`
	RepeatRuns              int    `json:"repeat_runs"`
	DeterministicMode       bool   `json:"deterministic_mode"`
	EnhancementTiersEnabled string `json:"enhancement_tiers_enabled"`
	TokenCounterMode        string `json:"token_counter_mode"`
	FixedSeed               int64  `json:"fixed_seed"`
	ExpectedRawHash         string `json:"expected_raw_hash"`
	ExpectedNormalizedHash  string `json:"expected_normalized_hash"`
}

type fixtureAssertions struct {
	RequireNonEmptyMap     bool  `json:"require_non_empty_map"`
	RequireRenderedEntries bool  `json:"require_rendered_entries"`
	RequireStageInvariants bool  `json:"require_stage_invariants"`
	RequireTrimOrder       []int `json:"require_trim_order"`
}

type verticalSliceResult struct {
	MapText                string
	Entries                []StageEntry
	ParityTokens           float64
	SafetyTokens           int
	RawHash                string
	NormalizedHash         string
	TrimmedStages          []int
	Stage0Preserved        bool
	RenderedFileEntryCount int
	ComparatorAccepted     bool
	ComparatorDelta        float64
}

// TestStageAssemblyInvariants validates the 3A.0 stage assembly semantics:
// - stage0 prelude (optional): Special files appended at top (prepend priority)
// - stage1 ranked defs: Ranked definitions with identifiers
// - stage2 graph-node filenames: Graph node file references
// - stage3 remaining filenames: Remaining repository files
//
// Tests are fixture-driven and verify:
// - Stage0 prepend priority when present
// - Non-decreasing stage order (0->1->2->3)
// - Stage1 entries have identifiers
// - Stage2/Stage3 entries are file references only
// - Trim order 3->2->1 while preserving stage0 prepend priority
func TestStageAssemblyInvariants(t *testing.T) {
	t.Parallel()

	fixtures := loadVerticalSliceFixtures(t)
	for _, fx := range fixtures {
		t.Run(fx.Name, func(t *testing.T) {
			t.Parallel()
			for _, profile := range fx.Profiles {
				t.Run(profile.Name, func(t *testing.T) {
					t.Parallel()

					const repeats = 5
					for run := range repeats {
						result := runVerticalSliceHarness(fx, profile)

						// Always assert stage invariants for Stage tests
						assertStageAssemblyInvariants(t, fx, result)

						// Assert trim order if required
						if len(fx.Assertions.RequireTrimOrder) > 0 {
							assertTrimOrderInvariant(t, fx.Assertions.RequireTrimOrder, result.TrimmedStages)
						}

						// Verify stage0 prepend priority when fixture has stage0
						if len(fx.Pipeline.Stage0) > 0 {
							if !result.Stage0Preserved {
								t.Errorf("run=%d profile=%q fixture=%q: stage0 files not preserved despite fixture having %d stage0 files",
									run, profile.Name, fx.Name, len(fx.Pipeline.Stage0))
							}
						}
					}
				})
			}
		})
	}
}

func TestVerticalSliceHarnessProfiles(t *testing.T) {
	t.Parallel()

	fixtures := loadVerticalSliceFixtures(t)
	for _, fx := range fixtures {
		t.Run(fx.Name, func(t *testing.T) {
			t.Parallel()
			for _, profile := range fx.Profiles {
				t.Run(profile.Name, func(t *testing.T) {
					t.Parallel()

					repeats := profile.RepeatRuns
					if repeats <= 0 {
						repeats = 10
					}

					hashes := make([]string, 0, repeats)
					results := make([]verticalSliceResult, 0, repeats)

					for run := range repeats {
						result := runVerticalSliceHarness(fx, profile)
						assertVerticalSliceResult(t, fx, profile, result, run)
						results = append(results, result)
						if profile.ParityMode {
							hashes = append(hashes, result.NormalizedHash)
						} else {
							hashes = append(hashes, result.RawHash)
						}
					}

					for i := 1; i < len(hashes); i++ {
						require.Equal(t, hashes[0], hashes[i], "determinism failure for profile=%q fixture=%q: run0=%s run%d=%s", profile.Name, fx.Name, hashes[0], i, hashes[i])
					}

					if profile.ExpectedRawHash != "" && results[0].RawHash != profile.ExpectedRawHash {
						require.Equal(t, profile.ExpectedRawHash, results[0].RawHash, "raw hash mismatch for profile=%q fixture=%q: want=%s got=%s", profile.Name, fx.Name, profile.ExpectedRawHash, results[0].RawHash)
					}
					if profile.ExpectedNormalizedHash != "" && results[0].NormalizedHash != profile.ExpectedNormalizedHash {
						require.Equal(t, profile.ExpectedNormalizedHash, results[0].NormalizedHash, "normalized hash mismatch for profile=%q fixture=%q: want=%s got=%s", profile.Name, fx.Name, profile.ExpectedNormalizedHash, results[0].NormalizedHash)
					}
				})
			}
		})
	}
}

func assertVerticalSliceResult(t *testing.T, fx verticalSliceFixture, profile fixtureProfile, result verticalSliceResult, run int) {
	t.Helper()

	if fx.Assertions.RequireNonEmptyMap {
		require.NotEmpty(t, strings.TrimSpace(result.MapText), "run=%d profile=%q fixture=%q: expected non-empty map", run, profile.Name, fx.Name)
	}
	if fx.Assertions.RequireRenderedEntries {
		require.Greater(t, result.RenderedFileEntryCount, 0, "run=%d profile=%q fixture=%q: expected at least one rendered file entry", run, profile.Name, fx.Name)
	}
	if fx.Assertions.RequireStageInvariants {
		assertStageAssemblyInvariants(t, fx, result)
	}
	if len(fx.Assertions.RequireTrimOrder) > 0 {
		assertTrimOrderInvariant(t, fx.Assertions.RequireTrimOrder, result.TrimmedStages)
	}

	if profile.ParityMode {
		require.True(t, profile.DeterministicMode, "run=%d profile=%q fixture=%q: parity mode requires deterministic_mode=true", run, profile.Name, fx.Name)
		require.Equal(t, "none", strings.ToLower(strings.TrimSpace(profile.EnhancementTiersEnabled)), "run=%d profile=%q fixture=%q: parity mode requires enhancement_tiers_enabled=none", run, profile.Name, fx.Name)
		counterMode := strings.ToLower(strings.TrimSpace(profile.TokenCounterMode))
		require.Equal(t, "tokenizer_backed", counterMode, "run=%d profile=%q fixture=%q: parity mode requires token_counter_mode tokenizer_backed", run, profile.Name, fx.Name)
		require.Greater(t, profile.FixedSeed, int64(0), "run=%d profile=%q fixture=%q: parity mode requires fixed_seed > 0", run, profile.Name, fx.Name)
		require.Greater(t, result.ParityTokens, float64(0), "run=%d profile=%q fixture=%q: expected parity tokens > 0", run, profile.Name, fx.Name)
		require.True(t, result.ComparatorAccepted, "run=%d profile=%q fixture=%q: expected comparator acceptance in parity mode", run, profile.Name, fx.Name)
	} else {
		require.LessOrEqual(t, result.SafetyTokens, profile.TokenBudget, "run=%d profile=%q fixture=%q: safety token violation: safety=%d budget=%d", run, profile.Name, fx.Name, result.SafetyTokens, profile.TokenBudget)
	}
}

func assertStageAssemblyInvariants(t *testing.T, fx verticalSliceFixture, result verticalSliceResult) {
	t.Helper()

	// Stage0 prelude invariant: If stage0 files exist in fixture,
	// they must ALL appear at the start of rendered output (prepend priority)
	stage0Files := fx.Pipeline.Stage0
	if len(stage0Files) > 0 && result.Stage0Preserved {
		// Verify all stage0 files are rendered first
		resultStage0Files := make([]string, 0)
		stageOrder := make([]int, 0, len(result.Entries))

		for _, e := range result.Entries {
			stageOrder = append(stageOrder, e.Stage)
			if e.Stage == 0 {
				resultStage0Files = append(resultStage0Files, e.File)
			}
		}

		// Verify stage0 files come first (non-decreasing order)
		require.True(t, isNonDecreasing(stageOrder), "fixture=%q: rendered stage order must remain 0->1->2->3, got=%v", fx.Name, stageOrder)

		// Verify all fixture stage0 files are in result
		for _, s0File := range stage0Files {
			require.True(t, slices.Contains(resultStage0Files, s0File), "fixture=%q: stage0 file %q from fixture not found in rendered result", fx.Name, s0File)
		}
	} else if len(stage0Files) == 0 {
		// When fixture has no stage0, verify result has none
		for _, e := range result.Entries {
			require.False(t, e.Stage == 0, "fixture=%q: result contains stage0 entries but fixture has none (entry: %q)", fx.Name, e.File)
		}

		// Still verify non-decreasing stage order (1->2->3)
		stageOrder := make([]int, 0, len(result.Entries))
		for _, e := range result.Entries {
			stageOrder = append(stageOrder, e.Stage)
		}
		require.True(t, isNonDecreasing(stageOrder), "fixture=%q: rendered stage order must remain 1->2->3 (no stage0), got=%v", fx.Name, stageOrder)
	}

	// Stage1 ranked defs invariant: Verify stage1 entries (if any) have identifiers
	for i, e := range result.Entries {
		if e.Stage == 1 {
			require.NotEmpty(t, e.Ident, "fixture=%q: stage1 entry at position %d with file %q must have identifier (ident)", fx.Name, i, e.File)
		}
	}

	// Stage2 graph nodes invariant: Stage2 entries should be file references only
	for i, e := range result.Entries {
		if e.Stage == 2 {
			require.Empty(t, e.Ident, "fixture=%q: stage2 entry at position %d should not have identifier (got: %q)", fx.Name, i, e.Ident)
		}
	}

	// Stage3 remaining files invariant: Stage3 entries should be file references only
	for i, e := range result.Entries {
		if e.Stage == 3 {
			require.Empty(t, e.Ident, "fixture=%q: stage3 entry at position %d should not have identifier (got: %q)", fx.Name, i, e.Ident)
		}
	}

	// Semantic assembly invariant: Verify stages appear in semantic order
	// Stage0 (prelude) -> Stage1 (ranked defs) -> Stage2 (graph nodes) -> Stage3 (remaining)
	// This is already covered by non-decreasing check, but we can be more explicit
	for i := 1; i < len(result.Entries); i++ {
		prev, curr := result.Entries[i-1].Stage, result.Entries[i].Stage
		require.GreaterOrEqual(t, curr, prev, "fixture=%q: stage assembly violation at position %d: previous=%d current=%d", fx.Name, i, prev, curr)
	}
}

func assertTrimOrderInvariant(t *testing.T, requiredOrder []int, observed []int) {
	t.Helper()

	if len(observed) == 0 {
		return
	}
	pos := make(map[int]int, len(requiredOrder))
	for i, stage := range requiredOrder {
		pos[stage] = i
	}
	for i := 1; i < len(observed); i++ {
		prev := observed[i-1]
		curr := observed[i]
		require.GreaterOrEqual(t, pos[curr], pos[prev], "trim order violation: required=%v observed=%v", requiredOrder, observed)
	}
}

func isNonDecreasing(values []int) bool {
	for i := 1; i < len(values); i++ {
		if values[i] < values[i-1] {
			return false
		}
	}
	return true
}

func runVerticalSliceHarness(fx verticalSliceFixture, profile fixtureProfile) verticalSliceResult {
	entries := extractFixtureEntries(fx)

	counter := TokenCounter(nil)
	model := ""
	if profile.ParityMode {
		counter = fakeCounter{out: profile.TokenBudget}
		model = "fixture-parity"
	}

	fit, err := FitToBudget(context.Background(), entries, BudgetProfile{
		ParityMode:   profile.ParityMode,
		TokenBudget:  profile.TokenBudget,
		Model:        model,
		LanguageHint: "default",
	}, counter)
	if err != nil {
		panic(fmt.Sprintf("fit fixture to budget: %v", err))
	}

	rendered := renderStageEntries(fit.Entries)

	return verticalSliceResult{
		MapText:                rendered,
		Entries:                fit.Entries,
		ParityTokens:           fit.ParityTokens,
		SafetyTokens:           fit.SafetyTokens,
		RawHash:                stableHash(rendered),
		NormalizedHash:         stableHash(normalizeParityMap(rendered)),
		TrimmedStages:          fit.TrimmedStages,
		Stage0Preserved:        allStage0Preserved(entries, fit.Entries),
		RenderedFileEntryCount: countRenderedFileEntries(fit.Entries),
		ComparatorAccepted:     fit.ComparatorAccepted,
		ComparatorDelta:        fit.ComparatorDelta,
	}
}

func extractFixtureEntries(fx verticalSliceFixture) []StageEntry {
	rankedDefs := make([]RankedDefinition, 0, len(fx.Pipeline.Stage1))
	for _, def := range fx.Pipeline.Stage1 {
		rankedDefs = append(rankedDefs, RankedDefinition(def))
	}
	return AssembleStageEntries(
		fx.Pipeline.Stage0,
		rankedDefs,
		fx.Pipeline.Stage2,
		fx.Pipeline.Stage3,
		nil,
		false,
	)
}

func allStage0Preserved(original []StageEntry, selected []StageEntry) bool {
	need := map[string]struct{}{}
	for _, e := range original {
		if e.Stage == 0 {
			need[e.File] = struct{}{}
		}
	}
	if len(need) == 0 {
		return true
	}
	for _, e := range selected {
		if e.Stage == 0 {
			delete(need, e.File)
		}
	}
	return len(need) == 0
}

func countRenderedFileEntries(entries []StageEntry) int {
	count := 0
	for _, e := range entries {
		if e.Stage == 1 || e.Stage == 2 || e.Stage == 3 {
			count++
		}
	}
	return count
}

func normalizeParityMap(text string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	stage3 := make([]string, 0)
	other := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "S3|") {
			stage3 = append(stage3, line)
		} else {
			other = append(other, line)
		}
	}
	sort.Strings(stage3)
	normalized := append(other, stage3...)
	return strings.Join(normalized, "\n") + "\n"
}

func stableHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func loadVerticalSliceFixtures(t *testing.T) []verticalSliceFixture {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join("testdata", "vertical_slice", "*.json"))
	require.NoError(t, err, "glob fixtures: %v", err)
	require.NotEmpty(t, paths, "no vertical slice fixtures found")
	sort.Strings(paths)

	fixtures := make([]verticalSliceFixture, 0, len(paths))
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr, "read fixture %q: %v", path, readErr)

		var fx verticalSliceFixture
		require.NoError(t, json.Unmarshal(data, &fx), "unmarshal fixture %q: %v", path)
		require.NotEmpty(t, strings.TrimSpace(fx.Name), "fixture %q missing name", path)
		require.NotEmpty(t, fx.Profiles, "fixture %q missing profiles", path)
		fixtures = append(fixtures, fx)
	}
	return fixtures
}

// TestBudget3A0 asserts budget semantics for 3A.0 profiles:
//
//   - Parity mode: Validates comparator acceptance path using parityTokenCount semantics.
//     The comparator must accept the result (delta <= 0.15) and parityTokens must be > 0.
//
//   - Enhancement mode: Enforces safetyTokenCount <= TokenBudget. The safety token
//     counter provides a conservative higher-bound estimate to ensure maps never
//     exceed the budget regardless of tokenizer variance.
//
// Both counters (parityTokens and safetyTokens) are recorded and asserted across
// all fixture profiles.
func TestBudget3A0(t *testing.T) {
	t.Parallel()

	fixtures := loadVerticalSliceFixtures(t)
	for _, fx := range fixtures {
		t.Run(fx.Name, func(t *testing.T) {
			t.Parallel()
			for _, profile := range fx.Profiles {
				t.Run(profile.Name, func(t *testing.T) {
					t.Parallel()

					repeats := profile.RepeatRuns
					if repeats <= 0 {
						repeats = 1
					}

					for run := 0; run < repeats; run++ {
						result := runVerticalSliceHarness(fx, profile)

						t.Logf("run=%d profile=%q: parityTokens=%.2f safetyTokens=%d comparatorDelta=%.4f comparatorAccepted=%v budget=%d",
							run, profile.Name, result.ParityTokens, result.SafetyTokens,
							result.ComparatorDelta, result.ComparatorAccepted, profile.TokenBudget)

						// Record both counters for 3A.0 criterion #2
						if result.ParityTokens <= 0 {
							t.Errorf("run=%d: parityTokens must be > 0, got %.2f", run, result.ParityTokens)
						}

						if result.SafetyTokens < 0 {
							t.Errorf("run=%d: safetyTokens must be non-negative, got %d", run, result.SafetyTokens)
						}

						// Parity mode: validate comparator acceptance path using parityTokenCount semantics
						if profile.ParityMode {
							if result.ParityTokens <= 0 {
								t.Errorf("parity mode: run=%d profile=%q: parityTokenCount must be positive for comparator evaluation, got %.2f",
									run, profile.Name, result.ParityTokens)
							}
							if !result.ComparatorAccepted {
								t.Errorf("parity mode: run=%d profile=%q: comparator must accept (delta <= 0.15), got delta=%.4f",
									run, profile.Name, result.ComparatorDelta)
							}
							if result.ComparatorDelta < 0 {
								t.Logf("parity mode: run=%d profile=%q: under budget (delta=%.4f < 0), tokens saved",
									run, profile.Name, result.ComparatorDelta)
							}
						}

						// Enhancement mode: enforce safetyTokenCount <= TokenBudget
						if !profile.ParityMode {
							if result.SafetyTokens > profile.TokenBudget {
								t.Errorf("enhancement mode: run=%d profile=%q: budget violation: safetyTokenCount=%d > TokenBudget=%d",
									run, profile.Name, result.SafetyTokens, profile.TokenBudget)
							}
							if result.SafetyTokens == profile.TokenBudget {
								t.Logf("enhancement mode: run=%d profile=%q: budget saturated (safetyTokens=%d == TokenBudget=%d)",
									run, profile.Name, result.SafetyTokens, profile.TokenBudget)
							}
						}
					}
				})
			}
		})
	}
}
