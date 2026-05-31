package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/filetracker"
)

const (
	batchMaxWorkers         = 8
	batchCharsPerToken      = 4
	batchDefaultTokenBudget = 200000
)

type batchReadResult struct {
	content string
	err     error
}

func handleBatchRead(
	ctx context.Context,
	params ViewParams,
	workingDir string,
	ft filetracker.Service,
	sessionID string,
) (fantasy.ToolResponse, error) {
	resolved := make([]string, 0, len(params.FilePaths))
	for _, p := range params.FilePaths {
		resolved = append(resolved, filepath.Join(workingDir, p))
	}

	unique := dedupBatchPaths(resolved)

	readFiles, _ := ft.ListReadFiles(ctx, sessionID)
	readSet := make(map[string]struct{}, len(readFiles))
	for _, p := range readFiles {
		readSet[p] = struct{}{}
	}

	results := batchReadFiles(ctx, unique, batchDefaultTokenBudget, readSet, nil)

	var b strings.Builder
	var paths []string
	for _, absPath := range unique {
		r, ok := results[absPath]
		if !ok {
			continue
		}
		if r.err != nil {
			fmt.Fprintf(&b, "<file path=\"%s\">\nError: %s\n</file>\n\n", relPath(workingDir, absPath), r.err)
			continue
		}
		rel := relPath(workingDir, absPath)
		content := r.content
		if params.Offset > 0 || params.Limit > 0 {
			content = sliceContent(content, params.Offset, params.Limit)
		}
		if !utf8.ValidString(content) {
			fmt.Fprintf(&b, "<file path=\"%s\">\nError: content is not valid UTF-8\n</file>\n\n", rel)
			continue
		}
		fmt.Fprintf(&b, "<file path=\"%s\">\n%s\n</file>\n\n", rel, addLineNumbers(content, params.Offset+1))
		paths = append(paths, absPath)
	}

	for _, p := range paths {
		ft.RecordRead(ctx, sessionID, p)
	}

	return fantasy.NewTextResponse(b.String()), nil
}

func batchReadFiles(ctx context.Context, paths []string, tokenBudget int, readSet map[string]struct{}, fileScores map[string]float64) map[string]batchReadResult {
	ordered := make([]string, len(paths))
	copy(ordered, paths)
	sort.SliceStable(ordered, func(i, j int) bool {
		pi := filePriority(ordered[i], readSet, fileScores)
		pj := filePriority(ordered[j], readSet, fileScores)
		if pi != pj {
			return pi > pj
		}
		return ordered[i] < ordered[j]
	})

	sem := make(chan struct{}, batchMaxWorkers)

	var (
		mu      sync.Mutex
		results = make(map[string]batchReadResult, len(ordered))
		tokens  atomic.Int64
		budget  = int64(tokenBudget) * batchCharsPerToken
	)

	var wg sync.WaitGroup
	for _, p := range ordered {
		if ctx.Err() != nil {
			break
		}
		if budget > 0 && tokens.Load() >= budget {
			break
		}

		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}
			if budget > 0 && tokens.Load() >= budget {
				return
			}

			data, err := os.ReadFile(path)
			if err != nil {
				mu.Lock()
				results[path] = batchReadResult{err: err}
				mu.Unlock()
				return
			}

			content := string(data)

			if budget > 0 {
				remaining := budget - tokens.Load()
				if remaining <= 0 {
					return
				}
				if int64(len(content)) > remaining {
					content = content[:int(remaining)]
				}
			}

			tokens.Add(int64(len(content)))

			mu.Lock()
			results[path] = batchReadResult{content: content}
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	return results
}

func dedupBatchPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

func relPath(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return rel
}

func sliceContent(content string, offset, limit int) string {
	lines := strings.Split(content, "\n")
	if offset > 0 {
		if offset >= len(lines) {
			return ""
		}
		lines = lines[offset:]
	}
	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
	}
	return strings.Join(lines, "\n")
}

const (
	priorityRecentlyRead = 1000
	priorityPageRankBase = 100
)

func filePriority(path string, readSet map[string]struct{}, fileScores map[string]float64) float64 {
	var score float64
	if _, ok := readSet[path]; ok {
		score += priorityRecentlyRead
	}
	if rank, ok := fileScores[path]; ok && rank > 0 {
		score += priorityPageRankBase + rank*10
	}
	return score
}
