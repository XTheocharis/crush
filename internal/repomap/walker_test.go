package repomap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

// writeFile is a test helper that creates a file with the given content,
// creating parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestWalkAllFilesRespectsGitignore(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Create .gitignore that ignores vendor/ and *.generated.go.
	writeFile(t, filepath.Join(root, ".gitignore"), "vendor/\n*.generated.go\n")

	// Create files that should be included.
	writeFile(t, filepath.Join(root, "main.go"), "package main")
	writeFile(t, filepath.Join(root, "lib", "util.go"), "package lib")

	// Create files that should be excluded by .gitignore.
	writeFile(t, filepath.Join(root, "vendor", "dep.go"), "package dep")
	writeFile(t, filepath.Join(root, "vendor", "nested", "deep.go"), "package nested")
	writeFile(t, filepath.Join(root, "schema.generated.go"), "package main")

	svc := &Service{rootDir: root}
	files := svc.walkAllFiles(context.Background())

	require.Contains(t, files, "main.go")
	require.Contains(t, files, "lib/util.go")
	require.NotContains(t, files, "vendor/dep.go")
	require.NotContains(t, files, "vendor/nested/deep.go")
	require.NotContains(t, files, "schema.generated.go")
}

func TestWalkAllFilesRespectsCrushignore(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Create .crushignore that ignores secret/.
	writeFile(t, filepath.Join(root, ".crushignore"), "secret/\n")

	// Create files that should be included.
	writeFile(t, filepath.Join(root, "app.go"), "package main")

	// Create files that should be excluded by .crushignore.
	writeFile(t, filepath.Join(root, "secret", "key.pem"), "secret-key")

	svc := &Service{rootDir: root}
	files := svc.walkAllFiles(context.Background())

	require.Contains(t, files, "app.go")
	require.NotContains(t, files, "secret/key.pem")
}

func TestWalkAllFilesSkipsGitDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, filepath.Join(root, "main.go"), "package main")
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main")
	writeFile(t, filepath.Join(root, ".git", "objects", "abc"), "blob")

	svc := &Service{rootDir: root}
	files := svc.walkAllFiles(context.Background())

	require.Contains(t, files, "main.go")
	for _, f := range files {
		require.False(t, filepath.Base(f) == ".git" || len(f) > 4 && f[:5] == ".git/",
			"file %q should not be under .git", f)
	}
}

func TestWalkAllFilesExcludeGlobs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, filepath.Join(root, "main.go"), "package main")
	writeFile(t, filepath.Join(root, "testdata", "fixture.json"), "{}")
	writeFile(t, filepath.Join(root, "testdata", "nested", "deep.txt"), "data")
	writeFile(t, filepath.Join(root, "docs", "readme.txt"), "docs")

	svc := &Service{
		rootDir: root,
		cfg: &config.RepoMapOptions{
			ExcludeGlobs: []string{"testdata/**", "docs/**"},
		},
	}
	files := svc.walkAllFiles(context.Background())

	require.Contains(t, files, "main.go")
	require.NotContains(t, files, "testdata/fixture.json")
	require.NotContains(t, files, "testdata/nested/deep.txt")
	require.NotContains(t, files, "docs/readme.txt")
}

func TestWalkAllFilesExcludeGlobsNilConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main")

	// Nil cfg should not cause a panic.
	svc := &Service{rootDir: root, cfg: nil}
	files := svc.walkAllFiles(context.Background())

	require.Contains(t, files, "main.go")
}

func TestWalkAllFilesExcludeGlobsEmptySlice(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main")

	svc := &Service{
		rootDir: root,
		cfg:     &config.RepoMapOptions{ExcludeGlobs: []string{}},
	}
	files := svc.walkAllFiles(context.Background())

	require.Contains(t, files, "main.go")
}

func TestWalkAllFilesMalformedGlobDoesNotCrash(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main")

	// A pattern with unmatched '[' is malformed for doublestar.
	svc := &Service{
		rootDir: root,
		cfg: &config.RepoMapOptions{
			ExcludeGlobs: []string{"[invalid"},
		},
	}

	// Should not panic; malformed patterns are silently skipped.
	files := svc.walkAllFiles(context.Background())
	require.Contains(t, files, "main.go")
}

func TestWalkAllFilesSortedOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "z.go"), "package z")
	writeFile(t, filepath.Join(root, "a.go"), "package a")
	writeFile(t, filepath.Join(root, "m", "b.go"), "package m")

	svc := &Service{rootDir: root}
	files := svc.walkAllFiles(context.Background())

	// Verify output is sorted.
	for i := 1; i < len(files); i++ {
		require.True(t, files[i-1] <= files[i],
			"files not sorted: %q > %q", files[i-1], files[i])
	}
}

func TestWalkAllFilesSlashSeparatedPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "sub", "dir", "file.go"), "package sub")

	svc := &Service{rootDir: root}
	files := svc.walkAllFiles(context.Background())

	require.Contains(t, files, "sub/dir/file.go")
}

func TestWalkAllFilesContextCancellation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"), "package a")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	svc := &Service{rootDir: root}
	files := svc.walkAllFiles(ctx)

	// With an immediately cancelled context the walker should stop early.
	// The exact result depends on timing, but it must not panic.
	_ = files
}

func TestWalkAllFilesSymlinksNotFollowed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "real.go"), "package main")

	// Create a circular symlink: root/link -> root.
	linkPath := filepath.Join(root, "link")
	err := os.Symlink(root, linkPath)
	if err != nil {
		t.Skip("symlinks not supported on this filesystem")
	}

	svc := &Service{rootDir: root}
	files := svc.walkAllFiles(context.Background())

	// The real file should be present.
	require.Contains(t, files, "real.go")

	// Symlink targets should not be followed (no link/real.go, etc.).
	for _, f := range files {
		require.False(t, len(f) >= 5 && f[:5] == "link/",
			"symlink was followed: %q", f)
	}
}

func TestWalkAllFilesEmptyRoot(t *testing.T) {
	t.Parallel()

	svc := &Service{rootDir: ""}
	files := svc.walkAllFiles(context.Background())
	require.Nil(t, files)
}

func TestWalkAllFilesRelativePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main")
	writeFile(t, filepath.Join(root, "pkg", "lib.go"), "package pkg")

	svc := &Service{rootDir: root}
	files := svc.walkAllFiles(context.Background())

	// All paths should be relative (no leading slash or root prefix).
	for _, f := range files {
		require.False(t, filepath.IsAbs(f),
			"path should be relative: %q", f)
	}
}
