package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestViewDirectoryListing(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workingDir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "a.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "subdir", "b.txt"), []byte("world"), 0o644))

	tool := newViewToolForTest(workingDir)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	resp := runViewTool(t, tool, ctx, ViewParams{
		FilePath: workingDir,
	})

	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "a.txt")
	require.Contains(t, resp.Content, "subdir/")
}

func TestViewDirectoryListing_Nonexistent(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()

	tool := newViewToolForTest(workingDir)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	resp := runViewTool(t, tool, ctx, ViewParams{
		FilePath: filepath.Join(workingDir, "no-such-dir"),
	})

	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "not found")
}

func TestViewDirectoryListing_EmptyDir(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	emptyDir := filepath.Join(workingDir, "empty")
	require.NoError(t, os.Mkdir(emptyDir, 0o755))

	tool := newViewToolForTest(workingDir)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	resp := runViewTool(t, tool, ctx, ViewParams{
		FilePath: emptyDir,
	})

	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "empty")
}

func TestViewBatchRead_MultipleFiles(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "a.txt"), []byte("alpha"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "b.txt"), []byte("bravo"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "c.txt"), []byte("charlie"), 0o644))

	tool := newViewToolForTest(workingDir)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	resp := runViewTool(t, tool, ctx, ViewParams{
		FilePaths: []string{"a.txt", "b.txt", "c.txt"},
	})

	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "alpha")
	require.Contains(t, resp.Content, "bravo")
	require.Contains(t, resp.Content, "charlie")
	require.Contains(t, resp.Content, `path="a.txt"`)
	require.Contains(t, resp.Content, `path="b.txt"`)
	require.Contains(t, resp.Content, `path="c.txt"`)
}

func TestViewBatchRead_WithOffset(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	var lines []string
	for i := range 10 {
		lines = append(lines, fmt.Sprintf("line %d", i+1))
	}
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "offset.txt"), []byte(strings.Join(lines, "\n")), 0o644))

	tool := newViewToolForTest(workingDir)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	resp := runViewTool(t, tool, ctx, ViewParams{
		FilePaths: []string{"offset.txt"},
		Offset:    5,
		Limit:     2,
	})

	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "line 6")
	require.Contains(t, resp.Content, "line 7")
	require.NotContains(t, resp.Content, "line 5")
	require.NotContains(t, resp.Content, "line 8")
}

func TestViewBatchRead_ExceedsTokenBudget(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	largeContent := strings.Repeat("x", 100*1024)
	for i := range 3 {
		require.NoError(t, os.WriteFile(filepath.Join(workingDir, fmt.Sprintf("big%d.txt", i)), []byte(largeContent), 0o644))
	}

	results := batchReadFiles(context.Background(), []string{
		filepath.Join(workingDir, "big0.txt"),
		filepath.Join(workingDir, "big1.txt"),
		filepath.Join(workingDir, "big2.txt"),
	}, 50)

	totalChars := 0
	for _, r := range results {
		totalChars += len(r.content)
	}

	require.LessOrEqual(t, totalChars, 50*batchCharsPerToken+batchMaxWorkers*100*1024)
}

func TestViewBatchRead_NonexistentFile(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "exists.txt"), []byte("hello"), 0o644))

	tool := newViewToolForTest(workingDir)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	resp := runViewTool(t, tool, ctx, ViewParams{
		FilePaths: []string{"exists.txt", "missing.txt"},
	})

	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "hello")
	require.Contains(t, resp.Content, "missing.txt")
}

func TestViewBatchRead_DuplicatePaths(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "dup.txt"), []byte("unique-content"), 0o644))

	tool := newViewToolForTest(workingDir)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	resp := runViewTool(t, tool, ctx, ViewParams{
		FilePaths: []string{"dup.txt", "dup.txt", "dup.txt"},
	})

	require.False(t, resp.IsError)
	count := strings.Count(resp.Content, "unique-content")
	require.Equal(t, 1, count, "content should appear exactly once despite 3 duplicate paths")
}
