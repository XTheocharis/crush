package repomap

import (
	"sort"
	"strings"
)

// RenderTreeContext renders a compact TreeContext-like text for repo-map output.
// It emits shown lines with a '│' prefix and collapsed gaps as '⋮'.
func RenderTreeContext(lines []string, showLines map[int]struct{}) string {
	if len(lines) == 0 || len(showLines) == 0 {
		return ""
	}

	indexes := make([]int, 0, len(showLines))
	for idx := range showLines {
		if idx < 0 || idx >= len(lines) {
			continue
		}
		indexes = append(indexes, idx)
	}
	if len(indexes) == 0 {
		return ""
	}
	sort.Ints(indexes)

	// Close single-line gaps.
	closed := make(map[int]struct{}, len(indexes)*2)
	for _, idx := range indexes {
		closed[idx] = struct{}{}
	}
	for i := 1; i < len(indexes); i++ {
		if indexes[i]-indexes[i-1] == 2 {
			closed[indexes[i-1]+1] = struct{}{}
		}
	}

	// Include blank-line adjacency around already shown lines.
	added := true
	for added {
		added = false
		for idx := range closed {
			if idx > 0 {
				if _, ok := closed[idx-1]; !ok && strings.TrimSpace(lines[idx-1]) == "" {
					closed[idx-1] = struct{}{}
					added = true
				}
			}
			if idx+1 < len(lines) {
				if _, ok := closed[idx+1]; !ok && strings.TrimSpace(lines[idx+1]) == "" {
					closed[idx+1] = struct{}{}
					added = true
				}
			}
		}
	}

	final := make([]int, 0, len(closed))
	for idx := range closed {
		final = append(final, idx)
	}
	sort.Ints(final)

	var b strings.Builder
	last := -1
	for _, idx := range final {
		if last >= 0 && idx-last > 1 {
			b.WriteString("⋮\n")
		}
		b.WriteString("│")
		b.WriteString(lines[idx])
		if !strings.HasSuffix(lines[idx], "\n") {
			b.WriteByte('\n')
		}
		last = idx
	}

	return b.String()
}
