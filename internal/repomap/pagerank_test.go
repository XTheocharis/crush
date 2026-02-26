package repomap

import (
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

func TestRankPageRankStarGraphCenterHighest(t *testing.T) {
	t.Parallel()

	g := &FileGraph{
		Nodes: []string{"center.go", "a.go", "b.go", "c.go"},
		Edges: []GraphEdge{
			{From: "a.go", To: "center.go", Ident: "Center", Weight: 1},
			{From: "b.go", To: "center.go", Ident: "Center", Weight: 1},
			{From: "c.go", To: "center.go", Ident: "Center", Weight: 1},
			{From: "center.go", To: "a.go", Ident: "LeafA", Weight: 1},
			{From: "center.go", To: "b.go", Ident: "LeafB", Weight: 1},
			{From: "center.go", To: "c.go", Ident: "LeafC", Weight: 1},
		},
	}

	defs := Rank(g, nil)
	require.NotEmpty(t, defs)
	require.Equal(t, "center.go", defs[0].File)
	require.Equal(t, "Center", defs[0].Ident)
}

func TestRankPageRankChainWithPersonalization(t *testing.T) {
	t.Parallel()

	g := &FileGraph{
		Nodes: []string{"a.go", "b.go", "c.go"},
		Edges: []GraphEdge{
			{From: "a.go", To: "b.go", Ident: "B", Weight: 1},
			{From: "b.go", To: "c.go", Ident: "C", Weight: 1},
		},
	}

	pers := map[string]float64{"a.go": 1}
	defs := Rank(g, pers)
	require.NotEmpty(t, defs)
	require.Equal(t, "b.go", defs[0].File)
	require.Equal(t, "B", defs[0].Ident)
}

func TestRankPageRankDisconnectedComponents(t *testing.T) {
	t.Parallel()

	g := &FileGraph{
		Nodes: []string{"a.go", "b.go", "x.go", "y.go"},
		Edges: []GraphEdge{
			{From: "a.go", To: "b.go", Ident: "B", Weight: 1},
			{From: "b.go", To: "a.go", Ident: "A", Weight: 1},
			{From: "x.go", To: "y.go", Ident: "Y", Weight: 1},
			{From: "y.go", To: "x.go", Ident: "X", Weight: 1},
		},
	}

	defs := Rank(g, nil)
	require.NotEmpty(t, defs)

	seen := map[string]struct{}{}
	for _, d := range defs {
		seen[d.File] = struct{}{}
	}
	require.Contains(t, seen, "a.go")
	require.Contains(t, seen, "b.go")
	require.Contains(t, seen, "x.go")
	require.Contains(t, seen, "y.go")
}

func TestRunPageRankConvergenceDeltaDecreasesMonotonic(t *testing.T) {
	t.Parallel()

	g := &FileGraph{
		Nodes: []string{"a.go", "b.go", "c.go"},
		Edges: []GraphEdge{
			{From: "a.go", To: "b.go", Ident: "B", Weight: 1},
			{From: "b.go", To: "c.go", Ident: "C", Weight: 1},
			{From: "c.go", To: "a.go", Ident: "A", Weight: 1},
		},
	}

	_, deltas, ok := runPageRank(g, nil)
	require.True(t, ok)
	require.NotEmpty(t, deltas)

	for i := 1; i < len(deltas); i++ {
		require.LessOrEqual(t, deltas[i], deltas[i-1]+1e-12)
	}
}

func TestRankDegenerateGraphFallsBackToEmpty(t *testing.T) {
	t.Parallel()

	g := &FileGraph{Nodes: []string{"a.go", "b.go"}}
	require.Empty(t, Rank(g, map[string]float64{"a.go": 1}))
}

func TestBuildPersonalizationRules(t *testing.T) {
	t.Parallel()

	allFiles := []string{"src/auth/login.py", "src/main.py", "README.md"}
	chatFiles := []string{"src/main.py"}
	mentionedFnames := []string{"README.md", "src/main.py"}
	mentionedIdents := []string{"auth", "login"}

	pers := BuildPersonalization(allFiles, chatFiles, mentionedFnames, mentionedIdents)
	require.NotNil(t, pers)

	base := 100.0 / 3.0
	require.InDelta(t, base, pers["src/main.py"], 1e-9)       // chat + mention(max)
	require.InDelta(t, base, pers["README.md"], 1e-9)         // mention(max)
	require.InDelta(t, base, pers["src/auth/login.py"], 1e-9) // path-component match once
}

func TestAggregateRankedFilesSumsAndSorts(t *testing.T) {
	t.Parallel()

	defs := []RankedDefinition{
		{File: "a.go", Ident: "A1", Rank: 0.4},
		{File: "a.go", Ident: "A2", Rank: 0.1},
		{File: "b.go", Ident: "B1", Rank: 0.3},
	}
	tags := []treesitter.Tag{
		{RelPath: "a.go", Name: "A1", Kind: "def", Line: 11},
		{RelPath: "a.go", Name: "A2", Kind: "def", Line: 22},
		{RelPath: "b.go", Name: "B1", Kind: "def", Line: 33},
	}

	files := AggregateRankedFiles(defs, tags)
	require.Len(t, files, 2)
	require.Equal(t, "a.go", files[0].Path)
	require.InDelta(t, 0.5, files[0].Rank, 1e-9)
	require.Len(t, files[0].Defs, 2)
	require.Equal(t, "A1", files[0].Defs[0].Name)
	require.Equal(t, 11, files[0].Defs[0].Line)
	require.Equal(t, "A2", files[0].Defs[1].Name)
	require.Equal(t, 22, files[0].Defs[1].Line)
}
