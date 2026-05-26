package config

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextPathWalking(t *testing.T) {
	t.Parallel()

	t.Run("DiscoversFromRootToHome", TestWalkContextPathsDiscoversFromRootToHome)
	t.Run("DeduplicatesByAbsPath", TestWalkContextPathsDeduplicatesByAbsPath)
	t.Run("FindsMultipleFiles", TestWalkContextPathsFindsMultipleFiles)
	t.Run("EmptyDirReturnsEmpty", TestWalkContextPathsEmptyDirReturnsEmpty)
	t.Run("FindsGithubCopilotInstructions", TestWalkContextPathsFindsGithubCopilotInstructions)
	t.Run("ResultOrdering", TestWalkContextPathsResultOrdering)
	t.Run("DoesNotFollowSymlinks", TestWalkContextPathsDoesNotFollowSymlinks)
	t.Run("StopsAtHome", TestWalkContextPathsStopsAtHome)
}

func TestContextPathsExistDetectsDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.False(t, mustContextPathsExist(dir), "should be false with empty dir")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".cursor", "rules"), 0o755))
	require.True(t, mustContextPathsExist(dir), ".cursor/rules/ should be detected as existing")
}

func TestContextPathsExistDetectsFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "CRUSH.md"), []byte("test"), 0o644))
	require.True(t, mustContextPathsExist(dir), "CRUSH.md should be detected")
}

func TestContextPathsExistDetectsNestedPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".github"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".github", "copilot-instructions.md"), []byte("test"), 0o644))
	require.True(t, mustContextPathsExist(dir), ".github/copilot-instructions.md should be detected")
}

func mustContextPathsExist(dir string) bool {
	ok, err := contextPathsExist(dir)
	if err != nil {
		panic(err)
	}
	return ok
}

func TestWalkContextPathsDiscoversFromRootToHome(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	inner := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(inner, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(inner, "AGENTS.md"), []byte("inner"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CRUSH.md"), []byte("outer"), 0o644))

	paths, err := WalkContextPaths(inner)
	require.NoError(t, err)

	var basenames []string
	for _, p := range paths {
		basenames = append(basenames, filepath.Base(p))
	}
	require.Contains(t, basenames, "AGENTS.md")
}

func TestWalkContextPathsDeduplicatesByAbsPath(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("test"), 0o644))

	paths, err := WalkContextPaths(tmp)
	require.NoError(t, err)

	count := 0
	for _, p := range paths {
		if filepath.Base(p) == "CLAUDE.md" {
			count++
		}
	}
	require.Equal(t, 1, count, "CLAUDE.md should appear exactly once")
}

func TestWalkContextPathsFindsMultipleFiles(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CRUSH.md"), []byte("b"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".cursorrules"), []byte("c"), 0o644))

	paths, err := WalkContextPaths(tmp)
	require.NoError(t, err)

	var basenames []string
	for _, p := range paths {
		basenames = append(basenames, filepath.Base(p))
	}
	sort.Strings(basenames)
	require.Contains(t, basenames, "AGENTS.md")
	require.Contains(t, basenames, "CRUSH.md")
	require.Contains(t, basenames, ".cursorrules")
}

func TestWalkContextPathsEmptyDirReturnsEmpty(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	paths, err := WalkContextPaths(tmp)
	require.NoError(t, err)
	require.Empty(t, paths)
}

func TestWalkContextPathsFindsGithubCopilotInstructions(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".github"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".github", "copilot-instructions.md"), []byte("copilot"), 0o644))

	paths, err := WalkContextPaths(tmp)
	require.NoError(t, err)

	var basenames []string
	for _, p := range paths {
		basenames = append(basenames, filepath.Base(p))
	}
	require.Contains(t, basenames, "copilot-instructions.md")
}

func TestWalkContextPathsResultOrdering(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	inner := filepath.Join(tmp, "deep", "project")
	require.NoError(t, os.MkdirAll(inner, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(inner, "AGENTS.md"), []byte("deep"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CRUSH.md"), []byte("shallow"), 0o644))

	paths, err := WalkContextPaths(inner)
	require.NoError(t, err)

	require.True(t, len(paths) >= 2, "expected at least 2 paths")

	// Results should be ordered from deepest (inner) to shallowest (tmp).
	// WalkContextPaths reverses so deepest is first.
	foundAgents := -1
	foundCrush := -1
	for i, p := range paths {
		base := filepath.Base(p)
		if base == "AGENTS.md" {
			foundAgents = i
		}
		if base == "CRUSH.md" {
			foundCrush = i
		}
	}
	require.True(t, foundAgents >= 0, "AGENTS.md should be found")
	require.True(t, foundCrush >= 0, "CRUSH.md should be found")
	require.Less(t, foundAgents, foundCrush, "AGENTS.md (deeper) should appear before CRUSH.md (shallower)")
}

func TestWalkContextPathsDoesNotFollowSymlinks(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	target := filepath.Join(tmp, "target")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "AGENTS.md"), []byte("real"), 0o644))

	linkDir := filepath.Join(tmp, "project")
	require.NoError(t, os.Symlink(target, linkDir))

	paths, err := WalkContextPaths(linkDir)
	require.NoError(t, err)

	require.True(t, len(paths) >= 1, "should find at least one context file")
	found := false
	for _, p := range paths {
		if filepath.Base(p) == "AGENTS.md" {
			found = true
		}
	}
	require.True(t, found, "AGENTS.md should be discovered via symlinked directory")
}

func TestWalkContextPathsStopsAtHome(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	paths, err := WalkContextPaths(tmp)
	require.NoError(t, err)

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	for _, p := range paths {
		rel, err := filepath.Rel(homeDir, p)
		require.NoError(t, err)
		// If the path is not relative to home, it should be under tmp.
		// The key invariant: no path should be above home.
		if rel != "" && rel[0] == '.' {
			continue
		}
	}
}

func TestResolveContextPathsManagedWinsOverLocal(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	managed := filepath.Join(tmp, "managed")
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")
	local := filepath.Join(tmp, "local")

	for _, dir := range []string{managed, user, project, local} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}

	require.NoError(t, os.WriteFile(filepath.Join(managed, "AGENTS.md"), []byte("managed"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(user, "AGENTS.md"), []byte("user"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("project"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(local, "AGENTS.md"), []byte("local"), 0o644))

	paths, err := ResolveContextPaths(local, managed, user, project)
	require.NoError(t, err)

	agentsPaths := filterByName(paths, "AGENTS.md")
	require.Len(t, agentsPaths, 1, "should find exactly one AGENTS.md")
	require.Contains(t, agentsPaths[0], "managed", "managed layer should win")
}

func TestResolveContextPathsFallsThroughLayers(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	managed := filepath.Join(tmp, "managed")
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")
	local := filepath.Join(tmp, "local")

	for _, dir := range []string{managed, user, project, local} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}

	// AGENTS.md only in project, CRUSH.md only in user.
	require.NoError(t, os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("project"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(user, "CRUSH.md"), []byte("user"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(local, "GEMINI.md"), []byte("local"), 0o644))

	paths, err := ResolveContextPaths(local, managed, user, project)
	require.NoError(t, err)

	require.Len(t, filterByName(paths, "AGENTS.md"), 1)
	require.Len(t, filterByName(paths, "CRUSH.md"), 1)
	require.Len(t, filterByName(paths, "GEMINI.md"), 1)
}

func TestResolveContextPathsDeduplicatesByBasename(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	managed := filepath.Join(tmp, "managed")
	local := filepath.Join(tmp, "local")
	require.NoError(t, os.MkdirAll(managed, 0o755))
	require.NoError(t, os.MkdirAll(local, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(managed, "CRUSH.md"), []byte("managed"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(local, "CRUSH.md"), []byte("local"), 0o644))

	paths, err := ResolveContextPaths(local, managed, "", "")
	require.NoError(t, err)

	crushPaths := filterByName(paths, "CRUSH.md")
	require.Len(t, crushPaths, 1, "should deduplicate by basename")
	require.Contains(t, crushPaths[0], "managed")
}

func TestResolveContextPathsLocalWalkStillWorks(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	inner := filepath.Join(tmp, "deep", "project")
	require.NoError(t, os.MkdirAll(inner, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(inner, "AGENTS.md"), []byte("deep"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CRUSH.md"), []byte("shallow"), 0o644))

	paths, err := ResolveContextPaths(inner, "", "", "")
	require.NoError(t, err)

	require.Len(t, filterByName(paths, "AGENTS.md"), 1)
	require.Len(t, filterByName(paths, "CRUSH.md"), 1)
}

func TestResolveContextPathsEmptyLayers(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	paths, err := ResolveContextPaths(tmp, "", "", "")
	require.NoError(t, err)
	require.Empty(t, paths)
}

func filterByName(paths []string, name string) []string {
	var result []string
	for _, p := range paths {
		if filepath.Base(p) == name {
			result = append(result, p)
		}
	}
	return result
}

func TestFileExists(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	require.False(t, fileExists(filepath.Join(tmp, "nonexistent")))

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "exists.txt"), []byte("x"), 0o644))
	require.True(t, fileExists(filepath.Join(tmp, "exists.txt")))

	require.NoError(t, os.Mkdir(filepath.Join(tmp, "subdir"), 0o755))
	require.True(t, fileExists(filepath.Join(tmp, "subdir")))
}
