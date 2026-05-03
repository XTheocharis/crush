package treesitter

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_julia "github.com/tree-sitter/tree-sitter-julia/bindings/go"
	tree_sitter_ocaml "github.com/tree-sitter/tree-sitter-ocaml/bindings/go"
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

func TestQueryLoaderExtractTagsJulia(t *testing.T) {
	t.Parallel()

	juliaLang := tree_sitter.NewLanguage(tree_sitter_julia.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(juliaLang))

	src, err := os.ReadFile("testdata/sample.jl")
	require.NoError(t, err)

	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("julia", juliaLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	tags, symbols, err := loader.ExtractTags("julia", "sample.jl", tree.RootNode(), src)
	require.NoError(t, err)
	require.NotEmpty(t, tags)
	require.NotEmpty(t, symbols)

	// Collect tags by kind and type for verification.
	type tagKey struct {
		name     string
		kind     string
		nodeType string
	}
	found := make(map[tagKey]bool)
	for _, tag := range tags {
		found[tagKey{tag.Name, tag.Kind, tag.NodeType}] = true
	}

	// Module definition.
	require.True(t, found[tagKey{"Geometry", "def", "module"}], "missing module def Geometry")

	// Struct definitions (both immutable and mutable).
	require.True(t, found[tagKey{"Point", "def", "class"}], "missing struct def Point")
	require.True(t, found[tagKey{"Counter", "def", "class"}], "missing mutable struct def Counter")

	// Abstract type definition.
	require.True(t, found[tagKey{"Shape", "def", "class"}], "missing abstract type def Shape")

	// Constant definition.
	require.True(t, found[tagKey{"MAX_SIZE", "def", "constant"}], "missing const def MAX_SIZE")

	// Function definitions.
	require.True(t, found[tagKey{"distance", "def", "function"}], "missing function def distance")
	require.True(t, found[tagKey{"identity", "def", "function"}], "missing function def identity")

	// Short-form function definition.
	require.True(t, found[tagKey{"area", "def", "function"}], "missing short-form function def area")

	// Macro definition.
	require.True(t, found[tagKey{"debug", "def", "macro"}], "missing macro def debug")

	// Call references.
	require.True(t, found[tagKey{"sqrt", "ref", "call"}], "missing call ref sqrt")

	// Export references.
	require.True(t, found[tagKey{"distance", "ref", "export"}], "missing export ref distance")
	require.True(t, found[tagKey{"area", "ref", "export"}], "missing export ref area")

	// Using references.
	require.True(t, found[tagKey{"LinearAlgebra", "ref", "module"}], "missing using ref LinearAlgebra")
}

func TestQueryLoaderExtractTagsOCamlInterface(t *testing.T) {
	t.Parallel()

	ocamlInterfaceLang := tree_sitter.NewLanguage(tree_sitter_ocaml.LanguageOCamlInterface())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(ocamlInterfaceLang))

	// Read the .mli fixture from testdata.
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	fixturePath := filepath.Join(filepath.Dir(thisFile), "testdata", "sample.mli")
	src, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	tree := p.Parse(src, nil)
	require.NotNil(t, tree)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("ocaml_interface", ocamlInterfaceLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	tags, symbols, err := loader.ExtractTags("ocaml_interface", "sample.mli", tree.RootNode(), src)
	require.NoError(t, err)

	// The .mli fixture should produce well over 16 tags from the modern
	// capture format. Accept a range to avoid brittleness against minor
	// grammar version changes.
	require.GreaterOrEqual(t, len(tags), 16, "expected at least 16 tags, got %d", len(tags))

	// Build a lookup set for specific expected tags.
	type tagKey struct {
		Name     string
		Kind     string
		NodeType string
	}
	tagSet := make(map[tagKey]bool, len(tags))
	for _, tag := range tags {
		tagSet[tagKey{tag.Name, tag.Kind, tag.NodeType}] = true
	}

	// Verify representative definitions from each converted pattern category.
	require.True(t, tagSet[tagKey{"Config", "def", "module"}], "missing Config module definition")
	require.True(t, tagSet[tagKey{"Storage", "def", "interface"}], "missing Storage module type definition")
	require.True(t, tagSet[tagKey{"base_handler", "def", "class"}], "missing base_handler class definition")
	require.True(t, tagSet[tagKey{"color", "def", "type"}], "missing color type definition")
	require.True(t, tagSet[tagKey{"point", "def", "type"}], "missing point type definition")
	require.True(t, tagSet[tagKey{"Red", "def", "enum_variant"}], "missing Red enum_variant definition")
	require.True(t, tagSet[tagKey{"Green", "def", "enum_variant"}], "missing Green enum_variant definition")
	require.True(t, tagSet[tagKey{"x", "def", "field"}], "missing x field definition")
	require.True(t, tagSet[tagKey{"y", "def", "field"}], "missing y field definition")
	require.True(t, tagSet[tagKey{"hash", "def", "function"}], "missing hash external definition")
	require.True(t, tagSet[tagKey{"run", "def", "function"}], "missing run value specification")
	require.True(t, tagSet[tagKey{"create", "def", "function"}], "missing create value specification")

	// Verify references are also captured.
	require.True(t, tagSet[tagKey{"float", "ref", "type"}], "missing float type reference")

	// Verify at least some symbols were produced (definitions only).
	require.NotEmpty(t, symbols)
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
