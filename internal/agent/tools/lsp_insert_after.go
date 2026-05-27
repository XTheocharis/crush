package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
)

type InsertAfterParams struct {
	FilePath string `json:"file_path" description:"The absolute path to the file"`
	Line     int    `json:"line" description:"The line number (1-based) to insert after"`
	Text     string `json:"text" description:"The text to insert"`
}

const InsertAfterToolName = "lsp_insert_after"

//go:embed lsp_insert_after.md
var insertAfterDescription string

func NewInsertAfterTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		InsertAfterToolName,
		insertAfterDescription,
		func(_ context.Context, params InsertAfterParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.FilePath == "" {
				return fantasy.NewTextErrorResponse("file_path is required"), nil
			}
			if params.Line < 1 {
				return fantasy.NewTextErrorResponse("line must be >= 1"), nil
			}
			if params.Text == "" {
				return fantasy.NewTextErrorResponse("text is required"), nil
			}

			if err := InsertAfterSymbol(params.FilePath, params.Line, params.Text); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to insert after symbol: %s", err)), nil
			}

			return fantasy.NewTextResponse(fmt.Sprintf("Inserted text after line %d in %s", params.Line, params.FilePath)), nil
		})
}
