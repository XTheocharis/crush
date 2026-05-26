package tools

import (
	"fmt"

	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

// findClientForFile returns the first LSP client that handles the given file
// path, or nil if no client matches.
func findClientForFile(lspManager *lsp.Manager, absPath string) *lsp.Client {
	for client := range lspManager.Clients().Seq() {
		if client.HandlesFile(absPath) {
			return client
		}
	}
	return nil
}

func formatHoverResult(hover *protocol.Hover, params HoverParams) string {
	if hover.Contents.Value == "" {
		return "No hover information available at the given position."
	}

	output := fmt.Sprintf("Hover at %s:%d:%d\n\n%s", params.FilePath, params.Line, params.Character, hover.Contents.Value)
	if hover.Range.Start.Line != 0 || hover.Range.End.Line != 0 {
		output += fmt.Sprintf("\n\nRange: line %d:%d to %d:%d",
			hover.Range.Start.Line+1, hover.Range.Start.Character+1,
			hover.Range.End.Line+1, hover.Range.End.Character+1)
	}
	return output
}
