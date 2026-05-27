package lsp

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

// WorkspaceSymbol searches for symbols across the workspace matching the
// given query string. It sends a workspace/symbol request to the LSP server
// and returns a list of SymbolInformation results.
func (c *Client) WorkspaceSymbol(ctx context.Context, query string) ([]protocol.SymbolInformation, error) {
	if c.GetServerState() != StateReady {
		return nil, errServerNotReady(c.name)
	}
	call, err := c.requireCallLSP()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	params := protocol.WorkspaceSymbolParams{
		Query: query,
	}

	var result any
	if err := call(ctx, "workspace/symbol", params, &result); err != nil {
		return nil, fmt.Errorf("workspace symbol request failed: %w", err)
	}

	return parseWorkspaceSymbolResult(result)
}

// parseWorkspaceSymbolResult extracts []SymbolInformation from a raw LSP
// workspace/symbol response. The response can be []SymbolInformation,
// []WorkspaceSymbol, or null.
func parseWorkspaceSymbolResult(result any) ([]protocol.SymbolInformation, error) {
	if result == nil {
		return nil, nil
	}

	// Try []SymbolInformation first (classic LSP 3.x response).
	var symbols []protocol.SymbolInformation
	if err := remarshal(result, &symbols); err != nil {
		return nil, fmt.Errorf("failed to parse workspace symbol result: %w", err)
	}
	return symbols, nil
}
