package repomap

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractNormalizeFileUniverseDeterministic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pathA := filepath.Join(root, "b", "main.go")
	pathB := filepath.Join(root, "a", "util.go")

	normalized, err := normalizeFileUniverse(root, []string{
		pathA,
		"./a/util.go",
		"a/./util.go",
		pathB,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"a/util.go", "b/main.go"}, normalized)
}

func TestExtractNormalizeRepoRelPathRejectsOutsideRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, err := normalizeRepoRelPath(root, "../outside.go")
	require.Error(t, err)
}

func TestExtractRepoKeyForRootStable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	k1 := repoKeyForRoot(root)
	k2 := repoKeyForRoot(filepath.Join(root, "."))

	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2)
}
