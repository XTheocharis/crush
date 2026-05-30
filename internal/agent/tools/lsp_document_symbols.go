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

type DocumentSymbolsParams struct {
	FilePath string `json:"file_path" description:"The absolute path to the file to get document symbols for"`
}

const DocumentSymbolsToolName = "lsp_document_symbols"

//go:embed lsp_document_symbols.md
var documentSymbolsDescription string

func NewDocumentSymbolsTool(lspManager *lsp.Manager) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		DocumentSymbolsToolName,
		documentSymbolsDescription,
		func(ctx context.Context, params DocumentSymbolsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.FilePath == "" {
				return fantasy.NewTextErrorResponse("file_path is required"), nil
			}

			if lspManager.Clients().Len() == 0 {
				return fantasy.NewTextErrorResponse("no LSP clients available. Configure LSP servers in crush.json to enable document symbols"), nil
			}

			absPath, err := filepath.Abs(params.FilePath)
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

			output := formatDocumentSymbols(symbols)
			return fantasy.NewTextResponse(output), nil
		},
	)
}

func formatDocumentSymbols(symbols []protocol.DocumentSymbol) string {
	var output strings.Builder
	formatDocumentSymbolTree(symbols, 0, &output)
	return output.String()
}

func formatDocumentSymbolTree(symbols []protocol.DocumentSymbol, depth int, output *strings.Builder) {
	indent := strings.Repeat("  ", depth)
	for _, sym := range symbols {
		kind := symbolKindName(sym.Kind)
		line := sym.Range.Start.Line + 1
		fmt.Fprintf(output, "%s%s %s (line %d)\n", indent, kind, sym.Name, line)
		if len(sym.Children) > 0 {
			formatDocumentSymbolTree(sym.Children, depth+1, output)
		}
	}
}

func symbolKindName(kind protocol.SymbolKind) string {
	switch kind {
	case protocol.File:
		return "File"
	case protocol.Module:
		return "Module"
	case protocol.Namespace:
		return "Namespace"
	case protocol.Package:
		return "Package"
	case protocol.Class:
		return "Class"
	case protocol.Method:
		return "Method"
	case protocol.Property:
		return "Property"
	case protocol.Field:
		return "Field"
	case protocol.Constructor:
		return "Constructor"
	case protocol.Enum:
		return "Enum"
	case protocol.Interface:
		return "Interface"
	case protocol.Function:
		return "Function"
	case protocol.Variable:
		return "Variable"
	case protocol.Constant:
		return "Constant"
	case protocol.String:
		return "String"
	case protocol.Number:
		return "Number"
	case protocol.Boolean:
		return "Boolean"
	case protocol.Array:
		return "Array"
	case protocol.Object:
		return "Object"
	case protocol.Key:
		return "Key"
	case protocol.Null:
		return "Null"
	case protocol.EnumMember:
		return "EnumMember"
	case protocol.Struct:
		return "Struct"
	case protocol.Event:
		return "Event"
	case protocol.Operator:
		return "Operator"
	case protocol.TypeParameter:
		return "TypeParameter"
	default:
		return fmt.Sprintf("Symbol(%d)", kind)
	}
}
