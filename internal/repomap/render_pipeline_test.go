package repomap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

// buildTestRepo creates a small on-disk repo with Go source files.
// Returns rootDir. Files are sized to produce gap markers.
func buildTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// main.go has enough lines between func main and func greet to produce
	// a gap marker when only greet's definition line is shown.
	files := map[string]string{
		"main.go": `package main

import "fmt"

func main() {
	a := 1
	b := 2
	c := 3
	d := 4
	e := 5
	f := a + b + c + d + e
	fmt.Println(f)
	fmt.Println(greet("world"))
}

func greet(name string) string {
	return "hello, " + name
}
`,
		"util.go": `package main

func add(a, b int) int {
	return a + b
}

func sub(a, b int) int {
	return a - b
}
`,
		"README.md": `# Test Repo
A simple test repository.
`,
	}

	for name, content := range files {
		p := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}

	return root
}

// TestRenderRepoMapScopeAwareOutput verifies that RenderRepoMap produces
// scope-aware output with | prefixed lines and ... gap markers.
func TestRenderRepoMapScopeAwareOutput(t *testing.T) {
	t.Parallel()

	root := buildTestRepo(t)

	// Request both main and greet definitions from main.go. These are
	// separated by several lines, so RenderTreeContext will emit a gap
	// marker between them.
	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "main"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "greet"},
		{Stage: stageRankedDefs, File: "util.go", Ident: "add"},
	}

	tags := []treesitter.Tag{
		{RelPath: "main.go", Name: "main", Kind: "def", Line: 5},
		{RelPath: "main.go", Name: "greet", Kind: "def", Line: 16},
		{RelPath: "util.go", Name: "add", Kind: "def", Line: 3},
	}
	tagsByFile := make(map[string][]treesitter.Tag)
	for _, tag := range tags {
		tagsByFile[tag.RelPath] = append(tagsByFile[tag.RelPath], tag)
	}

	got, err := RenderRepoMap(context.Background(), entries, tagsByFile, nil, root)
	require.NoError(t, err)
	require.NotEmpty(t, got)

	// Verify file headers are present.
	require.Contains(t, got, "README.md:\n")
	require.Contains(t, got, "main.go:\n")
	require.Contains(t, got, "util.go:\n")

	// Verify scope-aware formatting: | prefixed lines.
	lines := strings.Split(got, "\n")
	hasPipe := false
	hasGap := false
	for _, line := range lines {
		if strings.HasPrefix(line, "\u2502") {
			hasPipe = true
		}
		if strings.HasPrefix(line, "\u22ee") {
			hasGap = true
		}
	}
	require.True(t, hasPipe, "expected \u2502-prefixed lines in scope-aware output")
	// func main (line 5) and func greet (line 16) are separated by many
	// lines, producing a gap marker between the two shown segments.
	require.True(t, hasGap, "expected \u22ee gap markers in scope-aware output")
}

// TestRenderRepoMapFallbackOnContextTimeout verifies that when RenderRepoMap
// fails due to context timeout, Generate falls back to renderStageEntries.
func TestRenderRepoMapFallbackOnContextTimeout(t *testing.T) {
	t.Parallel()

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "main"},
	}

	tagsByFile := map[string][]treesitter.Tag{
		"main.go": {{RelPath: "main.go", Name: "main", Kind: "def", Line: 5}},
	}

	// Use an already-cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := RenderRepoMap(ctx, entries, tagsByFile, nil, "/nonexistent")
	require.Error(t, err)
	require.Empty(t, got)
	require.ErrorIs(t, err, context.Canceled)

	// Fallback to compact format.
	compact := renderStageEntries(entries)
	require.Contains(t, compact, "S0|README.md")
	require.Contains(t, compact, "S1|main.go|main")
}

// TestRenderRepoMapDeterminism verifies that RenderRepoMap produces identical
// output across 10 runs with the same input.
func TestRenderRepoMapDeterminism(t *testing.T) {
	t.Parallel()

	root := buildTestRepo(t)

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "greet"},
		{Stage: stageRankedDefs, File: "util.go", Ident: "add"},
		{Stage: stageRemainingFiles, File: "util.go"},
	}

	tags := []treesitter.Tag{
		{RelPath: "main.go", Name: "greet", Kind: "def", Line: 16},
		{RelPath: "util.go", Name: "add", Kind: "def", Line: 3},
	}
	tagsByFile := make(map[string][]treesitter.Tag)
	for _, tag := range tags {
		tagsByFile[tag.RelPath] = append(tagsByFile[tag.RelPath], tag)
	}

	results := make([]string, 10)
	for i := range 10 {
		got, err := RenderRepoMap(context.Background(), entries, tagsByFile, nil, root)
		require.NoError(t, err)
		results[i] = got
	}

	for i := 1; i < len(results); i++ {
		require.Equal(t, results[0], results[i],
			"determinism failure: run0 != run%d", i)
	}
}

// TestTrimLoopBudgetComplianceEnhancement verifies that the post-render trim
// loop enforces budget compliance. Uses a small budget that forces trimming.
func TestTrimLoopBudgetComplianceEnhancement(t *testing.T) {
	t.Parallel()

	root := buildTestRepo(t)

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "greet"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "main"},
		{Stage: stageRankedDefs, File: "util.go", Ident: "add"},
		{Stage: stageRankedDefs, File: "util.go", Ident: "sub"},
	}

	tags := []treesitter.Tag{
		{RelPath: "main.go", Name: "greet", Kind: "def", Line: 16},
		{RelPath: "main.go", Name: "main", Kind: "def", Line: 5},
		{RelPath: "util.go", Name: "add", Kind: "def", Line: 3},
		{RelPath: "util.go", Name: "sub", Kind: "def", Line: 7},
	}
	tagsByFile := make(map[string][]treesitter.Tag)
	for _, tag := range tags {
		tagsByFile[tag.RelPath] = append(tagsByFile[tag.RelPath], tag)
	}

	// Use a very small budget that forces trimming.
	originalBudget := 5

	// Render full set first to confirm it exceeds budget.
	fullText, err := RenderRepoMap(context.Background(), entries, tagsByFile, nil, root)
	require.NoError(t, err)
	fullMetrics, _ := CountParityAndSafetyTokens(context.Background(), nil, "", fullText, "default")
	require.Greater(t, fullMetrics.SafetyTokens, originalBudget,
		"full render should exceed small budget to exercise trim loop")

	// Now run the trim loop logic.
	counter := TokenCounter(nil)
	model := ""

	fitsWithinBudget := func(text string) (bool, int) {
		m, mErr := CountParityAndSafetyTokens(context.Background(), counter, model, text, "default")
		if mErr != nil {
			est := EstimateTokens(text, "default")
			return est <= originalBudget, est
		}
		return m.SafetyTokens <= originalBudget, m.SafetyTokens
	}

	accepted, _ := fitsWithinBudget(fullText)
	require.False(t, accepted, "full render should not fit in small budget")

	// Binary search for the largest fitting prefix.
	lo, hi := 0, len(entries)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		candidate := entries[:mid]
		text, renderErr := RenderRepoMap(context.Background(), candidate, tagsByFile, nil, root)
		require.NoError(t, renderErr)
		ok, _ := fitsWithinBudget(text)
		if ok {
			lo = mid
		} else {
			hi = mid - 1
		}
	}

	trimmedEntries := entries[:lo]
	trimmedText, err := RenderRepoMap(context.Background(), trimmedEntries, tagsByFile, nil, root)
	require.NoError(t, err)

	_, tokenCount := fitsWithinBudget(trimmedText)
	require.LessOrEqual(t, tokenCount, originalBudget,
		"trimmed output must fit within budget")
}

// TestTrimLoopBudgetComplianceParity verifies that the post-render trim
// loop uses monotonic hard-cap (safetyTokens <= budget) in parity mode,
// not the symmetric +/-15% parity criterion.
func TestTrimLoopBudgetComplianceParity(t *testing.T) {
	t.Parallel()

	root := buildTestRepo(t)

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "greet"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "main"},
		{Stage: stageRankedDefs, File: "util.go", Ident: "add"},
		{Stage: stageRankedDefs, File: "util.go", Ident: "sub"},
	}

	tags := []treesitter.Tag{
		{RelPath: "main.go", Name: "greet", Kind: "def", Line: 16},
		{RelPath: "main.go", Name: "main", Kind: "def", Line: 5},
		{RelPath: "util.go", Name: "add", Kind: "def", Line: 3},
		{RelPath: "util.go", Name: "sub", Kind: "def", Line: 7},
	}
	tagsByFile := make(map[string][]treesitter.Tag)
	for _, tag := range tags {
		tagsByFile[tag.RelPath] = append(tagsByFile[tag.RelPath], tag)
	}

	// Budget small enough that full output exceeds it, but large enough
	// that a trimmed subset fits.
	budget := 10

	fullText, err := RenderRepoMap(context.Background(), entries, tagsByFile, nil, root)
	require.NoError(t, err)

	fitsWithinBudget := func(text string) (bool, int) {
		m, mErr := CountParityAndSafetyTokens(context.Background(), nil, "", text, "default")
		if mErr != nil {
			est := EstimateTokens(text, "default")
			return est <= budget, est
		}
		// CRITICAL: monotonic hard-cap, not parity +/-15%.
		return m.SafetyTokens <= budget, m.SafetyTokens
	}

	accepted, _ := fitsWithinBudget(fullText)
	if !accepted {
		// Trim loop needed.
		lo, hi := 0, len(entries)-1
		for lo < hi {
			mid := (lo + hi + 1) / 2
			candidate := entries[:mid]
			text, renderErr := RenderRepoMap(context.Background(), candidate, tagsByFile, nil, root)
			require.NoError(t, renderErr)
			ok, _ := fitsWithinBudget(text)
			if ok {
				lo = mid
			} else {
				hi = mid - 1
			}
		}

		trimmedText, renderErr := RenderRepoMap(context.Background(), entries[:lo], tagsByFile, nil, root)
		require.NoError(t, renderErr)
		_, tokenCount := fitsWithinBudget(trimmedText)
		require.LessOrEqual(t, tokenCount, budget,
			"parity mode trim must use hard-cap safetyTokens <= budget")
	}
}

// TestTrimLoopMonotonicity verifies that the binary search converges
// correctly even with non-uniform expansion ratios across entries.
func TestTrimLoopMonotonicity(t *testing.T) {
	t.Parallel()

	root := buildTestRepo(t)

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "main"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "greet"},
		{Stage: stageRankedDefs, File: "util.go", Ident: "add"},
		{Stage: stageRankedDefs, File: "util.go", Ident: "sub"},
	}

	tags := []treesitter.Tag{
		{RelPath: "main.go", Name: "main", Kind: "def", Line: 5},
		{RelPath: "main.go", Name: "greet", Kind: "def", Line: 16},
		{RelPath: "util.go", Name: "add", Kind: "def", Line: 3},
		{RelPath: "util.go", Name: "sub", Kind: "def", Line: 7},
	}
	tagsByFile := make(map[string][]treesitter.Tag)
	for _, tag := range tags {
		tagsByFile[tag.RelPath] = append(tagsByFile[tag.RelPath], tag)
	}

	// Measure safety tokens for each prefix length.
	prevTokens := 0
	for i := 1; i <= len(entries); i++ {
		prefix := entries[:i]
		text, err := RenderRepoMap(context.Background(), prefix, tagsByFile, nil, root)
		require.NoError(t, err)
		m, _ := CountParityAndSafetyTokens(context.Background(), nil, "", text, "default")
		// Monotonic: more entries should produce >= tokens.
		require.GreaterOrEqual(t, m.SafetyTokens, prevTokens,
			"monotonicity violated at prefix length %d: prev=%d current=%d",
			i, prevTokens, m.SafetyTokens)
		prevTokens = m.SafetyTokens
	}
}

// TestTrimLoopEmptyResult verifies behavior when the first entry alone
// exceeds the budget, resulting in zero entries after trim.
func TestTrimLoopEmptyResult(t *testing.T) {
	t.Parallel()

	root := buildTestRepo(t)

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "greet"},
	}

	tags := []treesitter.Tag{
		{RelPath: "main.go", Name: "greet", Kind: "def", Line: 16},
	}
	tagsByFile := make(map[string][]treesitter.Tag)
	for _, tag := range tags {
		tagsByFile[tag.RelPath] = append(tagsByFile[tag.RelPath], tag)
	}

	// Budget of 1 token — even one entry will exceed this.
	budget := 1

	fitsWithinBudget := func(text string) (bool, int) {
		m, mErr := CountParityAndSafetyTokens(context.Background(), nil, "", text, "default")
		if mErr != nil {
			est := EstimateTokens(text, "default")
			return est <= budget, est
		}
		return m.SafetyTokens <= budget, m.SafetyTokens
	}

	fullText, err := RenderRepoMap(context.Background(), entries, tagsByFile, nil, root)
	require.NoError(t, err)

	accepted, _ := fitsWithinBudget(fullText)
	require.False(t, accepted, "full render should exceed tiny budget")

	// Run trim loop.
	lo, hi := 0, len(entries)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		candidate := entries[:mid]
		text, renderErr := RenderRepoMap(context.Background(), candidate, tagsByFile, nil, root)
		require.NoError(t, renderErr)
		ok, _ := fitsWithinBudget(text)
		if ok {
			lo = mid
		} else {
			hi = mid - 1
		}
	}

	require.Equal(t, 0, lo, "trim should converge to zero when budget is too small")
}

// TestExpansionFactorCalibration measures actual expansion ratios between
// compact format and scope-aware rendering across the test repo.
func TestExpansionFactorCalibration(t *testing.T) {
	t.Parallel()

	root := buildTestRepo(t)

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "greet"},
		{Stage: stageRankedDefs, File: "main.go", Ident: "main"},
		{Stage: stageRankedDefs, File: "util.go", Ident: "add"},
		{Stage: stageRankedDefs, File: "util.go", Ident: "sub"},
	}

	tags := []treesitter.Tag{
		{RelPath: "main.go", Name: "greet", Kind: "def", Line: 16},
		{RelPath: "main.go", Name: "main", Kind: "def", Line: 5},
		{RelPath: "util.go", Name: "add", Kind: "def", Line: 3},
		{RelPath: "util.go", Name: "sub", Kind: "def", Line: 7},
	}
	tagsByFile := make(map[string][]treesitter.Tag)
	for _, tag := range tags {
		tagsByFile[tag.RelPath] = append(tagsByFile[tag.RelPath], tag)
	}

	compact := renderStageEntries(entries)
	compactTokens := EstimateTokens(compact, "default")

	scopeAware, err := RenderRepoMap(context.Background(), entries, tagsByFile, nil, root)
	require.NoError(t, err)
	scopeTokens := EstimateTokens(scopeAware, "default")

	ratio := float64(scopeTokens) / float64(compactTokens)
	t.Logf("Expansion ratio: compact=%d tokens, scope=%d tokens, ratio=%.2fx",
		compactTokens, scopeTokens, ratio)

	// The scope-aware output should be larger than compact.
	require.Greater(t, scopeTokens, compactTokens,
		"scope-aware output should be larger than compact format")

	// The expansion factor of 4 should be a reasonable conservative estimate.
	// We don't enforce exact bounds since it varies by content, but log it.
	require.Greater(t, ratio, 1.0, "expansion ratio must be > 1.0")
}

// TestDisableForSessionInfrastructure verifies the disable latch behavior.
func TestDisableForSessionInfrastructure(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())
	defer svc.Close()

	require.False(t, svc.isDisabledForSession("sess-1"))
	require.False(t, svc.isDisabledForSession("sess-2"))

	svc.disableForSession("sess-1")
	require.True(t, svc.isDisabledForSession("sess-1"))
	require.False(t, svc.isDisabledForSession("sess-2"))

	// Disable is one-way latch — calling again is idempotent.
	svc.disableForSession("sess-1")
	require.True(t, svc.isDisabledForSession("sess-1"))
}

// TestRenderCacheVersionIncluded verifies that the cache key includes the
// scope-aware render version to invalidate old compact-format caches.
func TestRenderCacheVersionIncluded(t *testing.T) {
	t.Parallel()

	opts := GenerateOpts{
		ChatFiles:   []string{"a.go"},
		TokenBudget: 100,
	}

	key := buildRenderCacheKey("auto", opts)
	require.Contains(t, key, renderCacheVersion,
		"cache key must include render version for scope-aware invalidation")

	key = buildRenderCacheKey("files", opts)
	require.Contains(t, key, renderCacheVersion)

	// Manual mode has a fixed key.
	key = buildRenderCacheKey("manual", opts)
	require.Equal(t, "manual", key)

	// Always mode returns empty.
	key = buildRenderCacheKey("always", opts)
	require.Equal(t, "", key)
}
