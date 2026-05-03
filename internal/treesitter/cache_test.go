package treesitter

import (
	"testing"

	"github.com/stretchr/testify/require"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func parseGoTree(t *testing.T, src string) *tree_sitter.Tree {
	t.Helper()

	lang := tree_sitter.NewLanguage(tree_sitter_go.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(lang))

	tree := p.Parse([]byte(src), nil)
	require.NotNil(t, tree)
	return tree
}

func TestEstimateTreeBytes(t *testing.T) {
	t.Parallel()

	require.Equal(t, int64(32*1024), EstimateTreeBytes([]byte("x")))
	big := make([]byte, 8000)
	require.Equal(t, int64(80000), EstimateTreeBytes(big))
}

func TestCacheReturnsCloneAndTracksHitsMisses(t *testing.T) {
	t.Parallel()

	cache := NewCache(8, 10*1024*1024)
	t.Cleanup(func() { require.NoError(t, cache.Close()) })

	tree := parseGoTree(t, "package main\nfunc main() {}\n")
	cache.Put("a", tree, []byte("package main\nfunc main() {}\n"))

	clone, ok := cache.Get("a")
	require.True(t, ok)
	require.NotNil(t, clone)
	require.NotSame(t, tree, clone)
	clone.Close()

	_, ok = cache.Get("missing")
	require.False(t, ok)

	stats := cache.Stats()
	require.Equal(t, int64(1), stats.Hits)
	require.Equal(t, int64(1), stats.Misses)
}

func TestCacheEvictsByByteBudget(t *testing.T) {
	t.Parallel()

	cache := NewCache(16, 70*1024)
	t.Cleanup(func() { require.NoError(t, cache.Close()) })

	tree1 := parseGoTree(t, "package main\nfunc a() {}\n")
	tree2 := parseGoTree(t, "package main\nfunc b() {}\n")
	tree3 := parseGoTree(t, "package main\nfunc c() {}\n")

	cache.Put("1", tree1, []byte("small"))
	cache.Put("2", tree2, []byte("small"))
	cache.Put("3", tree3, []byte("small"))

	_, ok1 := cache.Get("1")
	_, ok2 := cache.Get("2")
	_, ok3 := cache.Get("3")

	require.False(t, ok1, "oldest entry should be evicted by byte budget")
	require.True(t, ok2)
	require.True(t, ok3)

	require.LessOrEqual(t, cache.TotalBytes(), int64(70*1024))
	require.GreaterOrEqual(t, cache.Stats().Evictions, int64(1))
}

func TestCacheClosePurgesEntriesAndAccountsBytes(t *testing.T) {
	t.Parallel()

	cache := NewCache(8, 10*1024*1024)
	tree := parseGoTree(t, "package main\nfunc main() {}\n")
	cache.Put("x", tree, []byte("package main\nfunc main() {}\n"))
	require.Greater(t, cache.TotalBytes(), int64(0))

	require.NoError(t, cache.Close())
	require.Equal(t, int64(0), cache.TotalBytes())

	_, ok := cache.Get("x")
	require.False(t, ok)
}
