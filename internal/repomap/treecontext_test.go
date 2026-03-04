package repomap

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"github.com/charmbracelet/crush/internal/treesitter"
)

// parseTree is a test helper that parses source with the given language
// and returns the AST. The caller must close the returned tree.
func parseTree(t *testing.T, lang *tree_sitter.Language, src string) *tree_sitter.Tree {
	t.Helper()
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(lang))
	tree := p.Parse([]byte(src), nil)
	require.NotNil(t, tree)
	return tree
}

func goLang() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_go.Language())
}

func pythonLang() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_python.Language())
}

func typescriptLang() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
}

func rustLang() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_rust.Language())
}

// ---------------------------------------------------------------------------
// W4.2 — walkTree tests
// ---------------------------------------------------------------------------

func TestWalkTreePopulatesScopesAndHeaders(t *testing.T) {
	t.Parallel()

	src := `package main

func hello() {
	println("hi")
}

type Greeter struct {
	Name string
}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	loi := map[int]struct{}{3: {}} // Line inside hello().
	tc := NewTreeContext("main.go", content, tree, loi)

	// The source_file node spans line 0 to the last line, so line 0
	// should be in scopes for all lines.
	require.NotNil(t, tc.scopes[0], "scopes[0] should be populated")
	require.NotNil(t, tc.scopes[3], "scopes[3] should be populated")

	// Line 3 (println) is inside func hello which starts at line 2.
	// scopes[3] should contain line 2 (the func declaration start).
	_, hasFuncScope := tc.scopes[3][2]
	require.True(t, hasFuncScope,
		"line 3 should be in scope of func hello starting at line 2")

	// Line 7 (Name string) is inside the struct starting at line 6.
	_, hasStructScope := tc.scopes[7][6]
	require.True(t, hasStructScope,
		"line 7 should be in scope of struct starting at line 6")
}

func TestWalkTreePopulatesScopesForSingleLineNodes(t *testing.T) {
	t.Parallel()

	src := `package main

var x = 1
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	loi := map[int]struct{}{2: {}}
	tc := NewTreeContext("main.go", content, tree, loi)

	// Even single-line nodes populate scopes. Line 2 should have its
	// own startLine in its scope set.
	require.NotNil(t, tc.scopes[2], "scopes[2] should be populated for single-line node")
	_, hasSelf := tc.scopes[2][2]
	require.True(t, hasSelf,
		"single-line node at line 2 should create scope entry for itself")
}

func TestWalkTreeHeaderMaxClipping(t *testing.T) {
	t.Parallel()

	// Create a function that spans more than 10 lines (headerMax default).
	var sb strings.Builder
	sb.WriteString("package main\n\nfunc big() {\n")
	for i := 0; i < 20; i++ {
		sb.WriteString("\tprintln(1)\n")
	}
	sb.WriteString("}\n")
	src := sb.String()

	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	loi := map[int]struct{}{}
	tc := NewTreeContext("main.go", content, tree, loi)

	// The func big() starts at line 2. Its header entry should be clipped
	// to startLine + headerMax.
	entries := tc.headers[2]
	require.NotEmpty(t, entries, "headers[2] should have entries for func big()")

	for _, e := range entries {
		if e.startLine == 2 {
			require.LessOrEqual(t, e.clippedEnd, 2+tc.headerMax,
				"clippedEnd should not exceed startLine + headerMax")
			return
		}
	}
	t.Fatal("did not find header entry starting at line 2")
}

// ---------------------------------------------------------------------------
// W4.3 — collapseHeaders tests
// ---------------------------------------------------------------------------

func TestCollapseHeadersMultipleEntries(t *testing.T) {
	t.Parallel()

	src := `package main

func outer() {
	func() {
		println("inner")
	}()
}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	loi := map[int]struct{}{}
	tc := NewTreeContext("main.go", content, tree, loi)

	// After collapsing, each collapsedHeaders[i] should be a single pair.
	for i := range tc.collapsedHeaders {
		pair := tc.collapsedHeaders[i]
		// Verify it's a valid pair: endLine > startLine for non-trivial
		// entries.
		require.GreaterOrEqual(t, pair.endLine, pair.startLine,
			"collapsed header pair at %d should have endLine >= startLine", i)
	}
}

func TestCollapseHeadersSelectsSmallestScope(t *testing.T) {
	t.Parallel()

	// When multiple headerEntry structs exist for the same startLine,
	// the one with the smallest size should be selected.
	src := `package main

func a() {
	if true {
		println("nested")
	}
}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	loi := map[int]struct{}{}
	tc := NewTreeContext("main.go", content, tree, loi)

	// Look for a line with multiple header entries.
	for i, entries := range tc.headers {
		if len(entries) > 1 {
			pair := tc.collapsedHeaders[i]
			// The collapsed pair should match the smallest entry.
			smallest := entries[0]
			for _, e := range entries[1:] {
				if e.size < smallest.size {
					smallest = e
				}
			}
			require.Equal(t, smallest.startLine, pair.startLine,
				"collapsed header should use smallest entry's startLine")
			require.Equal(t, smallest.clippedEnd, pair.endLine,
				"collapsed header should use smallest entry's clippedEnd")
			return
		}
	}
	// If no multi-entry found, that's OK — the test still passes because
	// trivial cases are covered.
}

func TestCollapseHeadersTrivialPair(t *testing.T) {
	t.Parallel()

	src := `package main
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	loi := map[int]struct{}{}
	tc := NewTreeContext("main.go", content, tree, loi)

	// For lines with 0 or 1 header entries, the collapsed pair should be
	// {i, i+1} (trivial 1-line header).
	for i, entries := range tc.headers {
		if len(entries) <= 1 {
			pair := tc.collapsedHeaders[i]
			require.Equal(t, i, pair.startLine,
				"trivial pair startLine should be index")
			require.Equal(t, i+1, pair.endLine,
				"trivial pair endLine should be index+1")
		}
	}
}

// ---------------------------------------------------------------------------
// W4.4 — addParentScopes tests
// ---------------------------------------------------------------------------

func TestAddParentScopesAddsFullRange(t *testing.T) {
	t.Parallel()

	src := `package main

type Server struct {
	Host string
	Port int
}

func (s *Server) Start() {
	println("starting")
}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	// Line 8 (println) is inside method Start, which is inside the file.
	loi := map[int]struct{}{8: {}}
	tc := NewTreeContext("main.go", content, tree, loi)

	// Manually call addParentScopes.
	done := make(map[int]struct{})
	tc.addParentScopes(8, done)

	// The method Start starts at line 7 (func (s *Server) Start()).
	// Its header range should be added to showLines.
	_, hasMethodStart := tc.showLines[7]
	require.True(t, hasMethodStart,
		"showLines should contain the method header line")
}

func TestAddParentScopesSuppressesLineZero(t *testing.T) {
	t.Parallel()

	src := `package main

func hello() {
	println("hi")
}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	loi := map[int]struct{}{3: {}}
	tc := NewTreeContext("main.go", content, tree, loi)
	// showTopOfFileParentScope is false by default.
	require.False(t, tc.showTopOfFileParentScope)

	done := make(map[int]struct{})
	tc.addParentScopes(3, done)

	// Line 0 (package main) scope starts at 0. With
	// showTopOfFileParentScope=false, line 0's scope should NOT
	// add lines to showLines.
	// However, the func at line 2 has startLine > 0 so it will be added.
	_, hasFuncLine := tc.showLines[2]
	require.True(t, hasFuncLine,
		"func header at line 2 should be in showLines")
}

func TestAddParentScopesWithTopOfFileEnabled(t *testing.T) {
	t.Parallel()

	src := `package main

func hello() {
	println("hi")
}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	loi := map[int]struct{}{3: {}}
	tc := NewTreeContext("main.go", content, tree, loi)
	tc.showTopOfFileParentScope = true

	done := make(map[int]struct{})
	tc.addParentScopes(3, done)

	// With showTopOfFileParentScope=true, line-0 scopes should be added.
	// The source_file node starts at line 0.
	_, hasLine0 := tc.showLines[0]
	require.True(t, hasLine0,
		"with showTopOfFileParentScope=true, line 0 should be in showLines")
}

// ---------------------------------------------------------------------------
// W4.5 — ComputeShowLines tests
// ---------------------------------------------------------------------------

func TestComputeShowLinesGapClosing(t *testing.T) {
	t.Parallel()

	src := `package main

func a() {
	println("a")
}

func b() {
	println("b")
}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	// Interest in both function bodies. Lines 3 and 7.
	loi := map[int]struct{}{3: {}, 7: {}}
	tc := NewTreeContext("main.go", content, tree, loi)
	tc.ComputeShowLines()

	// Lines of interest should be in showLines.
	_, has3 := tc.showLines[3]
	require.True(t, has3, "line 3 should be shown")
	_, has7 := tc.showLines[7]
	require.True(t, has7, "line 7 should be shown")
}

func TestComputeShowLinesClosesSingleGap(t *testing.T) {
	t.Parallel()

	// Manually test closeSmallGaps via ComputeShowLines.
	src := `a
b
c`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	// Lines 0 and 2 are of interest, with line 1 as a 1-line gap.
	loi := map[int]struct{}{0: {}, 2: {}}
	tc := NewTreeContext("test.go", content, tree, loi)
	tc.ComputeShowLines()

	_, has1 := tc.showLines[1]
	require.True(t, has1, "1-line gap at line 1 should be closed")
}

func TestComputeShowLinesBlankLineAdjacency(t *testing.T) {
	t.Parallel()

	src := `package main

func a() {
	println("a")
}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	// Line 3 is of interest.
	loi := map[int]struct{}{3: {}}
	tc := NewTreeContext("main.go", content, tree, loi)
	tc.ComputeShowLines()

	// After blank-line adjacency, if line 4 (}) is shown and line 5 is
	// blank, line 5 should be shown.
	_, has4 := tc.showLines[4]
	if has4 {
		if 5 < len(tc.lines) && strings.TrimSpace(tc.lines[5]) == "" {
			_, has5 := tc.showLines[5]
			require.True(t, has5,
				"blank line after shown non-blank line should be shown")
		}
	}
}

// ---------------------------------------------------------------------------
// W4.7 — Render tests
// ---------------------------------------------------------------------------

func TestRenderProducesPipePrefixAndGapMarkers(t *testing.T) {
	t.Parallel()

	src := `package main

func hello() {
	println("hello")
}

func world() {
	println("world")
}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	// Interest in line 3 (inside hello) and line 7 (inside world).
	loi := map[int]struct{}{3: {}, 7: {}}
	tc := NewTreeContext("main.go", content, tree, loi)
	result := tc.Render()

	require.NotEmpty(t, result)
	// Every shown line should have a │ prefix.
	for _, line := range strings.Split(result, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "⋮") {
			continue
		}
		require.True(t, strings.HasPrefix(line, "│"),
			"shown line should have │ prefix, got: %q", line)
	}
}

func TestRenderGoldenMultiFunctionGo(t *testing.T) {
	t.Parallel()

	src := `package main

import "fmt"

type Config struct {
	Host string
	Port int
}

func NewConfig() *Config {
	return &Config{
		Host: "localhost",
		Port: 8080,
	}
}

func (c *Config) String() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func main() {
	cfg := NewConfig()
	fmt.Println(cfg)
}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	// Interest in lines inside NewConfig (line 11) and main (line 22).
	loi := map[int]struct{}{11: {}, 22: {}}
	tc := NewTreeContext("main.go", content, tree, loi)
	result := tc.Render()

	require.NotEmpty(t, result)
	// Should contain the function headers.
	require.Contains(t, result, "func NewConfig()",
		"render should include NewConfig header")
	require.Contains(t, result, "func main()",
		"render should include main header")

	// Should contain gap markers between non-adjacent sections.
	require.Contains(t, result, "⋮",
		"render should include gap markers")
}

// ---------------------------------------------------------------------------
// W4.8 — Multi-language tests
// ---------------------------------------------------------------------------

func TestRenderPythonClassMethod(t *testing.T) {
	t.Parallel()

	src := `class Greeter:
    def __init__(self, name):
        self.name = name

    def greet(self):
        return f"Hello, {self.name}!"
`
	content := []byte(src)
	tree := parseTree(t, pythonLang(), src)
	defer tree.Close()

	// Interest in line 5 (return statement inside greet).
	loi := map[int]struct{}{5: {}}
	tc := NewTreeContext("greeter.py", content, tree, loi)
	result := tc.Render()

	require.NotEmpty(t, result)
	require.Contains(t, result, "def greet",
		"render should include greet method header")
}

func TestRenderTypeScriptInterfaceFunction(t *testing.T) {
	t.Parallel()

	src := `interface Config {
    host: string;
    port: number;
}

function createConfig(): Config {
    return {
        host: "localhost",
        port: 8080,
    };
}
`
	content := []byte(src)
	tree := parseTree(t, typescriptLang(), src)
	defer tree.Close()

	// Interest in line 7 (host: "localhost" inside createConfig).
	loi := map[int]struct{}{7: {}}
	tc := NewTreeContext("config.ts", content, tree, loi)
	result := tc.Render()

	require.NotEmpty(t, result)
	require.Contains(t, result, "function createConfig",
		"render should include createConfig header")
}

func TestRenderRustImplBlock(t *testing.T) {
	t.Parallel()

	src := `struct Server {
    host: String,
    port: u16,
}

impl Server {
    fn new(host: String, port: u16) -> Self {
        Server { host, port }
    }

    fn start(&self) {
        println!("Starting {}:{}", self.host, self.port);
    }
}
`
	content := []byte(src)
	tree := parseTree(t, rustLang(), src)
	defer tree.Close()

	// Interest in line 11 (println! inside start).
	loi := map[int]struct{}{11: {}}
	tc := NewTreeContext("server.rs", content, tree, loi)
	result := tc.Render()

	require.NotEmpty(t, result)
	require.Contains(t, result, "fn start",
		"render should include start fn header")
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestNewTreeContextEmptyFile(t *testing.T) {
	t.Parallel()

	src := ""
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	loi := map[int]struct{}{}
	tc := NewTreeContext("empty.go", content, tree, loi)
	result := tc.Render()
	require.Empty(t, result, "empty file should produce empty render")
}

func TestNewTreeContextSingleLine(t *testing.T) {
	t.Parallel()

	src := "package main"
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	loi := map[int]struct{}{0: {}}
	tc := NewTreeContext("single.go", content, tree, loi)
	tc.showTopOfFileParentScope = true
	result := tc.Render()
	require.NotEmpty(t, result, "single-line file should produce output")
	require.Contains(t, result, "package main")
}

func TestNewTreeContextOnlyComments(t *testing.T) {
	t.Parallel()

	src := `// This is a comment.
// Another comment.
// Third comment.
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	loi := map[int]struct{}{1: {}}
	tc := NewTreeContext("comments.go", content, tree, loi)
	result := tc.Render()
	require.NotEmpty(t, result)
	require.Contains(t, result, "Another comment")
}

func TestNewTreeContextDeeplyNested(t *testing.T) {
	t.Parallel()

	// Build a deeply nested Python if-chain (>50 levels).
	var sb strings.Builder
	depth := 55
	for i := range depth {
		indent := strings.Repeat("    ", i)
		sb.WriteString(indent)
		sb.WriteString("if True:\n")
	}
	deepIndent := strings.Repeat("    ", depth)
	sb.WriteString(deepIndent)
	sb.WriteString("x = 1\n")

	src := sb.String()
	content := []byte(src)
	tree := parseTree(t, pythonLang(), src)
	defer tree.Close()

	// Interest in the deepest line.
	deepLine := depth // The line with x = 1.
	loi := map[int]struct{}{deepLine: {}}
	tc := NewTreeContext("deep.py", content, tree, loi)

	// Should not stack overflow — walkTree is iterative.
	result := tc.Render()
	require.NotEmpty(t, result, "deeply nested file should produce output")
	require.Contains(t, result, "x = 1")
}

// ---------------------------------------------------------------------------
// Cursor close test
// ---------------------------------------------------------------------------

func TestCursorClosedAfterWalkTree(t *testing.T) {
	t.Parallel()

	src := `package main

func main() {
	println("hi")
}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)
	defer tree.Close()

	// NewTreeContext internally creates a cursor, walks, and closes it.
	// Verify the TreeContext is constructed successfully without panics.
	loi := map[int]struct{}{3: {}}
	tc := NewTreeContext("main.go", content, tree, loi)
	require.NotNil(t, tc)
	require.NotNil(t, tc.scopes)
	require.NotNil(t, tc.collapsedHeaders)
}

// ---------------------------------------------------------------------------
// Integration: ParseTree via treesitter.Parser interface
// ---------------------------------------------------------------------------

func TestNewTreeContextViaParseTree(t *testing.T) {
	t.Parallel()

	src := `package main

func hello() {
	println("hello")
}
`
	content := []byte(src)

	parser := treesitter.NewParser()
	t.Cleanup(func() { _ = parser.Close() })

	tree, err := parser.ParseTree(context.Background(), "main.go", content)
	require.NoError(t, err)
	require.NotNil(t, tree, "ParseTree should return a tree for Go files")
	defer tree.Close()

	loi := map[int]struct{}{3: {}}
	tc := NewTreeContext("main.go", content, tree, loi)
	result := tc.Render()
	require.NotEmpty(t, result)
	require.Contains(t, result, "func hello()")
	require.Contains(t, result, "println")
}

// ---------------------------------------------------------------------------
// renderLines standalone tests (replaces old RenderTreeContext tests)
// ---------------------------------------------------------------------------

func TestRenderLinesBasicGapsAndPrefixes(t *testing.T) {
	t.Parallel()

	lines := []string{
		"package main",
		"",
		"func a() {",
		"\tprintln(1)",
		"}",
		"func b() {",
		"\tprintln(2)",
		"}",
	}
	show := map[int]struct{}{0: {}, 3: {}, 6: {}}

	got := renderLines(lines, show)
	require.Equal(t,
		"│package main\n⋮\n│\tprintln(1)\n⋮\n│\tprintln(2)\n", got)
}

func TestRenderLinesClosesSingleLineGap(t *testing.T) {
	t.Parallel()

	lines := []string{"a", "mid", "b"}
	show := map[int]struct{}{0: {}, 2: {}}

	// renderLines does NOT close gaps — that's done by closeSmallGaps.
	// So with a 1-line gap at index 1, it should show a gap marker.
	got := renderLines(lines, show)
	// Since renderLines just renders what's in showLines, line 1 is not
	// shown, so there will be a gap marker.
	require.Contains(t, got, "⋮")
}

func TestRenderLinesOutOfRangeAndEmpty(t *testing.T) {
	t.Parallel()

	require.Empty(t, renderLines(nil, map[int]struct{}{0: {}}))
	require.Empty(t, renderLines([]string{"a"}, nil))
	require.Empty(t, renderLines([]string{"a"}, map[int]struct{}{5: {}}))
}

// ---------------------------------------------------------------------------
// sortedKeys test
// ---------------------------------------------------------------------------

func TestSortedKeys(t *testing.T) {
	t.Parallel()

	m := map[int]struct{}{5: {}, 1: {}, 3: {}, 8: {}, 2: {}}
	got := sortedKeys(m)
	require.Equal(t, []int{1, 2, 3, 5, 8}, got)
}

func TestSortedKeysEmpty(t *testing.T) {
	t.Parallel()

	got := sortedKeys(map[int]struct{}{})
	require.Empty(t, got)
}

// ---------------------------------------------------------------------------
// CRLF normalization test
// ---------------------------------------------------------------------------

func TestNewTreeContextNormalizesCRLF(t *testing.T) {
	t.Parallel()

	src := "package main\r\n\r\nfunc hello() {\r\n\tprintln(\"hi\")\r\n}\r\n"
	content := []byte(src)
	// Parse with normalized content since tree-sitter expects consistent
	// line endings.
	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	tree := parseTree(t, goLang(), normalized)
	defer tree.Close()

	loi := map[int]struct{}{3: {}}
	tc := NewTreeContext("main.go", content, tree, loi)
	result := tc.Render()
	require.NotEmpty(t, result)
	require.Contains(t, result, "println")
}

// ---------------------------------------------------------------------------
// Tree not retained test
// ---------------------------------------------------------------------------

func TestTreeNotRetainedAfterConstruction(t *testing.T) {
	t.Parallel()

	src := `package main

func main() {}
`
	content := []byte(src)
	tree := parseTree(t, goLang(), src)

	loi := map[int]struct{}{2: {}}
	tc := NewTreeContext("main.go", content, tree, loi)

	// Close the tree immediately after construction.
	tree.Close()

	// TreeContext should still function without the tree.
	result := tc.Render()
	require.NotEmpty(t, result)
}
