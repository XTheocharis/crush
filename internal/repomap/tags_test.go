package repomap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

type fakeParser struct {
	analyses map[string]*treesitter.FileAnalysis
}

func (f *fakeParser) Analyze(ctx context.Context, path string, content []byte) (*treesitter.FileAnalysis, error) {
	if f.analyses == nil {
		return &treesitter.FileAnalysis{}, nil
	}
	if got := f.analyses[path]; got != nil {
		clone := *got
		clone.Tags = append([]treesitter.Tag(nil), got.Tags...)
		clone.Symbols = append([]treesitter.SymbolInfo(nil), got.Symbols...)
		clone.Imports = append([]treesitter.ImportInfo(nil), got.Imports...)
		return &clone, nil
	}
	return &treesitter.FileAnalysis{}, nil
}

func (f *fakeParser) Languages() []string               { return []string{"go"} }
func (f *fakeParser) SupportsLanguage(lang string) bool { return true }
func (f *fakeParser) HasTags(lang string) bool          { return true }
func (f *fakeParser) Close() error                      { return nil }

type fakeParserFactory struct {
	lastConfig treesitter.ParserConfig
	parser     treesitter.Parser
}

func (f *fakeParserFactory) NewParserWithConfig(cfg treesitter.ParserConfig) treesitter.Parser {
	f.lastConfig = cfg
	if f.parser != nil {
		return f.parser
	}
	return &fakeParser{}
}

func TestTagsExtractPersistsAndDeterministicOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	dbDir := t.TempDir()

	conn, err := db.Connect(ctx, dbDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)

	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "a.go"), []byte("package src\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "b.go"), []byte("package src\n"), 0o644))

	svc := NewService(nil, q, conn, root, context.Background())
	svc.parser = &fakeParser{analyses: map[string]*treesitter.FileAnalysis{
		"src/a.go": {
			Language: "go",
			Tags: []treesitter.Tag{
				{RelPath: "src/a.go", Name: "zRef", Kind: "ref", Line: 12, Language: "go", NodeType: "call"},
				{RelPath: "src/a.go", Name: "A", Kind: "def", Line: 2, Language: "go", NodeType: "function"},
			},
		},
		"src/b.go": {
			Language: "go",
			Tags: []treesitter.Tag{
				{RelPath: "src/b.go", Name: "B", Kind: "def", Line: 3, Language: "go", NodeType: "function"},
			},
		},
	}}

	tags, err := svc.extractTags(ctx, root, []string{"src/b.go", "./src/a.go"}, false)
	require.NoError(t, err)
	require.Equal(t, []string{
		"src/a.go:2 def A [function]",
		"src/a.go:12 ref zRef [call]",
		"src/b.go:3 def B [function]",
	}, []string{tags[0].String(), tags[1].String(), tags[2].String()})

	repoKey := repoKeyForRoot(root)
	rows, err := q.GetRepoMapFileCache(ctx, repoKey)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	stored, err := q.ListRepoMapTags(ctx, repoKey)
	require.NoError(t, err)
	require.Len(t, stored, 3)
}

func TestTagsExtractPrunesDeletedPath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	dbDir := t.TempDir()

	conn, err := db.Connect(ctx, dbDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)

	require.NoError(t, os.WriteFile(filepath.Join(root, "alive.go"), []byte("package main\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "gone.go"), []byte("package main\n"), 0o644))

	svc := NewService(nil, q, conn, root, context.Background())
	svc.parser = &fakeParser{analyses: map[string]*treesitter.FileAnalysis{
		"alive.go": {Language: "go", Tags: []treesitter.Tag{{Name: "Alive", Kind: "def", Line: 1, Language: "go", NodeType: "function"}}},
		"gone.go":  {Language: "go", Tags: []treesitter.Tag{{Name: "Gone", Kind: "def", Line: 1, Language: "go", NodeType: "function"}}},
	}}

	_, err = svc.extractTags(ctx, root, []string{"alive.go", "gone.go"}, false)
	require.NoError(t, err)

	require.NoError(t, os.Remove(filepath.Join(root, "gone.go")))
	_, err = svc.extractTags(ctx, root, []string{"alive.go"}, false)
	require.NoError(t, err)

	repoKey := repoKeyForRoot(root)
	stored, err := q.ListRepoMapTags(ctx, repoKey)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, "alive.go", stored[0].RelPath)
}

func TestTagsExtractUpdatesChangedFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	dbDir := t.TempDir()

	conn, err := db.Connect(ctx, dbDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)

	path := filepath.Join(root, "x.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n"), 0o644))

	svc := NewService(nil, q, conn, root, context.Background())
	fp := &fakeParser{analyses: map[string]*treesitter.FileAnalysis{
		"x.go": {Language: "go", Tags: []treesitter.Tag{{Name: "Old", Kind: "def", Line: 1, Language: "go", NodeType: "function"}}},
	}}
	svc.parser = fp

	_, err = svc.extractTags(ctx, root, []string{"x.go"}, false)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, []byte("package main\n// changed\n"), 0o644))
	time.Sleep(5 * time.Millisecond)

	fp.analyses["x.go"] = &treesitter.FileAnalysis{Language: "go", Tags: []treesitter.Tag{{Name: "New", Kind: "def", Line: 2, Language: "go", NodeType: "function"}}}
	_, err = svc.extractTags(ctx, root, []string{"x.go"}, false)
	require.NoError(t, err)

	repoKey := repoKeyForRoot(root)
	stored, err := q.ListRepoMapTags(ctx, repoKey)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, "New", stored[0].Name)
	require.EqualValues(t, 2, stored[0].Line)
}

func TestEnsureParserRespectsConfiguredPoolSize(t *testing.T) {
	t.Parallel()

	factory := &fakeParserFactory{}
	svc := &Service{
		cfg:              &config.RepoMapOptions{ParserPoolSize: 7},
		newParserWithCfg: factory.NewParserWithConfig,
	}

	_ = svc.ensureParser()
	require.Equal(t, 7, factory.lastConfig.PoolSize)
}

func TestStringInternerDeduplicatesBackingStorage(t *testing.T) {
	t.Parallel()

	interner := newStringInterner(2)
	left := interner.Intern("identifier")
	right := interner.Intern(string([]byte("identifier")))

	leftHdr := (*[2]uintptr)(unsafe.Pointer(&left))
	rightHdr := (*[2]uintptr)(unsafe.Pointer(&right))
	require.Equal(t, leftHdr[0], rightHdr[0], "interned strings should share data pointer")
	require.Equal(t, leftHdr[1], rightHdr[1], "interned strings should share length")
}
