package tools

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/crush/internal/lsp/util"
	powernap "github.com/charmbracelet/x/powernap/pkg/lsp"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

type RenameParams struct {
	FilePath  string `json:"file_path" description:"The absolute path to the file"`
	Line      int    `json:"line" description:"The line number (1-based)"`
	Character int    `json:"character" description:"The character offset (1-based)"`
	NewName   string `json:"new_name" description:"The new name for the symbol"`
}

const RenameToolName = "lsp_rename"

//go:embed lsp_rename.md
var renameDescription string

func NewRenameTool(lspManager *lsp.Manager) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		RenameToolName,
		renameDescription,
		func(ctx context.Context, params RenameParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.FilePath == "" {
				return fantasy.NewTextErrorResponse("file_path is required"), nil
			}
			if params.Line < 1 {
				return fantasy.NewTextErrorResponse("line must be >= 1"), nil
			}
			if params.Character < 1 {
				return fantasy.NewTextErrorResponse("character must be >= 1"), nil
			}
			if params.NewName == "" {
				return fantasy.NewTextErrorResponse("new_name is required"), nil
			}

			if lspManager.Clients().Len() == 0 {
				return fantasy.NewTextErrorResponse("no LSP clients available. Configure LSP servers in crush.json to enable symbolic renaming"), nil
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

			edit, err := lspManager.RenameForServer(ctx, name, absPath, params.Line, params.Character, params.NewName)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("rename request failed: %s", err)), nil
			}

			if edit == nil {
				return fantasy.NewTextResponse("Rename returned no changes"), nil
			}

			output := formatWorkspaceEdit(edit)

			if err := util.ApplyWorkspaceEdit(*edit, powernap.UTF16); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("rename succeeded but applying edits failed: %s", err)), nil
			}

			return fantasy.NewTextResponse(output), nil
		})
}

func formatWorkspaceEdit(edit *protocol.WorkspaceEdit) string {
	if len(edit.Changes) == 0 {
		return "Rename returned no changes"
	}

	var output strings.Builder
	totalEdits := 0
	for uri, edits := range edit.Changes {
		totalEdits += len(edits)
		path, err := uri.Path()
		if err != nil {
			continue
		}
		fmt.Fprintf(&output, "%s (%d edit(s)):\n", path, len(edits))
		for _, e := range edits {
			fmt.Fprintf(&output, "  Line %d:%d -> Line %d:%d\n",
				e.Range.Start.Line+1, e.Range.Start.Character+1,
				e.Range.End.Line+1, e.Range.End.Character+1)
		}
		output.WriteString("\n")
	}

	return fmt.Sprintf("Rename will apply %d edit(s) across %d file(s):\n\n%s", totalEdits, len(edit.Changes), output.String())
}
