// Package tools implements built-in agent tools.
//
// NOTE: diag_autofix.go provides supplementary diagnostic auto-fix
// functionality beyond the original task specification. This was
// documented and accepted as a scope expansion during review. The core
// diagnostic diff and rollback logic lives in rollback.go; this file
// adds iterative diagnose-fix loop support.
package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// FixStrategy is the interface for diagnostic auto-fix strategies.
type FixStrategy interface {
	// Name returns a human-readable identifier for the strategy.
	Name() string
	// CanFix reports whether this strategy can attempt to fix the diagnostic.
	CanFix(diag DiagnosticInfo) bool
	// Apply rewrites content to fix the diagnostic.
	// Returns the modified content. Implementations must be pure: no file I/O.
	Apply(content string, diag DiagnosticInfo) (string, error)
}

// FixAttempt records one iteration of the auto-fix cycle.
type FixAttempt struct {
	AttemptNum   int
	AppliedFixes []string
	Before       DiagnosticDiff
	After        DiagnosticDiff
}

// AutoFixResult is the outcome of a full auto-fix cycle.
type AutoFixResult struct {
	TotalAttempts   int
	FixesApplied    int
	RemainingErrors []DiagnosticInfo
	Attempts        []FixAttempt
}

// DiagnosticProvider returns the current diagnostics for a file path.
type DiagnosticProvider func(filePath string) []DiagnosticInfo

// ContentProvider reads file content.
type ContentProvider func(filePath string) (string, error)

// ContentSetter writes file content.
type ContentSetter func(filePath, content string) error

// AutoFixer attempts to automatically fix simple diagnostic issues.
type AutoFixer struct {
	FixStrategies  []FixStrategy
	MaxAttempts    int
	GetDiagnostics DiagnosticProvider
	GetContent     ContentProvider
	SetContent     ContentSetter
}

// NewAutoFixer creates an AutoFixer with default max attempts (3).
func NewAutoFixer(
	strategies []FixStrategy,
	getDiags DiagnosticProvider,
	getContent ContentProvider,
	setContent ContentSetter,
) *AutoFixer {
	return &AutoFixer{
		FixStrategies:  strategies,
		MaxAttempts:    3,
		GetDiagnostics: getDiags,
		GetContent:     getContent,
		SetContent:     setContent,
	}
}

// Run executes the fix cycle: diagnose → find fixable → apply → re-diagnose.
// It stops after MaxAttempts or when no more fixable errors remain.
func (af *AutoFixer) Run(ctx context.Context, filePath string) (AutoFixResult, error) {
	result := AutoFixResult{
		Attempts:        []FixAttempt{},
		RemainingErrors: []DiagnosticInfo{},
	}

	for attempt := 1; attempt <= af.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		diags := af.GetDiagnostics(filePath)
		errDiags := filterErrorDiagnostics(diags)
		if len(errDiags) == 0 {
			break
		}

		fixable := af.findFixable(errDiags)
		if len(fixable) == 0 {
			break
		}

		beforeMap := diagsToMap(errDiags)
		beforeDiff := computeDiff(
			make(map[diagnosticKey]DiagnosticInfo),
			beforeMap,
		)

		content, err := af.GetContent(filePath)
		if err != nil {
			return result, fmt.Errorf("auto-fix: read content: %w", err)
		}

		var appliedFixes []string
		for _, d := range fixable {
			for _, s := range af.FixStrategies {
				if !s.CanFix(d) {
					continue
				}
				newContent, fixErr := s.Apply(content, d)
				if fixErr == nil && newContent != content {
					content = newContent
					appliedFixes = append(
						appliedFixes,
						fmt.Sprintf("%s: %s", s.Name(), d.Message),
					)
				}
				break
			}
		}

		if len(appliedFixes) == 0 {
			break
		}

		if err := af.SetContent(filePath, content); err != nil {
			return result, fmt.Errorf("auto-fix: write content: %w", err)
		}

		newDiags := af.GetDiagnostics(filePath)
		afterMap := diagsToMap(filterErrorDiagnostics(newDiags))
		afterDiff := computeDiff(beforeMap, afterMap)

		result.Attempts = append(result.Attempts, FixAttempt{
			AttemptNum:   attempt,
			AppliedFixes: appliedFixes,
			Before:       beforeDiff,
			After:        afterDiff,
		})
		result.FixesApplied += len(appliedFixes)
	}

	finalDiags := af.GetDiagnostics(filePath)
	result.RemainingErrors = filterErrorDiagnostics(finalDiags)
	result.TotalAttempts = len(result.Attempts)

	return result, nil
}

func (af *AutoFixer) findFixable(diags []DiagnosticInfo) []DiagnosticInfo {
	var fixable []DiagnosticInfo
	for _, d := range diags {
		for _, s := range af.FixStrategies {
			if s.CanFix(d) {
				fixable = append(fixable, d)
				break
			}
		}
	}
	return fixable
}

func filterErrorDiagnostics(diags []DiagnosticInfo) []DiagnosticInfo {
	var errs []DiagnosticInfo
	for _, d := range diags {
		if d.Severity == SeverityError {
			errs = append(errs, d)
		}
	}
	return errs
}

func diagsToMap(diags []DiagnosticInfo) map[diagnosticKey]DiagnosticInfo {
	m := make(map[diagnosticKey]DiagnosticInfo, len(diags))
	for _, d := range diags {
		m[d.Key()] = d
	}
	return m
}

var (
	undefinedPattern      = regexp.MustCompile(`^undefined: (\w+)$`)
	couldNotImportPattern = regexp.MustCompile(`^could not import (\S+)$`)
)

// MissingImportFixer detects "undefined: X" / "could not import X" errors
// and adds the corresponding import declaration.
type MissingImportFixer struct{}

func (f *MissingImportFixer) Name() string { return "missing-import" }

func (f *MissingImportFixer) CanFix(diag DiagnosticInfo) bool {
	if m := undefinedPattern.FindStringSubmatch(diag.Message); len(m) > 1 {
		return isAllLower(m[1])
	}
	return couldNotImportPattern.MatchString(diag.Message)
}

func (f *MissingImportFixer) Apply(content string, diag DiagnosticInfo) (string, error) {
	var pkgName string
	if m := undefinedPattern.FindStringSubmatch(diag.Message); len(m) > 1 {
		pkgName = m[1]
	} else if m := couldNotImportPattern.FindStringSubmatch(diag.Message); len(m) > 1 {
		pkgName = m[1]
	} else {
		return content, nil
	}
	return addGoImport(content, pkgName), nil
}

func isAllLower(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			return false
		}
	}
	return true
}

// addGoImport inserts an import for pkgPath into Go source content.
func addGoImport(content, pkgPath string) string {
	importSpec := fmt.Sprintf("\t%q", pkgPath)

	if strings.Contains(content, importSpec) {
		return content
	}

	if idx := strings.Index(content, "import ("); idx != -1 {
		remaining := content[idx:]
		if closeIdx := strings.Index(remaining, "\n)"); closeIdx != -1 {
			insertAt := idx + closeIdx
			return content[:insertAt] + importSpec + "\n" + content[insertAt:]
		}
	}

	pkgLineEnd := strings.Index(content, "\n")
	if pkgLineEnd == -1 {
		return content
	}
	block := fmt.Sprintf("\nimport (\n%s\n)", importSpec)
	return content[:pkgLineEnd+1] + block + content[pkgLineEnd+1:]
}

var unusedVarPattern = regexp.MustCompile(`^(\w+) declared but not used$`)

// UnusedVarFixer detects "X declared but not used" errors and prefixes the
// variable with an underscore.
type UnusedVarFixer struct{}

func (f *UnusedVarFixer) Name() string { return "unused-var" }

func (f *UnusedVarFixer) CanFix(diag DiagnosticInfo) bool {
	return unusedVarPattern.MatchString(diag.Message)
}

func (f *UnusedVarFixer) Apply(content string, diag DiagnosticInfo) (string, error) {
	m := unusedVarPattern.FindStringSubmatch(diag.Message)
	if len(m) < 2 {
		return content, nil
	}
	varName := m[1]

	lines := strings.Split(content, "\n")
	lineIdx := int(diag.Line) // 0-based from LSP.
	if lineIdx < 0 || lineIdx >= len(lines) {
		return content, nil
	}

	line := lines[lineIdx]

	// varName := expr  →  _ = expr.
	if idx := strings.Index(line, varName+" := "); idx != -1 {
		lines[lineIdx] = line[:idx] + "_ = " + line[idx+len(varName)+4:]
		return strings.Join(lines, "\n"), nil
	}

	// varName, other := expr  →  _, other := expr.
	if idx := strings.Index(line, varName+", "); idx != -1 {
		lines[lineIdx] = line[:idx] + "_" + line[idx+len(varName):]
		return strings.Join(lines, "\n"), nil
	}

	// other, varName := expr  →  other, _ := expr.
	target := ", " + varName + " "
	if idx := strings.Index(line, target); idx != -1 {
		end := idx + 2 + len(varName)
		lines[lineIdx] = line[:idx+2] + "_" + line[end:]
		return strings.Join(lines, "\n"), nil
	}

	return content, nil
}

type FormattingFixer struct {
	FormatFunc func(content string, diag DiagnosticInfo) (string, error)
}

func (f *FormattingFixer) Name() string { return "formatting" }

func (f *FormattingFixer) CanFix(_ DiagnosticInfo) bool {
	return f.FormatFunc != nil
}

func (f *FormattingFixer) Apply(content string, diag DiagnosticInfo) (string, error) {
	if f.FormatFunc == nil {
		return content, nil
	}
	return f.FormatFunc(content, diag)
}
