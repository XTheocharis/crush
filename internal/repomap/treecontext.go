package repomap

import (
	"sort"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// headerEntry captures one AST node's scope header for a given start line.
type headerEntry struct {
	size       int // Unclipped endLine - startLine (full node span).
	startLine  int
	clippedEnd int // min(startLine + headerMax, endLine) — NOT the raw endLine.
}

// headerPair is the post-collapse single header range for a start line.
type headerPair struct {
	startLine int
	endLine   int
}

// TreeContext performs AST-driven scope-aware line selection for repo map
// rendering. It walks the tree-sitter AST to discover scopes and headers,
// then selects which lines to show based on lines of interest and their
// enclosing parent scopes.
type TreeContext struct {
	filename                 string
	lines                    []string
	scopes                   []map[int]struct{} // scopes[line] = set of scope-creating lines.
	headers                  [][]headerEntry    // headers[startLine] = scope entries (pre-collapse).
	collapsedHeaders         []headerPair       // Post-collapse single pairs.
	showLines                map[int]struct{}
	linesOfInterest          map[int]struct{}
	headerMax                int  // Default 10.
	showTopOfFileParentScope bool // Default false.
}

// NewTreeContext creates a TreeContext from file content and its parsed AST.
// The tree is consumed only during construction; the TreeContext does NOT
// retain a reference to the tree. Callers may close the tree immediately
// after this returns.
func NewTreeContext(
	filename string,
	content []byte,
	tree *tree_sitter.Tree,
	linesOfInterest map[int]struct{},
) *TreeContext {
	// Normalize line endings and split into lines.
	normalized := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(string(content))
	lines := strings.Split(normalized, "\n")

	tc := &TreeContext{
		filename:                 filename,
		lines:                    lines,
		scopes:                   make([]map[int]struct{}, len(lines)),
		headers:                  make([][]headerEntry, len(lines)),
		linesOfInterest:          linesOfInterest,
		headerMax:                10,
		showTopOfFileParentScope: false,
		showLines:                make(map[int]struct{}),
	}

	// Walk the AST to populate scopes and headers.
	cursor := tree.Walk()
	tc.walkTree(cursor)
	cursor.Close()

	// Collapse multi-entry headers into single pairs.
	tc.collapseHeaders()

	return tc
}

// walkTree performs iterative depth-first AST traversal using cursor
// navigation. This avoids Go stack overflow on pathological ASTs.
func (tc *TreeContext) walkTree(cursor *tree_sitter.TreeCursor) {
	for {
		node := cursor.Node()
		startLine := int(node.StartPosition().Row)
		endLine := int(node.EndPosition().Row)
		size := endLine - startLine

		if size > 0 {
			ce := startLine + tc.headerMax
			if ce > endLine {
				ce = endLine
			}
			tc.headers[startLine] = append(tc.headers[startLine],
				headerEntry{size, startLine, ce})
		}

		// Every line in [startLine, endLine] belongs to this scope.
		// Runs unconditionally for ALL nodes (not only multi-line).
		// Matches Aider grep_ast.py:288-289.
		for i := startLine; i <= endLine; i++ {
			if tc.scopes[i] == nil {
				tc.scopes[i] = make(map[int]struct{})
			}
			tc.scopes[i][startLine] = struct{}{}
		}

		if cursor.GotoFirstChild() {
			continue
		}
		for !cursor.GotoNextSibling() {
			if !cursor.GotoParent() {
				return
			}
		}
	}
}

// collapseHeaders collapses each headers[i] from a list of headerEntry
// tuples into a single headerPair. Matches Aider (grep_ast.py:69-83).
func (tc *TreeContext) collapseHeaders() {
	tc.collapsedHeaders = make([]headerPair, len(tc.headers))
	for i, entries := range tc.headers {
		if len(entries) > 1 {
			sort.Slice(entries, func(a, b int) bool {
				return entries[a].size < entries[b].size
			})
			h := entries[0]
			tc.collapsedHeaders[i] = headerPair{h.startLine, h.clippedEnd}
		} else {
			// 0 or 1 entries: trivial 1-line header.
			// INTENTIONAL: reproduces Aider (grep_ast.py:80-81).
			tc.collapsedHeaders[i] = headerPair{i, i + 1}
		}
	}
}

// addParentScopes adds header line ranges for all scopes enclosing the
// given line. No recursion: in repomap usage TreeContext is constructed
// with last_line=False equivalent, so no recursion is needed.
func (tc *TreeContext) addParentScopes(line int, done map[int]struct{}) {
	if line >= len(tc.scopes) || tc.scopes[line] == nil {
		return
	}
	for scopeLine := range tc.scopes[line] {
		if _, ok := done[scopeLine]; ok {
			continue
		}
		done[scopeLine] = struct{}{}
		pair := tc.collapsedHeaders[scopeLine]
		if pair.startLine > 0 || tc.showTopOfFileParentScope {
			for l := pair.startLine; l < pair.endLine; l++ {
				tc.showLines[l] = struct{}{}
			}
		}
	}
}

// ComputeShowLines orchestrates line selection: adds lines of interest,
// discovers parent scopes, and closes small gaps.
func (tc *TreeContext) ComputeShowLines() {
	// Add all linesOfInterest to showLines.
	for line := range tc.linesOfInterest {
		tc.showLines[line] = struct{}{}
	}

	// For each line of interest, add parent scopes.
	done := make(map[int]struct{})
	for line := range tc.linesOfInterest {
		tc.addParentScopes(line, done)
	}

	// Close small gaps.
	tc.closeSmallGaps()
}

// closeSmallGaps closes 1-line gaps between shown lines and shows
// blank lines adjacent to shown non-blank lines. Matches Aider
// (grep_ast.py:189-206).
func (tc *TreeContext) closeSmallGaps() {
	// Phase 1: close 1-line gaps between shown lines.
	sorted := sortedKeys(tc.showLines)
	for i := 0; i+1 < len(sorted); i++ {
		if sorted[i+1]-sorted[i] == 2 {
			tc.showLines[sorted[i]+1] = struct{}{}
		}
	}

	// Phase 2: single forward pass — show next blank line after
	// each non-blank shown line.
	sorted = sortedKeys(tc.showLines)
	for _, i := range sorted {
		if i >= len(tc.lines) {
			continue
		}
		if strings.TrimSpace(tc.lines[i]) == "" {
			continue
		}
		next := i + 1
		if next < len(tc.lines) && strings.TrimSpace(tc.lines[next]) == "" {
			tc.showLines[next] = struct{}{}
		}
	}
}

// Render computes which lines to show and returns the formatted output
// with pipe prefixes and gap markers.
func (tc *TreeContext) Render() string {
	tc.ComputeShowLines()
	return renderLines(tc.lines, tc.showLines)
}

// renderLines formats the given lines with │ prefixes for shown lines
// and ⋮ gap markers for collapsed regions.
func renderLines(lines []string, showLines map[int]struct{}) string {
	if len(lines) == 0 || len(showLines) == 0 {
		return ""
	}

	final := make([]int, 0, len(showLines))
	for idx := range showLines {
		if idx >= 0 && idx < len(lines) {
			final = append(final, idx)
		}
	}
	if len(final) == 0 {
		return ""
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

// sortedKeys returns the keys of a map[int]struct{} in ascending order.
func sortedKeys(m map[int]struct{}) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
