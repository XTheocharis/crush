package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

// SafeDeleteResult holds the result of a safe-delete check.
type SafeDeleteResult struct {
	CanDelete  bool
	References []string
	Warning    string
}

// ReferencesFn queries references for a symbol at a given position.
// Injectable for testing without a real LSP server.
type ReferencesFn func(ctx context.Context, uri string, position protocol.Position) ([]protocol.Location, error)

// ReplaceSymbolBody replaces the body of a brace-delimited symbol (function,
// method, struct, etc.) at the given 1-based line with newBody.
func ReplaceSymbolBody(filePath string, line int, newBody string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	if line < 1 || line > len(lines) {
		return fmt.Errorf("line %d out of range (file has %d lines)", line, len(lines))
	}

	openLine, openCol, err := findOpeningBrace(lines, line-1)
	if err != nil {
		return err
	}

	closeLine, err := findMatchingCloseBrace(lines, openLine, openCol)
	if err != nil {
		return err
	}

	bodyLines := strings.Split(newBody, "\n")
	combined := make([]string, 0, len(lines)-(closeLine-openLine-1)+len(bodyLines))
	combined = append(combined, lines[:openLine+1]...)
	combined = append(combined, bodyLines...)
	combined = append(combined, lines[closeLine:]...)

	return os.WriteFile(filePath, []byte(strings.Join(combined, "\n")), 0o644)
}

// InsertBeforeSymbol inserts text before the given 1-based line.
func InsertBeforeSymbol(filePath string, line int, text string) error {
	return insertAtLine(filePath, line, text, before)
}

// InsertAfterSymbol inserts text after the given 1-based line.
func InsertAfterSymbol(filePath string, line int, text string) error {
	return insertAtLine(filePath, line, text, after)
}

type insertPos int

const (
	before insertPos = iota
	after
)

func insertAtLine(filePath string, line int, text string, pos insertPos) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	if line < 1 || line > len(lines) {
		return fmt.Errorf("line %d out of range (file has %d lines)", line, len(lines))
	}

	insertLines := strings.Split(text, "\n")
	combined := make([]string, 0, len(lines)+len(insertLines))

	splitIdx := line - 1
	if pos == after {
		splitIdx = line
	}

	combined = append(combined, lines[:splitIdx]...)
	combined = append(combined, insertLines...)
	combined = append(combined, lines[splitIdx:]...)

	return os.WriteFile(filePath, []byte(strings.Join(combined, "\n")), 0o644)
}

// SafeDeleteSymbol checks whether a symbol can be safely deleted by querying
// references via referencesFn. External references produce a warning.
func SafeDeleteSymbol(ctx context.Context, uri string, position protocol.Position, referencesFn ReferencesFn) (*SafeDeleteResult, error) {
	locations, err := referencesFn(ctx, uri, position)
	if err != nil {
		return nil, fmt.Errorf("failed to query references: %w", err)
	}

	var externalRefs []string
	for _, loc := range locations {
		if string(loc.URI) == uri &&
			loc.Range.Start.Line == position.Line &&
			loc.Range.Start.Character == position.Character {
			continue
		}
		externalRefs = append(externalRefs, string(loc.URI))
	}

	result := &SafeDeleteResult{
		CanDelete:  len(externalRefs) == 0,
		References: externalRefs,
	}

	if len(externalRefs) > 0 {
		result.Warning = fmt.Sprintf(
			"Symbol is referenced in %d location(s): %s",
			len(externalRefs),
			strings.Join(externalRefs, ", "),
		)
	}

	return result, nil
}

func findOpeningBrace(lines []string, startLine int) (lineIdx, colIdx int, err error) {
	for i := startLine; i < len(lines); i++ {
		col := strings.Index(lines[i], "{")
		if col >= 0 {
			return i, col, nil
		}
	}
	return 0, 0, fmt.Errorf("no opening brace found starting from line %d", startLine+1)
}

func findMatchingCloseBrace(lines []string, openLine, openCol int) (int, error) {
	depth := 0
	for i := openLine; i < len(lines); i++ {
		startCol := 0
		if i == openLine {
			startCol = openCol
		}
		for j := startCol; j < len(lines[i]); j++ {
			switch lines[i][j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return i, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("no matching closing brace found for brace at line %d", openLine+1)
}
