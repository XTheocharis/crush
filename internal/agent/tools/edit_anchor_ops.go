package tools

import (
	"fmt"
	"strings"
)

// InsertBefore inserts newContent as new lines immediately before the line
// identified by anchorHash. It resolves the anchor using drift-tolerant
// matching against content and returns the modified content string.
func InsertBefore(am *AnchorMap, anchorHash uint64, content string, newContent string) (string, error) {
	lineNum, err := resolveHash(am, anchorHash, content)
	if err != nil {
		return "", fmt.Errorf("insert_before: %w", err)
	}

	lines := strings.Split(content, "\n")
	insertLines := strings.Split(newContent, "\n")

	rebuilt := make([]string, 0, len(lines)+len(insertLines))
	rebuilt = append(rebuilt, lines[:lineNum]...)
	rebuilt = append(rebuilt, insertLines...)
	rebuilt = append(rebuilt, lines[lineNum:]...)

	return strings.Join(rebuilt, "\n"), nil
}

// InsertAfter inserts newContent as new lines immediately after the line
// identified by anchorHash. It resolves the anchor using drift-tolerant
// matching against content and returns the modified content string.
func InsertAfter(am *AnchorMap, anchorHash uint64, content string, newContent string) (string, error) {
	lineNum, err := resolveHash(am, anchorHash, content)
	if err != nil {
		return "", fmt.Errorf("insert_after: %w", err)
	}

	lines := strings.Split(content, "\n")
	insertLines := strings.Split(newContent, "\n")

	afterIdx := lineNum + 1

	rebuilt := make([]string, 0, len(lines)+len(insertLines))
	rebuilt = append(rebuilt, lines[:afterIdx]...)
	rebuilt = append(rebuilt, insertLines...)
	rebuilt = append(rebuilt, lines[afterIdx:]...)

	return strings.Join(rebuilt, "\n"), nil
}

// ReplaceRange replaces all lines from the start anchor (inclusive) to the end
// anchor (inclusive) with replacement. If start and end refer to the same
// anchor, only that single line is replaced. The anchors are automatically
// ordered by resolved line number regardless of argument order.
func ReplaceRange(am *AnchorMap, startHash uint64, endHash uint64, content string, replacement string) (string, error) {
	startLine, err := resolveHash(am, startHash, content)
	if err != nil {
		return "", fmt.Errorf("replace_range start: %w", err)
	}

	endLine, err := resolveHash(am, endHash, content)
	if err != nil {
		return "", fmt.Errorf("replace_range end: %w", err)
	}

	if startLine > endLine {
		startLine, endLine = endLine, startLine
	}

	lines := strings.Split(content, "\n")
	replacementLines := strings.Split(replacement, "\n")

	rebuilt := make([]string, 0, len(lines)-(endLine-startLine+1)+len(replacementLines))
	rebuilt = append(rebuilt, lines[:startLine]...)
	rebuilt = append(rebuilt, replacementLines...)
	rebuilt = append(rebuilt, lines[endLine+1:]...)

	return strings.Join(rebuilt, "\n"), nil
}

// DeleteRange removes all lines from the start anchor (inclusive) to the end
// anchor (inclusive). The anchors are automatically ordered by resolved line
// number regardless of argument order.
func DeleteRange(am *AnchorMap, startHash uint64, endHash uint64, content string) (string, error) {
	startLine, err := resolveHash(am, startHash, content)
	if err != nil {
		return "", fmt.Errorf("delete_range start: %w", err)
	}

	endLine, err := resolveHash(am, endHash, content)
	if err != nil {
		return "", fmt.Errorf("delete_range end: %w", err)
	}

	if startLine > endLine {
		startLine, endLine = endLine, startLine
	}

	lines := strings.Split(content, "\n")

	rebuilt := make([]string, 0, len(lines)-(endLine-startLine+1))
	rebuilt = append(rebuilt, lines[:startLine]...)
	rebuilt = append(rebuilt, lines[endLine+1:]...)

	return strings.Join(rebuilt, "\n"), nil
}

// resolveHash locates the anchor for the given hash in content using the
// AnchorMap's drift-tolerant resolution. Returns the 0-indexed line number.
func resolveHash(am *AnchorMap, hash uint64, content string) (int, error) {
	for i := range am.Anchors {
		if am.Anchors[i].Hash == hash {
			return ResolveAnchor(&am.Anchors[i], content)
		}
	}
	return -1, errAnchorNotFound
}
