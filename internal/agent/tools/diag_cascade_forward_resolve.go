//go:build treesitter

package tools

import (
	"context"
	"log/slog"

	"github.com/charmbracelet/crush/internal/treesitter"
)

func (dc *DiagnosticCascade) forwardResolve(ctx context.Context, importers []string) {
	tsParser, ok := dc.parser.(treesitter.Parser)
	if !ok {
		return
	}
	for _, importer := range importers {
		select {
		case <-ctx.Done():
			return
		default:
		}

		resolved, err := ForwardImportResolution(
			ctx, tsParser, importer, dc.projectRoot, dc.modulePath,
		)
		if err != nil {
			slog.Debug("Cascade: ForwardImportResolution failed",
				"file", importer, "error", err,
			)
			continue
		}
		if len(resolved) > 0 {
			slog.Debug("Cascade: ForwardImportResolution resolved",
				"file", importer, "imports", len(resolved),
			)
		}
	}
}
