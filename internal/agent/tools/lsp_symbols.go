package tools

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/lsp"
)

type SymbolsParams struct {
	FilePath string `json:"file_path,omitempty" description:"The absolute path to the file to get document symbols for. If provided, returns document symbols for that file."`
	Query    string `json:"query,omitempty" description:"The search query for workspace symbols. If provided without file_path, returns workspace symbols matching the query."`
}

const SymbolsToolName = "lsp_symbols"

//go:embed lsp_symbols.md
var symbolsDescription string

func NewSymbolsTool(lspManager *lsp.Manager) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SymbolsToolName,
		symbolsDescription,
		func(ctx context.Context, params SymbolsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if lspManager.Clients().Len() == 0 {
				return fantasy.NewTextErrorResponse("no LSP clients available. Configure LSP servers in crush.json to enable symbols"), nil
			}

			if params.FilePath != "" {
				return documentSymbols(ctx, lspManager, params.FilePath)
			}

			if params.Query != "" {
				return workspaceSymbols(ctx, lspManager, params.Query)
			}

			return fantasy.NewTextErrorResponse("either file_path or query must be provided"), nil
		},
	)
}

func documentSymbols(ctx context.Context, lspManager *lsp.Manager, filePath string) (fantasy.ToolResponse, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to resolve path: %s", err)), nil
	}

	lspManager.Start(ctx, absPath)

	client := findClientForFile(lspManager, absPath)
	if client == nil {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("no LSP server available for file type: %s", filepath.Ext(absPath))), nil
	}

	symbols, err := client.DocumentSymbols(ctx, absPath)
	if err != nil {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("document symbols request failed: %s", err)), nil
	}

	if len(symbols) == 0 {
		return fantasy.NewTextResponse("No document symbols found."), nil
	}

	var output strings.Builder
	fmt.Fprintf(&output, "Document symbols for %s:\n\n", absPath)
	output.WriteString(formatDocumentSymbols(symbols))
	return fantasy.NewTextResponse(output.String()), nil
}

func workspaceSymbols(ctx context.Context, lspManager *lsp.Manager, query string) (fantasy.ToolResponse, error) {
	var allResults []string
	for client := range lspManager.Clients().Seq() {
		symbols, err := client.WorkspaceSymbol(ctx, query)
		if err != nil {
			continue
		}
		for _, sym := range symbols {
			path, pathErr := sym.Location.URI.Path()
			if pathErr != nil {
				continue
			}
			line := sym.Location.Range.Start.Line + 1
			kind := symbolKindName(sym.Kind)
			container := ""
			if sym.ContainerName != "" {
				container = " in " + sym.ContainerName
			}
			allResults = append(allResults, fmt.Sprintf("%s %s%s — %s:%d", kind, sym.Name, container, filepath.Base(path), line))
		}
	}

	if len(allResults) == 0 {
		return fantasy.NewTextResponse(fmt.Sprintf("No workspace symbols found matching '%s'.", query)), nil
	}

	var output strings.Builder
	fmt.Fprintf(&output, "Found %d workspace symbol(s) matching '%s':\n\n", len(allResults), query)
	output.WriteString(strings.Join(allResults, "\n"))
	output.WriteString("\n")
	return fantasy.NewTextResponse(output.String()), nil
}
