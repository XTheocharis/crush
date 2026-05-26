//go:build treesitter
// +build treesitter

package repomap

import (
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

func TestBuildGraph_WithImports(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "internal/db/db.go", Name: "Queries", Kind: "def"},
		{RelPath: "internal/repomap/graph.go", Name: "buildGraph", Kind: "def"},
	}

	imports := []ImportEdge{
		{From: "internal/repomap/graph.go", ImportPath: "github.com/charmbracelet/crush/internal/db", Category: "local"},
		{From: "internal/repomap/graph.go", ImportPath: "fmt", Category: "stdlib"},
		{From: "internal/repomap/graph.go", ImportPath: "github.com/stretchr/testify", Category: "third_party"},
	}

	g := buildGraph(tags, nil, nil, BuildGraphOptions{Imports: imports})
	require.NotNil(t, g)

	localEdge := findEdge(t, g, "internal/repomap/graph.go", "internal/db/db.go", "import:github.com/charmbracelet/crush/internal/db")
	require.InDelta(t, 1.0, localEdge.Weight, 1e-9)
	require.Equal(t, 1, localEdge.RefCount)

	for _, e := range g.Edges {
		if e.From == "internal/repomap/graph.go" && e.To == "internal/repomap/graph.go" {
			continue
		}
		require.NotContains(t, e.Ident, "import:fmt",
			"stdlib import should not produce an edge")
		require.NotContains(t, e.Ident, "import:github.com/stretchr/testify",
			"third-party import should not produce an edge")
	}
}

func TestBuildGraph_WithImports_SkipsSelfEdges(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "internal/db/db.go", Name: "Queries", Kind: "def"},
	}

	imports := []ImportEdge{
		{From: "internal/db/db.go", ImportPath: "github.com/charmbracelet/crush/internal/db", Category: "local"},
	}

	g := buildGraph(tags, nil, nil, BuildGraphOptions{Imports: imports})
	require.NotNil(t, g)

	for _, e := range g.Edges {
		require.NotContains(t, e.Ident, "import:",
			"self-import should not produce an edge")
	}
}

func TestBuildGraph_WithImports_NoImportsUnchanged(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "def/a.go", Name: "process_item", Kind: "def"},
		{RelPath: "ref/r.go", Name: "process_item", Kind: "ref"},
	}

	g := buildGraph(tags, nil, nil)
	require.NotNil(t, g)
	require.Equal(t, []string{"def/a.go", "ref/r.go"}, g.Nodes)
}

func TestBuildGraph_WithImports_ResolvesDirectoryPrefix(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "pkg/handler/handler.go", Name: "Handler", Kind: "def"},
		{RelPath: "pkg/handler/util.go", Name: "Helper", Kind: "def"},
		{RelPath: "cmd/main.go", Name: "Main", Kind: "def"},
	}

	imports := []ImportEdge{
		{From: "cmd/main.go", ImportPath: "example.com/project/pkg/handler", Category: "local"},
	}

	g := buildGraph(tags, nil, nil, BuildGraphOptions{Imports: imports})
	require.NotNil(t, g)

	edgesToHandler := 0
	for _, e := range g.Edges {
		if e.From == "cmd/main.go" && e.Ident == "import:example.com/project/pkg/handler" {
			edgesToHandler++
			require.Equal(t, 1.0, e.Weight)
		}
	}
	require.Equal(t, 2, edgesToHandler,
		"import should create edges to all files in the imported directory")
}

func TestResolveImportToFiles(t *testing.T) {
	t.Parallel()

	dirToFiles := map[string][]string{
		"internal/db":         {"internal/db/db.go", "internal/db/models.go"},
		"internal/treesitter": {"internal/treesitter/parser.go"},
	}

	tests := []struct {
		name       string
		importPath string
		expected   int
	}{
		{name: "direct match", importPath: "internal/db", expected: 2},
		{name: "suffix match strips module", importPath: "github.com/charmbracelet/crush/internal/db", expected: 2},
		{name: "no match", importPath: "nonexistent/pkg", expected: 0},
		{name: "empty path", importPath: "", expected: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := resolveImportToFiles(tc.importPath, dirToFiles)
			require.Len(t, result, tc.expected)
		})
	}
}

func TestPageRank_WithImports(t *testing.T) {
	t.Parallel()

	g := &FileGraph{
		Nodes: []string{"hub.go", "a.go", "b.go", "c.go", "d.go"},
		Edges: []GraphEdge{
			{From: "a.go", To: "hub.go", Ident: "import:pkg/hub", Weight: 1.0, RefCount: 1},
			{From: "b.go", To: "hub.go", Ident: "import:pkg/hub", Weight: 1.0, RefCount: 1},
			{From: "c.go", To: "hub.go", Ident: "import:pkg/hub", Weight: 1.0, RefCount: 1},
			{From: "d.go", To: "a.go", Ident: "import:pkg/a", Weight: 1.0, RefCount: 1},
			{From: "d.go", To: "b.go", Ident: "import:pkg/b", Weight: 1.0, RefCount: 1},
		},
	}

	defs := Rank(g, nil)
	require.NotEmpty(t, defs)

	hubRank := 0.0
	dRank := 0.0
	for _, d := range defs {
		if d.File == "hub.go" {
			hubRank += d.Rank
		}
		if d.File == "d.go" {
			dRank += d.Rank
		}
	}
	require.Greater(t, hubRank, dRank,
		"hub.go with 3 incoming import edges should rank higher than d.go with 0")
}
