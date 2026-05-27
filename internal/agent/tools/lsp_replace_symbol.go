package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
)

type ReplaceSymbolParams struct {
	FilePath string `json:"file_path" description:"The absolute path to the file"`
	Line     int    `json:"line" description:"The line number (1-based) where the symbol starts"`
	NewBody  string `json:"new_body" description:"The new body content to replace the symbol body with"`
}

const ReplaceSymbolToolName = "lsp_replace_symbol"

//go:embed lsp_replace_symbol.md
var replaceSymbolDescription string

func NewReplaceSymbolTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ReplaceSymbolToolName,
		replaceSymbolDescription,
		func(_ context.Context, params ReplaceSymbolParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.FilePath == "" {
				return fantasy.NewTextErrorResponse("file_path is required"), nil
			}
			if params.Line < 1 {
				return fantasy.NewTextErrorResponse("line must be >= 1"), nil
			}
			if params.NewBody == "" {
				return fantasy.NewTextErrorResponse("new_body is required"), nil
			}

			if err := ReplaceSymbolBody(params.FilePath, params.Line, params.NewBody); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to replace symbol body: %s", err)), nil
			}

			return fantasy.NewTextResponse(fmt.Sprintf("Replaced symbol body at %s line %d", params.FilePath, params.Line)), nil
		})
}
