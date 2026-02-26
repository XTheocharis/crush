package repomap

import (
	"math"
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

func TestBuildGraphConstructsWeightedCrossFileEdges(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "def/a.go", Name: "process_item", Kind: "def"},
		{RelPath: "ref/r.go", Name: "process_item", Kind: "ref"},
		{RelPath: "ref/r.go", Name: "process_item", Kind: "ref"},
	}

	g := buildGraph(tags, []string{"ref/r.go"}, []string{"process_item"})
	require.NotNil(t, g)
	require.Equal(t, []string{"def/a.go", "ref/r.go"}, g.Nodes)
	require.Len(t, g.Edges, 2)

	cross := findEdge(t, g, "ref/r.go", "def/a.go", "process_item")
	require.Equal(t, 2, cross.RefCount)
	// mul=10 (mentioned) * 10 (long structured), use_mul*=50 (chat file),
	// weight = use_mul * sqrt(num_refs) = 5000 * sqrt(2).
	require.InDelta(t, 5000*math.Sqrt(2), cross.Weight, 1e-9)

	self := findEdge(t, g, "def/a.go", "def/a.go", "process_item")
	require.Equal(t, 1, self.RefCount)
	require.InDelta(t, 100.0, self.Weight, 1e-9)
}

func TestBuildGraphFallbackOrderingLexicalBeforeGlobal(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "a.go", Name: "Alpha", Kind: "def"},
		{RelPath: "b.go", Name: "op+", Kind: "def"},
	}

	g := buildGraph(tags, nil, nil)
	require.NotNil(t, g)
	require.Equal(t, []string{"a.go", "b.go"}, g.Nodes)
	require.Len(t, g.Edges, 2)

	alpha := findEdge(t, g, "a.go", "a.go", "Alpha")
	require.Equal(t, 1, alpha.RefCount)
	require.InDelta(t, 1.0, alpha.Weight, 1e-9)

	// Because lexical backfill already produced references (from Alpha),
	// global fallback must not run. So non-lexical "op+" remains orphan and
	// gets only the orphan self-edge (0.1), not a cross-file fallback edge.
	op := findEdge(t, g, "b.go", "b.go", "op+")
	require.Equal(t, 0, op.RefCount)
	require.InDelta(t, 0.1, op.Weight, 1e-9)

	require.Equal(t, 1, countEdgesForIdent(g, "op+"))
}

func TestBuildGraphGlobalFallbackWhenNoLexicalBackfillPossible(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "a.go", Name: "op+", Kind: "def"},
		{RelPath: "b.go", Name: "++", Kind: "def"},
	}

	g := buildGraph(tags, nil, nil)
	require.NotNil(t, g)
	require.Equal(t, []string{"a.go", "b.go"}, g.Nodes)
	require.Len(t, g.Edges, 2)

	e1 := findEdge(t, g, "a.go", "a.go", "op+")
	require.Equal(t, 1, e1.RefCount)
	require.InDelta(t, 1.0, e1.Weight, 1e-9)

	e2 := findEdge(t, g, "b.go", "b.go", "++")
	require.Equal(t, 1, e2.RefCount)
	require.InDelta(t, 1.0, e2.Weight, 1e-9)
}

func TestBuildGraphAddsOrphanSelfEdgesWhenOtherRefsExist(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "a.go", Name: "Orphan", Kind: "def"},
		{RelPath: "a.go", Name: "Used", Kind: "ref"},
		{RelPath: "b.go", Name: "Used", Kind: "def"},
		{RelPath: "c.go", Name: "Used", Kind: "ref"},
	}

	g := buildGraph(tags, nil, nil)
	require.NotNil(t, g)
	require.Equal(t, []string{"a.go", "b.go", "c.go"}, g.Nodes)
	require.Len(t, g.Edges, 4)

	orphan := findEdge(t, g, "a.go", "a.go", "Orphan")
	require.Equal(t, 0, orphan.RefCount)
	require.InDelta(t, 0.1, orphan.Weight, 1e-9)

	aUsed := findEdge(t, g, "a.go", "b.go", "Used")
	require.Equal(t, 1, aUsed.RefCount)
	require.InDelta(t, 1.0, aUsed.Weight, 1e-9)

	cUsed := findEdge(t, g, "c.go", "b.go", "Used")
	require.Equal(t, 1, cUsed.RefCount)
	require.InDelta(t, 1.0, cUsed.Weight, 1e-9)
}

func TestBuildGraphPathNormalization(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "./src/a.go", Name: "Alpha", Kind: "def"},
		{RelPath: "src/./b.go", Name: "Alpha", Kind: "ref"},
	}

	g := buildGraph(tags, []string{"./src/b.go"}, nil)
	require.NotNil(t, g)
	require.Equal(t, []string{"src/a.go", "src/b.go"}, g.Nodes)

	e := findEdge(t, g, "src/b.go", "src/a.go", "Alpha")
	require.Equal(t, 1, e.RefCount)
	require.InDelta(t, 50.0, e.Weight, 1e-9)
}

func TestBuildGraphAppliesHighFrequencyDefinitionAttenuation(t *testing.T) {
	t.Parallel()

	tags := make([]treesitter.Tag, 0, 16)
	for i := range 6 {
		path := "def/" + string(rune('a'+i)) + ".go"
		tags = append(tags,
			treesitter.Tag{RelPath: path, Name: "Common", Kind: "def"},
			treesitter.Tag{RelPath: path, Name: "noDef", Kind: "ref"},
		)
	}
	tags = append(tags, treesitter.Tag{RelPath: "ref/caller.go", Name: "Common", Kind: "ref"})

	g := buildGraph(tags, nil, nil)
	require.NotNil(t, g)

	for i := range 6 {
		to := "def/" + string(rune('a'+i)) + ".go"
		e := findEdge(t, g, "ref/caller.go", to, "Common")
		require.Equal(t, 1, e.RefCount)
		require.InDelta(t, 0.1, e.Weight, 1e-9)
	}
}

func TestBuildGraphAppliesPrivateIdentifierAttenuation(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "def/private.go", Name: "_x", Kind: "def"},
		{RelPath: "def/private.go", Name: "noDef", Kind: "ref"},
		{RelPath: "ref/caller.go", Name: "_x", Kind: "ref"},
	}

	g := buildGraph(tags, nil, nil)
	require.NotNil(t, g)

	e := findEdge(t, g, "ref/caller.go", "def/private.go", "_x")
	require.Equal(t, 1, e.RefCount)
	require.InDelta(t, 0.1, e.Weight, 1e-9)
}

func TestBuildGraphDeterministicAcrossRuns(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "b/b.go", Name: "Helper", Kind: "def"},
		{RelPath: "a/a.go", Name: "MainHandler", Kind: "def"},
		{RelPath: "c/c.go", Name: "MainHandler", Kind: "ref"},
		{RelPath: "b/b.go", Name: "MainHandler", Kind: "ref"},
	}

	first := buildGraph(tags, []string{"c/c.go"}, []string{"MainHandler"})
	require.NotNil(t, first)

	for range 20 {
		got := buildGraph(tags, []string{"c/c.go"}, []string{"MainHandler"})
		require.Equal(t, first.Nodes, got.Nodes)
		require.Equal(t, first.Edges, got.Edges)
	}
}

func findEdge(t *testing.T, g *FileGraph, from, to, ident string) GraphEdge {
	t.Helper()
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Ident == ident {
			return e
		}
	}
	t.Fatalf("edge not found: %s -> %s (%s)", from, to, ident)
	return GraphEdge{}
}

func countEdgesForIdent(g *FileGraph, ident string) int {
	n := 0
	for _, e := range g.Edges {
		if e.Ident == ident {
			n++
		}
	}
	return n
}
