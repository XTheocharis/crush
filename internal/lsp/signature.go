package lsp

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

// SignatureHelp requests signature help for the symbol at the given position.
// It sends a textDocument/signatureHelp request to the LSP server.
func (c *Client) SignatureHelp(ctx context.Context, filepath string, line, character int) (*protocol.SignatureHelp, error) {
	if c.GetServerState() != StateReady {
		return nil, errServerNotReady(c.name)
	}
	if err := c.OpenFileOnDemand(ctx, filepath); err != nil {
		return nil, err
	}
	call, err := c.requireCallLSP()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	uri := string(protocol.URIFromPath(filepath))
	params := protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI(uri)},
			Position:     protocol.Position{Line: uint32(line - 1), Character: uint32(character - 1)},
		},
	}

	var result any
	if err := call(ctx, "textDocument/signatureHelp", params, &result); err != nil {
		return nil, fmt.Errorf("signature help request failed: %w", err)
	}

	if result == nil {
		return nil, nil
	}

	var help protocol.SignatureHelp
	if err := remarshal(result, &help); err != nil {
		return nil, fmt.Errorf("failed to parse signature help result: %w", err)
	}
	return &help, nil
}
