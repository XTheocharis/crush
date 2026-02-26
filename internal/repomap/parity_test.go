package repomap

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
)

func TestParityGateA1RankingConcordance(t *testing.T) {
	t.Parallel()

	if err := runGateA1RankingConcordance(t); err != nil {
		t.Fatal(err)
	}
}

func TestParityGateA1RankingConcordanceThresholdEnforcement(t *testing.T) {
	t.Parallel()

	// Intentional failure-path test: this pair should violate both thresholds.
	aider := []string{"a.go", "b.go", "c.go", "d.go", "e.go"}
	crush := []string{"z.go", "y.go", "x.go", "w.go", "v.go"}

	metrics, err := computeRankingConcordance(aider, crush, 30)
	if err != nil {
		t.Fatalf("compute ranking concordance: %v", err)
	}
	if err := enforceRankingThresholds(metrics); err == nil {
		t.Fatalf("expected threshold enforcement error, got nil")
	}
}

func TestParityGateA2StageRenderFidelity(t *testing.T) {
	t.Parallel()

	if err := runGateA2StageRenderFidelity(t); err != nil {
		t.Fatal(err)
	}
}

func TestParityGateA3TokenSafetyAccounting(t *testing.T) {
	t.Parallel()

	if err := runGateA3TokenSafetyAccounting(t); err != nil {
		t.Fatal(err)
	}
}

func TestParityGateA4RefreshSemantics(t *testing.T) {
	t.Parallel()

	if err := runGateA4RefreshSemantics(t); err != nil {
		t.Fatal(err)
	}
}

func TestParityGateA5Determinism(t *testing.T) {
	t.Parallel()

	if err := runGateA5Determinism(t); err != nil {
		t.Fatal(err)
	}
}

func TestParityGateA6ParityLeakageGuard(t *testing.T) {
	t.Parallel()

	if err := runGateA6ParityLeakageGuard(); err != nil {
		t.Fatal(err)
	}
}

func TestParityGateAAggregate(t *testing.T) {
	if err := runGateA1RankingConcordance(t); err != nil {
		t.Fatalf("A1 ranking concordance failed: %v", err)
	}
	if err := runGateA2StageRenderFidelity(t); err != nil {
		t.Fatalf("A2 stage/render fidelity failed: %v", err)
	}
	if err := runGateA3TokenSafetyAccounting(t); err != nil {
		t.Fatalf("A3 token safety/accounting failed: %v", err)
	}
	if err := runGateA4RefreshSemantics(t); err != nil {
		t.Fatalf("A4 refresh semantics failed: %v", err)
	}
	if err := runGateA5Determinism(t); err != nil {
		t.Fatalf("A5 determinism failed: %v", err)
	}
	if err := runGateA6ParityLeakageGuard(); err != nil {
		t.Fatalf("A6 parity leakage guard failed: %v", err)
	}
}

func runGateA1RankingConcordance(t *testing.T) error {
	t.Helper()

	fx, err := fixtureByName(t, "basic_go")
	if err != nil {
		return err
	}

	aiderRanking := uniqueOrdered(fx.Repository.Files)
	crushRanking := fixturePipelineRanking(fx)

	metrics, err := computeRankingConcordance(aiderRanking, crushRanking, 30)
	if err != nil {
		return err
	}
	if err := enforceRankingThresholds(metrics); err != nil {
		return fmt.Errorf("ranking concordance thresholds failed: %w", err)
	}
	return nil
}

func runGateA2StageRenderFidelity(t *testing.T) error {
	t.Helper()

	fx, err := fixtureByName(t, "basic_go")
	if err != nil {
		return err
	}

	for _, profile := range fx.Profiles {
		result := runVerticalSliceHarness(fx, profile)
		assertStageAssemblyInvariants(t, fx, result)

		if fx.Assertions.RequireRenderedEntries && result.RenderedFileEntryCount <= 0 {
			return fmt.Errorf("profile %q: expected rendered entries > 0", profile.Name)
		}

		lines := strings.Split(strings.TrimSpace(result.MapText), "\n")
		if strings.TrimSpace(result.MapText) != "" && len(lines) != len(result.Entries) {
			return fmt.Errorf("profile %q: render fidelity mismatch lines=%d entries=%d", profile.Name, len(lines), len(result.Entries))
		}

		if len(fx.Assertions.RequireTrimOrder) > 0 {
			assertTrimOrderInvariant(t, fx.Assertions.RequireTrimOrder, result.TrimmedStages)
		}
	}

	return nil
}

func runGateA3TokenSafetyAccounting(t *testing.T) error {
	t.Helper()

	fx, err := fixtureByName(t, "basic_go")
	if err != nil {
		return err
	}

	var parityProfile *fixtureProfile
	var enhancementProfile *fixtureProfile
	for i := range fx.Profiles {
		p := fx.Profiles[i]
		if p.ParityMode && parityProfile == nil {
			parityProfile = &p
		}
		if !p.ParityMode && enhancementProfile == nil {
			enhancementProfile = &p
		}
	}
	if parityProfile == nil || enhancementProfile == nil {
		return fmt.Errorf("fixture %q must include both parity and enhancement profiles", fx.Name)
	}

	parityResult := runVerticalSliceHarness(fx, *parityProfile)
	if !parityResult.ComparatorAccepted || parityResult.ComparatorDelta > 0.15 {
		return fmt.Errorf("parity comparator acceptance failed: accepted=%v delta=%.4f", parityResult.ComparatorAccepted, parityResult.ComparatorDelta)
	}
	if parityResult.ParityTokens <= 0 {
		return fmt.Errorf("parity token accounting failed: parity_tokens=%.2f", parityResult.ParityTokens)
	}

	enhancementResult := runVerticalSliceHarness(fx, *enhancementProfile)
	if enhancementResult.SafetyTokens > enhancementProfile.TokenBudget {
		return fmt.Errorf("enhancement safety budget violation: safety=%d budget=%d", enhancementResult.SafetyTokens, enhancementProfile.TokenBudget)
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
		SessionID:       "gate-a4",
		ChatFiles:       []string{"a.go"},
		MentionedFnames: []string{"b.go"},
		MentionedIdents: []string{"Foo"},
		TokenBudget:     100,
	}

	modes := []string{"auto", "files", "manual", "always"}
	for _, mode := range modes {
		svc := NewService(&config.Config{Options: &config.Options{RepoMap: &config.RepoMapOptions{RefreshMode: mode}}}, nil, nil, ".", context.Background())
		defer svc.Close()

		cacheKey := buildRenderCacheKey(mode, opts)
		if cacheKey != "" {
			svc.renderCaches.GetOrCreate(opts.SessionID).Set(cacheKey, "cached-map", 11)
		}

		coldMap, coldTok, err := svc.Generate(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("mode %q cold generate: %w", mode, err)
		}

		switch mode {
		case "auto", "files":
			if coldMap != "cached-map" || coldTok != 11 {
				return fmt.Errorf("mode %q: expected render-cache fallback on cold start", mode)
			}
		case "manual", "always":
			if coldMap != "" || coldTok != 0 {
				return fmt.Errorf("mode %q: expected cold start empty map/tokens", mode)
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
		runs := profile.RepeatRuns
		if runs < 10 {
			runs = 10
		}
		seen := make(map[string]struct{}, runs)
		for i := 0; i < runs; i++ {
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
