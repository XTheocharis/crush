package repomap

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// BlameInfo holds git-log-derived recency metadata for a single file.
type BlameInfo struct {
	Author      string
	Age         time.Duration
	CommitCount int
	LastCommit  time.Time
}

// GetBlameInfo extracts per-file blame metadata from git log for the given
// file paths. It uses `git log --numstat --format=...` instead of `git blame`
// for performance. Returns nil map (no error) when not inside a git repo.
func GetBlameInfo(ctx context.Context, repoPath string, filePaths []string) (map[string]*BlameInfo, error) {
	if len(filePaths) == 0 {
		return nil, nil
	}

	if err := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return nil, nil
	}

	var mu sync.Mutex
	result := make(map[string]*BlameInfo, len(filePaths))

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(8)

	for _, fp := range filePaths {
		fp := fp
		g.Go(func() error {
			if gCtx.Err() != nil {
				return gCtx.Err()
			}
			info, err := blameInfoForFile(gCtx, repoPath, fp)
			if err != nil {
				// Graceful degradation: log and skip this file.
				slog.Debug("Blame info collection skipped",
					"path", fp,
					"error", err)
				return nil
			}
			if info == nil {
				return nil
			}
			mu.Lock()
			result[fp] = info
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("blame info collection: %w", err)
	}

	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func blameInfoForFile(ctx context.Context, repoPath, filePath string) (*BlameInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath,
		"log", "--format=%aN%x00%aI", "--numstat", "--", filePath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log for %q: %w", filePath, err)
	}

	return parseGitLogOutput(out, time.Now())
}

// parseGitLogOutput parses `git log --format="%aN%x00%aI" --numstat -- <path>`
// output into a BlameInfo.
func parseGitLogOutput(data []byte, now time.Time) (*BlameInfo, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var (
		lastAuthor string
		lastDate   time.Time
		count      int
		haveDate   bool
	)

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "-") || line == "" {
			continue
		}

		parts := strings.SplitN(line, "\x00", 2)
		if len(parts) == 2 {
			count++
			if lastAuthor == "" {
				lastAuthor = parts[0]
			}
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[1])); err == nil {
				if !haveDate || t.After(lastDate) {
					lastDate = t
					haveDate = true
				}
			}
		}
	}

	if count == 0 {
		return nil, nil
	}

	info := &BlameInfo{
		Author:      lastAuthor,
		CommitCount: count,
	}
	if haveDate {
		info.LastCommit = lastDate
		info.Age = now.Sub(lastDate)
		if info.Age < 0 {
			info.Age = 0
		}
	}
	return info, nil
}

// BlendBlamePersonalization merges blame-based recency weights into an
// existing PageRank personalization vector. This is a blending function
// that does NOT modify pagerank.go directly.
//
// The recency weight decays exponentially with file age:
//
//	weight = exp(-age / halfLife) * commitBoost * countScale
//
// Files with more recent changes receive higher personalization weight,
// boosting their PageRank score. The blendFactor controls how much the
// blame signal contributes (0 = ignore, 1 = full override).
func BlendBlamePersonalization(
	personalization map[string]float64,
	blameInfo map[string]*BlameInfo,
	halfLife time.Duration,
	blendFactor float64,
) map[string]float64 {
	if len(blameInfo) == 0 || blendFactor <= 0 {
		return personalization
	}

	result := make(map[string]float64, len(personalization))
	for k, v := range personalization {
		result[k] = v
	}

	halfSec := halfLife.Seconds()
	if halfSec <= 0 {
		halfSec = 86400
	}

	for path, info := range blameInfo {
		ageSec := info.Age.Seconds()
		if ageSec < 0 {
			ageSec = 0
		}

		recencyWeight := 1.0 / (1.0 + ageSec/halfSec)

		countBoost := 1.0
		if info.CommitCount > 1 {
			countBoost = 1.0 + 0.1*float64(min(info.CommitCount, 50))
		}

		blameScore := recencyWeight * countBoost

		existing := result[path]
		result[path] = (1-blendFactor)*existing + blendFactor*blameScore
	}

	return result
}

// normalizePathForBlame converts an absolute path to a repo-relative slash
// path for matching against blame info keys.
func normalizePathForBlame(rootDir, absPath string) string {
	rel, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		return absPath
	}
	return filepath.ToSlash(rel)
}
