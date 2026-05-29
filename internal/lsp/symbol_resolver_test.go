//go:build treesitter

package lsp

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
	"github.com/stretchr/testify/require"
)

func TestParseQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		wantMode     ResolutionMode
		wantName     string
		wantPkg      string
		wantModule   string
		wantOverload int
	}{
		{
			name:         "simple name",
			input:        "foo",
			wantMode:     ModeSimple,
			wantName:     "foo",
			wantOverload: -1,
		},
		{
			name:         "relative package.symbol",
			input:        "pkg.foo",
			wantMode:     ModeRelative,
			wantName:     "foo",
			wantPkg:      "pkg",
			wantOverload: -1,
		},
		{
			name:         "absolute module path",
			input:        "github.com/org/repo/pkg.foo",
			wantMode:     ModeAbsolute,
			wantName:     "foo",
			wantModule:   "github.com/org/repo/pkg",
			wantOverload: -1,
		},
		{
			name:         "simple with overload index",
			input:        "foo[2]",
			wantMode:     ModeSimple,
			wantName:     "foo",
			wantOverload: 2,
		},
		{
			name:         "relative with overload index",
			input:        "pkg.foo[0]",
			wantMode:     ModeRelative,
			wantName:     "foo",
			wantPkg:      "pkg",
			wantOverload: 0,
		},
		{
			name:         "absolute with overload index",
			input:        "github.com/org/repo/pkg.foo[3]",
			wantMode:     ModeAbsolute,
			wantName:     "foo",
			wantModule:   "github.com/org/repo/pkg",
			wantOverload: 3,
		},
		{
			name:         "whitespace trimmed",
			input:        "  foo  ",
			wantMode:     ModeSimple,
			wantName:     "foo",
			wantOverload: -1,
		},
		{
			name:         "absolute without dot after slash",
			input:        "github.com/org/repo",
			wantMode:     ModeAbsolute,
			wantName:     "repo",
			wantModule:   "github.com/org",
			wantOverload: -1,
		},
		{
			name:         "nested relative",
			input:        "subpkg.inner.MyFunc",
			wantMode:     ModeRelative,
			wantName:     "MyFunc",
			wantPkg:      "subpkg.inner",
			wantOverload: -1,
		},
		{
			name:         "empty overload brackets ignored",
			input:        "foo[]",
			wantMode:     ModeSimple,
			wantName:     "foo[]",
			wantOverload: -1,
		},
		{
			name:         "non-numeric overload ignored",
			input:        "foo[abc]",
			wantMode:     ModeSimple,
			wantName:     "foo[abc]",
			wantOverload: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ParseQuery(tt.input)
			require.Equal(t, tt.wantMode, got.Mode)
			require.Equal(t, tt.wantName, got.Name)
			require.Equal(t, tt.wantPkg, got.Package)
			require.Equal(t, tt.wantModule, got.ModuleRoot)
			require.Equal(t, tt.wantOverload, got.OverloadIndex)
		})
	}
}

func TestSymbolResolverModes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	uri := protocol.URIFromPath("/workspace/main.go")

	c := &Client{
		name:        "test",
		fileTypes:   []string{".go"},
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
		xrushClientFields: xrushClientFields{
			callLSP: func(_ context.Context, method string, _ any, result any) error {
				if method != "workspace/symbol" {
					return nil
				}
				*(result.(*any)) = []protocol.SymbolInformation{
					{
						Name:          "Foo",
						Kind:          protocol.Class,
						ContainerName: "github.com/org/repo/pkg",
						Location: protocol.Location{
							URI: uri,
							Range: protocol.Range{
								Start: protocol.Position{Line: 10, Character: 5},
								End:   protocol.Position{Line: 20, Character: 1},
							},
						},
					},
					{
						Name:          "Bar",
						Kind:          protocol.Function,
						ContainerName: "other",
						Location: protocol.Location{
							URI: uri,
							Range: protocol.Range{
								Start: protocol.Position{Line: 30, Character: 0},
								End:   protocol.Position{Line: 35, Character: 0},
							},
						},
					},
				}
				return nil
			},
		},
	}
	c.serverState.Store(StateReady)

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		executor: NewTaskExecutor(0),
		callback: func(string, *Client) {},
	}
	mgr.executor.Start()
	t.Cleanup(mgr.executor.Stop)
	mgr.clients.Set("gopls", c)

	resolver := NewSymbolResolver(mgr)

	t.Run("simple mode filters by current package", func(t *testing.T) {
		t.Parallel()
		candidates, err := resolver.Resolve(ctx, "Foo", "pkg", "")
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		require.Equal(t, "Foo", candidates[0].Name)
		require.Equal(t, "class", candidates[0].Kind)
	})

	t.Run("relative mode filters by package name", func(t *testing.T) {
		t.Parallel()
		candidates, err := resolver.Resolve(ctx, "other.Bar", "", "")
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		require.Equal(t, "Bar", candidates[0].Name)
		require.Equal(t, "function", candidates[0].Kind)
	})

	t.Run("absolute mode filters by module root", func(t *testing.T) {
		t.Parallel()
		candidates, err := resolver.Resolve(ctx, "github.com/org/repo/pkg.Foo", "", "github.com/org/repo/pkg")
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		require.Equal(t, "Foo", candidates[0].Name)
	})
}

func TestOverloadIndex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	uri := protocol.URIFromPath("/workspace/main.go")

	c := &Client{
		name:        "test",
		fileTypes:   []string{".go"},
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
		xrushClientFields: xrushClientFields{
			callLSP: func(_ context.Context, method string, _ any, result any) error {
				if method != "workspace/symbol" {
					return nil
				}
				*(result.(*any)) = []protocol.SymbolInformation{
					{
						Name:          "process",
						Kind:          protocol.Function,
						ContainerName: "pkg",
						Location: protocol.Location{
							URI:   uri,
							Range: protocol.Range{Start: protocol.Position{Line: 1}, End: protocol.Position{Line: 2}},
						},
					},
					{
						Name:          "process",
						Kind:          protocol.Function,
						ContainerName: "pkg",
						Location: protocol.Location{
							URI:   uri,
							Range: protocol.Range{Start: protocol.Position{Line: 10}, End: protocol.Position{Line: 20}},
						},
					},
					{
						Name:          "process",
						Kind:          protocol.Function,
						ContainerName: "pkg",
						Location: protocol.Location{
							URI:   uri,
							Range: protocol.Range{Start: protocol.Position{Line: 30}, End: protocol.Position{Line: 40}},
						},
					},
				}
				return nil
			},
		},
	}
	c.serverState.Store(StateReady)

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		executor: NewTaskExecutor(0),
		callback: func(string, *Client) {},
	}
	mgr.executor.Start()
	t.Cleanup(mgr.executor.Stop)
	mgr.clients.Set("gopls", c)

	resolver := NewSymbolResolver(mgr)

	candidates, err := resolver.Resolve(ctx, "process[1]", "pkg", "")
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, uint32(10), candidates[0].Location.Range.Start.Line)
}

func TestOverloadIndexOutOfRange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	uri := protocol.URIFromPath("/workspace/main.go")

	c := &Client{
		name:        "test",
		fileTypes:   []string{".go"},
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
		xrushClientFields: xrushClientFields{
			callLSP: func(_ context.Context, method string, _ any, result any) error {
				if method != "workspace/symbol" {
					return nil
				}
				*(result.(*any)) = []protocol.SymbolInformation{
					{
						Name:          "Foo",
						Kind:          protocol.Function,
						ContainerName: "pkg",
						Location: protocol.Location{
							URI:   uri,
							Range: protocol.Range{Start: protocol.Position{Line: 1}, End: protocol.Position{Line: 2}},
						},
					},
				}
				return nil
			},
		},
	}
	c.serverState.Store(StateReady)

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		executor: NewTaskExecutor(0),
		callback: func(string, *Client) {},
	}
	mgr.executor.Start()
	t.Cleanup(mgr.executor.Stop)
	mgr.clients.Set("gopls", c)

	resolver := NewSymbolResolver(mgr)

	candidates, err := resolver.Resolve(ctx, "Foo[5]", "pkg", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "overload index 5 out of range")
	require.Nil(t, candidates)
}
