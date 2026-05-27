package tools

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

type DiagnosticSeverity int

const (
	SeverityError DiagnosticSeverity = iota
	SeverityWarning
	SeverityInfo
	SeverityHint
)

type DiagnosticInfo struct {
	FilePath  string
	Line      uint32
	Character uint32
	Severity  DiagnosticSeverity
	Message   string
}

type diagnosticKey struct {
	FilePath  string
	Line      uint32
	Character uint32
	Message   string
}

func (d DiagnosticInfo) Key() diagnosticKey {
	return diagnosticKey{
		FilePath:  d.FilePath,
		Line:      d.Line,
		Character: d.Character,
		Message:   d.Message,
	}
}

type DiagnosticDiff struct {
	Added     []DiagnosticInfo
	Removed   []DiagnosticInfo
	Unchanged []DiagnosticInfo
}

type GateResult struct {
	Pass      bool
	Diff      DiagnosticDiff
	NewErrors []DiagnosticInfo
	Warnings  []DiagnosticInfo
	NoLSP     bool
}

func (r GateResult) Message() string {
	if r.NoLSP {
		return "no LSP servers available — gate passed (unverified)"
	}
	if !r.Pass {
		return fmt.Sprintf("gate FAILED: %d new error(s) introduced", len(r.NewErrors))
	}
	if len(r.Warnings) > 0 {
		return fmt.Sprintf("gate passed with %d new warning(s)", len(r.Warnings))
	}
	if len(r.Diff.Added) == 0 && len(r.Diff.Removed) == 0 {
		return "gate passed: no diagnostic changes"
	}
	return fmt.Sprintf("gate passed: %d added, %d removed", len(r.Diff.Added), len(r.Diff.Removed))
}

type DiagnosticGate struct {
	manager  *lsp.Manager
	baseline map[diagnosticKey]DiagnosticInfo
}

func NewDiagnosticGate(manager *lsp.Manager) *DiagnosticGate {
	return &DiagnosticGate{
		manager:  manager,
		baseline: make(map[diagnosticKey]DiagnosticInfo),
	}
}

func (g *DiagnosticGate) CaptureBaseline(ctx context.Context, filePaths []string) {
	if g.manager == nil {
		return
	}

	g.baseline = make(map[diagnosticKey]DiagnosticInfo)
	for _, fp := range filePaths {
		openInLSPs(ctx, g.manager, fp)
		waitForLSPDiagnostics(ctx, g.manager, fp, defaultDiagWait)

		for _, di := range collectFileDiagnostics(fp, g.manager) {
			g.baseline[di.Key()] = di
		}
	}
}

func (g *DiagnosticGate) Compare(ctx context.Context, filePaths []string) GateResult {
	if g.manager == nil {
		return GateResult{Pass: true, NoLSP: true}
	}

	for _, fp := range filePaths {
		notifyLSPs(ctx, g.manager, fp)
	}

	postMap := make(map[diagnosticKey]DiagnosticInfo)
	for _, fp := range filePaths {
		for _, di := range collectFileDiagnostics(fp, g.manager) {
			postMap[di.Key()] = di
		}
	}

	diff := computeDiff(g.baseline, postMap)

	result := GateResult{
		Pass: true,
		Diff: diff,
	}

	for _, di := range diff.Added {
		switch di.Severity {
		case SeverityError:
			result.NewErrors = append(result.NewErrors, di)
			result.Pass = false
		case SeverityWarning:
			result.Warnings = append(result.Warnings, di)
		}
	}

	return result
}

const defaultDiagWait = 2 * time.Second

func collectFileDiagnostics(filePath string, manager *lsp.Manager) []DiagnosticInfo {
	var infos []DiagnosticInfo

	for _, client := range manager.Clients().Seq2() {
		for uri, diags := range client.GetDiagnostics() {
			path, err := uri.Path()
			if err != nil {
				slog.Error("Failed to convert diagnostic URI to path", "uri", uri, "error", err)
				continue
			}
			if path != filePath {
				continue
			}
			for _, diag := range diags {
				infos = append(infos, protocolDiagToInfo(path, diag))
			}
		}
	}

	return infos
}

func protocolDiagToInfo(filePath string, d protocol.Diagnostic) DiagnosticInfo {
	return DiagnosticInfo{
		FilePath:  filePath,
		Line:      d.Range.Start.Line,
		Character: d.Range.Start.Character,
		Severity:  convertSeverity(d.Severity),
		Message:   d.Message,
	}
}

func convertSeverity(s protocol.DiagnosticSeverity) DiagnosticSeverity {
	switch s {
	case protocol.SeverityError:
		return SeverityError
	case protocol.SeverityWarning:
		return SeverityWarning
	case protocol.SeverityInformation:
		return SeverityInfo
	case protocol.SeverityHint:
		return SeverityHint
	default:
		return SeverityInfo
	}
}

func computeDiff(baseline, post map[diagnosticKey]DiagnosticInfo) DiagnosticDiff {
	var diff DiagnosticDiff

	for key, di := range post {
		if _, ok := baseline[key]; ok {
			diff.Unchanged = append(diff.Unchanged, di)
		} else {
			diff.Added = append(diff.Added, di)
		}
	}

	for key, di := range baseline {
		if _, ok := post[key]; !ok {
			diff.Removed = append(diff.Removed, di)
		}
	}

	return diff
}

// ImportCascadeConfig controls the depth of import cascade diagnostics.
type ImportCascadeConfig struct {
	// MaxDepth is the maximum cascade depth. Default: 1 (only direct
	// importers). Maximum allowed: 2.
	MaxDepth int
}

// CascadeDiagnostics collects diagnostics for files that import the edited
// file, one level deep by default. It uses findImporters to discover files
// that depend on the edited file's package, and getDiagnostics to check each
// importing file for issues.
//
// The findImporters callback receives a package path (directory of the source
// file) and returns file paths that import that package. This callback-based
// design keeps the function testable without a full Go module on disk.
func CascadeDiagnostics(
	ctx context.Context,
	editedFilePath string,
	getDiagnostics func(filePath string) ([]DiagnosticInfo, error),
	findImporters func(packagePath string) ([]string, error),
	cfg ImportCascadeConfig,
) (map[string][]DiagnosticInfo, error) {
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = 1
	}
	if cfg.MaxDepth > 2 {
		cfg.MaxDepth = 2
	}

	result := make(map[string][]DiagnosticInfo)

	// Use the edited file's directory as the package path identifier.
	pkgPath := filepath.Dir(editedFilePath)

	// Track visited packages to avoid cycles.
	visited := map[string]bool{pkgPath: true}
	toProcess := []string{pkgPath}

	for depth := 0; depth < cfg.MaxDepth; depth++ {
		var nextLevel []string
		for _, pkg := range toProcess {
			importers, err := findImporters(pkg)
			if err != nil {
				return nil, fmt.Errorf("cascade: finding importers for %s: %w", pkg, err)
			}
			for _, impFile := range importers {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				diags, err := getDiagnostics(impFile)
				if err != nil {
					return nil, fmt.Errorf("cascade: getting diagnostics for %s: %w", impFile, err)
				}
				if len(diags) > 0 {
					result[impFile] = diags
				}

				impPkg := filepath.Dir(impFile)
				if !visited[impPkg] {
					visited[impPkg] = true
					nextLevel = append(nextLevel, impPkg)
				}
			}
		}
		toProcess = nextLevel
	}

	return result, nil
}
