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
	require.Len(t, out, 3)
	require.Contains(t, out, ".github/workflows/ci.yml")
	require.Contains(t, out, "README.md")
	require.Contains(t, out, "app.yaml")
}

func TestBuildSpecialPreludeEnhancementDeterministicOrder(t *testing.T) {
	t.Parallel()

	other := []string{"README.md", "go.mod", ".github/workflows/zz.yml", ".github/workflows/aa.yml"}
	out := BuildSpecialPrelude(other, nil, false)
	require.Equal(t, []string{".github/workflows/aa.yml", ".github/workflows/zz.yml", "README.md", "go.mod"}, out)
}

func TestIsSpecialFileExpandedPatterns(t *testing.T) {
	t.Parallel()

	// Lock files.
	require.True(t, IsSpecialFile("package-lock.json"))
	require.True(t, IsSpecialFile("yarn.lock"))
	require.True(t, IsSpecialFile("Gemfile.lock"))
	require.True(t, IsSpecialFile("Cargo.lock"))
	require.True(t, IsSpecialFile("go.sum"))

	// CI/CD.
	require.True(t, IsSpecialFile(".github/workflows/ci.yml"))
	require.True(t, IsSpecialFile(".travis.yml"))
	require.True(t, IsSpecialFile(".circleci/config.yml"))
	require.True(t, IsSpecialFile("Jenkinsfile"))

	// Config.
	require.True(t, IsSpecialFile("tsconfig.json"))
	require.True(t, IsSpecialFile(".eslintrc"))
	require.True(t, IsSpecialFile(".prettierrc"))
	require.True(t, IsSpecialFile("pyproject.toml"))
	require.True(t, IsSpecialFile("setup.cfg"))

	// Build tools.
	require.True(t, IsSpecialFile("build.gradle"))
	require.True(t, IsSpecialFile("pom.xml"))
	require.True(t, IsSpecialFile("build.sbt"))

	// Documentation.
	require.True(t, IsSpecialFile("README.md"))
	require.True(t, IsSpecialFile("CHANGELOG.md"))
	require.True(t, IsSpecialFile("CONTRIBUTING.md"))
	require.True(t, IsSpecialFile("LICENSE"))

	// Container.
	require.True(t, IsSpecialFile("docker-compose.yml"))
	require.True(t, IsSpecialFile("Dockerfile"))

	// Other.
	require.True(t, IsSpecialFile("requirements.txt"))
	require.True(t, IsSpecialFile("setup.py"))
	require.True(t, IsSpecialFile(".gitignore"))
	require.True(t, IsSpecialFile(".editorconfig"))
}

func TestIsSpecialFileExclusions(t *testing.T) {
	t.Parallel()

	// Makefile is NOT in Aider's list.
	require.False(t, IsSpecialFile("Makefile"))

	// CMakeLists.txt is NOT in Aider's list.
	require.False(t, IsSpecialFile("CMakeLists.txt"))
}

func TestIsSpecialFileRootScopingSubdir(t *testing.T) {
	t.Parallel()

	// Root-scoped files must NOT match in subdirectories.
	require.False(t, IsSpecialFile("subdir/README.md"))
	require.False(t, IsSpecialFile("lib/package.json"))
	require.False(t, IsSpecialFile("pkg/go.mod"))
	require.False(t, IsSpecialFile("nested/deep/Dockerfile"))
}

func TestIsSpecialFileGitHubWorkflowsDynamic(t *testing.T) {
	t.Parallel()

	// .github/workflows/*.yml and *.yaml should match dynamically.
	require.True(t, IsSpecialFile(".github/workflows/deploy.yaml"))
	require.True(t, IsSpecialFile(".github/workflows/test.yml"))
	require.True(t, IsSpecialFile(".github/workflows/release.yaml"))

	// Non-yml/yaml files in .github/workflows should NOT match.
	require.False(t, IsSpecialFile(".github/workflows/script.sh"))
	require.False(t, IsSpecialFile(".github/workflows/README.md"))
}

func TestIsSpecialFileDirectoryScopedEntries(t *testing.T) {
	t.Parallel()

	// Directory-scoped entries from the map.
	require.True(t, IsSpecialFile(".circleci/config.yml"))
	require.True(t, IsSpecialFile(".github/dependabot.yml"))

	// But not arbitrary paths under those directories.
	require.False(t, IsSpecialFile(".circleci/other.yml"))
	require.False(t, IsSpecialFile(".github/other.yml"))
}

func TestSpecialRootFilesCount(t *testing.T) {
	t.Parallel()

	require.Len(t, specialRootFiles, 153)
}
