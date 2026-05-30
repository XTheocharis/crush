package tools

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

type WorkspaceSymbolsParams struct {
	Query string `json:"query" description:"The search query to find workspace symbols (e.g., function name, type name)"`
}

const WorkspaceSymbolsToolName = "lsp_workspace_symbols"

//go:embed lsp_workspace_symbols.md
var workspaceSymbolsDescription string

func NewWorkspaceSymbolsTool(lspManager *lsp.Manager) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		WorkspaceSymbolsToolName,
		workspaceSymbolsDescription,
		func(ctx context.Context, params WorkspaceSymbolsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Query == "" {
				return fantasy.NewTextErrorResponse("query is required"), nil
			}

			if lspManager.Clients().Len() == 0 {
				return fantasy.NewTextErrorResponse("no LSP clients available. Configure LSP servers in crush.json to enable workspace symbols"), nil
			}

			var allSymbols []protocol.SymbolInformation
			for client := range lspManager.Clients().Seq() {
				symbols, err := client.WorkspaceSymbol(ctx, params.Query)
				if err != nil {
					continue
				}
				allSymbols = append(allSymbols, symbols...)
			}

			if len(allSymbols) == 0 {
				return fantasy.NewTextResponse(fmt.Sprintf("No workspace symbols found matching '%s'.", params.Query)), nil
			}

			output := formatWorkspaceSymbols(allSymbols)
			return fantasy.NewTextResponse(output), nil
		},
	)
}

func formatWorkspaceSymbols(symbols []protocol.SymbolInformation) string {
	var output strings.Builder
	fmt.Fprintf(&output, "Found %d workspace symbol(s):\n\n", len(symbols))

	for _, sym := range symbols {
		kind := symbolKindName(sym.Kind)
		path, err := sym.Location.URI.Path()
		if err != nil {
			continue
		}
		line := sym.Location.Range.Start.Line + 1
		container := ""
		if sym.ContainerName != "" {
			container = " in " + sym.ContainerName
		}
		fmt.Fprintf(&output, "%s %s%s — %s:%d\n", kind, sym.Name, container, filepath.Base(path), line)
	}

	return output.String()
}
