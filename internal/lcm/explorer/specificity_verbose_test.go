//go:build treesitter

package explorer

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

func TestExplorerSpecificityTiers(t *testing.T) {
	t.Parallel()

	t.Run("tree-sitter explorer is specialized", func(t *testing.T) {
		t.Parallel()
		e := &TreeSitterExplorer{}
		require.Equal(t, SpecificitySpecialized, explorerSpecificity(e))
	})

	t.Run("data format explorers are family", func(t *testing.T) {
		t.Parallel()
		familyExplorers := []Explorer{
			&ArchiveExplorer{},
			&JSONExplorer{},
			&CSVExplorer{},
			&YAMLExplorer{},
			&ShellExplorer{},
			&MarkdownExplorer{},
			&PDFExplorer{},
			&ImageExplorer{},
			&ExecutableExplorer{},
		}
		for _, e := range familyExplorers {
			require.Equal(t, SpecificityFamily, explorerSpecificity(e),
				"expected %T to be family tier", e)
		}
	})

	t.Run("generic explorers are generic", func(t *testing.T) {
		t.Parallel()
		genericExplorers := []Explorer{
			&BinaryExplorer{},
			&TextExplorer{},
			&FallbackExplorer{},
		}
		for _, e := range genericExplorers {
			require.Equal(t, SpecificityGeneric, explorerSpecificity(e),
				"expected %T to be generic tier", e)
		}
	})

	t.Run("SpecificityTier String", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "specialized", SpecificitySpecialized.String())
		require.Equal(t, "family", SpecificityFamily.String())
		require.Equal(t, "generic", SpecificityGeneric.String())
	})
}

func TestExplorerSpecificityTiers_Dispatch(t *testing.T) {
	t.Parallel()

	t.Run("tree-sitter dispatches as specialized for Go", func(t *testing.T) {
		t.Parallel()
		mockParser := &mockTreeSitterParser{
			supports: map[string]bool{"go": true},
			hasTags:  map[string]bool{"go": true},
			analysis: &treesitter.FileAnalysis{
				Language: "go",
				Symbols:  []treesitter.SymbolInfo{{Name: "main", Kind: "function", Line: 2}},
			},
		}
		reg := NewRegistry(WithTreeSitter(mockParser))
		result, err := reg.Explore(context.Background(), ExploreInput{
			Path:    "main.go",
			Content: []byte("package main\nfunc main() {}"),
		})
		require.NoError(t, err)
		require.Equal(t, "treesitter", result.ExplorerUsed)
		require.Equal(t, SpecificitySpecialized, result.SpecificityTier)
	})

	t.Run("JSON dispatches as family", func(t *testing.T) {
		t.Parallel()
		reg := NewRegistry()
		result, err := reg.Explore(context.Background(), ExploreInput{
			Path:    "data.json",
			Content: []byte(`{"key": "value"}`),
		})
		require.NoError(t, err)
		require.Equal(t, "json", result.ExplorerUsed)
		require.Equal(t, SpecificityFamily, result.SpecificityTier)
	})

	t.Run("text file dispatches as generic", func(t *testing.T) {
		t.Parallel()
		reg := NewRegistry()
		result, err := reg.Explore(context.Background(), ExploreInput{
			Path:    "notes.txt",
			Content: []byte("plain text content\n"),
		})
		require.NoError(t, err)
		require.Equal(t, "text", result.ExplorerUsed)
		require.Equal(t, SpecificityGeneric, result.SpecificityTier)
	})

	t.Run("binary file dispatches as generic", func(t *testing.T) {
		t.Parallel()
		content := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}
		reg := NewRegistry()
		result, err := reg.Explore(context.Background(), ExploreInput{
			Path:    "audio.wav",
			Content: content,
		})
		require.NoError(t, err)
		require.Equal(t, "binary", result.ExplorerUsed)
		require.Equal(t, SpecificityGeneric, result.SpecificityTier)
	})

	t.Run("Go file without tree-sitter falls to generic text", func(t *testing.T) {
		t.Parallel()
		reg := NewRegistry()
		result, err := reg.Explore(context.Background(), ExploreInput{
			Path:    "main.go",
			Content: []byte("package main\nfunc main() {}\n"),
		})
		require.NoError(t, err)
		require.Equal(t, "text", result.ExplorerUsed)
		require.Equal(t, SpecificityGeneric, result.SpecificityTier)
	})

	t.Run("shell script dispatches as family", func(t *testing.T) {
		t.Parallel()
		reg := NewRegistry()
		result, err := reg.Explore(context.Background(), ExploreInput{
			Path:    "script.sh",
			Content: []byte("#!/bin/bash\necho hello\n"),
		})
		require.NoError(t, err)
		require.Equal(t, "shell", result.ExplorerUsed)
		require.Equal(t, SpecificityFamily, result.SpecificityTier)
	})

	t.Run("specialized takes priority over family for Go with tree-sitter", func(t *testing.T) {
		t.Parallel()
		mockParser := &mockTreeSitterParser{
			supports: map[string]bool{"go": true},
			hasTags:  map[string]bool{"go": true},
			analysis: &treesitter.FileAnalysis{
				Language: "go",
				Symbols:  []treesitter.SymbolInfo{{Name: "main", Kind: "function", Line: 2}},
			},
		}
		reg := NewRegistry(WithTreeSitter(mockParser))
		result, err := reg.Explore(context.Background(), ExploreInput{
			Path:    "main.go",
			Content: []byte("package main\nfunc main() {}"),
		})
		require.NoError(t, err)
		require.Equal(t, "treesitter", result.ExplorerUsed)
		require.Equal(t, SpecificitySpecialized, result.SpecificityTier)
	})
}

func TestExplorerVerboseProfile(t *testing.T) {
	t.Parallel()

	t.Run("verbose shows all items without truncation", func(t *testing.T) {
		t.Parallel()
		raw := `Go file: main.go
Imports:
  - fmt
  - os
  - strings
  - context
  - io
  - net
  - http
  - encoding
  - json
  - time
  - sync
`
		verbose := formatSummary(raw, OutputProfileVerbose)
		for _, item := range []string{"fmt", "os", "strings", "context", "io", "net", "http", "encoding", "json", "time", "sync"} {
			require.Contains(t, verbose, item, "verbose should include all items, missing %q", item)
		}
		require.NotContains(t, verbose, "... and", "verbose should not have truncation markers")
		require.NotContains(t, verbose, "(+)", "verbose should not have parity truncation markers")
	})

	t.Run("compact truncates like parity", func(t *testing.T) {
		t.Parallel()
		raw := `Go file: main.go
Imports:
  - fmt
  - os
  - strings
  - context
  - io
  - net
  - http
  - encoding
  - json
`
		compact := formatSummary(raw, normalizeProfile(OutputProfileCompact))
		require.Contains(t, compact, "(+1 more)", "compact should use parity-style markers")
	})

	t.Run("standard truncates like enhancement", func(t *testing.T) {
		t.Parallel()
		raw := `Go file: main.go
Imports:
  - fmt
  - os
  - strings
  - context
  - io
  - net
  - http
  - encoding
  - json
`
		standard := formatSummary(raw, normalizeProfile(OutputProfileStandard))
		require.Contains(t, standard, "... and 1 more", "standard should use enhancement-style markers")
	})

	t.Run("verbose raw content has no line limit", func(t *testing.T) {
		t.Parallel()
		lines := make([]string, 30)
		for i := range lines {
			lines[i] = "line content here"
		}
		raw := "Text file: notes.txt\nContent:\n" + strings.Join(lines, "\n")
		verbose := formatSummary(raw, OutputProfileVerbose)
		lineCount := strings.Count(verbose, "- line content here")
		require.Equal(t, 30, lineCount, "verbose should include all content lines")
	})

	t.Run("verbose with registry produces no truncation", func(t *testing.T) {
		t.Parallel()
		content := make([]byte, 0, 1000)
		for i := 0; i < 20; i++ {
			content = append(content, []byte("line of text content\n")...)
		}
		reg := NewRegistry(WithOutputProfile(OutputProfileVerbose))
		result, err := reg.Explore(context.Background(), ExploreInput{
			Path:    "notes.txt",
			Content: content,
		})
		require.NoError(t, err)
		require.NotContains(t, result.Summary, "[TRUNCATED]", "verbose should not truncate")
	})
}
