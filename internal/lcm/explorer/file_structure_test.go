package explorer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFileStructure_SymbolsWithVisibility(t *testing.T) {
	t.Parallel()

	summary := `Tree-sitter file: main.go
Language: go

Symbols:
  - function main (exported, line 10)
  - function helper (unexported, line 25)
  - struct Server (exported, line 40)`

	fs := ParseFileStructure(summary)
	require.Len(t, fs.Symbols, 3)
	require.Equal(t, "main", fs.Symbols[0].Name)
	require.Equal(t, "function", fs.Symbols[0].Kind)
	require.Equal(t, 10, fs.Symbols[0].StartLine)
	require.Equal(t, "helper", fs.Symbols[1].Name)
	require.Equal(t, 25, fs.Symbols[1].StartLine)
	require.Equal(t, "Server", fs.Symbols[2].Name)
	require.Equal(t, "Server", fs.Sections[2].Name)
	require.Equal(t, "function", fs.Sections[0].Type)
}

func TestParseFileStructure_SymbolsWithLineOnly(t *testing.T) {
	t.Parallel()

	summary := `Shell script
Symbols:
  - function deploy (line 5)
  - function cleanup (line 12)`

	fs := ParseFileStructure(summary)
	require.Len(t, fs.Symbols, 2)
	require.Equal(t, "deploy", fs.Symbols[0].Name)
	require.Equal(t, 5, fs.Symbols[0].StartLine)
	require.Equal(t, "cleanup", fs.Symbols[1].Name)
	require.Equal(t, 12, fs.Symbols[1].StartLine)
}

func TestParseFileStructure_Imports(t *testing.T) {
	t.Parallel()

	summary := `Tree-sitter file: app.go
Language: go

Imports:
  - fmt (stdlib)
  - os (stdlib)
  - github.com/foo/bar (third_party)

Symbols:
  - function main (exported, line 15)`

	fs := ParseFileStructure(summary)
	require.Len(t, fs.Imports, 3)
	require.Equal(t, "fmt", fs.Imports[0])
	require.Equal(t, "os", fs.Imports[1])
	require.Equal(t, "github.com/foo/bar", fs.Imports[2])
	require.Len(t, fs.Symbols, 1)
}

func TestParseFileStructure_Empty(t *testing.T) {
	t.Parallel()

	fs := ParseFileStructure("")
	require.Empty(t, fs.Symbols)
	require.Empty(t, fs.Imports)
	require.Empty(t, fs.Sections)
}

func TestParseFileStructure_PlainText(t *testing.T) {
	t.Parallel()

	fs := ParseFileStructure("Just some text without any structure")
	require.Empty(t, fs.Symbols)
	require.Empty(t, fs.Imports)
}

func TestFileStructureString(t *testing.T) {
	t.Parallel()

	fs := &FileStructure{
		Imports: []string{"fmt", "os"},
		Symbols: []SymbolInfo{
			{Name: "main", Kind: "function", StartLine: 10},
			{Name: "Config", Kind: "struct"},
		},
	}

	result := fs.String()
	require.Contains(t, result, "Imports:")
	require.Contains(t, result, "  - fmt")
	require.Contains(t, result, "  - os")
	require.Contains(t, result, "Symbols:")
	require.Contains(t, result, "  - function main (line 10)")
	require.Contains(t, result, "  - struct Config")
}

func TestExploreStructuredDefault(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	fs, err := reg.ExploreStructured(context.Background(), ExploreInput{
		Path:    "deploy.sh",
		Content: []byte("#!/bin/bash\nfunction deploy() {\n  echo hi\n}\n"),
	})
	require.NoError(t, err)
	require.NotNil(t, fs)
}
