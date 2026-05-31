package tools

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

type SafeDeleteParams struct {
	FilePath  string `json:"file_path" description:"The absolute path to the file"`
	Line      int    `json:"line" description:"The line number (1-based)"`
	Character int    `json:"character" description:"The character offset (1-based)"`
}

const SafeDeleteToolName = "lsp_safe_delete"

//go:embed lsp_safe_delete.md
var safeDeleteDescription string

func NewSafeDeleteTool(lspManager *lsp.Manager) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SafeDeleteToolName,
		safeDeleteDescription,
		func(ctx context.Context, params SafeDeleteParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.FilePath == "" {
				return fantasy.NewTextErrorResponse("file_path is required"), nil
			}
			if params.Line < 1 {
				return fantasy.NewTextErrorResponse("line must be >= 1"), nil
			}
			if params.Character < 1 {
				return fantasy.NewTextErrorResponse("character must be >= 1"), nil
			}

			if lspManager.Clients().Len() == 0 {
				return fantasy.NewTextErrorResponse("no LSP clients available. Configure LSP servers in crush.json to enable safe symbol deletion"), nil
			}

			absPath, err := filepath.Abs(params.FilePath)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to resolve path: %s", err)), nil
			}

			lspManager.Start(ctx, absPath)

			name, client := lspManager.FindClientForFile(absPath)
			if client == nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("no LSP server available for file type: %s", filepath.Ext(absPath))), nil
			}

			uri := protocol.DocumentURI("file://" + absPath)
			position := protocol.Position{Line: uint32(params.Line - 1), Character: uint32(params.Character - 1)}

			refsFn := func(ctx context.Context, _ string, pos protocol.Position) ([]protocol.Location, error) {
				return lspManager.SafeDeleteForServer(ctx, name, absPath, int(pos.Line)+1, int(pos.Character)+1)
			}

			result, err := SafeDeleteSymbol(ctx, string(uri), position, refsFn)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("safe delete check failed: %s", err)), nil
			}

			if !result.CanDelete {
				return fantasy.NewTextResponse(fmt.Sprintf("Cannot safely delete symbol. %s", result.Warning)), nil
			}

			return fantasy.NewTextResponse("Symbol can be safely deleted: no external references found."), nil
		})
}
