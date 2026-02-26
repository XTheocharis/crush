package treesitter

import (
	"testing"

	"github.com/stretchr/testify/require"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

func TestQueryLoaderExtractTagsGo(t *testing.T) {
	t.Parallel()

	goLang := tree_sitter.NewLanguage(tree_sitter_go.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(goLang))

	src := []byte(`package main

type Config struct{}

func Run() {
	Run()
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("go", goLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	tags, symbols, err := loader.ExtractTags("go", "a.go", tree.RootNode(), src)
	require.NoError(t, err)
	require.NotEmpty(t, tags)

	hasRunDef := false
	hasRunRef := false
	hasConfigType := false
	for _, tag := range tags {
		if tag.Name == "Run" && tag.Kind == "def" && tag.NodeType == "function" {
			hasRunDef = true
		}
		if tag.Name == "Run" && tag.Kind == "ref" && tag.NodeType == "call" {
			hasRunRef = true
		}
		if tag.Name == "Config" && tag.Kind == "def" && tag.NodeType == "class" {
			hasConfigType = true
		}
	}
	require.True(t, hasRunDef)
	require.True(t, hasRunRef)
	require.True(t, hasConfigType)
	require.NotEmpty(t, symbols)
}

func TestQueryLoaderExtractTagsPython(t *testing.T) {
	t.Parallel()

	pyLang := tree_sitter.NewLanguage(tree_sitter_python.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(pyLang))

	src := []byte(`class Item:
    pass

def make():
    make()
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("python", pyLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	tags, _, err := loader.ExtractTags("python", "a.py", tree.RootNode(), src)
	require.NoError(t, err)
	require.NotEmpty(t, tags)

	hasClassDef := false
	hasCallRef := false
	for _, tag := range tags {
		if tag.Name == "Item" && tag.Kind == "def" && tag.NodeType == "class" {
			hasClassDef = true
		}
		if tag.Name == "make" && tag.Kind == "ref" && tag.NodeType == "call" {
			hasCallRef = true
		}
	}
	require.True(t, hasClassDef)
	require.True(t, hasCallRef)
}

func TestQueryLoaderCachesCompiledQuery(t *testing.T) {
	t.Parallel()

	goLang := tree_sitter.NewLanguage(tree_sitter_go.Language())
	loader := NewQueryLoader()
	loader.RegisterLanguage("go", goLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	q1, err := loader.LoadQuery("go")
	require.NoError(t, err)
	q2, err := loader.LoadQuery("go")
	require.NoError(t, err)
	require.Same(t, q1, q2)
}
