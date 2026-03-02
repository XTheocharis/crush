package repomap

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

func TestParityGateA1RankingConcordance(t *testing.T) {
	t.Parallel()

	require.NoError(t, runGateA1RankingConcordance(t))
}

func TestParityGateA1RankingConcordanceThresholdEnforcement(t *testing.T) {
	t.Parallel()

	// Intentional failure-path test: this pair should violate both thresholds.
	aider := []string{"a.go", "b.go", "c.go", "d.go", "e.go"}
	crush := []string{"z.go", "y.go", "x.go", "w.go", "v.go"}

	metrics, err := computeRankingConcordance(aider, crush, 30)
	require.NoError(t, err, "compute ranking concordance")
	require.Error(t, enforceRankingThresholds(metrics), "expected threshold enforcement error")
}

func TestParityGateA1RealPipelineRanking(t *testing.T) {
	t.Parallel()

	// Build a real repo with cross-references to verify the pipeline produces
	// meaningful rankings. main.go calls Hello() from lib.go and add() from
	// util.go, so lib.go and util.go should appear in the ranked output.
	dir := initGitRepo(t, map[string]string{
		"main.go": "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(Hello())\n\tfmt.Println(add(1, 2))\n}\n",
		"lib.go":  "package main\n\n// Hello returns a greeting.\nfunc Hello() string {\n\treturn \"hello\"\n}\n",
		"util.go": "package main\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n\nfunc unused() {}\n",
	})

	// Extract tags using a real tree-sitter parser.
	parser := treesitter.NewParser()
	defer parser.Close()

	var allTags []treesitter.Tag
	for _, name := range []string{"main.go", "lib.go", "util.go"} {
		content, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		analysis, err := parser.Analyze(context.Background(), name, content)
		require.NoError(t, err)
		allTags = append(allTags, analysis.Tags...)
	}
	require.NotEmpty(t, allTags, "expected tags from Go files")

	// Build graph, rank, assemble stages, render.
	graph := buildGraph(allTags, nil, nil)
	ranked := Rank(graph, nil)
	rankedFiles := AggregateRankedFiles(ranked, allTags)

	fileRanking := make([]string, 0, len(rankedFiles))
	for _, rf := range rankedFiles {
		fileRanking = append(fileRanking, rf.Path)
	}
	require.NotEmpty(t, fileRanking, "expected ranked files")

	// lib.go and util.go define symbols called by main.go — they should rank.
	require.Contains(t, fileRanking, "lib.go", "lib.go should appear in ranking")
	require.Contains(t, fileRanking, "util.go", "util.go should appear in ranking")

	// Build tag map and render.
	tagsByFile := make(map[string][]treesitter.Tag)
	for _, tag := range allTags {
		tagsByFile[tag.RelPath] = append(tagsByFile[tag.RelPath], tag)
	}

	entries := AssembleStageEntries(nil, ranked, nil, nil, nil, false)
	require.NotEmpty(t, entries, "expected stage entries")

	rendered, err := RenderRepoMap(context.Background(), entries, tagsByFile, parser, dir)
	require.NoError(t, err)
	require.NotEmpty(t, rendered, "expected non-empty rendered repo map")
	require.Contains(t, rendered, "Hello", "Hello function should appear in rendered map")
}

func TestParityGateA2StageRenderFidelity(t *testing.T) {
	t.Parallel()

	require.NoError(t, runGateA2StageRenderFidelity(t))
}

func TestParityGateA3TokenSafetyAccounting(t *testing.T) {
	t.Parallel()

	require.NoError(t, runGateA3TokenSafetyAccounting(t))
}

func TestParityGateA4RefreshSemantics(t *testing.T) {
	t.Parallel()

	require.NoError(t, runGateA4RefreshSemantics(t))
}

func TestParityGateA5Determinism(t *testing.T) {
	t.Parallel()

	require.NoError(t, runGateA5Determinism(t))
}

func TestParityGateA6ParityLeakageGuard(t *testing.T) {
	t.Parallel()

	require.NoError(t, runGateA6ParityLeakageGuard())
}

func TestParityGateAAggregate(t *testing.T) {
	require.NoError(t, runGateA1RankingConcordance(t), "A1 ranking concordance failed")
	require.NoError(t, runGateA2StageRenderFidelity(t), "A2 stage/render fidelity failed")
	require.NoError(t, runGateA3TokenSafetyAccounting(t), "A3 token safety/accounting failed")
	require.NoError(t, runGateA4RefreshSemantics(t), "A4 refresh semantics failed")
	require.NoError(t, runGateA5Determinism(t), "A5 determinism failed")
	require.NoError(t, runGateA6ParityLeakageGuard(), "A6 parity leakage guard failed")
}

func runGateA1RankingConcordance(t *testing.T) error {
	t.Helper()

	fixtures, err := LoadParityAiderFixtures(".")
	if err != nil {
		return err
	}
	if len(fixtures) == 0 {
		return fmt.Errorf("no parity fixtures found")
	}

	for _, fx := range fixtures {
		aiderRanking := uniqueOrdered(fx.Repository.Files)
		if len(aiderRanking) == 0 {
			return fmt.Errorf("fixture %q missing repository files ranking", fx.FixtureID)
		}

		for _, budget := range []int{1024, 2048, 4096} {
			crushRanking, ok := parityRankingForBudget(fx, budget)
			if !ok {
				return fmt.Errorf("fixture %q missing parity profile for token budget %d", fx.FixtureID, budget)
			}

			metrics, err := computeRankingConcordance(aiderRanking, crushRanking, 30)
			if err != nil {
				return fmt.Errorf("fixture %q budget %d ranking concordance: %w", fx.FixtureID, budget, err)
			}
			if err := enforceRankingThresholds(metrics); err != nil {
				return fmt.Errorf("fixture %q budget %d ranking concordance thresholds failed: %w", fx.FixtureID, budget, err)
			}
		}
	}
	return nil
}

func runGateA2StageRenderFidelity(t *testing.T) error {
	t.Helper()

	fixtures := loadVerticalSliceFixtures(t)
	for _, fx := range fixtures {
		for _, profile := range fx.Profiles {
			result := runVerticalSliceHarness(fx, profile)
			assertStageAssemblyInvariants(t, fx, result)

			if fx.Assertions.RequireRenderedEntries && result.RenderedFileEntryCount <= 0 {
				return fmt.Errorf("fixture %q profile %q: expected rendered entries > 0", fx.Name, profile.Name)
			}

			lines := strings.Split(strings.TrimSpace(result.MapText), "\n")
			if strings.TrimSpace(result.MapText) != "" && len(lines) != len(result.Entries) {
				return fmt.Errorf("fixture %q profile %q: render fidelity mismatch lines=%d entries=%d", fx.Name, profile.Name, len(lines), len(result.Entries))
			}

			if len(fx.Assertions.RequireTrimOrder) > 0 {
				assertTrimOrderInvariant(t, fx.Assertions.RequireTrimOrder, result.TrimmedStages)
			}
		}
	}

	return nil
}

func runGateA3TokenSafetyAccounting(t *testing.T) error {
	t.Helper()

	fixtures := loadVerticalSliceFixtures(t)
	for _, fx := range fixtures {
		for _, profile := range fx.Profiles {
			result := runVerticalSliceHarness(fx, profile)
			if profile.ParityMode {
				if !result.ComparatorAccepted || result.ComparatorDelta > 0.15 {
					return fmt.Errorf("fixture %q profile %q parity comparator acceptance failed: accepted=%v delta=%.4f", fx.Name, profile.Name, result.ComparatorAccepted, result.ComparatorDelta)
				}
				if result.ParityTokens <= 0 {
					return fmt.Errorf("fixture %q profile %q parity token accounting failed: parity_tokens=%.2f", fx.Name, profile.Name, result.ParityTokens)
				}
			} else {
				if result.SafetyTokens > profile.TokenBudget {
					return fmt.Errorf("fixture %q profile %q enhancement safety budget violation: safety=%d budget=%d", fx.Name, profile.Name, result.SafetyTokens, profile.TokenBudget)
				}
			}
		}
	}

	bundle := validParityGateBundle()
	bundle.TokenizerID = "nonexistent-tokenizer"
	bundle.TokenizerVersion = "v9.9.9"
	negErr := RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: true,
		Profile:                validParityGatePreflightProfile(),
	})
	if negErr == nil || !strings.Contains(negErr.Error(), "tokenizer tuple") {
		return fmt.Errorf("expected tokenizer tuple preflight failure, got: %v", negErr)
	}

	bundle = validParityGateBundle()
	bundle.AiderCommitSHA = "bad"
	negErr = RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: false,
		Profile:                validParityGatePreflightProfile(),
	})
	if negErr == nil || !strings.Contains(negErr.Error(), "aider_commit_sha") {
		return fmt.Errorf("expected provenance preflight failure, got: %v", negErr)
	}

	return nil
}

func runGateA4RefreshSemantics(t *testing.T) error {
	t.Helper()

	opts := GenerateOpts{
		SessionID:            "gate-a4",
		ChatFiles:            []string{"a.go"},
		MentionedFnames:      []string{"b.go"},
		MentionedIdents:      []string{"Foo"},
		TokenBudget:          100,
		ParityMode:           true,
		PromptCachingEnabled: true,
	}

	modes := []string{"auto", "files", "manual", "always"}
	for _, mode := range modes {
		svc := NewService(&config.Config{Options: &config.Options{RepoMap: &config.RepoMapOptions{RefreshMode: mode}}}, nil, nil, ".", context.Background())
		defer svc.Close()

		effective := svc.effectiveRefreshMode(opts)
		if mode == "auto" && effective != "files" {
			return fmt.Errorf("mode %q parity prompt-caching coercion failed: expected files, got %q", mode, effective)
		}

		cacheKey := buildRenderCacheKey(effective, opts)
		if cacheKey != "" {
			svc.renderCaches.GetOrCreate(opts.SessionID).Set(cacheKey, "cached-map", 11)
		}

		coldMap, coldTok, err := svc.Generate(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("mode %q cold generate: %w", mode, err)
		}

		switch effective {
		case "files":
			if coldMap != "cached-map" || coldTok != 11 {
				return fmt.Errorf("mode %q (effective %q): expected render-cache fallback on cold start", mode, effective)
			}
		case "manual", "always":
			if coldMap != "" || coldTok != 0 {
				return fmt.Errorf("mode %q (effective %q): expected cold start empty map/tokens", mode, effective)
			}
		}

		svc.sessionCaches.Store(opts.SessionID, "last-good", 7)
		warmMap, warmTok, err := svc.Generate(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("mode %q warm generate: %w", mode, err)
		}
		if warmMap != "last-good" || warmTok != 7 {
			return fmt.Errorf("mode %q: expected last-good precedence", mode)
		}
	}

	filesSvc := NewService(&config.Config{Options: &config.Options{RepoMap: &config.RepoMapOptions{RefreshMode: "files"}}}, nil, nil, ".", context.Background())
	defer filesSvc.Close()
	filesSvc.sessionCaches.Store(opts.SessionID, "refresh-map", 9)
	if _, _, err := filesSvc.Refresh(context.Background(), opts.SessionID, opts); err != nil {
		return fmt.Errorf("files mode refresh failed: %w", err)
	}
	filesKey := buildRenderCacheKey("files", opts)
	if m, tok, ok := filesSvc.renderCaches.GetOrCreate(opts.SessionID).Get(filesKey); !ok || m != "refresh-map" || tok != 9 {
		return fmt.Errorf("files mode refresh did not persist keyed render cache")
	}

	alwaysSvc := NewService(&config.Config{Options: &config.Options{RepoMap: &config.RepoMapOptions{RefreshMode: "always"}}}, nil, nil, ".", context.Background())
	defer alwaysSvc.Close()
	alwaysSvc.sessionCaches.Store(opts.SessionID, "always-refresh", 5)
	if _, _, err := alwaysSvc.Refresh(context.Background(), opts.SessionID, opts); err != nil {
		return fmt.Errorf("always mode refresh failed: %w", err)
	}
	if key := buildRenderCacheKey("always", opts); key != "" {
		return fmt.Errorf("always mode render cache key must be empty, got %q", key)
	}
	if _, _, ok := alwaysSvc.renderCaches.GetOrCreate(opts.SessionID).Get(buildRenderCacheKey("files", opts)); ok {
		return fmt.Errorf("always mode should not backfill keyed render cache")
	}

	return nil
}

func runGateA5Determinism(t *testing.T) error {
	t.Helper()

	fx, err := fixtureByName(t, "basic_go")
	if err != nil {
		return err
	}

	for _, profile := range fx.Profiles {
		runs := max(profile.RepeatRuns, 10)
		seen := make(map[string]struct{}, runs)
		for range runs {
			result := runVerticalSliceHarness(fx, profile)
			h := determinismHashForProfile(profile, result)
			seen[h] = struct{}{}
		}
		if len(seen) != 1 {
			return fmt.Errorf("profile %q determinism mismatch: got %d unique hashes", profile.Name, len(seen))
		}
	}

	sample := verticalSliceResult{RawHash: "raw-hash", NormalizedHash: "normalized-hash"}
	if got := determinismHashForProfile(fixtureProfile{ParityMode: true}, sample); got != "normalized-hash" {
		return fmt.Errorf("parity determinism hash selector mismatch: got %q", got)
	}
	if got := determinismHashForProfile(fixtureProfile{ParityMode: false}, sample); got != "raw-hash" {
		return fmt.Errorf("enhancement determinism hash selector mismatch: got %q", got)
	}

	return nil
}

func runGateA6ParityLeakageGuard() error {
	bundle := validParityGateBundle()
	baseProfile := validParityGatePreflightProfile()

	cases := []struct {
		name    string
		mutate  func(*ParityPreflightProfile)
		expects string
	}{
		{
			name: "reject non-deterministic parity profile",
			mutate: func(p *ParityPreflightProfile) {
				p.DeterministicMode = false
			},
			expects: "deterministic_mode",
		},
		{
			name: "reject enhancement tiers in parity mode",
			mutate: func(p *ParityPreflightProfile) {
				p.EnhancementTiersEnabled = "all"
			},
			expects: "enhancement_tiers_enabled",
		},
		{
			name: "reject invalid token counter mode",
			mutate: func(p *ParityPreflightProfile) {
				p.TokenCounterMode = "experimental"
			},
			expects: "token_counter_mode",
		},
		{
			name: "reject heuristic token counter mode in parity",
			mutate: func(p *ParityPreflightProfile) {
				p.TokenCounterMode = "heuristic"
			},
			expects: "token_counter_mode",
		},
		{
			name: "reject missing fixed seed",
			mutate: func(p *ParityPreflightProfile) {
				p.FixedSeed = 0
			},
			expects: "fixed_seed",
		},
	}

	for _, tc := range cases {
		p := *baseProfile
		tc.mutate(&p)
		err := RunParityHarnessPreflight(bundle, ParityPreflightOptions{
			RequireComparatorTuple: true,
			Profile:                &p,
		})
		if err == nil || !strings.Contains(err.Error(), tc.expects) {
			return fmt.Errorf("%s: expected error containing %q, got %v", tc.name, tc.expects, err)
		}
	}

	// Non-parity profiles should not be rejected by parity-only leakage guards.
	nonParity := &ParityPreflightProfile{
		ID:                      "enhancement-profile",
		TokenBudget:             512,
		RepeatRuns:              2,
		DeterministicMode:       false,
		EnhancementTiersEnabled: "all",
		TokenCounterMode:        "experimental",
		FixedSeed:               0,
		ParityMode:              false,
	}
	if err := RunParityHarnessPreflight(bundle, ParityPreflightOptions{
		RequireComparatorTuple: false,
		Profile:                nonParity,
	}); err != nil {
		return fmt.Errorf("non-parity profile should bypass parity leakage guard, got: %w", err)
	}

	return nil
}

type rankingConcordance struct {
	Jaccard    float64
	Spearman   float64
	SharedTopN int
	SpearmanNA bool
}

func computeRankingConcordance(aider, crush []string, topN int) (rankingConcordance, error) {
	aTop := topUnique(aider, topN)
	cTop := topUnique(crush, topN)
	if len(aTop) == 0 || len(cTop) == 0 {
		return rankingConcordance{}, fmt.Errorf("empty ranking inputs")
	}

	aSet := make(map[string]struct{}, len(aTop))
	for _, v := range aTop {
		aSet[v] = struct{}{}
	}
	cSet := make(map[string]struct{}, len(cTop))
	for _, v := range cTop {
		cSet[v] = struct{}{}
	}

	intersection := 0
	shared := make([]string, 0, minInt(len(aTop), len(cTop)))
	for v := range aSet {
		if _, ok := cSet[v]; ok {
			intersection++
			shared = append(shared, v)
		}
	}
	union := len(aSet) + len(cSet) - intersection
	if union == 0 {
		return rankingConcordance{}, fmt.Errorf("invalid ranking union size")
	}

	res := rankingConcordance{
		Jaccard:    float64(intersection) / float64(union),
		SharedTopN: intersection,
	}
	if intersection < 3 {
		res.SpearmanNA = true
		res.Spearman = math.NaN()
		return res, nil
	}

	aRank := make(map[string]int, len(aTop))
	for i, v := range aTop {
		aRank[v] = i + 1
	}
	cRank := make(map[string]int, len(cTop))
	for i, v := range cTop {
		cRank[v] = i + 1
	}

	sumD2 := 0.0
	n := float64(len(shared))
	for _, id := range shared {
		d := float64(aRank[id] - cRank[id])
		sumD2 += d * d
	}
	res.Spearman = 1 - (6*sumD2)/(n*(n*n-1))
	return res, nil
}

func enforceRankingThresholds(metrics rankingConcordance) error {
	if metrics.Jaccard < 0.85 {
		return fmt.Errorf("jaccard(top-30)=%.4f below threshold 0.85", metrics.Jaccard)
	}
	if !metrics.SpearmanNA && metrics.Spearman < 0.80 {
		return fmt.Errorf("spearman(shared top-30)=%.4f below threshold 0.80", metrics.Spearman)
	}
	return nil
}

func topUnique(ranking []string, topN int) []string {
	if topN <= 0 {
		return nil
	}
	seen := make(map[string]struct{}, minInt(len(ranking), topN))
	out := make([]string, 0, minInt(len(ranking), topN))
	for _, v := range ranking {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
		if len(out) == topN {
			break
		}
	}
	return out
}

func fixtureByName(t *testing.T, name string) (verticalSliceFixture, error) {
	t.Helper()

	fixtures := loadVerticalSliceFixtures(t)
	for _, fx := range fixtures {
		if fx.Name == name {
			return fx, nil
		}
	}
	return verticalSliceFixture{}, fmt.Errorf("fixture %q not found", name)
}

func fixturePipelineRanking(fx verticalSliceFixture) []string {
	ordered := make([]string, 0, len(fx.Pipeline.Stage0)+len(fx.Pipeline.Stage1)+len(fx.Pipeline.Stage2)+len(fx.Pipeline.Stage3))
	ordered = append(ordered, fx.Pipeline.Stage0...)
	for _, d := range fx.Pipeline.Stage1 {
		ordered = append(ordered, d.File)
	}
	ordered = append(ordered, fx.Pipeline.Stage2...)
	ordered = append(ordered, fx.Pipeline.Stage3...)
	return uniqueOrdered(ordered)
}

func parityRankingForBudget(fx ParityAiderFixture, tokenBudget int) ([]string, bool) {
	for _, profile := range fx.Profiles {
		if !profile.ParityMode {
			continue
		}
		if profile.TokenBudget != tokenBudget {
			continue
		}
		ranking := uniqueOrdered(profile.ExpectedResults.TopFiles)
		if len(ranking) == 0 {
			return nil, false
		}
		return ranking, true
	}
	return nil, false
}

func uniqueOrdered(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func determinismHashForProfile(profile fixtureProfile, result verticalSliceResult) string {
	if profile.ParityMode {
		return result.NormalizedHash
	}
	return result.RawHash
}

func validParityGateBundle() ParityProvenanceBundle {
	return ParityProvenanceBundle{
		AiderCommitSHA:    strings.Repeat("a", 40),
		ComparatorPath:    "../aider",
		FixturesSHA256:    strings.Repeat("b", 64),
		GrepASTProvenance: "grep-ast@v1.2.3",
		TokenizerID:       "cl100k_base",
		TokenizerVersion:  "v0.1.0",
	}
}

func validParityGatePreflightProfile() *ParityPreflightProfile {
	return &ParityPreflightProfile{
		ID:                      "parity-gate",
		TokenBudget:             1024,
		RepeatRuns:              2,
		DeterministicMode:       true,
		EnhancementTiersEnabled: "none",
		TokenCounterMode:        "tokenizer_backed",
		FixedSeed:               1337,
		ParityMode:              true,
	}
}

func TestExplorerFamilyMatrixFamiliesNonEmpty(t *testing.T) {
	t.Parallel()

	efm, err := LoadExplorerFamilyMatrix()
	require.NoError(t, err, "failed to load explorer family matrix")
	require.NotEmpty(t, efm.Families, "explorer family matrix families array must not be empty")

	for i, fam := range efm.Families {
		require.NotEmpty(t, fam.Family, "families[%d]: family name must not be empty", i)
		require.Greater(t, fam.ScoreWeight, 0.0, "families[%d]: score_weight must be positive", i)
		require.Greater(t, fam.Threshold, 0.0, "families[%d]: threshold must be positive", i)
	}
}
