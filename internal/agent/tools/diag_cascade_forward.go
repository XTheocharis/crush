//go:build treesitter

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/charmbracelet/crush/internal/treesitter"
)

// ForwardImportResolution extracts import statements from a Go source file
// using tree-sitter, resolves each project-local import to its exported
// symbols, and returns a map from import path to symbol names. Stdlib and
// third-party imports are skipped.
func ForwardImportResolution(
	ctx context.Context,
	parser treesitter.Parser,
	filePath string,
	projectRoot string,
	modulePath string,
) (map[string][]string, error) {
	result := make(map[string][]string)

	if parser == nil || filePath == "" {
		return result, nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", filePath, err)
	}

	analysis, err := parser.Analyze(ctx, filePath, content)
	if err != nil {
		return nil, fmt.Errorf("analyze file %s: %w", filePath, err)
	}
	if analysis == nil || len(analysis.Imports) == 0 {
		return result, nil
	}

	for _, imp := range analysis.Imports {
		if imp.Category == treesitter.ImportCategoryStdlib {
			continue
		}

		if modulePath != "" && !strings.HasPrefix(imp.Path, modulePath+"/") && imp.Path != modulePath {
			continue
		}

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		symbols, resolveErr := resolvePackageSymbols(
			ctx, parser, projectRoot, modulePath, imp.Path,
		)
		if resolveErr != nil {
			continue
		}
		if len(symbols) > 0 {
			result[imp.Path] = symbols
		}
	}

	return result, nil
}

// resolvePackageSymbols reads Go files in the package directory corresponding
// to importPath and returns the set of exported symbol names.
func resolvePackageSymbols(
	ctx context.Context,
	parser treesitter.Parser,
	projectRoot string,
	modulePath string,
	importPath string,
) ([]string, error) {
	var relPath string
	if modulePath != "" {
		relPath = strings.TrimPrefix(importPath, modulePath)
		relPath = strings.TrimPrefix(relPath, "/")
	} else {
		relPath = importPath
	}

	pkgDir := filepath.Join(projectRoot, filepath.FromSlash(relPath))

	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", pkgDir, err)
	}

	var symbols []string
	seen := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, ".") {
			continue
		}

		select {
		case <-ctx.Done():
			return symbols, ctx.Err()
		default:
		}

		goFilePath := filepath.Join(pkgDir, name)
		goContent, readErr := os.ReadFile(goFilePath)
		if readErr != nil {
			continue
		}

		goAnalysis, analyzeErr := parser.Analyze(ctx, goFilePath, goContent)
		if analyzeErr != nil || goAnalysis == nil {
			continue
		}

		for _, sym := range goAnalysis.Symbols {
			if isGoExported(sym.Name) && !seen[sym.Name] {
				seen[sym.Name] = true
				symbols = append(symbols, sym.Name)
			}
		}
	}

	return symbols, nil
}

// isGoExported reports whether a Go identifier is exported.
func isGoExported(name string) bool {
	if name == "" {
		return false
	}
	r := rune(name[0])
	return unicode.IsUpper(r) && unicode.IsLetter(r)
}
