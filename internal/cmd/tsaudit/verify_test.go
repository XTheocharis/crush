package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeParserAndGoModFixture(t *testing.T, repoRoot string, runtimes map[string]string, deps []string) {
	t.Helper()

	parserPath := filepath.Join(repoRoot, "internal", "treesitter", "parser.go")
	goModPath := filepath.Join(repoRoot, "go.mod")
	require.NoError(t, os.MkdirAll(filepath.Dir(parserPath), 0o755))

	var imports strings.Builder
	var cases strings.Builder
	for lang, mod := range runtimes {
		alias := "tree_sitter_" + strings.ReplaceAll(lang, "-", "_")
		fmt.Fprintf(&imports, "\t%s \"%s/bindings/go\"\n", alias, mod)
		fmt.Fprintf(&cases, "\tcase \"%s\":\n\t\treturn %s.Language()\n", lang, alias)
	}

	parserSource := "package treesitter\n\nimport (\n" + imports.String() + ")\n\nfunc languageForQueryKey(queryKey string) any {\n\tswitch queryKey {\n" + cases.String() + "\tdefault:\n\t\treturn nil\n\t}\n}\n"
	require.NoError(t, os.WriteFile(parserPath, []byte(parserSource), 0o644))

	var reqBlock strings.Builder
	for _, dep := range deps {
		fmt.Fprintf(&reqBlock, "\t%s v0.1.0\n", dep)
	}
	goMod := "module example.com/test\n\ngo 1.26.0\n\nrequire (\n" + reqBlock.String() + ")\n"
	require.NoError(t, os.WriteFile(goModPath, []byte(goMod), 0o644))
}

func TestRunVerifyOK(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	manifestPath := filepath.Join(repoRoot, "internal", "treesitter", "languages.json")
	queriesDir := filepath.Join(repoRoot, "internal", "treesitter", "queries")
	primaryQueriesDir := filepath.Join(repoRoot, "primary-src")

	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.MkdirAll(queriesDir, 0o755))
	require.NoError(t, os.MkdirAll(primaryQueriesDir, 0o755))

	manifest := `{
  "languages": [
    {"name": "go"},
    {"name": "ocaml_interface"}
  ]
}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifest), 0o644))

	goQuery := "(function_declaration name: (identifier) @name.definition.function)"
	legacyQuery := "(value_definition (identifier) @name @definition.function)"

	require.NoError(t, os.WriteFile(filepath.Join(primaryQueriesDir, "go-tags.scm"), []byte(goQuery), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(primaryQueriesDir, "ocaml_interface-tags.scm"), []byte(legacyQuery), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "go-tags.scm"), []byte(goQuery), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "ocaml_interface-tags.scm"), []byte(legacyQuery), 0o644))
	writeParserAndGoModFixture(t, repoRoot,
		map[string]string{
			"go":              "github.com/tree-sitter/tree-sitter-go",
			"ocaml_interface": "github.com/tree-sitter/tree-sitter-ocaml",
		},
		[]string{
			"github.com/tree-sitter/tree-sitter-go",
			"github.com/tree-sitter/tree-sitter-ocaml",
		},
	)

	report, err := runVerify(verifyOptions{
		repoRoot:          repoRoot,
		manifestPath:      "internal/treesitter/languages.json",
		queriesDir:        "internal/treesitter/queries",
		primaryQueriesDir: "primary-src",
	})
	require.NoError(t, err)
	require.False(t, report.HasIssues())
	require.Empty(t, report.MissingVendored)
	require.Empty(t, report.MissingUpstreamSource)
	require.Empty(t, report.ContentMismatches)
	require.Empty(t, report.InheritsDirectives)
	require.Empty(t, report.Uninterpretable)
}

func TestRunVerifyDetectsPlanRequiredFailures(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	manifestPath := filepath.Join(repoRoot, "internal", "treesitter", "languages.json")
	queriesDir := filepath.Join(repoRoot, "internal", "treesitter", "queries")
	primaryQueriesDir := filepath.Join(repoRoot, "primary-src")
	fallbackQueriesDir := filepath.Join(repoRoot, "fallback-src")

	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.MkdirAll(queriesDir, 0o755))
	require.NoError(t, os.MkdirAll(primaryQueriesDir, 0o755))
	require.NoError(t, os.MkdirAll(fallbackQueriesDir, 0o755))

	manifest := `{
  "languages": [
    {"name": "go"},
    {"name": "java"},
    {"name": "python"},
    {"name": "rust"}
  ]
}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifest), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(primaryQueriesDir, "go-tags.scm"), []byte("; inherits: javascript\n(x) @name.definition.function\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(primaryQueriesDir, "java-tags.scm"), []byte("(x) @name.definition.class\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fallbackQueriesDir, "java-tags.scm"), []byte("(x) @name.reference.class\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fallbackQueriesDir, "python-tags.scm"), []byte("(x) @doc\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "go-tags.scm"), []byte("; inherits: javascript\n(x) @name.definition.function\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "java-tags.scm"), []byte("(x) @name.reference.class\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "python-tags.scm"), []byte("(x) @doc\n"), 0o644))
	writeParserAndGoModFixture(t, repoRoot,
		map[string]string{
			"go":     "github.com/tree-sitter/tree-sitter-go",
			"java":   "github.com/tree-sitter/tree-sitter-java",
			"python": "github.com/tree-sitter/tree-sitter-python",
		},
		[]string{
			"github.com/tree-sitter/tree-sitter-go",
			"github.com/tree-sitter/tree-sitter-java",
			"github.com/tree-sitter/tree-sitter-python",
		},
	)

	report, err := runVerify(verifyOptions{
		repoRoot:           repoRoot,
		manifestPath:       "internal/treesitter/languages.json",
		queriesDir:         "internal/treesitter/queries",
		primaryQueriesDir:  "primary-src",
		fallbackQueriesDir: "fallback-src",
	})
	require.NoError(t, err)
	require.True(t, report.HasIssues())

	require.Equal(t, []string{"rust"}, report.MissingVendored)
	require.Equal(t, []string{"rust"}, report.MissingUpstreamSource)
	require.Equal(t, []string{"java"}, report.ContentMismatches)
	require.Equal(t, []string{"go"}, report.InheritsDirectives)
	require.Equal(t, []string{"python"}, report.Uninterpretable)
}

func TestRunVerifyDetectsRuntimeAndDependencyMismatches(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	manifestPath := filepath.Join(repoRoot, "internal", "treesitter", "languages.json")
	queriesDir := filepath.Join(repoRoot, "internal", "treesitter", "queries")
	parserPath := filepath.Join(repoRoot, "internal", "treesitter", "parser.go")
	goModPath := filepath.Join(repoRoot, "go.mod")

	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.MkdirAll(queriesDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(parserPath), 0o755))

	manifest := `{
  "languages": [
    {"name": "go", "grammar_module": "github.com/tree-sitter/tree-sitter-go"},
    {"name": "swift", "grammar_module": "github.com/alex-pinkus/tree-sitter-swift"}
  ]
}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "go-tags.scm"), []byte("(x) @name.definition.function\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "swift-tags.scm"), []byte("(x) @name.definition.class\n"), 0o644))

	parserSource := `package treesitter

import (
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func languageForQueryKey(queryKey string) any {
	switch queryKey {
	case "go":
		return tree_sitter_go.Language()
	default:
		return nil
	}
}
`
	require.NoError(t, os.WriteFile(parserPath, []byte(parserSource), 0o644))

	goMod := `module example.com/test

go 1.26.0

require (
	github.com/tree-sitter/tree-sitter-go v0.1.0
)
`
	require.NoError(t, os.WriteFile(goModPath, []byte(goMod), 0o644))

	report, err := runVerify(verifyOptions{
		repoRoot:     repoRoot,
		manifestPath: "internal/treesitter/languages.json",
		queriesDir:   "internal/treesitter/queries",
	})
	require.NoError(t, err)
	require.Contains(t, report.MissingRuntimeGrammar, "swift")
	require.Empty(t, report.RuntimeNotInManifest)
	require.Empty(t, report.MissingDependency)
}

func TestRunVerifyDetectsMissingDependencyForRegisteredRuntimeGrammar(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	manifestPath := filepath.Join(repoRoot, "internal", "treesitter", "languages.json")
	queriesDir := filepath.Join(repoRoot, "internal", "treesitter", "queries")
	parserPath := filepath.Join(repoRoot, "internal", "treesitter", "parser.go")
	goModPath := filepath.Join(repoRoot, "go.mod")

	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.MkdirAll(queriesDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(parserPath), 0o755))

	manifest := `{
  "languages": [
    {"name": "go", "grammar_module": "github.com/tree-sitter/tree-sitter-go"}
  ]
}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "go-tags.scm"), []byte("(x) @name.definition.function\n"), 0o644))

	parserSource := `package treesitter

import (
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func languageForQueryKey(queryKey string) any {
	switch queryKey {
	case "go":
		return tree_sitter_go.Language()
	default:
		return nil
	}
}
`
	require.NoError(t, os.WriteFile(parserPath, []byte(parserSource), 0o644))

	goMod := `module example.com/test

go 1.26.0

require (
	github.com/tree-sitter/tree-sitter-python v0.1.0
)
`
	require.NoError(t, os.WriteFile(goModPath, []byte(goMod), 0o644))

	report, err := runVerify(verifyOptions{
		repoRoot:     repoRoot,
		manifestPath: "internal/treesitter/languages.json",
		queriesDir:   "internal/treesitter/queries",
	})
	require.NoError(t, err)
	require.Empty(t, report.MissingRuntimeGrammar)
	require.Empty(t, report.RuntimeNotInManifest)
	require.Equal(t, []string{"go (github.com/tree-sitter/tree-sitter-go)"}, report.MissingDependency)
}

func TestRunVerifyDetectsRuntimeNotInManifest(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	manifestPath := filepath.Join(repoRoot, "internal", "treesitter", "languages.json")
	queriesDir := filepath.Join(repoRoot, "internal", "treesitter", "queries")
	parserPath := filepath.Join(repoRoot, "internal", "treesitter", "parser.go")
	goModPath := filepath.Join(repoRoot, "go.mod")

	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.MkdirAll(queriesDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(parserPath), 0o755))

	manifest := `{
  "languages": [
    {"name": "go", "grammar_module": "github.com/tree-sitter/tree-sitter-go"}
  ]
}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "go-tags.scm"), []byte("(x) @name.definition.function\n"), 0o644))

	parserSource := `package treesitter

import (
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

func languageForQueryKey(queryKey string) any {
	switch queryKey {
	case "go":
		return tree_sitter_go.Language()
	case "python":
		return tree_sitter_python.Language()
	default:
		return nil
	}
}
`
	require.NoError(t, os.WriteFile(parserPath, []byte(parserSource), 0o644))

	goMod := `module example.com/test

go 1.26.0

require (
	github.com/tree-sitter/tree-sitter-go v0.1.0
	github.com/tree-sitter/tree-sitter-python v0.1.0
)
`
	require.NoError(t, os.WriteFile(goModPath, []byte(goMod), 0o644))

	report, err := runVerify(verifyOptions{
		repoRoot:     repoRoot,
		manifestPath: "internal/treesitter/languages.json",
		queriesDir:   "internal/treesitter/queries",
	})
	require.NoError(t, err)
	require.Empty(t, report.MissingRuntimeGrammar)
	require.Equal(t, []string{"python"}, report.RuntimeNotInManifest)
	require.Empty(t, report.MissingDependency)
}

func TestRunVerifyAllowsOfflineContractChecks(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	manifestPath := filepath.Join(repoRoot, "internal", "treesitter", "languages.json")
	queriesDir := filepath.Join(repoRoot, "internal", "treesitter", "queries")
	parserPath := filepath.Join(repoRoot, "internal", "treesitter", "parser.go")
	goModPath := filepath.Join(repoRoot, "go.mod")

	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.MkdirAll(queriesDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(parserPath), 0o755))

	manifest := `{
  "languages": [
    {"name": "go", "grammar_module": "github.com/tree-sitter/tree-sitter-go"}
  ]
}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "go-tags.scm"), []byte("(x) @name.definition.function\n"), 0o644))

	parserSource := `package treesitter

import (
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func languageForQueryKey(queryKey string) any {
	switch queryKey {
	case "go":
		return tree_sitter_go.Language()
	default:
		return nil
	}
}
`
	require.NoError(t, os.WriteFile(parserPath, []byte(parserSource), 0o644))

	goMod := `module example.com/test

go 1.26.0

require (
	github.com/tree-sitter/tree-sitter-go v0.1.0
)
`
	require.NoError(t, os.WriteFile(goModPath, []byte(goMod), 0o644))

	report, err := runVerify(verifyOptions{
		repoRoot:     repoRoot,
		manifestPath: "internal/treesitter/languages.json",
		queriesDir:   "internal/treesitter/queries",
	})
	require.NoError(t, err)
	require.False(t, report.HasIssues())
}
