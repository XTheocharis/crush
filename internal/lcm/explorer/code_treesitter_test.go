package explorer

import (
	"context"
	"errors"
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

type mockTreeSitterParser struct {
	supports map[string]bool
	hasTags  map[string]bool
	analysis *treesitter.FileAnalysis
	err      error
	calls    int
}

func (m *mockTreeSitterParser) Analyze(_ context.Context, _ string, _ []byte) (*treesitter.FileAnalysis, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	if m.analysis != nil {
		return m.analysis, nil
	}
	return &treesitter.FileAnalysis{}, nil
}

func (m *mockTreeSitterParser) Languages() []string { return nil }

func (m *mockTreeSitterParser) SupportsLanguage(lang string) bool {
	if m.supports == nil {
		return false
	}
	return m.supports[lang]
}

func (m *mockTreeSitterParser) HasTags(lang string) bool {
	if m.hasTags == nil {
		return false
	}
	return m.hasTags[lang]
}

func (m *mockTreeSitterParser) Close() error { return nil }

func TestTreeSitterExplorerCanHandleRequiresSupportsAndTags(t *testing.T) {
	t.Parallel()

	t.Run("requires support and tags", func(t *testing.T) {
		t.Parallel()
		e := NewTreeSitterExplorer(&mockTreeSitterParser{
			supports: map[string]bool{"go": true},
			hasTags:  map[string]bool{"go": true},
		})
		require.True(t, e.CanHandle("main.go", []byte("package main")))
	})

	t.Run("rejects supported without tags", func(t *testing.T) {
		t.Parallel()
		e := NewTreeSitterExplorer(&mockTreeSitterParser{
			supports: map[string]bool{"go": true},
			hasTags:  map[string]bool{"go": false},
		})
		require.False(t, e.CanHandle("main.go", []byte("package main")))
	})

	t.Run("rejects tags without support", func(t *testing.T) {
		t.Parallel()
		e := NewTreeSitterExplorer(&mockTreeSitterParser{
			supports: map[string]bool{"go": false},
			hasTags:  map[string]bool{"go": true},
		})
		require.False(t, e.CanHandle("main.go", []byte("package main")))
	})
}

func TestTreeSitterExplorerExploreCallsAnalyze(t *testing.T) {
	t.Parallel()

	p := &mockTreeSitterParser{
		supports: map[string]bool{"go": true},
		hasTags:  map[string]bool{"go": true},
		analysis: &treesitter.FileAnalysis{
			Language: "go",
			Symbols:  []treesitter.SymbolInfo{{Name: "Main", Kind: "function", Line: 3}},
			Imports:  []treesitter.ImportInfo{{Path: "fmt"}},
			Tags:     []treesitter.Tag{{Name: "Main", Kind: "def", Line: 3}},
		},
	}
	e := NewTreeSitterExplorer(p)

	result, err := e.Explore(context.Background(), ExploreInput{Path: "main.go", Content: []byte("package main\nimport \"fmt\"\nfunc Main() {}")})
	require.NoError(t, err)
	require.Equal(t, 1, p.calls)
	require.Equal(t, "treesitter", result.ExplorerUsed)
	require.Contains(t, result.Summary, "Tree-sitter file: main.go")
	require.Contains(t, result.Summary, "Language: go")
	require.Contains(t, result.Summary, "Imports:")
	require.Contains(t, result.Summary, "fmt (stdlib)")
	require.Contains(t, result.Summary, "function Main (public")
	require.Contains(t, result.Summary, "def Main")
}

func TestTreeSitterExplorerExploreMaxFullLoadSizeGuard(t *testing.T) {
	t.Parallel()

	p := &mockTreeSitterParser{}
	e := NewTreeSitterExplorer(p)
	content := make([]byte, MaxFullLoadSize+1)

	result, err := e.Explore(context.Background(), ExploreInput{Path: "big.go", Content: content})
	require.NoError(t, err)
	require.Equal(t, 0, p.calls)
	require.Equal(t, "treesitter", result.ExplorerUsed)
	require.Contains(t, result.Summary, "File too large: big.go")
}

func TestTreeSitterExplorerExploreAnalyzeError(t *testing.T) {
	t.Parallel()

	p := &mockTreeSitterParser{err: errors.New("analyze failed")}
	e := NewTreeSitterExplorer(p)

	_, err := e.Explore(context.Background(), ExploreInput{Path: "main.go", Content: []byte("package main")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "analyze failed")
}
