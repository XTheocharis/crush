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

type SignatureHelpParams struct {
	FilePath  string `json:"file_path" description:"The absolute path to the file"`
	Line      int    `json:"line" description:"The line number (1-based)"`
	Character int    `json:"character" description:"The character offset (1-based)"`
}

const SignatureHelpToolName = "lsp_signature_help"

//go:embed lsp_signature_help.md
var signatureHelpDescription string

func NewSignatureHelpTool(lspManager *lsp.Manager) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SignatureHelpToolName,
		signatureHelpDescription,
		func(ctx context.Context, params SignatureHelpParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
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
				return fantasy.NewTextErrorResponse("no LSP clients available. Configure LSP servers in crush.json to enable signature help"), nil
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

			help, err := client.SignatureHelp(ctx, absPath, params.Line, params.Character)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("signature help request failed: %s", err)), nil
			}

			if help == nil || len(help.Signatures) == 0 {
				return fantasy.NewTextResponse("No signature help available at the given position."), nil
			}

			output := formatSignatureHelp(help)
			return fantasy.NewTextResponse(output), nil
		},
	)
}

func formatSignatureHelp(help *protocol.SignatureHelp) string {
	var sb strings.Builder
	for i, sig := range help.Signatures {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "Signature: %s\n", sig.Label)
		for j, param := range sig.Parameters {
			fmt.Fprintf(&sb, "  Param %d: %v\n", j+1, param.Label.Value)
		}
	}
	if help.ActiveSignature < uint32(len(help.Signatures)) {
		fmt.Fprintf(&sb, "\nActive signature: %d\n", help.ActiveSignature)
	}
	if help.ActiveParameter > 0 {
		fmt.Fprintf(&sb, "Active parameter: %d\n", help.ActiveParameter)
	}
	return sb.String()
}
