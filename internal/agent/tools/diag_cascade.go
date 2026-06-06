package tools

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

const defaultCascadeMaxDepth = 3

// CascadeResult tracks the outcome of a diagnostic cascade run.
type CascadeResult struct {
	// FilesChecked is the set of file paths that were checked for diagnostics.
	FilesChecked []string
	// FileDiagnostics maps file paths to the diagnostics found for that file.
	FileDiagnostics map[string][]DiagnosticInfo
	// HasWarnings indicates whether any file in the cascade had diagnostics
	// with severity >= Warning.
	HasWarnings bool
}

// DiagnosticCascade runs LSP diagnostics on an edited file and, if any
// diagnostic has severity >= Warning, cascades to files that import the
// edited file's package. The cascade is depth-limited and uses LSP
// reference resolution (best-effort) for discovering importers.
type DiagnosticCascade struct {
	lspManager  *lsp.Manager
	maxDepth    int
	parser      any
	projectRoot string
	modulePath  string
}

// NewDiagnosticCascade creates a cascade with the given LSP manager and
// depth limit. If maxDepth <= 0, defaults to 3. The parser, projectRoot,
// and modulePath enable forward import resolution when treesitter is
// available; they may be zero-valued if unavailable.
func NewDiagnosticCascade(lspManager *lsp.Manager, maxDepth int, parser any, projectRoot, modulePath string) *DiagnosticCascade {
	if maxDepth <= 0 {
		maxDepth = defaultCascadeMaxDepth
	}
	return &DiagnosticCascade{
		lspManager:  lspManager,
		maxDepth:    maxDepth,
		parser:      parser,
		projectRoot: projectRoot,
		modulePath:  modulePath,
	}
}

// RunCascade runs diagnostics on the given file and cascades to importing
// files if any diagnostic with severity >= Warning is found. The cascade
// is bounded by maxDepth. It does not block the caller on errors — errors
// are logged and the cascade continues.
func (dc *DiagnosticCascade) RunCascade(ctx context.Context, filePath string) CascadeResult {
	result := CascadeResult{
		FileDiagnostics: make(map[string][]DiagnosticInfo),
	}
	if dc.lspManager == nil || filePath == "" {
		return result
	}

	visited := make(map[string]bool)
	dc.runCascadeRecursive(ctx, filePath, 0, visited, &result)
	return result
}

func (dc *DiagnosticCascade) runCascadeRecursive(
	ctx context.Context,
	filePath string,
	depth int,
	visited map[string]bool,
	result *CascadeResult,
) {
	if depth > dc.maxDepth {
		return
	}

	// Normalise to absolute path for dedup.
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}
	if visited[absPath] {
		return
	}
	visited[absPath] = true

	select {
	case <-ctx.Done():
		return
	default:
	}

	// Collect diagnostics for this file.
	diags := collectFileDiagnostics(absPath, dc.lspManager)
	result.FilesChecked = append(result.FilesChecked, absPath)
	if len(diags) > 0 {
		result.FileDiagnostics[absPath] = diags
	}

	// Only cascade if there are diagnostics with severity >= Warning.
	hasWarningOrAbove := false
	for _, d := range diags {
		if d.Severity == SeverityError || d.Severity == SeverityWarning {
			hasWarningOrAbove = true
			result.HasWarnings = true
			break
		}
	}
	if !hasWarningOrAbove {
		return
	}

	// Find importers and recurse.
	importers := dc.findImporters(ctx, absPath)

	// Forward import resolution: resolve which symbols each importing file
	// uses from the edited file via tree-sitter analysis.
	if dc.parser != nil && dc.projectRoot != "" && len(importers) > 0 {
		dc.resolveForwardImports(ctx, importers)
	}

	for _, importer := range importers {
		dc.runCascadeRecursive(ctx, importer, depth+1, visited, result)
	}
}

// findImporters discovers files that reference the given file's symbols. It
// uses LSP textDocument/references via the manager when available. This is
// best-effort: if the LSP doesn't support it or the call fails, we return an
// empty slice.
func (dc *DiagnosticCascade) findImporters(ctx context.Context, filePath string) []string {
	if dc.lspManager == nil {
		return nil
	}

	seen := make(map[string]bool)
	var importers []string

	for lspName, client := range dc.lspManager.Clients().Seq2() {
		if !client.HandlesFile(filePath) {
			continue
		}

		// Try to find references to any symbol defined in this file.
		// We use a heuristic: query references at line 1, col 0 which often
		// resolves to the file's package declaration. If the LSP can resolve
		// it, we get back locations of files referencing this package.
		refs, err := dc.lspManager.FindReferencesForServer(
			ctx, lspName, filePath, 1, 1, false,
		)
		if err != nil {
			slog.Debug("Cascade: FindReferences failed",
				"file", filePath, "server", lspName, "error", err,
			)
			continue
		}

		for _, loc := range refs {
			path, pathErr := loc.URI.Path()
			if pathErr != nil {
				continue
			}
			// Don't include the file itself.
			if path == filePath {
				continue
			}
			if !seen[path] {
				seen[path] = true
				importers = append(importers, path)
			}
		}
	}

	return importers
}

// resolveForwardImports delegates to build-tagged implementation.
func (dc *DiagnosticCascade) resolveForwardImports(ctx context.Context, importers []string) {
	dc.forwardResolve(ctx, importers)
}

// FormatCascadeResult formats a CascadeResult into a human-readable string
// suitable for inclusion in a tool response.
func FormatCascadeResult(result CascadeResult) string {
	if len(result.FilesChecked) <= 1 {
		// Only the original file was checked — nothing to report.
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n<diagnostic_cascade>\n")
	fmt.Fprintf(&sb, "Cascade checked %d file(s)", len(result.FilesChecked))
	if result.HasWarnings {
		sb.WriteString(" (warnings found)")
	}
	sb.WriteString("\n")

	totalDiags := 0
	for _, diags := range result.FileDiagnostics {
		totalDiags += len(diags)
	}
	if totalDiags > 0 {
		for _, fp := range result.FilesChecked {
			diags, ok := result.FileDiagnostics[fp]
			if !ok || len(diags) == 0 {
				continue
			}
			for _, d := range diags {
				if d.Severity == SeverityError || d.Severity == SeverityWarning {
					fmt.Fprintf(&sb, "  %s:%d: %s: %s\n",
						shortPath(fp), d.Line+1, severityName(d.Severity), d.Message,
					)
				}
			}
		}
	} else {
		sb.WriteString("  No additional diagnostics in importing files\n")
	}

	sb.WriteString("</diagnostic_cascade>\n")
	return sb.String()
}

// runDiagnosticCascade is the convenience function called from edit/write
// tools. It runs the cascade and returns the formatted result string. If the
// cascade fails or produces no additional results, it returns an empty string.
func runDiagnosticCascade(ctx context.Context, lspManager *lsp.Manager, filePath string) string {
	if lspManager == nil || lspManager.Clients().Len() == 0 {
		return ""
	}

	cascade := NewDiagnosticCascade(lspManager, defaultCascadeMaxDepth, nil, "", "")
	result := cascade.RunCascade(ctx, filePath)
	return FormatCascadeResult(result)
}

// collectDiagnosticsForFile is a helper that collects protocol.Diagnostics
// for a single file path from all LSP clients.
func collectDiagnosticsForFile(filePath string, manager *lsp.Manager) []protocol.Diagnostic {
	if manager == nil {
		return nil
	}

	var diags []protocol.Diagnostic
	for _, client := range manager.Clients().Seq2() {
		for uri, fileDiags := range client.GetDiagnostics() {
			path, err := uri.Path()
			if err != nil {
				continue
			}
			if path == filePath {
				diags = append(diags, fileDiags...)
			}
		}
	}
	return diags
}

func shortPath(p string) string {
	parts := strings.Split(p, string(filepath.Separator))
	if len(parts) > 3 {
		return strings.Join(parts[len(parts)-3:], string(filepath.Separator))
	}
	return p
}

func severityName(s DiagnosticSeverity) string {
	switch s {
	case SeverityError:
		return "Error"
	case SeverityWarning:
		return "Warn"
	case SeverityInfo:
		return "Info"
	case SeverityHint:
		return "Hint"
	default:
		return "Unknown"
	}
}

// cascadeDiagnosticsConcurrent runs diagnostics for multiple files in
// parallel and returns a merged result. Used internally by the cascade.
func cascadeDiagnosticsConcurrent(
	ctx context.Context,
	manager *lsp.Manager,
	filePaths []string,
) map[string][]DiagnosticInfo {
	result := make(map[string][]DiagnosticInfo)
	if manager == nil {
		return result
	}

	var mu sync.Mutex

	var wg sync.WaitGroup
	for _, fp := range filePaths {
		wg.Go(func() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			diags := collectFileDiagnostics(fp, manager)
			if len(diags) > 0 {
				mu.Lock()
				result[fp] = diags
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	return result
}
