package repomap

import (
	"path/filepath"
	"sort"
	"strings"
)

// NormalizeParityMap applies deterministic normalization to rendered repo map
// text for cross-platform parity comparison. It normalizes:
//   - CRLF line endings to LF
//   - Backslash path separators to forward slashes
//   - Stage-3 line ordering (sorted lexicographically)
func NormalizeParityMap(text string) string {
	if text == "" {
		return ""
	}
	// Normalize CRLF to LF.
	text = strings.ReplaceAll(text, "\r\n", "\n")

	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	stage3 := make([]string, 0)
	other := make([]string, 0, len(lines))
	for _, line := range lines {
		// Normalize path separators.
		line = filepath.ToSlash(line)
		if strings.HasPrefix(line, "S3|") {
			stage3 = append(stage3, line)
		} else {
			other = append(other, line)
		}
	}
	sort.Strings(stage3)
	normalized := append(other, stage3...)
	return strings.Join(normalized, "\n") + "\n"
}
