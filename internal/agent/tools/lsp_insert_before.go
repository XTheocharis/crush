package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
)

type InsertBeforeParams struct {
	FilePath string `json:"file_path" description:"The absolute path to the file"`
	Line     int    `json:"line" description:"The line number (1-based) to insert before"`
	Text     string `json:"text" description:"The text to insert"`
}

const InsertBeforeToolName = "lsp_insert_before"

//go:embed lsp_insert_before.md
var insertBeforeDescription string

func NewInsertBeforeTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		InsertBeforeToolName,
		insertBeforeDescription,
		func(_ context.Context, params InsertBeforeParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.FilePath == "" {
				return fantasy.NewTextErrorResponse("file_path is required"), nil
			}
			if params.Line < 1 {
				return fantasy.NewTextErrorResponse("line must be >= 1"), nil
			}
			if params.Text == "" {
				return fantasy.NewTextErrorResponse("text is required"), nil
			}

			if err := InsertBeforeSymbol(params.FilePath, params.Line, params.Text); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to insert before symbol: %s", err)), nil
			}

			return fantasy.NewTextResponse(fmt.Sprintf("Inserted text before line %d in %s", params.Line, params.FilePath)), nil
		})
}
