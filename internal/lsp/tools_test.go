package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
	"github.com/stretchr/testify/require"
)

// newToolsTestClient creates a Client ready for LSP tool method tests.
// The callLSP function is the mock: it receives (method, params) and writes
// the response into result. The client is set to StateReady and has the
// given file pre-opened in its open-files map.
func newToolsTestClient(callLSP func(ctx context.Context, method string, params any, result any) error) *Client {
	c := &Client{
		name:        "mock-server",
		cwd:         "/workspace",
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
		xrushClientFields: xrushClientFields{
			callLSP:     callLSP,
			healthCheck: func() bool { return true },
		},
	}
	c.serverState.Store(StateReady)
	return c
}

// openFileInClient registers a URI as open in the client, bypassing the
// real OpenFile path (which reads from disk).
func openFileInClient(c *Client, filePath string) {
	uri := string(protocol.URIFromPath(filePath))
	c.openFiles.Set(uri, &OpenFileInfo{
		Version: 1,
		URI:     protocol.DocumentURI(uri),
	})
}

// ---------------------------------------------------------------------------
// Definition lookup
// ---------------------------------------------------------------------------

func TestDefinition_KnownSymbol(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")

	var calledMethod string
	c := newToolsTestClient(func(_ context.Context, method string, _ any, result any) error {
		calledMethod = method
		locs := []protocol.Location{
			{
				URI: protocol.DocumentURI("file:///workspace/lib.go"),
				Range: protocol.Range{
					Start: protocol.Position{Line: 10, Character: 5},
					End:   protocol.Position{Line: 10, Character: 15},
				},
			},
		}
		raw, _ := json.Marshal(locs)
		return json.Unmarshal(raw, result)
	})
	openFileInClient(c, filePath)

	locs, err := c.Definition(t.Context(), filePath, 5, 8)
	require.NoError(t, err)
	require.Equal(t, "textDocument/definition", calledMethod)
	require.Len(t, locs, 1)
	require.Equal(t, protocol.DocumentURI("file:///workspace/lib.go"), locs[0].URI)
	require.Equal(t, uint32(10), locs[0].Range.Start.Line)
	require.Equal(t, uint32(5), locs[0].Range.Start.Character)
}

func TestDefinition_EmptyResult(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")
	c := newToolsTestClient(func(_ context.Context, method string, _ any, result any) error {
		// LSP can return null for no definition found.
		return nil
	})
	openFileInClient(c, filePath)

	locs, err := c.Definition(t.Context(), filePath, 1, 1)
	require.NoError(t, err)
	require.Nil(t, locs)
}

func TestDefinition_ServerNotReady(t *testing.T) {
	t.Parallel()

	c := newToolsTestClient(nil)
	c.SetServerState(StateError)

	_, err := c.Definition(t.Context(), "/tmp/test.go", 1, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not ready")
}

func TestDefinition_CallLSPError(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")
	c := newToolsTestClient(func(_ context.Context, _ string, _ any, _ any) error {
		return fmt.Errorf("server internal error")
	})
	openFileInClient(c, filePath)

	_, err := c.Definition(t.Context(), filePath, 1, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "definition request failed")
}

// ---------------------------------------------------------------------------
// Hover / Doc symbols
// ---------------------------------------------------------------------------

func TestHover_KnownSymbol(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")

	var calledMethod string
	c := newToolsTestClient(func(_ context.Context, method string, _ any, result any) error {
		calledMethod = method
		hover := protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  "markdown",
				Value: "```go\nfunc Hello() string\n```",
			},
			Range: protocol.Range{
				Start: protocol.Position{Line: 3, Character: 5},
				End:   protocol.Position{Line: 3, Character: 10},
			},
		}
		raw, _ := json.Marshal(hover)
		return json.Unmarshal(raw, result)
	})
	openFileInClient(c, filePath)

	hover, err := c.Hover(t.Context(), filePath, 4, 6)
	require.NoError(t, err)
	require.Equal(t, "textDocument/hover", calledMethod)
	require.NotNil(t, hover)
	require.Equal(t, protocol.MarkupKind("markdown"), hover.Contents.Kind)
}

func TestHover_NilResult(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")
	c := newToolsTestClient(func(_ context.Context, _ string, _ any, result any) error {
		// Server returns null for no hover info.
		return nil
	})
	openFileInClient(c, filePath)

	hover, err := c.Hover(t.Context(), filePath, 1, 1)
	require.NoError(t, err)
	require.NotNil(t, hover)
}

func TestDocumentSymbols_KnownFile(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")

	var calledMethod string
	c := newToolsTestClient(func(_ context.Context, method string, _ any, result any) error {
		calledMethod = method
		symbols := []protocol.DocumentSymbol{
			{
				Name:   "main",
				Kind:   protocol.Function,
				Range:  protocol.Range{Start: protocol.Position{Line: 5, Character: 0}, End: protocol.Position{Line: 10, Character: 1}},
			},
			{
				Name:   "Config",
				Kind:   protocol.Struct,
				Range:  protocol.Range{Start: protocol.Position{Line: 1, Character: 0}, End: protocol.Position{Line: 3, Character: 1}},
			},
		}
		raw, _ := json.Marshal(symbols)
		return json.Unmarshal(raw, result)
	})
	openFileInClient(c, filePath)

	symbols, err := c.DocumentSymbols(t.Context(), filePath)
	require.NoError(t, err)
	require.Equal(t, "textDocument/documentSymbol", calledMethod)
	require.Len(t, symbols, 2)
	require.Equal(t, "main", symbols[0].Name)
	require.Equal(t, protocol.Function, symbols[0].Kind)
	require.Equal(t, "Config", symbols[1].Name)
}

// ---------------------------------------------------------------------------
// Rename / Code Action protocol messages
// ---------------------------------------------------------------------------

func TestRename_BuildsCorrectMessage(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")

	var (
		calledMethod string
		calledParams protocol.RenameParams
	)
	c := newToolsTestClient(func(_ context.Context, method string, params any, result any) error {
		calledMethod = method
		// Capture the params the client sends.
		raw, _ := json.Marshal(params)
		_ = json.Unmarshal(raw, &calledParams)

		edit := protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				protocol.DocumentURI("file:///workspace/main.go"): {
					{
						Range: protocol.Range{
							Start: protocol.Position{Line: 4, Character: 5},
							End:   protocol.Position{Line: 4, Character: 8},
						},
						NewText: "NewName",
					},
				},
			},
		}
		raw, _ = json.Marshal(edit)
		return json.Unmarshal(raw, result)
	})
	openFileInClient(c, filePath)

	edit, err := c.Rename(t.Context(), filePath, 5, 6, "NewName")
	require.NoError(t, err)
	require.Equal(t, "textDocument/rename", calledMethod)
	require.Equal(t, "NewName", calledParams.NewName)
	require.NotNil(t, edit)
	require.Len(t, edit.Changes, 1)
}

func TestRename_ServerNotReady(t *testing.T) {
	t.Parallel()

	c := newToolsTestClient(nil)
	c.SetServerState(StateStarting)

	_, err := c.Rename(t.Context(), "/tmp/test.go", 1, 1, "newName")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not ready")
}

func TestCodeAction_BuildsCorrectMessage(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")

	var (
		calledMethod string
		calledParams protocol.CodeActionParams
	)
	c := newToolsTestClient(func(_ context.Context, method string, params any, result any) error {
		calledMethod = method
		raw, _ := json.Marshal(params)
		_ = json.Unmarshal(raw, &calledParams)

		actions := []protocol.CodeAction{
			{
				Title: "Organize imports",
				Kind:  protocol.SourceOrganizeImports,
			 Edit: &protocol.WorkspaceEdit{
					Changes: map[protocol.DocumentURI][]protocol.TextEdit{},
				},
			},
		}
		raw, _ = json.Marshal(actions)
		return json.Unmarshal(raw, result)
	})
	openFileInClient(c, filePath)

	rng := protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   protocol.Position{Line: 20, Character: 0},
	}
	actions, err := c.CodeAction(t.Context(), filePath, rng, protocol.SourceOrganizeImports)
	require.NoError(t, err)
	require.Equal(t, "textDocument/codeAction", calledMethod)
	require.Len(t, actions, 1)
	require.Equal(t, "Organize imports", actions[0].Title)
	require.Equal(t, protocol.SourceOrganizeImports, actions[0].Kind)
	// Verify the context-only filter was sent.
	require.Len(t, calledParams.Context.Only, 1)
	require.Equal(t, protocol.SourceOrganizeImports, calledParams.Context.Only[0])
}

func TestCodeAction_EmptyKind(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")

	var calledParams protocol.CodeActionParams
	c := newToolsTestClient(func(_ context.Context, _ string, params any, result any) error {
		raw, _ := json.Marshal(params)
		_ = json.Unmarshal(raw, &calledParams)

		raw, _ = json.Marshal([]protocol.CodeAction{})
		return json.Unmarshal(raw, result)
	})
	openFileInClient(c, filePath)

	rng := protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   protocol.Position{Line: 10, Character: 0},
	}
	_, err := c.CodeAction(t.Context(), filePath, rng, "")
	require.NoError(t, err)
	require.Empty(t, calledParams.Context.Only, "empty kind should send no filter")
}

// ---------------------------------------------------------------------------
// Formatting / Completion
// ---------------------------------------------------------------------------

func TestFormatting_ParsesEdits(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")

	var calledMethod string
	c := newToolsTestClient(func(_ context.Context, method string, _ any, result any) error {
		calledMethod = method
		edits := []protocol.TextEdit{
			{
				Range: protocol.Range{
					Start: protocol.Position{Line: 0, Character: 0},
					End:   protocol.Position{Line: 0, Character: 0},
				},
				NewText: "package main\n\n",
			},
			{
				Range: protocol.Range{
					Start: protocol.Position{Line: 2, Character: 0},
					End:   protocol.Position{Line: 2, Character: 5},
				},
				NewText: "\t",
			},
		}
		raw, _ := json.Marshal(edits)
		return json.Unmarshal(raw, result)
	})
	openFileInClient(c, filePath)

	edits, err := c.Formatting(t.Context(), filePath)
	require.NoError(t, err)
	require.Equal(t, "textDocument/formatting", calledMethod)
	require.Len(t, edits, 2)
	require.Equal(t, "package main\n\n", edits[0].NewText)
}

func TestFormatting_ServerNotReady(t *testing.T) {
	t.Parallel()

	c := newToolsTestClient(nil)
	c.SetServerState(StateStopped)

	_, err := c.Formatting(t.Context(), "/tmp/test.go")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not ready")
}

func TestCompletion_ParsesItemList(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")

	var calledMethod string
	c := newToolsTestClient(func(_ context.Context, method string, _ any, result any) error {
		calledMethod = method
		items := []protocol.CompletionItem{
			{Label: "fmt.Println", Kind: protocol.FunctionCompletion},
			{Label: "fmt.Sprintf", Kind: protocol.FunctionCompletion},
		}
		// Simulate LSP returning a CompletionList wrapper.
		list := map[string]any{
			"isIncomplete": false,
			"items":        items,
		}
		raw, _ := json.Marshal(list)
		return json.Unmarshal(raw, result)
	})
	openFileInClient(c, filePath)

	items, err := c.Completion(t.Context(), filePath, 10, 5)
	require.NoError(t, err)
	require.Equal(t, "textDocument/completion", calledMethod)
	require.Len(t, items, 2)
	require.Equal(t, "fmt.Println", items[0].Label)
	require.Equal(t, protocol.FunctionCompletion, items[0].Kind)
}

func TestCompletion_PlainArrayResponse(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")
	c := newToolsTestClient(func(_ context.Context, _ string, _ any, result any) error {
		// Some LSP servers return a plain array of CompletionItem.
		items := []protocol.CompletionItem{
			{Label: "Foo", Kind: protocol.VariableCompletion},
		}
		raw, _ := json.Marshal(items)
		return json.Unmarshal(raw, result)
	})
	openFileInClient(c, filePath)

	items, err := c.Completion(t.Context(), filePath, 1, 1)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Foo", items[0].Label)
}

func TestCompletion_NilResult(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")
	c := newToolsTestClient(func(_ context.Context, _ string, _ any, _ any) error {
		return nil
	})
	openFileInClient(c, filePath)

	items, err := c.Completion(t.Context(), filePath, 1, 1)
	require.NoError(t, err)
	require.Empty(t, items)
}

// ---------------------------------------------------------------------------
// Workspace symbol search
// ---------------------------------------------------------------------------

func TestWorkspaceSymbol_ReturnsResults(t *testing.T) {
	t.Parallel()

	var calledMethod string
	c := newToolsTestClient(func(_ context.Context, method string, _ any, result any) error {
		calledMethod = method
		symbols := []protocol.SymbolInformation{
			{
				Name: "Handler",
				Kind: protocol.Function,
				Location: protocol.Location{
					URI: protocol.DocumentURI("file:///workspace/handler.go"),
					Range: protocol.Range{
						Start: protocol.Position{Line: 15, Character: 5},
						End:   protocol.Position{Line: 15, Character: 12},
					},
				},
			},
		}
		raw, _ := json.Marshal(symbols)
		return json.Unmarshal(raw, result)
	})

	symbols, err := c.WorkspaceSymbol(t.Context(), "Handler")
	require.NoError(t, err)
	require.Equal(t, "workspace/symbol", calledMethod)
	require.Len(t, symbols, 1)
	require.Equal(t, "Handler", symbols[0].Name)
}

func TestWorkspaceSymbol_NilResult(t *testing.T) {
	t.Parallel()

	c := newToolsTestClient(func(_ context.Context, _ string, _ any, _ any) error {
		return nil
	})

	symbols, err := c.WorkspaceSymbol(t.Context(), "nonexistent")
	require.NoError(t, err)
	require.Nil(t, symbols)
}

// ---------------------------------------------------------------------------
// Signature help
// ---------------------------------------------------------------------------

func TestSignatureHelp_ReturnsSignature(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")

	var calledMethod string
	c := newToolsTestClient(func(_ context.Context, method string, _ any, result any) error {
		calledMethod = method
		help := protocol.SignatureHelp{
			Signatures: []protocol.SignatureInformation{
				{
					Label: "Printf(format string, a ...any) (n int, err error)",
				},
			},
			ActiveSignature: 0,
			ActiveParameter: 1,
		}
		raw, _ := json.Marshal(help)
		return json.Unmarshal(raw, result)
	})
	openFileInClient(c, filePath)

	help, err := c.SignatureHelp(t.Context(), filePath, 5, 10)
	require.NoError(t, err)
	require.Equal(t, "textDocument/signatureHelp", calledMethod)
	require.NotNil(t, help)
	require.Len(t, help.Signatures, 1)
	require.Contains(t, help.Signatures[0].Label, "Printf")
}

func TestSignatureHelp_NilResult(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")
	c := newToolsTestClient(func(_ context.Context, _ string, _ any, _ any) error {
		return nil
	})
	openFileInClient(c, filePath)

	help, err := c.SignatureHelp(t.Context(), filePath, 1, 1)
	require.NoError(t, err)
	require.Nil(t, help)
}

// ---------------------------------------------------------------------------
// Crash recovery integration
// ---------------------------------------------------------------------------

func TestCrashRecovery_ToolMethodGracefulFailure(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")

	// Simulate a server that is in error state after a crash.
	c := newToolsTestClient(nil)
	c.SetServerState(StateError)

	// All tool methods should fail gracefully, not panic.
	_, defErr := c.Definition(t.Context(), filePath, 1, 1)
	require.Error(t, defErr)

	_, hoverErr := c.Hover(t.Context(), filePath, 1, 1)
	require.Error(t, hoverErr)

	_, renameErr := c.Rename(t.Context(), filePath, 1, 1, "x")
	require.Error(t, renameErr)

	rng := protocol.Range{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 1, Character: 0}}
	_, actionErr := c.CodeAction(t.Context(), filePath, rng, "")
	require.Error(t, actionErr)

	_, fmtErr := c.Formatting(t.Context(), filePath)
	require.Error(t, fmtErr)

	_, compErr := c.Completion(t.Context(), filePath, 1, 1)
	require.Error(t, compErr)
}

func TestCrashRecovery_ServerCrashTriggersRestart(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	crashCount := 2

	cr := NewCrashRecovery("crashy-server", ExponentialBackoff{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     5 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      5,
	}, func(ctx context.Context) error {
		n := attempts.Add(1)
		if n <= int32(crashCount) {
			return ErrServerCrashed
		}
		return nil
	})

	err := cr.Run(context.Background())
	require.NoError(t, err)
	require.Equal(t, int32(crashCount+1), attempts.Load())
	require.True(t, cr.LastCrashed())
}

func TestCrashRecovery_NonCrashErrorNoRetry(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	cr := NewCrashRecovery("err-server", ExponentialBackoff{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     5 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      5,
	}, func(ctx context.Context) error {
		attempts.Add(1)
		return ErrClientNotFound // Non-crash error.
	})

	err := cr.Run(context.Background())
	require.ErrorIs(t, err, ErrClientNotFound)
	require.Equal(t, int32(1), attempts.Load())
	require.False(t, cr.LastCrashed())
}

func TestCrashRecovery_MaxRetriesExceeded(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	cr := NewCrashRecovery("doomed-server", ExponentialBackoff{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     5 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      2,
	}, func(ctx context.Context) error {
		attempts.Add(1)
		return ErrServerCrashed
	})

	err := cr.Run(context.Background())
	require.ErrorIs(t, err, ErrMaxRestartsExceeded)
	require.Equal(t, int32(3), attempts.Load(), "initial + 2 retries")
}

// ---------------------------------------------------------------------------
// IsAlive health check
// ---------------------------------------------------------------------------

func TestIsAlive_WithHealthCheck(t *testing.T) {
	t.Parallel()

	c := &Client{
		xrushClientFields: xrushClientFields{
			healthCheck: func() bool { return true },
		},
	}
	require.True(t, c.IsAlive())

	c.healthCheck = func() bool { return false }
	require.False(t, c.IsAlive())
}

func TestIsAlive_NilHealthCheck_NilClient(t *testing.T) {
	t.Parallel()

	c := &Client{}
	require.False(t, c.IsAlive())
}

// ---------------------------------------------------------------------------
// CallLSP not configured
// ---------------------------------------------------------------------------

func TestToolMethods_CallNotConfigured(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "main.go")

	// Client with no callLSP set (nil).
	c := newToolsTestClient(nil)
	openFileInClient(c, filePath)

	_, defErr := c.Definition(t.Context(), filePath, 1, 1)
	require.Error(t, defErr)
	require.Contains(t, defErr.Error(), "does not support direct protocol calls")

	_, hoverErr := c.Hover(t.Context(), filePath, 1, 1)
	require.Error(t, hoverErr)

	_, renameErr := c.Rename(t.Context(), filePath, 1, 1, "x")
	require.Error(t, renameErr)

	rng := protocol.Range{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 1, Character: 0}}
	_, actionErr := c.CodeAction(t.Context(), filePath, rng, "")
	require.Error(t, actionErr)

	_, fmtErr := c.Formatting(t.Context(), filePath)
	require.Error(t, fmtErr)

	_, compErr := c.Completion(t.Context(), filePath, 1, 1)
	require.Error(t, compErr)

	_, symErr := c.DocumentSymbols(t.Context(), filePath)
	require.Error(t, symErr)

	_, sigErr := c.SignatureHelp(t.Context(), filePath, 1, 1)
	require.Error(t, sigErr)

	_, wsErr := c.WorkspaceSymbol(t.Context(), "test")
	require.Error(t, wsErr)
}

// ---------------------------------------------------------------------------
// parseLocationArray / parseCompletionResult unit tests
// ---------------------------------------------------------------------------

func TestParseLocationArray_Nil(t *testing.T) {
	t.Parallel()

	locs, err := parseLocationArray(nil)
	require.NoError(t, err)
	require.Nil(t, locs)
}

func TestParseLocationArray_Valid(t *testing.T) {
	t.Parallel()

	input := []map[string]any{
		{
			"uri": "file:///test.go",
			"range": map[string]any{
				"start": map[string]any{"line": float64(1), "character": float64(0)},
				"end":   map[string]any{"line": float64(1), "character": float64(5)},
			},
		},
	}
	locs, err := parseLocationArray(input)
	require.NoError(t, err)
	require.Len(t, locs, 1)
	require.Equal(t, protocol.DocumentURI("file:///test.go"), locs[0].URI)
}

func TestParseCompletionResult_Nil(t *testing.T) {
	t.Parallel()

	var items []protocol.CompletionItem
	err := parseCompletionResult(nil, &items)
	require.NoError(t, err)
	require.Nil(t, items)
}

func TestParseCompletionResult_PlainArray(t *testing.T) {
	t.Parallel()

	input := []any{
		map[string]any{"label": "foo", "kind": float64(3)},
	}
	var items []protocol.CompletionItem
	err := parseCompletionResult(input, &items)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "foo", items[0].Label)
}

func TestParseCompletionResult_CompletionListMap(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"isIncomplete": false,
		"items": []any{
			map[string]any{"label": "bar"},
		},
	}
	var items []protocol.CompletionItem
	err := parseCompletionResult(input, &items)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "bar", items[0].Label)
}

// ---------------------------------------------------------------------------
// remarshal helper
// ---------------------------------------------------------------------------

func TestRemarshal_Nil(t *testing.T) {
	t.Parallel()

	var dst map[string]string
	err := remarshal(nil, &dst)
	require.NoError(t, err)
	require.Nil(t, dst)
}

func TestRemarshal_RoundTrip(t *testing.T) {
	t.Parallel()

	src := map[string]any{"key": "value", "num": float64(42)}
	var dst map[string]any
	err := remarshal(src, &dst)
	require.NoError(t, err)
	require.Equal(t, "value", dst["key"])
}

// ---------------------------------------------------------------------------
// FindReferences
// ---------------------------------------------------------------------------

func TestFindReferences_RequiresPowernapClient(t *testing.T) {
	t.Parallel()

	c := newTestClient()
	c.SetServerState(StateReady)

	// FindReferences uses the powernap client directly, not callLSP.
	// Without a real powernap client it panics, so we test that it's not
	// callable via our mock setup. This documents the architectural gap.
	require.Panics(t, func() {
		_, _ = c.FindReferences(t.Context(), "/tmp/test.go", 1, 1, true)
	})
}

// ---------------------------------------------------------------------------
// Diagnostic handling with mock callback
// ---------------------------------------------------------------------------

func TestDiagnostics_CallbackOnPublish(t *testing.T) {
	t.Parallel()

	c := newTestClient()

	var callbackCount atomic.Int32
	c.SetDiagnosticsCallback(func(name string, count int) {
		callbackCount.Add(1)
	})

	params := json.RawMessage(`{
		"uri": "file:///workspace/main.go",
		"diagnostics": [
			{"message": "unused variable", "severity": 2}
		]
	}`)
	HandleDiagnostics(c, params)

	require.Eventually(t, func() bool {
		return callbackCount.Load() == 1
	}, time.Second, 50*time.Millisecond)

	diags := c.GetDiagnostics()
	require.Len(t, diags, 1)
}

func TestDiagnostics_CountsBySeverity(t *testing.T) {
	t.Parallel()

	c := newTestClient()

	params := json.RawMessage(`{
		"uri": "file:///workspace/main.go",
		"diagnostics": [
			{"message": "err1", "severity": 1},
			{"message": "warn1", "severity": 2},
			{"message": "info1", "severity": 3},
			{"message": "hint1", "severity": 4}
		]
	}`)
	HandleDiagnostics(c, params)

	counts := c.GetDiagnosticCounts()
	require.Equal(t, 1, counts.Error)
	require.Equal(t, 1, counts.Warning)
	require.Equal(t, 1, counts.Information)
	require.Equal(t, 1, counts.Hint)
}

// ---------------------------------------------------------------------------
// OpenFile / HandlesFile with temp directory
// ---------------------------------------------------------------------------

func TestOpenFile_WithRealFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.go")
	require.NoError(t, os.WriteFile(filePath, []byte("package main\n"), 0o644))

	c := &Client{
		name:      "test",
		cwd:       tmpDir,
		fileTypes: []string{".go"},
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
	}
	c.serverState.Store(StateReady)

	// Without a real LSP server, OpenFile will fail when trying to notify.
	// But HandlesFile should work.
	require.True(t, c.HandlesFile(filePath))
	require.False(t, c.HandlesFile(filepath.Join(tmpDir, "test.py")))
}

// ---------------------------------------------------------------------------
// Server state transitions
// ---------------------------------------------------------------------------

func TestServerStateTransitions(t *testing.T) {
	t.Parallel()

	c := newTestClient()

	states := []ServerState{StateStarting, StateReady, StateError, StateStopped, StateDisabled}
	for _, s := range states {
		c.SetServerState(s)
		require.Equal(t, s, c.GetServerState())
	}
}

