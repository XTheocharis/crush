package filetracker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// xrush: ListRecentReadFiles returns files read within the given time window,
// deduplicated by path, sorted by most recent first.
func (s *service) ListRecentReadFiles(ctx context.Context, since time.Duration) ([]string, error) {
	cutoff := time.Now().Add(-since).Unix()
	readFiles, err := s.q.ListRecentReadFiles(ctx, cutoff)
	if err != nil {
		return nil, fmt.Errorf("listing recent read files: %w", err)
	}

	basepath, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	seen := make(map[string]bool)
	paths := make([]string, 0, len(readFiles))
	for _, rf := range readFiles {
		abs := filepath.Join(basepath, rf.Path)
		if !seen[abs] {
			seen[abs] = true
			paths = append(paths, abs)
		}
	}
	return paths, nil
}
