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

func TestFilePriority_RecentlyReadBeatsUnread(t *testing.T) {
	t.Parallel()

	path := "/tmp/recent.go"
	readSet := map[string]struct{}{path: {}}
	got := filePriority(path, readSet, nil)
	gotOther := filePriority("/tmp/other.go", readSet, nil)
	require.Greater(t, got, gotOther, "recently read file should have higher priority")
}

func TestFilePriority_PageRankBeatsNoRank(t *testing.T) {
	t.Parallel()

	fileScores := map[string]float64{"/tmp/important.go": 0.5}
	got := filePriority("/tmp/important.go", nil, fileScores)
	gotOther := filePriority("/tmp/unimportant.go", nil, fileScores)
	require.Greater(t, got, gotOther, "file with PageRank should have higher priority")
}

func TestFilePriority_RecentlyReadBeatsPageRank(t *testing.T) {
	t.Parallel()

	readSet := map[string]struct{}{"/tmp/recent.go": {}}
	fileScores := map[string]float64{"/tmp/ranked.go": 1.0}
	gotRead := filePriority("/tmp/recent.go", readSet, fileScores)
	gotRanked := filePriority("/tmp/ranked.go", readSet, fileScores)
	require.Greater(t, gotRead, gotRanked, "recently read should beat PageRank")
}

func TestFilePriority_AlphabeticalTiebreaker(t *testing.T) {
	t.Parallel()

	p1 := filePriority("/tmp/a.go", nil, nil)
	p2 := filePriority("/tmp/b.go", nil, nil)
	require.Equal(t, p1, p2, "files with no signals should have equal priority")
}

func TestBatchReadFiles_PriorityOrdering(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "alpha.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "bravo.txt"), []byte("b"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "charlie.txt"), []byte("c"), 0o644))

	readSet := map[string]struct{}{
		filepath.Join(workingDir, "charlie.txt"): {},
	}

	paths := []string{
		filepath.Join(workingDir, "alpha.txt"),
		filepath.Join(workingDir, "charlie.txt"),
		filepath.Join(workingDir, "bravo.txt"),
	}

	results := batchReadFiles(context.Background(), paths, batchDefaultTokenBudget, readSet, nil)
	require.Len(t, results, 3)

	require.Contains(t, results[filepath.Join(workingDir, "charlie.txt")].content, "c")
	require.Contains(t, results[filepath.Join(workingDir, "alpha.txt")].content, "a")
	require.Contains(t, results[filepath.Join(workingDir, "bravo.txt")].content, "b")
}

func TestBatchReadFiles_PageRankOrdering(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "low.go"), []byte("low"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "high.go"), []byte("high"), 0o644))

	fileScores := map[string]float64{
		filepath.Join(workingDir, "high.go"): 0.9,
		filepath.Join(workingDir, "low.go"):  0.1,
	}

	paths := []string{
		filepath.Join(workingDir, "low.go"),
		filepath.Join(workingDir, "high.go"),
	}

	results := batchReadFiles(context.Background(), paths, batchDefaultTokenBudget, nil, fileScores)
	require.Len(t, results, 2)
	require.Contains(t, results[filepath.Join(workingDir, "high.go")].content, "high")
	require.Contains(t, results[filepath.Join(workingDir, "low.go")].content, "low")
}

func TestBatchReadFiles_NilSignalsFallsBackToAlphabetical(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "c.txt"), []byte("c"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "b.txt"), []byte("b"), 0o644))

	paths := []string{
		filepath.Join(workingDir, "c.txt"),
		filepath.Join(workingDir, "a.txt"),
		filepath.Join(workingDir, "b.txt"),
	}

	results := batchReadFiles(context.Background(), paths, batchDefaultTokenBudget, nil, nil)
	require.Len(t, results, 3)
	require.Contains(t, results[filepath.Join(workingDir, "a.txt")].content, "a")
	require.Contains(t, results[filepath.Join(workingDir, "b.txt")].content, "b")
	require.Contains(t, results[filepath.Join(workingDir, "c.txt")].content, "c")
}

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
	}, 50, nil, nil)

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

type mockFileScoreProvider struct {
	scores map[string]float64
}

func (m *mockFileScoreProvider) FileScores(_ context.Context, _ string) map[string]float64 {
	return m.scores
}

func TestHandleBatchRead_WithFileScores(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "low.go"), []byte("low content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "high.go"), []byte("high content"), 0o644))

	ft := mockFileTracker{}
	scores := &mockFileScoreProvider{
		scores: map[string]float64{
			"high.go": 0.9,
			"low.go":  0.1,
		},
	}

	ctx := context.Background()
	resp, err := handleBatchRead(ctx, ViewParams{
		FilePaths: []string{"low.go", "high.go"},
	}, workingDir, ft, "test-session", scores)

	require.NoError(t, err)
	require.Contains(t, resp.Content, "high content")
	require.Contains(t, resp.Content, "low content")
}

func TestHandleBatchRead_NilScoreProvider(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "a.txt"), []byte("alpha"), 0o644))

	ft := mockFileTracker{}

	ctx := context.Background()
	resp, err := handleBatchRead(ctx, ViewParams{
		FilePaths: []string{"a.txt"},
	}, workingDir, ft, "test-session", nil)

	require.NoError(t, err)
	require.Contains(t, resp.Content, "alpha")
}

func TestHandleBatchRead_ScoreProviderReturnsNil(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "a.txt"), []byte("alpha"), 0o644))

	ft := mockFileTracker{}
	scores := &mockFileScoreProvider{scores: nil}

	ctx := context.Background()
	resp, err := handleBatchRead(ctx, ViewParams{
		FilePaths: []string{"a.txt"},
	}, workingDir, ft, "test-session", scores)

	require.NoError(t, err)
	require.Contains(t, resp.Content, "alpha")
}
