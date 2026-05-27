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

type CompletionParams struct {
	FilePath  string `json:"file_path" description:"The absolute path to the file"`
	Line      int    `json:"line" description:"The line number (1-based)"`
	Character int    `json:"character" description:"The character offset (1-based)"`
}

const CompletionToolName = "lsp_completion"

//go:embed lsp_completion.md
var completionDescription string

func NewCompletionTool(lspManager *lsp.Manager) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		CompletionToolName,
		completionDescription,
		func(ctx context.Context, params CompletionParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
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
				return fantasy.NewTextErrorResponse("no LSP clients available. Configure LSP servers in crush.json to enable completions"), nil
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

			items, err := client.Completion(ctx, absPath, params.Line, params.Character)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("completion request failed: %s", err)), nil
			}

			if len(items) == 0 {
				return fantasy.NewTextResponse("No completions available at the given position."), nil
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "Found %d completion(s):\n\n", len(items))
			for i, item := range items {
				fmt.Fprintf(&sb, "%d. %s", i+1, item.Label)
				if item.Detail != "" {
					fmt.Fprintf(&sb, " — %s", item.Detail)
				}
				sb.WriteString("\n")
			}
			return fantasy.NewTextResponse(sb.String()), nil
		},
	)
}
