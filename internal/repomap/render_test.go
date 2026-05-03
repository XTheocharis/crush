package repomap

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func TestRenderBudgetGoldenBasic(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "src/a.go", Name: "A", Kind: "def", Line: 10},
		{RelPath: "src/b.go", Name: "B", Kind: "def", Line: 20},
		{RelPath: "src/c.go", Name: "C", Kind: "def", Line: 30},
	}
	defs := []RankedDefinition{
		{File: "src/a.go", Ident: "A", Rank: 0.9},
		{File: "src/b.go", Ident: "B", Rank: 0.8},
	}
	special := BuildSpecialPrelude([]string{"README.md", "go.mod"}, []string{"src/a.go", "src/b.go"}, false)
	entries := AssembleStageEntries(
		special,
		defs,
		[]string{"src/a.go", "src/c.go"},
		[]string{"src/c.go", "README.md", "docs/notes.md"},
		nil,
		false,
	)

	res, err := FitToBudget(context.Background(), entries, BudgetProfile{
		ParityMode:   false,
		TokenBudget:  8,
		LanguageHint: "default",
	}, nil)
	require.NoError(t, err)

	rankedFiles := AggregateRankedFiles(defs, tags)
	require.NotEmpty(t, rankedFiles)

	// Render via stage-entry renderer to produce deterministic budget trace.
	got := renderStageEntries(res.Entries)

	wantPath := filepath.Join("testdata", "render_budget", "basic_enhancement.golden")
	assertGolden(t, wantPath, got)
}

func TestRenderBudgetGoldenParityCounters(t *testing.T) {
	t.Parallel()

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "src/a.go", Ident: "A"},
	}

	res, err := FitToBudget(context.Background(), entries, BudgetProfile{
		ParityMode:   true,
		TokenBudget:  12,
		Model:        "stub",
		LanguageHint: "default",
	}, fakeCounter{out: 12})
	require.NoError(t, err)
	require.True(t, res.ComparatorAccepted)
	require.Equal(t, 12, res.SafetyTokens)

	got := renderStageEntries(res.Entries)
	wantPath := filepath.Join("testdata", "render_budget", "basic_parity.golden")
	assertGolden(t, wantPath, got)
}

func assertGolden(t *testing.T, path string, got string) {
	t.Helper()
	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}

// ---------- RenderRepoMap tests ----------

// renderTestParser is a mock parser for RenderRepoMap tests.
// It delegates to a real parser for supported languages and allows
// controlling which languages are supported.
type renderTestParser struct {
	real       treesitter.Parser
	supported  map[string]bool
	parseError error
}

func newRenderTestParser(t *testing.T) *renderTestParser {
	t.Helper()
	p := treesitter.NewParser()
	t.Cleanup(func() { _ = p.Close() })
	return &renderTestParser{
		real:      p,
		supported: make(map[string]bool),
	}
}

func (p *renderTestParser) Analyze(ctx context.Context, path string, content []byte) (*treesitter.FileAnalysis, error) {
	return p.real.Analyze(ctx, path, content)
}

func (p *renderTestParser) ParseTree(ctx context.Context, path string, content []byte) (*tree_sitter.Tree, error) {
	if p.parseError != nil {
		return nil, p.parseError
	}
	return p.real.ParseTree(ctx, path, content)
}

func (p *renderTestParser) Languages() []string { return p.real.Languages() }

func (p *renderTestParser) SupportsLanguage(lang string) bool {
	if v, ok := p.supported[lang]; ok {
		return v
	}
	return p.real.SupportsLanguage(lang)
}

func (p *renderTestParser) HasTags(lang string) bool { return p.real.HasTags(lang) }
func (p *renderTestParser) Close() error             { return nil }

// unsupportedLangParser always reports languages as unsupported.
type unsupportedLangParser struct{}

func (p *unsupportedLangParser) Analyze(_ context.Context, _ string, _ []byte) (*treesitter.FileAnalysis, error) {
	return &treesitter.FileAnalysis{}, nil
}

func (p *unsupportedLangParser) ParseTree(_ context.Context, _ string, _ []byte) (*tree_sitter.Tree, error) {
	return nil, fmt.Errorf("unsupported")
}

func (p *unsupportedLangParser) Languages() []string            { return nil }
func (p *unsupportedLangParser) SupportsLanguage(_ string) bool { return false }
func (p *unsupportedLangParser) HasTags(_ string) bool          { return false }
func (p *unsupportedLangParser) Close() error                   { return nil }

func TestBuildLinesOfInterestConverts1IndexedTo0Indexed(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "a.go", Name: "Foo", Kind: "def", Line: 5},
	}
	entries := []StageEntry{
		{Stage: stageRankedDefs, File: "a.go", Ident: "Foo"},
	}

	loi := buildLinesOfInterest(entries, tags)
	_, has4 := loi[4]
	_, has5 := loi[5]
	require.True(t, has4, "Tag.Line 5 should map to 0-indexed 4")
	require.False(t, has5, "Tag.Line 5 should NOT remain as 5 (1-indexed)")
}

func TestBuildLinesOfInterestDuplicateMethodNames(t *testing.T) {
	t.Parallel()

	// Two different types with the same method name "String".
	tags := []treesitter.Tag{
		{RelPath: "a.go", Name: "String", Kind: "def", Line: 10},
		{RelPath: "a.go", Name: "String", Kind: "def", Line: 50},
	}
	entries := []StageEntry{
		{Stage: stageRankedDefs, File: "a.go", Ident: "String"},
	}

	loi := buildLinesOfInterest(entries, tags)
	_, has9 := loi[9]
	_, has49 := loi[49]
	require.True(t, has9, "First String() at line 10 should produce LOI 9")
	require.True(t, has49, "Second String() at line 50 should produce LOI 49")
	require.Len(t, loi, 2)
}

func TestBuildLinesOfInterestSkipsRefs(t *testing.T) {
	t.Parallel()

	tags := []treesitter.Tag{
		{RelPath: "a.go", Name: "Foo", Kind: "ref", Line: 3},
		{RelPath: "a.go", Name: "Bar", Kind: "def", Line: 7},
	}
	entries := []StageEntry{
		{Stage: stageRankedDefs, File: "a.go", Ident: "Foo"},
		{Stage: stageRankedDefs, File: "a.go", Ident: "Bar"},
	}

	loi := buildLinesOfInterest(entries, tags)
	// Only Bar's def should be in LOI, not Foo's ref.
	require.Len(t, loi, 1)
	_, has6 := loi[6]
	require.True(t, has6, "Bar def at line 7 should produce LOI 6")
}

func TestRenderRepoMapStage0BareFilename(t *testing.T) {
	t.Parallel()

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageSpecialPrelude, File: "go.mod"},
	}
	tags := map[string][]treesitter.Tag{}

	got, err := RenderRepoMap(context.Background(), entries, tags, nil, "")
	require.NoError(t, err)
	require.Equal(t, "README.md\ngo.mod\n", got)
}

func TestRenderRepoMapStage2And3BareFilenames(t *testing.T) {
	t.Parallel()

	entries := []StageEntry{
		{Stage: stageGraphNodes, File: "src/graph.go"},
		{Stage: stageRemainingFiles, File: "docs/notes.md"},
	}
	tags := map[string][]treesitter.Tag{}

	got, err := RenderRepoMap(context.Background(), entries, tags, nil, "")
	require.NoError(t, err)
	require.Equal(t, "src/graph.go\ndocs/notes.md\n", got)
}

func TestRenderRepoMapStage1WithTreeContext(t *testing.T) {
	t.Parallel()

	// Create a temp directory with a Go source file.
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	goSource := `package main

func Hello() string {
	return "hello"
}

func World() string {
	return "world"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(goSource), 0o644))

	parser := newRenderTestParser(t)
	entries := []StageEntry{
		{Stage: stageRankedDefs, File: "src/main.go", Ident: "Hello"},
	}
	tags := map[string][]treesitter.Tag{
		"src/main.go": {
			{RelPath: "src/main.go", Name: "Hello", Kind: "def", Line: 3},
		},
	}

	got, err := RenderRepoMap(context.Background(), entries, tags, parser, tmpDir)
	require.NoError(t, err)
	// Should have filename: header followed by │-prefixed lines.
	require.Contains(t, got, "src/main.go:\n")
	require.Contains(t, got, "│")
}

func TestRenderRepoMapMixedStages(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	goSource := `package main

func Foo() {}
`
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "app.go"), []byte(goSource), 0o644))

	parser := newRenderTestParser(t)
	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
		{Stage: stageRankedDefs, File: "src/app.go", Ident: "Foo"},
		{Stage: stageGraphNodes, File: "lib/util.go"},
		{Stage: stageRemainingFiles, File: "docs/api.md"},
	}
	tags := map[string][]treesitter.Tag{
		"src/app.go": {
			{RelPath: "src/app.go", Name: "Foo", Kind: "def", Line: 3},
		},
	}

	got, err := RenderRepoMap(context.Background(), entries, tags, parser, tmpDir)
	require.NoError(t, err)

	// Stage 0: bare filename.
	require.Contains(t, got, "README.md\n")
	// Stage 1: filename with colon.
	require.Contains(t, got, "src/app.go:\n")
	// Stage 2: bare filename.
	require.Contains(t, got, "lib/util.go\n")
	// Stage 3: bare filename.
	require.Contains(t, got, "docs/api.md\n")
}

func TestRenderRepoMapMissingFileFallback(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	parser := newRenderTestParser(t)

	entries := []StageEntry{
		{Stage: stageRankedDefs, File: "src/missing.go", Ident: "Foo"},
	}
	tags := map[string][]treesitter.Tag{
		"src/missing.go": {
			{RelPath: "src/missing.go", Name: "Foo", Kind: "def", Line: 5},
		},
	}

	got, err := RenderRepoMap(context.Background(), entries, tags, parser, tmpDir)
	require.NoError(t, err)
	// Should fall back to flat S1|file|ident format.
	require.Equal(t, "S1|src/missing.go|Foo\n", got)
}

func TestRenderRepoMapUnsupportedLanguageFallback(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	// Write a file with an extension the parser doesn't support.
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, "data.xyz"),
		[]byte("some content"),
		0o644,
	))

	parser := &unsupportedLangParser{}
	entries := []StageEntry{
		{Stage: stageRankedDefs, File: "data.xyz", Ident: "Foo"},
	}
	tags := map[string][]treesitter.Tag{
		"data.xyz": {
			{RelPath: "data.xyz", Name: "Foo", Kind: "def", Line: 1},
		},
	}

	got, err := RenderRepoMap(context.Background(), entries, tags, parser, tmpDir)
	require.NoError(t, err)
	// Unsupported language: falls back to flat format silently.
	require.Equal(t, "S1|data.xyz|Foo\n", got)
}

func TestRenderRepoMapParseFailureFallback(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, "broken.go"),
		[]byte("this is not valid go but we parse anyway"),
		0o644,
	))

	parser := newRenderTestParser(t)
	parser.parseError = fmt.Errorf("parse explosion")

	entries := []StageEntry{
		{Stage: stageRankedDefs, File: "broken.go", Ident: "X"},
	}
	tags := map[string][]treesitter.Tag{
		"broken.go": {
			{RelPath: "broken.go", Name: "X", Kind: "def", Line: 1},
		},
	}

	got, err := RenderRepoMap(context.Background(), entries, tags, parser, tmpDir)
	require.NoError(t, err)
	// Parse failure: falls back to flat format.
	require.Equal(t, "S1|broken.go|X\n", got)
}

func TestRenderRepoMapContextCancelledImmediately(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	entries := []StageEntry{
		{Stage: stageSpecialPrelude, File: "README.md"},
	}

	got, err := RenderRepoMap(ctx, entries, nil, nil, "")
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, got)
}

func TestRenderRepoMapContextCancelledMidRender(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	// Create many entries so there's a loop to iterate.
	entries := make([]StageEntry, 100)
	for i := range entries {
		entries[i] = StageEntry{
			Stage: stageRemainingFiles,
			File:  fmt.Sprintf("file_%03d.txt", i),
		}
	}

	// Cancel after a very short delay.
	go func() {
		time.Sleep(time.Millisecond)
		cancel()
	}()

	got, err := RenderRepoMap(ctx, entries, nil, nil, "")
	// Either completes (if fast enough) or returns context error.
	if err != nil {
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, got)
	}
	// If no error, it completed before cancellation — that's also valid.
}

func TestRenderRepoMapEmptyEntries(t *testing.T) {
	t.Parallel()

	got, err := RenderRepoMap(context.Background(), nil, nil, nil, "")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestRenderRepoMapNilParser(t *testing.T) {
	t.Parallel()

	entries := []StageEntry{
		{Stage: stageRankedDefs, File: "a.go", Ident: "Foo"},
	}
	tags := map[string][]treesitter.Tag{
		"a.go": {
			{RelPath: "a.go", Name: "Foo", Kind: "def", Line: 1},
		},
	}

	// Nil parser should fall back to flat format.
	got, err := RenderRepoMap(context.Background(), entries, tags, nil, "")
	require.NoError(t, err)
	require.Equal(t, "S1|a.go|Foo\n", got)
}

func TestRenderRepoMapStage1GoldenScope(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	goSource := `package main

import "fmt"

type Server struct {
	port int
}

func NewServer(port int) *Server {
	return &Server{port: port}
}

func (s *Server) Start() error {
	fmt.Println("starting")
	return nil
}

func (s *Server) handleRequest() {
	fmt.Println("handling")
}
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "server.go"), []byte(goSource), 0o644))

	parser := newRenderTestParser(t)
	entries := []StageEntry{
		{Stage: stageRankedDefs, File: "server.go", Ident: "NewServer"},
		{Stage: stageRankedDefs, File: "server.go", Ident: "handleRequest"},
	}
	tags := map[string][]treesitter.Tag{
		"server.go": {
			{RelPath: "server.go", Name: "Server", Kind: "def", Line: 5},
			{RelPath: "server.go", Name: "NewServer", Kind: "def", Line: 9},
			{RelPath: "server.go", Name: "Start", Kind: "def", Line: 13},
			{RelPath: "server.go", Name: "handleRequest", Kind: "def", Line: 18},
		},
	}

	got, err := RenderRepoMap(context.Background(), entries, tags, parser, tmpDir)
	require.NoError(t, err)

	// Verify structure: filename with colon, │-prefixed lines, ⋮ gap markers.
	require.Contains(t, got, "server.go:\n")
	require.Contains(t, got, "│")

	// The rendered output should include the definition lines for
	// NewServer (line 9, 0-indexed 8) and handleRequest (line 18, 0-indexed 17).
	require.Contains(t, got, "NewServer")
	require.Contains(t, got, "handleRequest")
}
