package tools

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLSPSymbolsTools(t *testing.T) {
	t.Parallel()

	t.Run("tool names are correct", func(t *testing.T) {
		require.Equal(t, "lsp_symbols", SymbolsToolName)
		require.Equal(t, "lsp_document_symbols", DocumentSymbolsToolName)
		require.Equal(t, "lsp_workspace_symbols", WorkspaceSymbolsToolName)
	})

	t.Run("tools can be created with nil manager", func(t *testing.T) {
		tool := NewSymbolsTool(nil)
		require.NotNil(t, tool)
		require.Equal(t, SymbolsToolName, tool.Info().Name)

		tool = NewDocumentSymbolsTool(nil)
		require.NotNil(t, tool)
		require.Equal(t, DocumentSymbolsToolName, tool.Info().Name)

		tool = NewWorkspaceSymbolsTool(nil)
		require.NotNil(t, tool)
		require.Equal(t, WorkspaceSymbolsToolName, tool.Info().Name)
	})

	t.Run("tool descriptions are loaded", func(t *testing.T) {
		require.NotEmpty(t, symbolsDescription)
		require.NotEmpty(t, documentSymbolsDescription)
		require.NotEmpty(t, workspaceSymbolsDescription)
	})
}
