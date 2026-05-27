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

type FormattingParams struct {
	FilePath string `json:"file_path" description:"The absolute path to the file to format"`
}

const FormattingToolName = "lsp_formatting"

//go:embed lsp_formatting.md
var formattingDescription string

func NewFormattingTool(lspManager *lsp.Manager) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		FormattingToolName,
		formattingDescription,
		func(ctx context.Context, params FormattingParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.FilePath == "" {
				return fantasy.NewTextErrorResponse("file_path is required"), nil
			}

			if lspManager.Clients().Len() == 0 {
				return fantasy.NewTextErrorResponse("no LSP clients available. Configure LSP servers in crush.json to enable formatting"), nil
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

			edits, err := client.Formatting(ctx, absPath)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("formatting request failed: %s", err)), nil
			}

			if len(edits) == 0 {
				return fantasy.NewTextResponse("File is already formatted. No changes needed."), nil
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "Formatting returned %d edit(s):\n\n", len(edits))
			for i, edit := range edits {
				fmt.Fprintf(&sb, "Edit %d: line %d:%d -> %d:%d\n", i+1,
					edit.Range.Start.Line+1, edit.Range.Start.Character+1,
					edit.Range.End.Line+1, edit.Range.End.Character+1)
				if edit.NewText != "" {
					fmt.Fprintf(&sb, "  New text: %q\n", edit.NewText)
				}
			}
			return fantasy.NewTextResponse(sb.String()), nil
		},
	)
}
