package treesitter

import (
	"context"
	"fmt"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	ImportCategoryStdlib     = "stdlib"
	ImportCategoryThirdParty = "third_party"
	ImportCategoryLocal      = "local"
	ImportCategoryUnknown    = "unknown"
)

// Tag is a single identifier occurrence emitted by tags queries.
type Tag struct {
	RelPath  string
	Name     string
	Kind     string // "def" or "ref"
	Line     int
	Language string
	NodeType string
}

// String returns stable diagnostic text used by tests and artifacts.
// Format: "<rel_path>:<line> <kind> <name> [<node_type>]".
func (t Tag) String() string {
	return fmt.Sprintf("%s:%d %s %s [%s]", t.RelPath, t.Line, t.Kind, t.Name, t.NodeType)
}

// SymbolInfo contains definition metadata extracted from source.
type SymbolInfo struct {
	Name       string
	Kind       string
	Line       int
	EndLine    int
	Params     string
	ReturnType string
	Modifiers  []string
	Decorators []string
	Parent     string
	DocComment string
}

// FileAnalysis is the parser output for one file.
type FileAnalysis struct {
	Language string
	Tags     []Tag
	Symbols  []SymbolInfo
	Imports  []ImportInfo
}

// ImportInfo captures one import statement classification.
type ImportInfo struct {
	Path     string
	Names    []string
	Category string // "stdlib", "third_party", "local", "unknown"
}

// Parser is the tree-sitter analysis interface used by repo-map service.
type Parser interface {
	Analyze(ctx context.Context, path string, content []byte) (*FileAnalysis, error)
	ParseTree(ctx context.Context, path string, content []byte) (*tree_sitter.Tree, error)
	Languages() []string
	SupportsLanguage(lang string) bool
	HasTags(lang string) bool
	Close() error
}
