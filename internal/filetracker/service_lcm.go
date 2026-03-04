package filetracker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/crush/internal/db"
)

// ListRecentReadFiles returns paths of files read in a session since the
// given Unix timestamp. This method is not on the Service interface; callers
// use a duck-typed assertion (see recentFileReader in coordinator_opts.go).
func (s *service) ListRecentReadFiles(ctx context.Context, sessionID string, sinceUnix int64) ([]string, error) {
	readFiles, err := s.q.ListRecentSessionReadFiles(ctx, db.ListRecentSessionReadFilesParams{
		SessionID: sessionID,
		ReadAt:    sinceUnix,
	})
	if err != nil {
		return nil, fmt.Errorf("listing recent read files: %w", err)
	}

	basepath := s.workingDir
	if basepath == "" {
		var err error
		basepath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}
	}

	paths := make([]string, 0, len(readFiles))
	for _, f := range readFiles {
		paths = append(paths, filepath.Join(basepath, f.Path))
	}
	return paths, nil
}
