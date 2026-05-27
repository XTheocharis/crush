//go:build treesitter
// +build treesitter

package repomap

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func setupGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		require.NoError(t, cmd.Run())
	}
}

func commitFile(t *testing.T, dir, relPath, content, message string) {
	t.Helper()
	absPath := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0o755))
	require.NoError(t, os.WriteFile(absPath, []byte(content), 0o644))
	cmd := exec.Command("git", "add", relPath)
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
}

func TestGetBlameInfoBasic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	setupGitRepo(t, dir)

	commitFile(t, dir, "main.go", "package main\n", "initial")
	commitFile(t, dir, "util.go", "package util\n", "add util")

	info, err := GetBlameInfo(context.Background(), dir, []string{"main.go", "util.go"})
	require.NoError(t, err)
	require.NotNil(t, info)

	mainInfo, ok := info["main.go"]
	require.True(t, ok, "expected main.go in blame info")
	require.Equal(t, "Test User", mainInfo.Author)
	require.Equal(t, 1, mainInfo.CommitCount)
	require.False(t, mainInfo.LastCommit.IsZero())

	utilInfo, ok := info["util.go"]
	require.True(t, ok, "expected util.go in blame info")
	require.Equal(t, "Test User", utilInfo.Author)
	require.Equal(t, 1, utilInfo.CommitCount)
}

func TestGetBlameInfoEmptyPaths(t *testing.T) {
	t.Parallel()

	info, err := GetBlameInfo(context.Background(), t.TempDir(), nil)
	require.NoError(t, err)
	require.Nil(t, info)
}

func TestGetBlameInfoNotGitRepo(t *testing.T) {
	t.Parallel()

	info, err := GetBlameInfo(context.Background(), t.TempDir(), []string{"main.go"})
	require.NoError(t, err)
	require.Nil(t, info)
}

func TestGetBlameInfoCancelledContext(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	setupGitRepo(t, dir)
	commitFile(t, dir, "main.go", "package main\n", "initial")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Graceful degradation: cancelled context returns nil/nil.
	info, err := GetBlameInfo(ctx, dir, []string{"main.go"})
	require.NoError(t, err)
	require.Nil(t, info)
}

func TestParseGitLogOutputBasic(t *testing.T) {
	t.Parallel()
	now := time.Now()
	lastCommit := now.Add(-2 * time.Hour)

	input := "Test User\x00" + lastCommit.Format(time.RFC3339) + "\n" +
		"1\t0\tmain.go\n" +
		"Other User\x00" + lastCommit.Add(-24*time.Hour).Format(time.RFC3339) + "\n" +
		"3\t1\tmain.go\n"

	info, err := parseGitLogOutput([]byte(input), now)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "Test User", info.Author)
	require.Equal(t, 2, info.CommitCount)
	require.WithinDuration(t, lastCommit, info.LastCommit, time.Second)
	require.WithinDuration(t, now.Add(-2*time.Hour), now.Add(-info.Age), time.Minute)
}

func TestParseGitLogOutputEmpty(t *testing.T) {
	t.Parallel()
	info, err := parseGitLogOutput(nil, time.Now())
	require.NoError(t, err)
	require.Nil(t, info)
}

func TestParseGitLogOutputSingleCommit(t *testing.T) {
	t.Parallel()
	now := time.Now()
	commitTime := now.Add(-30 * time.Minute)

	input := "Alice\x00" + commitTime.Format(time.RFC3339) + "\n" +
		"5\t2\tservice.go\n"

	info, err := parseGitLogOutput([]byte(input), now)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "Alice", info.Author)
	require.Equal(t, 1, info.CommitCount)
	require.WithinDuration(t, now.Add(-30*time.Minute), now.Add(-info.Age), time.Minute)
}

func TestBlendBlamePersonalizationNoBlame(t *testing.T) {
	t.Parallel()
	orig := map[string]float64{"a.go": 1.0, "b.go": 0.5}
	result := BlendBlamePersonalization(orig, nil, 24*time.Hour, 0.15)
	require.Equal(t, orig, result)
}

func TestBlendBlamePersonalizationZeroBlend(t *testing.T) {
	t.Parallel()
	orig := map[string]float64{"a.go": 1.0}
	blame := map[string]*BlameInfo{"a.go": {Author: "X", CommitCount: 5, Age: 0}}
	result := BlendBlamePersonalization(orig, blame, 24*time.Hour, 0)
	require.Equal(t, orig, result)
}

func TestBlendBlamePersonalizationBlends(t *testing.T) {
	t.Parallel()
	orig := map[string]float64{"a.go": 1.0}
	blame := map[string]*BlameInfo{
		"a.go": {Author: "X", CommitCount: 10, Age: 0},
	}

	result := BlendBlamePersonalization(orig, blame, 24*time.Hour, 0.5)
	require.NotNil(t, result)

	// With Age=0: recencyWeight = 1/(1+0) = 1.0, countBoost = 1+0.1*10 = 2.0
	// blameScore = 1.0 * 2.0 = 2.0
	// result = 0.5*1.0 + 0.5*2.0 = 1.5
	require.InDelta(t, 1.5, result["a.go"], 0.001)
}

func TestBlendBlamePersonalizationOldFileDecays(t *testing.T) {
	t.Parallel()
	orig := map[string]float64{"old.go": 1.0}
	blame := map[string]*BlameInfo{
		"old.go": {Author: "X", CommitCount: 1, Age: 30 * 24 * time.Hour},
	}

	result := BlendBlamePersonalization(orig, blame, 7*24*time.Hour, 0.5)
	require.NotNil(t, result)

	// With Age=30 days, halfLife=7 days: recencyWeight = 1/(1+30/7) ≈ 0.189
	// countBoost = 1.0 (only 1 commit)
	// blameScore ≈ 0.189
	// result = 0.5*1.0 + 0.5*0.189 ≈ 0.595
	require.Less(t, result["old.go"], 0.7)
	require.Greater(t, result["old.go"], 0.5)
}

func TestBlendBlamePersonalizationNewPathAdded(t *testing.T) {
	t.Parallel()
	orig := map[string]float64{"a.go": 1.0}
	blame := map[string]*BlameInfo{
		"b.go": {Author: "Y", CommitCount: 3, Age: time.Hour},
	}

	result := BlendBlamePersonalization(orig, blame, 24*time.Hour, 0.15)
	require.NotNil(t, result)
	_, hasA := result["a.go"]
	require.True(t, hasA, "original key should be preserved")
	_, hasB := result["b.go"]
	require.True(t, hasB, "blame-only path should be added")
}

func TestNormalizePathForBlame(t *testing.T) {
	t.Parallel()
	require.Equal(t, "foo/bar.go", normalizePathForBlame("/root", "/root/foo/bar.go"))
	require.Equal(t, "baz.go", normalizePathForBlame("/root", "/root/baz.go"))
}
