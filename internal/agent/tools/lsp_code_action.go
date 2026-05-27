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

type CodeActionParams struct {
	FilePath  string `json:"file_path" description:"The absolute path to the file"`
	StartLine int    `json:"start_line" description:"Start line (1-based)"`
	StartChar int    `json:"start_char" description:"Start character (1-based)"`
	EndLine   int    `json:"end_line" description:"End line (1-based)"`
	EndChar   int    `json:"end_char" description:"End character (1-based)"`
	Kind      string `json:"kind,omitempty" description:"Optional code action kind filter (e.g., quickfix, refactor, source.organizeImports)"`
}

const CodeActionToolName = "lsp_code_action"

//go:embed lsp_code_action.md
var codeActionDescription string

func NewCodeActionTool(lspManager *lsp.Manager) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		CodeActionToolName,
		codeActionDescription,
		func(ctx context.Context, params CodeActionParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.FilePath == "" {
				return fantasy.NewTextErrorResponse("file_path is required"), nil
			}
			if params.StartLine < 1 || params.EndLine < 1 {
				return fantasy.NewTextErrorResponse("start_line and end_line must be >= 1"), nil
			}
			if params.StartChar < 1 || params.EndChar < 1 {
				return fantasy.NewTextErrorResponse("start_char and end_char must be >= 1"), nil
			}

			if lspManager.Clients().Len() == 0 {
				return fantasy.NewTextErrorResponse("no LSP clients available. Configure LSP servers in crush.json to enable code actions"), nil
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

			rng := protocol.Range{
				Start: protocol.Position{
					Line:      uint32(params.StartLine - 1),
					Character: uint32(params.StartChar - 1),
				},
				End: protocol.Position{
					Line:      uint32(params.EndLine - 1),
					Character: uint32(params.EndChar - 1),
				},
			}

			actions, err := client.CodeAction(ctx, absPath, rng, protocol.CodeActionKind(params.Kind))
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("code action request failed: %s", err)), nil
			}

			if len(actions) == 0 {
				return fantasy.NewTextResponse("No code actions available for the given range"), nil
			}

			output := formatCodeActions(actions)
			return fantasy.NewTextResponse(output), nil
		})
}

func formatCodeActions(actions []protocol.CodeAction) string {
	var output strings.Builder
	fmt.Fprintf(&output, "Found %d code action(s):\n\n", len(actions))

	for i, action := range actions {
		fmt.Fprintf(&output, "%d. %s", i+1, action.Title)
		if action.Kind != "" {
			fmt.Fprintf(&output, " [%s]", action.Kind)
		}
		output.WriteString("\n")

		if action.Edit != nil && action.Edit.Changes != nil {
			for uri, edits := range action.Edit.Changes {
				path, err := uri.Path()
				if err != nil {
					continue
				}
				fmt.Fprintf(&output, "   %s: %d edit(s)\n", path, len(edits))
			}
		}
	}

	return output.String()
}
