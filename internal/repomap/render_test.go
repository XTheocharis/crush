package repomap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

func TestRenderBudgetGoldenBasic(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "src/a.go", Name: "A", Kind: "def", Line: 10},
		{RelPath: "src/b.go", Name: "B", Kind: "def", Line: 20},
		{RelPath: "src/c.go", Name: "C", Kind: "def", Line: 30},
	}
	defs := []RankedDefinition{
		{File: "src/a.go", Ident: "A", Rank: 0.9},
		{File: "src/b.go", Ident: "B", Rank: 0.8},
	}
	special := BuildSpecialPrelude([]string{"README.md", "go.mod"}, []string{"src/a.go", "src/b.go"}, false)
	entries := AssembleStageEntries(
		special,
		defs,
		[]string{"src/a.go", "src/c.go"},
		[]string{"src/c.go", "README.md", "docs/notes.md"},
		nil,
		false,
	)

	res, err := FitToBudget(context.Background(), entries, BudgetProfile{
		ParityMode:   false,
		TokenBudget:  8,
		LanguageHint: "default",
	}, nil)
	require.NoError(t, err)

	rankedFiles := AggregateRankedFiles(defs, tags)
	require.NotEmpty(t, rankedFiles)

	// Render via stage-entry renderer to produce deterministic budget trace.
	got := renderStageEntries(res.Entries)

	wantPath := filepath.Join("testdata", "render_budget", "basic_enhancement.golden")
	assertGolden(t, wantPath, got)
}

func TestRenderBudgetGoldenParityCounters(t *testing.T) {
	t.Parallel()

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "src/a.go", Ident: "A"},
	}

	res, err := FitToBudget(context.Background(), entries, BudgetProfile{
		ParityMode:   true,
		TokenBudget:  12,
		Model:        "stub",
		LanguageHint: "default",
	}, fakeCounter{out: 12})
	require.NoError(t, err)
	require.True(t, res.ComparatorAccepted)
	require.Equal(t, 12, res.SafetyTokens)

	got := renderStageEntries(res.Entries)
	wantPath := filepath.Join("testdata", "render_budget", "basic_parity.golden")
	assertGolden(t, wantPath, got)
}

func assertGolden(t *testing.T, path string, got string) {
	t.Helper()
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
