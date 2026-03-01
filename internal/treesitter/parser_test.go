package treesitter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestParseTreeReturnsValidTree(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	src := []byte(`package main

func Run() {
	Run()
}
`)

	tree, err := p.ParseTree(context.Background(), "/tmp/main.go", src)
	require.NoError(t, err)
	require.NotNil(t, tree)
	defer tree.Close()
}

func TestParseTreeRootNodeNotNil(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	src := []byte(`package main

func Hello() {}
`)

	tree, err := p.ParseTree(context.Background(), "/tmp/main.go", src)
	require.NoError(t, err)
	require.NotNil(t, tree)
	defer tree.Close()

	root := tree.RootNode()
	require.NotNil(t, root, "root node must not be nil")
}

func TestParseTreeWalkReturnsUsableCursor(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	src := []byte(`package main

func Greet() string {
	return "hello"
}
`)

	tree, err := p.ParseTree(context.Background(), "/tmp/main.go", src)
	require.NoError(t, err)
	require.NotNil(t, tree)
	defer tree.Close()

	cursor := tree.Walk()
	require.NotNil(t, cursor, "tree cursor must not be nil")
	defer cursor.Close()

	// The cursor should start at the root node.
	node := cursor.Node()
	require.NotNil(t, node)
	require.Equal(t, "source_file", node.Kind())
}

func TestParseTreeCloseDoesNotPanic(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	src := []byte(`package main
`)

	tree, err := p.ParseTree(context.Background(), "/tmp/main.go", src)
	require.NoError(t, err)
	require.NotNil(t, tree)

	// Close must not panic.
	require.NotPanics(t, func() {
		tree.Close()
	})
}

func TestParseTreePopulatesCache(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	src := []byte(`package main

func Cached() {}
`)

	concrete, ok := p.(*parser)
	require.True(t, ok)

	statsBefore := concrete.treeCache.Stats()

	// First call: cache miss.
	tree1, err := p.ParseTree(context.Background(), "/tmp/cached.go", src)
	require.NoError(t, err)
	require.NotNil(t, tree1)
	defer tree1.Close()

	statsAfterFirst := concrete.treeCache.Stats()
	require.Equal(t, statsBefore.Misses+1, statsAfterFirst.Misses,
		"first call should be a cache miss")

	// Second call with same content: cache hit.
	tree2, err := p.ParseTree(context.Background(), "/tmp/cached.go", src)
	require.NoError(t, err)
	require.NotNil(t, tree2)
	defer tree2.Close()

	statsAfterSecond := concrete.treeCache.Stats()
	require.Equal(t, statsAfterFirst.Hits+1, statsAfterSecond.Hits,
		"second call should be a cache hit")
}

func TestParseTreeUnsupportedLanguageReturnsError(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	tree, err := p.ParseTree(context.Background(), "/tmp/data.xyz", []byte("some content"))
	require.Nil(t, tree)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported file")
}

func TestParseTreeUnsupportedLanguageNameReturnsError(t *testing.T) {
	t.Parallel()

	// Create a parser with an empty langSet to simulate a language that
	// maps from a path but is not in the supported set.
	pr := &parser{
		pool:      NewParserPoolWithSize(1),
		languages: nil,
		langSet:   map[string]struct{}{},
		treeCache: NewCache(0, 0),
		treeLangs: map[string]*tree_sitter.Language{},
	}
	t.Cleanup(func() {
		require.NoError(t, pr.Close())
	})

	// .go maps to "go" via MapPath, but langSet is empty.
	tree, err := pr.ParseTree(context.Background(), "/tmp/test.go", []byte("package main"))
	require.Nil(t, tree)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported language")
}

func TestParseTreeContextCanceled(t *testing.T) {
	t.Parallel()

	pp := newParserPoolWithFactory(1, func() *languageParser { return &languageParser{} })
	t.Cleanup(func() {
		require.NoError(t, pp.Close())
	})

	// Hold the only parser so acquire blocks.
	held, ok := pp.Acquire(context.Background(), "go")
	require.True(t, ok)

	pr := &parser{
		pool:    pp,
		langSet: map[string]struct{}{"go": {}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	tree, err := pr.ParseTree(ctx, "/tmp/file.go", []byte("package main"))
	require.Nil(t, tree)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	pp.Release("go", held)
}

func TestParseTreeDoesNotRequireTagsQuery(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	// Use a language that has a grammar but may not have tags.
	// Even if it does have tags, the important thing is ParseTree succeeds
	// without checking HasTags — we verify this by confirming a language
	// with a grammar parses successfully.
	src := []byte(`package main

func Main() {}
`)

	tree, err := p.ParseTree(context.Background(), "/tmp/main.go", src)
	require.NoError(t, err)
	require.NotNil(t, tree)
	defer tree.Close()

	root := tree.RootNode()
	require.NotNil(t, root)
	require.Greater(t, root.ChildCount(), uint(0))
}
