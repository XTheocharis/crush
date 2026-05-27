package lsp

import (
	"path/filepath"
	"strings"
)

// NamePathMatcher matches file paths against glob patterns.
// It supports standard filepath.Match patterns plus a "**" recursive wildcard
// that matches any number of path segments.
type NamePathMatcher struct {
	patterns []string
}

// NewNamePathMatcher creates a matcher from the given glob patterns.
func NewNamePathMatcher(patterns []string) *NamePathMatcher {
	cleaned := make([]string, 0, len(patterns))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return &NamePathMatcher{patterns: cleaned}
}

// Match returns true if the file path matches any of the patterns.
func (m *NamePathMatcher) Match(filePath string) bool {
	if len(m.patterns) == 0 {
		return false
	}

	// Normalize path separators.
	filePath = filepath.ToSlash(filePath)

	for _, pattern := range m.patterns {
		if matchPattern(pattern, filePath) {
			return true
		}
	}
	return false
}

// matchPattern checks a single pattern against a path.
// Supports "**" for recursive matching.
func matchPattern(pattern, path string) bool {
	pattern = filepath.ToSlash(pattern)

	if strings.Contains(pattern, "**") {
		return matchDoublestar(pattern, path)
	}

	matched, err := filepath.Match(pattern, filepath.Base(path))
	if err == nil && matched {
		return true
	}

	matched, err = filepath.Match(pattern, path)
	if err == nil && matched {
		return true
	}

	return false
}

// matchDoublestar handles patterns with "**" (matches any path segments).
func matchDoublestar(pattern, path string) bool {
	segments := strings.Split(pattern, "**")
	idx := 0

	for i, segment := range segments {
		segment = strings.Trim(segment, "/")

		// Last segment can be empty (trailing **) and matches anything remaining.
		if i == len(segments)-1 {
			if segment == "" {
				return true
			}
			return strings.HasSuffix(path[idx:], "/"+segment) ||
				strings.HasSuffix(path[idx:], segment) ||
				(path[idx:] == segment)
		}

		if segment == "" {
			continue
		}

		// Find the next occurrence of this segment in the remaining path.
		searchStr := "/" + segment + "/"
		pos := strings.Index(path[idx:], searchStr)
		if pos < 0 {
			// Also try matching at the start (no leading slash for first segment).
			if strings.HasPrefix(path[idx:], segment+"/") {
				pos = 0
				idx += len(segment) + 1
				continue
			}
			// Also try matching at the end.
			if strings.HasSuffix(path[idx:], "/"+segment) || path[idx:] == segment {
				idx = len(path)
				continue
			}
			return false
		}
		idx += pos + len(searchStr)
	}

	return true
}

// Patterns returns a copy of the configured patterns.
func (m *NamePathMatcher) Patterns() []string {
	if m.patterns == nil {
		return nil
	}
	return append([]string(nil), m.patterns...)
}
