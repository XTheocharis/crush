package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestRunVerifyAllowsOfflineContractChecks(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	manifestPath := filepath.Join(repoRoot, "internal", "treesitter", "languages.json")
	queriesDir := filepath.Join(repoRoot, "internal", "treesitter", "queries")

	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.MkdirAll(queriesDir, 0o755))

	manifest := `{
  "languages": [
    {"name": "go"}
  ]
}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "go-tags.scm"), []byte("(x) @name.definition.function\n"), 0o644))

	report, err := runVerify(verifyOptions{
		repoRoot:     repoRoot,
		manifestPath: "internal/treesitter/languages.json",
		queriesDir:   "internal/treesitter/queries",
	})
	require.NoError(t, err)
	require.False(t, report.HasIssues())
}
