package repomap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSpecialFileRootScopingAndWorkflowException(t *testing.T) {
	t.Parallel()

	require.True(t, IsSpecialFile("README.md"))
	require.True(t, IsSpecialFile("go.mod"))
	require.True(t, IsSpecialFile(".github/workflows/ci.yml"))
	require.True(t, IsSpecialFile(".github/workflows/release.yaml"))

	require.False(t, IsSpecialFile("src/README.md"))
	require.False(t, IsSpecialFile("docs/go.mod"))
	require.False(t, IsSpecialFile(".github/workflows/ci.txt"))
	require.False(t, IsSpecialFile("Makefile"))
}

func TestBuildSpecialPreludeFromOtherFnamesOnlyAndDedupRanked(t *testing.T) {
	t.Parallel()

	other := []string{"README.md", "src/README.md", "go.mod", "app.yaml", ".github/workflows/ci.yml"}
	ranked := []string{"go.mod"}

	out := BuildSpecialPrelude(other, ranked, true)
	require.Equal(t, []string{".github/workflows/ci.yml", "README.md"}, out)
}

func TestBuildSpecialPreludeEnhancementDeterministicOrder(t *testing.T) {
	t.Parallel()

	other := []string{"README.md", "go.mod", ".github/workflows/zz.yml", ".github/workflows/aa.yml"}
	out := BuildSpecialPrelude(other, nil, false)
	require.Equal(t, []string{".github/workflows/aa.yml", ".github/workflows/zz.yml", "README.md", "go.mod"}, out)
}
