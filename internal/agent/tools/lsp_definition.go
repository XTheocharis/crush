package tools

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/lsp"
)

type DefinitionParams struct {
	FilePath  string `json:"file_path" description:"The absolute path to the file"`
	Line      int    `json:"line" description:"The line number (1-based)"`
	Character int    `json:"character" description:"The character offset (1-based)"`
}

const DefinitionToolName = "lsp_definition"

//go:embed lsp_definition.md
var definitionDescription string

func NewDefinitionTool(lspManager *lsp.Manager) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		DefinitionToolName,
		definitionDescription,
		func(ctx context.Context, params DefinitionParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
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
				return fantasy.NewTextErrorResponse("no LSP clients available. Configure LSP servers in crush.json to enable symbolic navigation"), nil
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

			locations, err := client.Definition(ctx, absPath, params.Line, params.Character)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("definition request failed: %s", err)), nil
			}

			if len(locations) == 0 {
				return fantasy.NewTextResponse("No definition found for the symbol at the given position"), nil
			}

			output := formatReferences(cleanupLocations(locations))
			return fantasy.NewTextResponse(output), nil
		})
}
