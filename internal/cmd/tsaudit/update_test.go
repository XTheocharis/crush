package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunUpdateDryRunDoesNotWriteFiles(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	manifestPath := filepath.Join(repoRoot, "internal", "treesitter", "languages.json")
	queriesDir := filepath.Join(repoRoot, "internal", "treesitter", "queries")
	primaryQueriesDir := filepath.Join(repoRoot, "testdata", "primary")
	fallbackQueriesDir := filepath.Join(repoRoot, "testdata", "fallback")
	primaryList := filepath.Join(repoRoot, "primary.json")
	fallbackList := filepath.Join(repoRoot, "fallback.json")

	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.MkdirAll(queriesDir, 0o755))
	require.NoError(t, os.MkdirAll(primaryQueriesDir, 0o755))
	require.NoError(t, os.MkdirAll(fallbackQueriesDir, 0o755))

	manifest := `{
  "generated": "2026-02-22T00:00:00Z",
  "languages": [
    {"name": "go", "grammar_module": "github.com/tree-sitter/tree-sitter-go"},
    {"name": "oldlang"}
  ]
}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifest), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "go-tags.scm"), []byte("GO_OLD\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "oldlang-tags.scm"), []byte("OLD\n"), 0o644))

	require.NoError(t, os.WriteFile(primaryList, []byte(`["go","python"]`), 0o644))
	require.NoError(t, os.WriteFile(fallbackList, []byte(`["python","java"]`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(primaryQueriesDir, "go-tags.scm"), []byte("GO_PRIMARY\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(primaryQueriesDir, "python-tags.scm"), []byte("PY_PRIMARY\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fallbackQueriesDir, "go-tags.scm"), []byte("GO_FALLBACK\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fallbackQueriesDir, "java-tags.scm"), []byte("JAVA_FALLBACK\n"), 0o644))

	manifestBefore, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	goBefore, err := os.ReadFile(filepath.Join(queriesDir, "go-tags.scm"))
	require.NoError(t, err)

	report, err := runUpdate(updateOptions{
		repoRoot:           repoRoot,
		manifestPath:       "internal/treesitter/languages.json",
		queriesDir:         "internal/treesitter/queries",
		primaryFile:        "primary.json",
		fallbackFile:       "fallback.json",
		primaryQueriesDir:  "testdata/primary",
		fallbackQueriesDir: "testdata/fallback",
		dryRun:             true,
	})
	require.NoError(t, err)

	require.Equal(t, []string{"java", "python"}, report.ManifestAdded)
	require.Equal(t, []string{"oldlang"}, report.ManifestRemoved)
	require.Equal(t, []string{"java", "python"}, report.QueriesAdded)
	require.Equal(t, []string{"oldlang"}, report.QueriesRemoved)
	require.Equal(t, []string{"go"}, report.QueriesUpdated)

	manifestAfter, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.Equal(t, manifestBefore, manifestAfter)

	goAfter, err := os.ReadFile(filepath.Join(queriesDir, "go-tags.scm"))
	require.NoError(t, err)
	require.Equal(t, goBefore, goAfter)

	_, err = os.Stat(filepath.Join(queriesDir, "python-tags.scm"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunUpdateApplyWritesManifestAndQueries(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	manifestPath := filepath.Join(repoRoot, "internal", "treesitter", "languages.json")
	queriesDir := filepath.Join(repoRoot, "internal", "treesitter", "queries")
	primaryQueriesDir := filepath.Join(repoRoot, "primary-src")
	fallbackQueriesDir := filepath.Join(repoRoot, "fallback-src")
	primaryList := filepath.Join(repoRoot, "primary.json")
	fallbackList := filepath.Join(repoRoot, "fallback.json")

	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.MkdirAll(queriesDir, 0o755))
	require.NoError(t, os.MkdirAll(primaryQueriesDir, 0o755))
	require.NoError(t, os.MkdirAll(fallbackQueriesDir, 0o755))

	manifest := `{
  "generated": "2026-02-22T00:00:00Z",
  "languages": [
    {"name": "go", "grammar_module": "github.com/tree-sitter/tree-sitter-go"},
    {"name": "oldlang", "query_source": "pack"}
  ]
}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifest), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "go-tags.scm"), []byte("GO_OLD\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(queriesDir, "oldlang-tags.scm"), []byte("OLD\n"), 0o644))

	require.NoError(t, os.WriteFile(primaryList, []byte(`["go","python"]`), 0o644))
	require.NoError(t, os.WriteFile(fallbackList, []byte(`["python","java"]`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(primaryQueriesDir, "go-tags.scm"), []byte("GO_PRIMARY\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(primaryQueriesDir, "python-tags.scm"), []byte("PY_PRIMARY\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fallbackQueriesDir, "go-tags.scm"), []byte("GO_FALLBACK\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fallbackQueriesDir, "java-tags.scm"), []byte("JAVA_FALLBACK\n"), 0o644))

	report, err := runUpdate(updateOptions{
		repoRoot:           repoRoot,
		manifestPath:       "internal/treesitter/languages.json",
		queriesDir:         "internal/treesitter/queries",
		primaryFile:        "primary.json",
		fallbackFile:       "fallback.json",
		primaryQueriesDir:  "primary-src",
		fallbackQueriesDir: "fallback-src",
		dryRun:             false,
	})
	require.NoError(t, err)
	require.True(t, report.HasChanges())

	manifestData, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(manifestData, &doc))
	require.Equal(t, "2026-02-22T00:00:00Z", doc["generated"])

	langs := languageEntriesByName(doc["languages"])
	require.Contains(t, langs, "go")
	require.Contains(t, langs, "python")
	require.Contains(t, langs, "java")
	require.NotContains(t, langs, "oldlang")
	require.Equal(t, "github.com/tree-sitter/tree-sitter-go", langs["go"]["grammar_module"])

	goData, err := os.ReadFile(filepath.Join(queriesDir, "go-tags.scm"))
	require.NoError(t, err)
	require.Equal(t, "GO_PRIMARY\n", string(goData))

	javaData, err := os.ReadFile(filepath.Join(queriesDir, "java-tags.scm"))
	require.NoError(t, err)
	require.Equal(t, "JAVA_FALLBACK\n", string(javaData))

	pythonData, err := os.ReadFile(filepath.Join(queriesDir, "python-tags.scm"))
	require.NoError(t, err)
	require.Equal(t, "PY_PRIMARY\n", string(pythonData))

	_, err = os.Stat(filepath.Join(queriesDir, "oldlang-tags.scm"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunUpdateRequiresAtLeastOneLanguage(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	primaryList := filepath.Join(repoRoot, "primary.json")
	fallbackList := filepath.Join(repoRoot, "fallback.json")
	require.NoError(t, os.WriteFile(primaryList, []byte(`[]`), 0o644))
	require.NoError(t, os.WriteFile(fallbackList, []byte(`[]`), 0o644))

	_, err := runUpdate(updateOptions{
		repoRoot:     repoRoot,
		primaryFile:  "primary.json",
		fallbackFile: "fallback.json",
	})
	require.ErrorContains(t, err, "no languages loaded")
}
