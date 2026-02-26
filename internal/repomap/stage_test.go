package repomap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssembleStageEntriesOrderingAndMembership(t *testing.T) {
	t.Parallel()

	special := []string{"README.md", "go.mod", "src/a.go"}
	ranked := []RankedDefinition{
		{File: "src/a.go", Ident: "A", Rank: 0.9},
		{File: "src/b.go", Ident: "B", Rank: 0.8},
		{File: "src/a.go", Ident: "AA", Rank: 0.7},
	}
	graphNodes := []string{"src/a.go", "src/c.go", "src/d.go"}
	repoFiles := []string{"src/e.go", "src/c.go", "src/b.go", "README.md", "src/f.go"}
	chat := []string{"src/f.go"}

	entries := AssembleStageEntries(special, ranked, graphNodes, repoFiles, chat, false)
	require.NotEmpty(t, entries)

	stages := make([]int, 0, len(entries))
	for _, e := range entries {
		stages = append(stages, e.Stage)
	}
	require.True(t, nonDecreasing(stages))

	// Stage0 should exclude ranked files and include special prelude items.
	stage0 := filterStage(entries, stageSpecialPrelude)
	require.Equal(t, []string{"README.md", "go.mod"}, stageFiles(stage0))

	// Stage1 contains ranked defs excluding chat files.
	stage1 := filterStage(entries, stageRankedDefs)
	require.Len(t, stage1, 3)
	require.Equal(t, "src/a.go", stage1[0].File)
	require.Equal(t, "A", stage1[0].Ident)
	require.Equal(t, "src/b.go", stage1[1].File)
	require.Equal(t, "B", stage1[1].Ident)
	require.Equal(t, "src/a.go", stage1[2].File)
	require.Equal(t, "AA", stage1[2].Ident)

	// Stage2 should contain graph nodes not already in stage1 and not in chat.
	stage2 := filterStage(entries, stageGraphNodes)
	require.Equal(t, []string{"src/c.go", "src/d.go"}, stageFiles(stage2))

	// Stage3 should contain remaining repo files only.
	stage3 := filterStage(entries, stageRemainingFiles)
	require.Equal(t, []string{"README.md", "src/e.go"}, stageFiles(stage3))
}

func TestAssembleStageEntriesParityModePreservesStage3InputOrder(t *testing.T) {
	t.Parallel()

	repoFiles := []string{"z.go", "b.go", "a.go"}
	entriesParity := AssembleStageEntries(nil, nil, nil, repoFiles, nil, true)
	require.Equal(t, []string{"z.go", "b.go", "a.go"}, stageFiles(filterStage(entriesParity, stageRemainingFiles)))

	entriesEnh := AssembleStageEntries(nil, nil, nil, repoFiles, nil, false)
	require.Equal(t, []string{"a.go", "b.go", "z.go"}, stageFiles(filterStage(entriesEnh, stageRemainingFiles)))
}

func TestAssembleStageEntriesChatFilesExcluded(t *testing.T) {
	t.Parallel()

	ranked := []RankedDefinition{{File: "chat.go", Ident: "Chat", Rank: 1}}
	graphNodes := []string{"chat.go", "node.go"}
	repoFiles := []string{"chat.go", "tail.go"}
	chat := []string{"chat.go"}

	entries := AssembleStageEntries(nil, ranked, graphNodes, repoFiles, chat, false)

	for _, e := range entries {
		require.NotEqual(t, "chat.go", e.File)
	}
}

func filterStage(entries []StageEntry, stage int) []StageEntry {
	out := make([]StageEntry, 0)
	for _, e := range entries {
		if e.Stage == stage {
			out = append(out, e)
		}
	}
	return out
}

func stageFiles(entries []StageEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.File)
	}
	return out
}

func nonDecreasing(v []int) bool {
	for i := 1; i < len(v); i++ {
		if v[i] < v[i-1] {
			return false
		}
	}
	return true
}
