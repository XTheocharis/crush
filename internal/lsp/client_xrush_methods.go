package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

// xrushClientFields holds xrush-specific fields for the Client struct.
type xrushClientFields struct {
	// Optional callLSP function for making arbitrary LSP protocol requests.
	callLSP func(ctx context.Context, method string, params any, result any) error
	// Optional healthCheck function for checking if the server is alive.
	healthCheck func() bool
}

// errServerNotReady is returned when an LSP method is called while the server
// is not in the StateReady state.
func errServerNotReady(name string) error {
	return fmt.Errorf("LSP server %s is not ready", name)
}

func errCallNotConfigured(name string) error {
	return fmt.Errorf("LSP server %s does not support direct protocol calls", name)
}

func (c *Client) requireCallLSP() (func(ctx context.Context, method string, params any, result any) error, error) {
	if c.callLSP == nil {
		return nil, errCallNotConfigured(c.name)
	}
	return c.callLSP, nil
}

// remarshal round-trips a value through JSON to decode it into the target type.
// This handles the common case where callLSP returns map[string]any or []any
// from a JSON-RPC response that needs to be decoded into a typed Go struct.
func remarshal(src any, dst any) error {
	if src == nil {
		return nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// Definition finds the definition of the symbol at the given position.
func (c *Client) Definition(ctx context.Context, filepath string, line, character int) ([]protocol.Location, error) {
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
	params := protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI(uri)},
			Position:     protocol.Position{Line: uint32(line - 1), Character: uint32(character - 1)},
		},
	}

	var result any
	if err := call(ctx, "textDocument/definition", params, &result); err != nil {
		return nil, fmt.Errorf("definition request failed: %w", err)
	}

	return parseLocationArray(result)
}

// Rename renames the symbol at the given position to newName and returns the
// resulting workspace edit.
func (c *Client) Rename(ctx context.Context, filepath string, line, character int, newName string) (*protocol.WorkspaceEdit, error) {
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
	params := protocol.RenameParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI(uri)},
		Position:     protocol.Position{Line: uint32(line - 1), Character: uint32(character - 1)},
		NewName:      newName,
	}

	var result any
	if err := call(ctx, "textDocument/rename", params, &result); err != nil {
		return nil, fmt.Errorf("rename request failed: %w", err)
	}

	var edit protocol.WorkspaceEdit
	if err := remarshal(result, &edit); err != nil {
		return nil, fmt.Errorf("failed to parse rename result: %w", err)
	}
	return &edit, nil
}

// CodeAction requests code actions for the given range, optionally filtered by kind.
func (c *Client) CodeAction(ctx context.Context, filepath string, rng protocol.Range, kind protocol.CodeActionKind) ([]protocol.CodeAction, error) {
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
	contextKinds := []protocol.CodeActionKind{}
	if kind != "" {
		contextKinds = append(contextKinds, kind)
	}
	params := protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI(uri)},
		Range:        rng,
		Context: protocol.CodeActionContext{
			Diagnostics: nil,
			Only:        contextKinds,
		},
	}

	var result any
	if err := call(ctx, "textDocument/codeAction", params, &result); err != nil {
		return nil, fmt.Errorf("code action request failed: %w", err)
	}

	var actions []protocol.CodeAction
	if err := remarshal(result, &actions); err != nil {
		return nil, fmt.Errorf("failed to parse code action result: %w", err)
	}
	return actions, nil
}

// Hover returns hover information for the symbol at the given position.
func (c *Client) Hover(ctx context.Context, filepath string, line, character int) (*protocol.Hover, error) {
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
	params := protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI(uri)},
			Position:     protocol.Position{Line: uint32(line - 1), Character: uint32(character - 1)},
		},
	}

	var result any
	if err := call(ctx, "textDocument/hover", params, &result); err != nil {
		return nil, fmt.Errorf("hover request failed: %w", err)
	}

	var hover protocol.Hover
	if err := remarshal(result, &hover); err != nil {
		return nil, fmt.Errorf("failed to parse hover result: %w", err)
	}
	return &hover, nil
}

// DocumentSymbols returns all document symbols in the given file.
func (c *Client) DocumentSymbols(ctx context.Context, filepath string) ([]protocol.DocumentSymbol, error) {
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
	params := protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI(uri)},
	}

	var result any
	if err := call(ctx, "textDocument/documentSymbol", params, &result); err != nil {
		return nil, fmt.Errorf("document symbols request failed: %w", err)
	}

	var symbols []protocol.DocumentSymbol
	if err := remarshal(result, &symbols); err != nil {
		return nil, fmt.Errorf("failed to parse document symbols result: %w", err)
	}
	return symbols, nil
}

// Completion returns completion items at the given position.
func (c *Client) Completion(ctx context.Context, filepath string, line, character int) ([]protocol.CompletionItem, error) {
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
	params := protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI(uri)},
			Position:     protocol.Position{Line: uint32(line - 1), Character: uint32(character - 1)},
		},
		Context: protocol.CompletionContext{
			TriggerKind: protocol.Invoked,
		},
	}

	var result any
	if err := call(ctx, "textDocument/completion", params, &result); err != nil {
		return nil, fmt.Errorf("completion request failed: %w", err)
	}

	var items []protocol.CompletionItem
	if err := parseCompletionResult(result, &items); err != nil {
		return nil, fmt.Errorf("failed to parse completion result: %w", err)
	}
	return items, nil
}

// Formatting returns text edits for formatting the given file.
func (c *Client) Formatting(ctx context.Context, filepath string) ([]protocol.TextEdit, error) {
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
	params := protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI(uri)},
		Options:      protocol.FormattingOptions{TabSize: 4, InsertSpaces: true},
	}

	var result any
	if err := call(ctx, "textDocument/formatting", params, &result); err != nil {
		return nil, fmt.Errorf("formatting request failed: %w", err)
	}

	var edits []protocol.TextEdit
	if err := remarshal(result, &edits); err != nil {
		return nil, fmt.Errorf("failed to parse formatting result: %w", err)
	}
	return edits, nil
}

// parseLocationArray extracts []protocol.Location from a raw LSP response.
// textDocument/definition can return Location[], LocationLink[], or null.
func parseLocationArray(result any) ([]protocol.Location, error) {
	if result == nil {
		return nil, nil
	}
	var locs []protocol.Location
	if err := remarshal(result, &locs); err != nil {
		return nil, err
	}
	return locs, nil
}

// parseCompletionResult handles the two valid LSP completion response shapes:
// a CompletionList object or a plain array of CompletionItem.
func parseCompletionResult(result any, items *[]protocol.CompletionItem) error {
	if result == nil {
		return nil
	}
	switch v := result.(type) {
	case *protocol.CompletionList:
		*items = v.Items
		return nil
	case protocol.CompletionList:
		*items = v.Items
		return nil
	case map[string]any:
		var list protocol.CompletionList
		if err := remarshal(v, &list); err != nil {
			return err
		}
		*items = list.Items
	case []any:
		return remarshal(v, items)
	default:
		return remarshal(result, items)
	}
	return nil
}

// [XRUSH: begin: IsAlive method]
// IsAlive reports whether the LSP server process is still running.
func (c *Client) IsAlive() bool {
	if c.healthCheck != nil {
		return c.healthCheck()
	}
	if c.client == nil {
		return false
	}
	return c.client.IsRunning()
}

// [XRUSH: end]
