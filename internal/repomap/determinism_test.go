package repomap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestVerticalSliceDeterminism asserts that the vertical slice harness produces
// deterministic results across repeated runs. It validates:
//
//   - Files refresh mode: 10 repeated runs with unchanged inputs produce
//     byte-identical output (raw hash comparison).
//   - Parity mode: Comparator-normalized hashes are stable across repeats.
//   - Enhancement mode: Raw byte-identical hashes are stable across repeats.
//
// All determinism assertions are fixture-driven and use stable hashing.
func TestVerticalSliceDeterminism(t *testing.T) {
	t.Parallel()

	fixtures := loadVerticalSliceFixtures(t)
	for _, fx := range fixtures {
		t.Run(fx.Name, func(t *testing.T) {
			t.Parallel()
			for _, profile := range fx.Profiles {
				t.Run(profile.Name, func(t *testing.T) {
					t.Parallel()

					// Require 10 repeated runs as specified in 3A.0
					const requiredRuns = 10
					if profile.RepeatRuns <= 0 {
						profile.RepeatRuns = requiredRuns
					} else if profile.RepeatRuns < requiredRuns {
						t.Fatalf("fixture profile %q requires at least %d repeat runs for determinism validation, got %d",
							profile.Name, requiredRuns, profile.RepeatRuns)
					}

					// Collect hashes across all runs
					hashes := make([]string, 0, profile.RepeatRuns)
					results := make([]verticalSliceResult, 0, profile.RepeatRuns)

					for run := 0; run < profile.RepeatRuns; run++ {
						result := runVerticalSliceHarness(fx, profile)
						results = append(results, result)

						if profile.ParityMode {
							// Parity mode: Compare comparator-normalized hashes
							hashes = append(hashes, result.NormalizedHash)
						} else {
							// Enhancement/files refresh mode: Compare raw byte-identical hashes
							hashes = append(hashes, result.RawHash)
						}
					}

					// Assert determinism: All runs must produce identical hashes
					for i := 1; i < len(hashes); i++ {
						if hashes[i] != hashes[0] {
							t.Errorf("determinism failure for profile=%q fixture=%q: run0=%s run%d=%s\n"+
								"First 100 chars of run0 map: %q\n"+
								"First 100 chars of run%d map: %q",
								profile.Name, fx.Name, hashes[0], i, hashes[i],
								truncateString(results[0].MapText, 100), i, truncateString(results[i].MapText, 100))
						}
					}

					// Additional stability assertions
					assertDeterminismInvariants(t, fx, profile, results)
				})
			}
		})
	}
}

// TestVerticalSliceFilesRefreshModeDeterminism specifically validates the files
// refresh mode determinism requirements: 10 repeated runs with unchanged inputs
// must produce byte-identical output (raw hash comparison).
func TestVerticalSliceFilesRefreshModeDeterminism(t *testing.T) {
	t.Parallel()

	// Build a synthetic fixture for files refresh mode testing
	fx := verticalSliceFixture{
		Name: "files_refresh_determinism",
		Repository: fixtureRepository{
			Root:  "",
			Files: nil,
		},
		Pipeline: fixturePipeline{
			Stage0: []string{"README.md"},
			Stage1: []fixtureRankedDef{
				{File: "cmd/main.go", Ident: "main", Rank: 0.88},
				{File: "internal/core/service.go", Ident: "Run", Rank: 0.81},
				{File: "internal/core/model.go", Ident: "Process", Rank: 0.75},
			},
			Stage2: []string{"internal/core/model.go"},
			Stage3: []string{
				"internal/util/strings.go",
				"go.mod",
				"docs/notes.txt",
				"internal/helpers/time.go",
			},
		},
		Profiles: []fixtureProfile{
			{
				Name:        "files_refresh_mode",
				ParityMode:  false,
				TokenBudget: 35,
				RepeatRuns:  10,
			},
		},
		Assertions: fixtureAssertions{
			RequireNonEmptyMap:     true,
			RequireRenderedEntries: true,
			RequireStageInvariants: true,
			RequireTrimOrder:       []int{3, 2, 1},
		},
	}

	// 10 repeated runs with unchanged inputs as required
	const runs = 10
	rawHashes := make([]string, runs)
	maps := make([]string, runs)

	for i := range runs {
		result := runVerticalSliceHarness(fx, fx.Profiles[0])
		rawHashes[i] = result.RawHash
		maps[i] = result.MapText
	}

	// enhancement mode: compare raw byte-identical hashes
	for i := 1; i < runs; i++ {
		if rawHashes[i] != rawHashes[0] {
			t.Errorf("files refresh mode determinism failure: run0=%s run%d=%s\n"+
				"run0 map (first 200 chars): %q\n"+
				"run%d map (first 200 chars): %q",
				rawHashes[0], i, rawHashes[i],
				truncateString(maps[0], 200), i, truncateString(maps[i], 200))
		}
	}

	// Verify byte-level identity across runs
	firstMapBytes := []byte(maps[0])
	for i := 1; i < runs; i++ {
		if string(firstMapBytes) != maps[i] {
			t.Errorf("files refresh mode byte-identity failure: run%d differs byte-by-byte from run0", i)
		}
	}
}

// TestVerticalSliceParityModeDeterminism specifically validates the parity
// mode determinism requirements: comparator-normalized hashes must be stable
// across repeated runs, regardless of stage3 ordering variations.
func TestVerticalSliceParityModeDeterminism(t *testing.T) {
	t.Parallel()

	// Build a synthetic fixture for parity mode testing
	fx := verticalSliceFixture{
		Name: "parity_mode_determinism",
		Repository: fixtureRepository{
			Root:  "",
			Files: nil,
		},
		Pipeline: fixturePipeline{
			Stage0: []string{"README.md"},
			Stage1: []fixtureRankedDef{
				{File: "cmd/main.go", Ident: "main", Rank: 0.88},
				{File: "internal/core/service.go", Ident: "Run", Rank: 0.81},
			},
			Stage2: []string{"internal/core/model.go"},
			Stage3: []string{
				"internal/util/strings.go",
				"go.mod",
				"docs/notes.txt",
				"pkg/http/client.go",
				"pkg/http/router.go",
			},
		},
		Profiles: []fixtureProfile{
			{
				Name:        "parity_mode",
				ParityMode:  true,
				TokenBudget: 35,
				RepeatRuns:  10,
			},
		},
		Assertions: fixtureAssertions{
			RequireNonEmptyMap:     true,
			RequireRenderedEntries: true,
			RequireStageInvariants: true,
		},
	}

	// 10 repeated runs as required
	const runs = 10
	normalizedHashes := make([]string, runs)

	for i := range runs {
		result := runVerticalSliceHarness(fx, fx.Profiles[0])
		normalizedHashes[i] = result.NormalizedHash

		// Verify comparator accepted the result in parity mode
		if !result.ComparatorAccepted {
			t.Errorf("run %d: parity mode expected comparator acceptance", i)
		}
	}

	// parity mode: compare comparator-normalized hashes
	for i := 1; i < runs; i++ {
		if normalizedHashes[i] != normalizedHashes[0] {
			t.Errorf("parity mode determinism failure: run0=%s run%d=%s",
				normalizedHashes[0], i, normalizedHashes[i])
		}
	}
}

// TestVerticalSliceEnhancementModeDeterminism specifically validates the
// enhancement mode determinism requirements: raw byte-identical hashes must
// be stable across repeated runs, preserving exact output including ordering.
func TestVerticalSliceEnhancementModeDeterminism(t *testing.T) {
	t.Parallel()

	// Build a synthetic fixture for enhancement mode testing
	fx := verticalSliceFixture{
		Name: "enhancement_mode_determinism",
		Repository: fixtureRepository{
			Root:  "",
			Files: nil,
		},
		Pipeline: fixturePipeline{
			Stage0: []string{"README.md"},
			Stage1: []fixtureRankedDef{
				{File: "cmd/main.go", Ident: "main", Rank: 0.88},
				{File: "internal/core/service.go", Ident: "Run", Rank: 0.81},
				{File: "internal/core/model.go", Ident: "Process", Rank: 0.75},
			},
			Stage2: []string{"internal/core/model.go"},
			Stage3: []string{
				"internal/util/strings.go",
				"go.mod",
			},
		},
		Profiles: []fixtureProfile{
			{
				Name:        "enhancement_mode",
				ParityMode:  false,
				TokenBudget: 30,
				RepeatRuns:  10,
			},
		},
		Assertions: fixtureAssertions{
			RequireNonEmptyMap:     true,
			RequireRenderedEntries: true,
			RequireStageInvariants: true,
			RequireTrimOrder:       []int{3, 2, 1},
		},
	}

	// 10 repeated runs as required
	const runs = 10
	rawHashes := make([]string, runs)
	entriesCount := make([]int, runs)

	for i := range runs {
		result := runVerticalSliceHarness(fx, fx.Profiles[0])
		rawHashes[i] = result.RawHash
		entriesCount[i] = len(result.Entries)

		// Verify safety token budget is respected
		if result.SafetyTokens > fx.Profiles[0].TokenBudget {
			t.Errorf("run %d: safety token violation: safety=%d budget=%d",
				i, result.SafetyTokens, fx.Profiles[0].TokenBudget)
		}
	}

	// enhancement mode: compare raw byte-identical hashes
	for i := 1; i < runs; i++ {
		if rawHashes[i] != rawHashes[0] {
			t.Errorf("enhancement mode determinism failure: run0=%s run%d=%s",
				rawHashes[0], i, rawHashes[i])
		}
	}

	// Entry count must also be stable
	for i := 1; i < runs; i++ {
		if entriesCount[i] != entriesCount[0] {
			t.Errorf("enhancement mode entry count instability: run0=%d run%d=%d",
				entriesCount[0], i, entriesCount[i])
		}
	}
}

// TestVerticalSliceDeterminismAcrossModes verifies that the determinism
// assertions apply consistently across all operational modes.
func TestVerticalSliceDeterminismAcrossModes(t *testing.T) {
	t.Parallel()

	modes := []struct {
		name       string
		parityMode bool
		expected   string // "normalized" or "raw"
	}{
		{"files_refresh", false, "raw"},
		{"enhancement", false, "raw"},
		{"parity", true, "normalized"},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()

			fx := verticalSliceFixture{
				Name: fmt.Sprintf("mode_%s_determinism", mode.name),
				Repository: fixtureRepository{
					Root:  "",
					Files: nil,
				},
				Pipeline: fixturePipeline{
					Stage0: []string{"README.md"},
					Stage1: []fixtureRankedDef{
						{File: "cmd/main.go", Ident: "main", Rank: 0.88},
					},
					Stage2: []string{"internal/core/model.go"},
					Stage3: []string{"go.mod"},
				},
				Profiles: []fixtureProfile{
					{
						Name:        mode.name,
						ParityMode:  mode.parityMode,
						TokenBudget: 20,
						RepeatRuns:  10,
					},
				},
				Assertions: fixtureAssertions{
					RequireNonEmptyMap: true,
				},
			}

			const runs = 10
			hashes := make(map[string]int)

			for i := range runs {
				result := runVerticalSliceHarness(fx, fx.Profiles[0])
				var hash string
				if mode.parityMode {
					hash = result.NormalizedHash
				} else {
					hash = result.RawHash
				}
				hashes[hash]++

				// Track which hash types we're using
				t.Logf("run %d: used %s hash %s", i, mode.expected, hash[:8])
			}

			// All runs must produce identical hashes
			if len(hashes) != 1 {
				uniqueHashes := make([]string, 0, len(hashes))
				for h := range hashes {
					uniqueHashes = append(uniqueHashes, h)
				}
				sort.Strings(uniqueHashes)
				t.Errorf("mode %s produced %d distinct hashes instead of 1: %v",
					mode.name, len(hashes), uniqueHashes)
			}
		})
	}
}

func assertDeterminismInvariants(t *testing.T, fx verticalSliceFixture, profile fixtureProfile, results []verticalSliceResult) {
	t.Helper()

	// All runs should have identical entry counts
	firstCount := len(results[0].Entries)
	for i, r := range results {
		if len(r.Entries) != firstCount {
			t.Errorf("entry count instability: run0=%d run%d=%d",
				firstCount, i, len(r.Entries))
		}
	}

	// All runs should have identical trimmed stages
	for i, r := range results {
		if !slicesEqual(r.TrimmedStages, results[0].TrimmedStages) {
			t.Errorf("trimmed stages instability: run0=%v run%d=%v",
				results[0].TrimmedStages, i, r.TrimmedStages)
		}
	}

	// Parity mode specific invariants
	if profile.ParityMode {
		for i, r := range results {
			if !r.ComparatorAccepted {
				t.Errorf("run %d: parity mode expected comparator acceptance", i)
			}
			if r.ParityTokens <= 0 {
				t.Errorf("run %d: parity mode expected positive parity tokens", i)
			}
		}
	}

	// Stage invariants if required
	if fx.Assertions.RequireStageInvariants {
		for _, r := range results {
			assertStageAssemblyInvariants(t, fx, r)
		}
	}
}

func slicesEqual[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// loadVerticalSliceFixturesForDeterminism is a variant that ensures fixtures meet
// determinism test requirements.
func loadVerticalSliceFixturesForDeterminism(t *testing.T) []verticalSliceFixture {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join("testdata", "vertical_slice", "*.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("no vertical slice fixtures found")
	}
	sort.Strings(paths)

	fixtures := make([]verticalSliceFixture, 0, len(paths))
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read fixture %q: %v", path, readErr)
		}
		var fx verticalSliceFixture
		if unmarshalErr := json.Unmarshal(data, &fx); unmarshalErr != nil {
			t.Fatalf("unmarshal fixture %q: %v", path, unmarshalErr)
		}

		// Validate determinism-specific requirements
		if strings.TrimSpace(fx.Name) == "" {
			t.Fatalf("fixture %q missing name", path)
		}
		if len(fx.Profiles) == 0 {
			t.Fatalf("fixture %q missing profiles", path)
		}

		fixtures = append(fixtures, fx)
	}
	return fixtures
}
