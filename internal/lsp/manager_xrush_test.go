package lsp

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	powernapconfig "github.com/charmbracelet/x/powernap/pkg/config"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
	"github.com/stretchr/testify/require"
)

func TestStartAllConcurrentStartup(t *testing.T) {
	t.Parallel()

	pnMgr := powernapconfig.NewManager()
	pnMgr.LoadDefaults()

	mgr := &Manager{
		clients:     csync.NewMap[string, *Client](),
		unavailable: csync.NewMap[string, *serverRetryState](),
		manager:     pnMgr,
		backoff:     DefaultBackoff(),
		now:         time.Now,
		callback:    func(string, *Client) {},
	}

	err := mgr.StartAll(context.Background())
	require.NoError(t, err)
}

func TestSaveAllCachesRoundTrip(t *testing.T) {
	t.Parallel()

	c := &Client{
		openFiles: csync.NewMap[string, *OpenFileInfo](),
	}
	c.serverState.Store(StateReady)

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		callback: func(string, *Client) {},
	}
	mgr.clients.Set("test", c)

	err := mgr.SaveAllCaches(context.Background())
	require.NoError(t, err)
}

func TestRestartLanguageServerPreservesCache(t *testing.T) {
	t.Parallel()

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		callback: func(string, *Client) {},
	}

	err := mgr.RestartLanguageServer(context.Background(), "nonexistent")
	require.ErrorIs(t, err, ErrClientNotFound)
}

func TestRequestFullSymbolTree(t *testing.T) {
	t.Parallel()

	tmpdir := t.TempDir()
	tmpfile, err := os.CreateTemp(tmpdir, "main.*.go")
	require.NoError(t, err)
	tmpPath := tmpfile.Name()
	_, err = tmpfile.WriteString("package main\nfunc main() {}\n")
	require.NoError(t, err)
	tmpfile.Close()

	var calls atomic.Int32

	c := &Client{
		name:        "test",
		fileTypes:   []string{".go"},
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
		xrushClientFields: xrushClientFields{
			callLSP: func(_ context.Context, _ string, _ any, result any) error {
				calls.Add(1)
				*(result.(*any)) = []protocol.DocumentSymbol{
					{
						Name: "main",
						Kind: protocol.Function,
						Range: protocol.Range{
							Start: protocol.Position{Line: 0, Character: 0},
							End:   protocol.Position{Line: 1, Character: 0},
						},
					},
				}
				return nil
			},
		},
		cwd:    tmpdir,
		config: config.LSPConfig{FileTypes: []string{".go"}},
	}
	c.serverState.Store(StateReady)

	uri := string(protocol.URIFromPath(tmpPath))
	c.openFiles.Set(uri, &OpenFileInfo{Version: 1, URI: protocol.DocumentURI(uri)})

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		executor: NewTaskExecutor(0),
		callback: func(string, *Client) {},
	}
	mgr.executor.Start()
	t.Cleanup(mgr.executor.Stop)
	mgr.clients.Set("gopls", c)

	symbols, err := mgr.RequestFullSymbolTree(context.Background(), uri)
	require.NoError(t, err)
	require.Len(t, symbols, 1)
	require.Equal(t, "main", symbols[0].Name)
	require.Equal(t, int32(1), calls.Load())
}

func TestRequestFullSymbolTree_NoClient(t *testing.T) {
	t.Parallel()

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		callback: func(string, *Client) {},
	}

	symbols, err := mgr.RequestFullSymbolTree(context.Background(), "file:///workspace/main.go")
	require.ErrorIs(t, err, ErrClientNotFound)
	require.Nil(t, symbols)
}

func TestStartServerWaitForReadyCleansUpZombie(t *testing.T) {
	t.Parallel()

	mgr := &Manager{
		clients:     csync.NewMap[string, *Client](),
		unavailable: csync.NewMap[string, *serverRetryState](),
		backoff:     DefaultBackoff(),
		now:         time.Now,
		callback:    func(string, *Client) {},
	}

	zombie := &Client{
		name:        "zombie-lsp",
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
	}
	zombie.serverState.Store(StateError)
	mgr.clients.Set("zombie-lsp", zombie)

	_, exists := mgr.clients.Get("zombie-lsp")
	require.True(t, exists, "precondition: zombie should be in clients map")

	mgr.clients.Del("zombie-lsp")
	mgr.markUnavailable("zombie-lsp")

	_, exists = mgr.clients.Get("zombie-lsp")
	require.False(t, exists, "zombie client should be removed from clients map")
	require.True(t, mgr.recentlyUnavailable("zombie-lsp"), "server should be marked unavailable for backoff")
}

func TestStartServerCleansUpOnWaitForReadyError(t *testing.T) {
	t.Parallel()

	mgr := &Manager{
		clients:     csync.NewMap[string, *Client](),
		unavailable: csync.NewMap[string, *serverRetryState](),
		backoff:     DefaultBackoff(),
		now:         time.Now,
		callback:    func(string, *Client) {},
	}

	_, exists := mgr.clients.Get("any-server")
	require.False(t, exists)
	require.False(t, mgr.recentlyUnavailable("any-server"))

	mgr.markUnavailable("test-server")
	require.True(t, mgr.recentlyUnavailable("test-server"))

	mgr.clearUnavailable("test-server")
	require.False(t, mgr.recentlyUnavailable("test-server"))
}

func TestRenameForServer(t *testing.T) {
	t.Parallel()

	tmpdir := t.TempDir()
	tmpfile, err := os.CreateTemp(tmpdir, "main.*.go")
	require.NoError(t, err)
	tmpPath := tmpfile.Name()
	_, err = tmpfile.WriteString("package main\nfunc main() {}\n")
	require.NoError(t, err)
	tmpfile.Close()

	var calls atomic.Int32

	c := &Client{
		name:        "gopls",
		fileTypes:   []string{".go"},
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
		xrushClientFields: xrushClientFields{
			callLSP: func(_ context.Context, _ string, _ any, result any) error {
				calls.Add(1)
				*(result.(*any)) = map[string]any{
					"changes": map[string]any{},
				}
				return nil
			},
		},
		cwd:    tmpdir,
		config: config.LSPConfig{FileTypes: []string{".go"}},
	}
	c.serverState.Store(StateReady)

	uri := string(protocol.URIFromPath(tmpPath))
	c.openFiles.Set(uri, &OpenFileInfo{Version: 1, URI: protocol.DocumentURI(uri)})

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		executor: NewTaskExecutor(0),
		callback: func(string, *Client) {},
	}
	mgr.executor.Start()
	t.Cleanup(mgr.executor.Stop)
	mgr.clients.Set("gopls", c)

	edit, err := mgr.RenameForServer(context.Background(), "gopls", tmpPath, 1, 5, "newMain")
	require.NoError(t, err)
	require.NotNil(t, edit)
	require.Equal(t, int32(1), calls.Load())
}

func TestRenameForServerClientNotFound(t *testing.T) {
	t.Parallel()

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		executor: NewTaskExecutor(0),
		callback: func(string, *Client) {},
	}
	mgr.executor.Start()
	t.Cleanup(mgr.executor.Stop)

	edit, err := mgr.RenameForServer(context.Background(), "missing", "/tmp/test.go", 1, 1, "newName")
	require.ErrorIs(t, err, ErrClientNotFound)
	require.Nil(t, edit)
}

func TestSafeDeleteForServerClientNotFound(t *testing.T) {
	t.Parallel()

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		executor: NewTaskExecutor(0),
		callback: func(string, *Client) {},
	}
	mgr.executor.Start()
	t.Cleanup(mgr.executor.Stop)

	locs, err := mgr.SafeDeleteForServer(context.Background(), "missing", "/tmp/test.go", 1, 1)
	require.ErrorIs(t, err, ErrClientNotFound)
	require.Nil(t, locs)
}

func TestFindClientForFile(t *testing.T) {
	t.Parallel()

	tmpdir := t.TempDir()

	goClient := &Client{
		name:      "gopls",
		fileTypes: []string{".go"},
		cwd:       tmpdir,
		config:    config.LSPConfig{FileTypes: []string{".go"}},
	}
	goClient.serverState.Store(StateReady)

	tsClient := &Client{
		name:      "typescript-language-server",
		fileTypes: []string{".ts"},
		cwd:       tmpdir,
		config:    config.LSPConfig{FileTypes: []string{".ts"}},
	}
	tsClient.serverState.Store(StateReady)

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		callback: func(string, *Client) {},
	}
	mgr.clients.Set("gopls", goClient)
	mgr.clients.Set("typescript-language-server", tsClient)

	name, client := mgr.FindClientForFile(tmpdir + "/main.go")
	require.Equal(t, "gopls", name)
	require.NotNil(t, client)

	name, client = mgr.FindClientForFile(tmpdir + "/app.ts")
	require.Equal(t, "typescript-language-server", name)
	require.NotNil(t, client)

	name, client = mgr.FindClientForFile(tmpdir + "/unknown.py")
	require.Equal(t, "", name)
	require.Nil(t, client)
}

func TestCrashRecoveryFieldDefaultsEnabled(t *testing.T) {
	t.Parallel()

	mgr := &Manager{
		clients:       csync.NewMap[string, *Client](),
		unavailable:   csync.NewMap[string, *serverRetryState](),
		backoff:       DefaultBackoff(),
		now:           time.Now,
		callback:      func(string, *Client) {},
		crashRecovery: true,
	}
	require.True(t, mgr.crashRecovery)
}

func TestCrashRecoveryDisabled(t *testing.T) {
	t.Parallel()

	mgr := &Manager{
		clients:       csync.NewMap[string, *Client](),
		unavailable:   csync.NewMap[string, *serverRetryState](),
		backoff:       DefaultBackoff(),
		now:           time.Now,
		callback:      func(string, *Client) {},
		crashRecovery: false,
	}
	require.False(t, mgr.crashRecovery)
}

func TestCrashRecoveryRestartsOnCrash(t *testing.T) {
	t.Parallel()

	attempts := 0
	cr := NewCrashRecovery("test-server", ExponentialBackoff{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     5 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      3,
	}, func(_ context.Context) error {
		attempts++
		if attempts < 2 {
			return ErrServerCrashed
		}
		return nil
	})

	err := cr.Run(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, cr.Attempts())
	require.True(t, cr.LastCrashed())
}

func TestCrashRecoveryExhaustsRetries(t *testing.T) {
	t.Parallel()

	cr := NewCrashRecovery("test-server", ExponentialBackoff{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     5 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      2,
	}, func(_ context.Context) error {
		return ErrServerCrashed
	})

	err := cr.Run(context.Background())
	require.ErrorIs(t, err, ErrMaxRestartsExceeded)
}

func TestCrashRecoveryNonCrashErrorPropagates(t *testing.T) {
	t.Parallel()

	cr := NewCrashRecovery("test-server", DefaultBackoff(), func(_ context.Context) error {
		return context.DeadlineExceeded
	})

	err := cr.Run(context.Background())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, cr.Attempts())
	require.False(t, cr.LastCrashed())
}
