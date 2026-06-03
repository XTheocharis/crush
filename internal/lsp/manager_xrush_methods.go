package lsp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"sort"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/lsp/catalog"
	powernapconfig "github.com/charmbracelet/x/powernap/pkg/config"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
	"golang.org/x/sync/errgroup"
)

// [XRUSH: begin: server priority system]
// Startup priority levels control the order in which LSP servers are started.
const (
	PriorityCritical = 0 // Essential language servers (gopls, rust-analyzer, etc.).
	PriorityHigh     = 1
	PriorityNormal   = 2 // Default priority for unrecognised servers.
	PriorityLow      = 3
)

// ErrClientNotFound is returned for operations on non-running servers.
var ErrClientNotFound = errors.New("lsp: client not found")

// Critical servers for startup ordering.
var criticalServers = map[string]bool{
	"gopls":                      true,
	"typescript-language-server": true,
	"vscode-css-langserver":      true,
	"vscode-html-languageserver": true,
	"json-language-server":       true,
	"yaml-language-server":       true,
	"rust-analyzer":              true,
	"pyright":                    true,
	"clangd":                     true,
	"kotlin-language-server":     true,
	"csharp-ls":                  true,
	"dockerfile-languageserver":  true,
}

// serverPriority returns the startup priority for the given server name.
func serverPriority(name string) int {
	if criticalServers[name] {
		return PriorityCritical
	}
	return PriorityNormal
}

// [XRUSH: end]

// [XRUSH: begin: auto-download LSP servers]
func (s *Manager) resolveAutoDownload(ctx context.Context, name string, server *powernapconfig.ServerConfig) {
	if userCfg, ok := s.cfg.Config().LSP[name]; ok && userCfg.AutoDownload != nil && userCfg.AutoDownload.URL != "" {
		resolved, dlErr := ResolveDownloadPath(ctx, name, server.Command, *userCfg.AutoDownload)
		if dlErr != nil {
			slog.Warn("Auto-download failed for LSP server", "name", name, "error", dlErr)
		} else if resolved != server.Command {
			server.Command = resolved
		}
		return
	}

	url, sha256, downloadType, ok := catalog.ResolveDownloadURL(name, runtime.GOOS, runtime.GOARCH)
	if ok {
		cfg := config.AutoDownloadConfig{URL: url, SHA256: sha256, DownloadType: downloadType}
		resolved, dlErr := ResolveDownloadPath(ctx, name, server.Command, cfg)
		if dlErr != nil {
			slog.Debug("Catalog auto-download failed for LSP server", "name", name, "error", dlErr)
		} else if resolved != server.Command {
			server.Command = resolved
		}
		return
	}

	installCfg, ok := catalog.ResolveInstallMethod(name)
	if ok {
		s.resolveViaInstaller(ctx, name, server, installCfg)
		return
	}
}

// [XRUSH: end]

// [XRUSH: begin: user match patterns for LSP]
func (s *Manager) userMatchPatterns(name string) []string {
	if userCfg, ok := s.cfg.Config().LSP[name]; ok {
		return userCfg.MatchPatterns
	}
	return nil
}

// [XRUSH: end]

// serverEntry pairs a server name with its config for sorted iteration.
type serverEntry struct {
	Name   string
	Config *powernapconfig.ServerConfig
}

// SortServersByPriority sorts LSP servers by startup priority.
// Critical servers (priority 0) appear first.
func sortServersByPriority(servers map[string]*powernapconfig.ServerConfig) []serverEntry {
	entries := make([]serverEntry, 0, len(servers))
	for name, cfg := range servers {
		entries = append(entries, serverEntry{Name: name, Config: cfg})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return serverPriority(entries[i].Name) < serverPriority(entries[j].Name)
	})
	return entries
}

// Close stops the task executor and all running LSP clients. This should be
// called when the manager is being shut down.
func (s *Manager) Close(ctx context.Context) {
	if s.healthChecker != nil {
		s.healthChecker.Stop()
	}
	s.executor.Stop()
	s.StopAll(ctx)
}

// GetDiagnosticsForServer returns diagnostics from the named LSP server,
// serialised through the task executor so that at most one request per server
// is in-flight at a time. Returns nil if the server is not running.
func (s *Manager) GetDiagnosticsForServer(ctx context.Context, name string) map[protocol.DocumentURI][]protocol.Diagnostic {
	client, ok := s.clients.Get(name)
	if !ok {
		return nil
	}

	var result map[protocol.DocumentURI][]protocol.Diagnostic
	err := s.executor.Submit(ctx, name, func() error {
		result = client.GetDiagnostics()
		return nil
	})
	if err != nil {
		return nil
	}
	return result
}

// FindReferencesForServer finds all references to the symbol at the given
// position using the named LSP server. The call is serialised through the
// task executor. Returns an error if the server is not running.
func (s *Manager) FindReferencesForServer(ctx context.Context, name, filepath string, line, character int, includeDeclaration bool) ([]protocol.Location, error) {
	client, ok := s.clients.Get(name)
	if !ok {
		return nil, ErrClientNotFound
	}

	var result []protocol.Location
	err := s.executor.Submit(ctx, name, func() error {
		var ferr error
		result, ferr = client.FindReferences(ctx, filepath, line, character, includeDeclaration)
		return ferr
	})
	return result, err
}

// RenameForServer renames the symbol at the given position using the named
// LSP server. The call is serialised through the task executor. Returns
// ErrClientNotFound if the server is not running.
func (s *Manager) RenameForServer(ctx context.Context, name, filepath string, line, character int, newName string) (*protocol.WorkspaceEdit, error) {
	client, ok := s.clients.Get(name)
	if !ok {
		return nil, ErrClientNotFound
	}

	var result *protocol.WorkspaceEdit
	err := s.executor.Submit(ctx, name, func() error {
		var ferr error
		result, ferr = client.Rename(ctx, filepath, line, character, newName)
		return ferr
	})
	return result, err
}

// SafeDeleteForServer checks whether the symbol at the given position can be
// safely deleted by querying references through the named LSP server. The call
// is serialised through the task executor. Returns ErrClientNotFound if the
// server is not running.
func (s *Manager) SafeDeleteForServer(ctx context.Context, name, filepath string, line, character int) ([]protocol.Location, error) {
	client, ok := s.clients.Get(name)
	if !ok {
		return nil, ErrClientNotFound
	}

	var result []protocol.Location
	err := s.executor.Submit(ctx, name, func() error {
		var ferr error
		result, ferr = client.FindReferences(ctx, filepath, line, character, true)
		return ferr
	})
	return result, err
}

// FindClientForFile returns the server name and client that handles the given
// file path, or ("", nil) if no match is found.
func (s *Manager) FindClientForFile(absPath string) (string, *Client) {
	for name, client := range s.clients.Seq2() {
		if client.HandlesFile(absPath) {
			return name, client
		}
	}
	return "", nil
}

// StartAll starts all configured LSP servers concurrently using an errgroup.
// Only servers that are not already running are started. It does not block on
// individual server readiness.
func (s *Manager) StartAll(ctx context.Context) error {
	if s.cfg == nil || s.manager == nil {
		return nil
	}
	servers := s.manager.GetServers()
	if len(servers) == 0 {
		return nil
	}

	eg, egCtx := errgroup.WithContext(ctx)

	for name, server := range servers {
		cfg := s.buildConfig(name, server)
		if cfg.Disabled {
			continue
		}
		if s.isUserConfigured(name) {
			if _, ok := s.clients.Get(name); ok {
				continue
			}
		} else {
			if s.recentlyUnavailable(name) {
				continue
			}
		}

		n, sc := name, server
		eg.Go(func() error {
			s.startServer(egCtx, n, s.cfg.WorkingDir(), sc)
			return nil
		})
	}

	return eg.Wait()
}

// SaveAllCaches persists document symbol caches from all running clients.
func (s *Manager) SaveAllCaches(ctx context.Context) error {
	var errs []error
	for name, client := range s.clients.Seq2() {
		if client.GetServerState() != StateReady {
			continue
		}
		for uri := range client.openFiles.Seq2() {
			if _, err := client.DocumentSymbols(ctx, string(uri)); err != nil {
				slog.Warn("Failed to cache symbols", "server", name, "uri", uri, "error", err)
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// RestartLanguageServer restarts the LSP server with the given name.
func (s *Manager) RestartLanguageServer(ctx context.Context, lang string) error {
	client, ok := s.clients.Get(lang)
	if !ok {
		return ErrClientNotFound
	}

	if err := client.Restart(); err != nil {
		return fmt.Errorf("restart failed for %s: %w", lang, err)
	}

	s.callback(lang, client)
	return nil
}

// RequestFullSymbolTree returns the recursive document symbol hierarchy for
// the file at the given URI. It finds the first running client that handles
// the file and delegates to its DocumentSymbols method.
func (s *Manager) RequestFullSymbolTree(ctx context.Context, uri string) ([]protocol.DocumentSymbol, error) {
	for name, client := range s.clients.Seq2() {
		if client.GetServerState() != StateReady {
			continue
		}
		path, err := protocol.DocumentURI(uri).Path()
		if err != nil {
			continue
		}
		if !client.HandlesFile(path) {
			continue
		}

		var result []protocol.DocumentSymbol
		err = s.executor.Submit(ctx, name, func() error {
			var ferr error
			result, ferr = client.DocumentSymbols(ctx, path)
			return ferr
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, ErrClientNotFound
}

// [XRUSH: begin: crash recovery goroutine]
// startCrashRecovery monitors an LSP server for crashes and attempts recovery.
func (s *Manager) startCrashRecovery(ctx context.Context, name string, cfg config.LSPConfig) {
	if s.crashRecovery {
		recoveryCfg := cfg
		recoveryBackoff := s.backoff
		recoveryName := name
		recoveryMgr := s
		go func() {
			cr := NewCrashRecovery(recoveryName, recoveryBackoff, func(recoveryCtx context.Context) error {
				for {
					if current, ok := recoveryMgr.clients.Get(recoveryName); !ok || current.GetServerState() != StateReady {
						break
					}
					select {
					case <-recoveryCtx.Done():
						return recoveryCtx.Err()
					case <-time.After(5 * time.Second):
					}
				}

				slog.Warn("LSP server crashed, attempting recovery", "name", recoveryName)

				newClient, err := New(
					recoveryCtx,
					recoveryName,
					recoveryCfg,
					recoveryMgr.cfg.Resolver(),
					recoveryMgr.cfg.WorkingDir(),
					recoveryMgr.cfg.Config().Options.DebugLSP,
				)
				if err != nil {
					slog.Error("Crash recovery failed to create LSP client", "name", recoveryName, "error", err)
					return ErrServerCrashed
				}

				initRecoveryCtx, cancel := context.WithTimeout(recoveryCtx, time.Duration(cmp.Or(recoveryCfg.Timeout, 30))*time.Second)
				defer cancel()

				if _, err := newClient.Initialize(initRecoveryCtx, recoveryMgr.cfg.WorkingDir()); err != nil {
					slog.Error("Crash recovery initialization failed", "name", recoveryName, "error", err)
					_ = newClient.Close(recoveryCtx)
					return ErrServerCrashed
				}

				if err := newClient.WaitForServerReady(initRecoveryCtx); err != nil {
					slog.Error("Crash recovery server not ready", "name", recoveryName, "error", err)
					_ = newClient.Close(recoveryCtx)
					return ErrServerCrashed
				}

				newClient.SetServerState(StateReady)
				recoveryMgr.clients.Set(recoveryName, newClient)
				slog.Info("LSP server recovered after crash", "name", recoveryName)
				return nil
			})
			if err := cr.Run(context.Background()); err != nil {
				slog.Error("Crash recovery exhausted", "name", recoveryName, "error", err)
			}
		}()
	}
}

// [XRUSH: end]

// handleServerReadyFailure handles the xrush-specific error path when
// WaitForServerReady fails: cleanup client, remove from map, mark unavailable.
func (s *Manager) handleServerReadyFailure(ctx context.Context, client *Client, name string) {
	_ = client.Close(ctx)
	s.clients.Del(name)
	s.markUnavailable(name)
}

// handleServerReadySuccess handles the xrush-specific success path after
// WaitForServerReady succeeds: start crash recovery.
func (s *Manager) handleServerReadySuccess(ctx context.Context, name string, cfg config.LSPConfig) {
	s.startCrashRecovery(ctx, name, cfg)
}

func (s *Manager) resolveViaInstaller(ctx context.Context, name string, server *powernapconfig.ServerConfig, cfg catalog.InstallConfig) {
	if cfg.Method == "path" {
		if _, err := exec.LookPath(server.Command); err != nil {
			slog.Warn("LSP server not found on PATH", "name", name, "command", server.Command)
		}
		return
	}

	if cfg.RuntimeDep != "" && !IsRuntimeAvailable(cfg.RuntimeDep) {
		slog.Warn("LSP server runtime not available, skipping", "name", name, "runtime", cfg.RuntimeDep)
		return
	}

	installer, ok := s.installers[cfg.Method]
	if !ok {
		slog.Warn("No installer for method", "name", name, "method", cfg.Method)
		return
	}

	resolved, err := installer.Install(ctx, name, cfg)
	if err != nil {
		slog.Warn("LSP server install failed", "name", name, "method", cfg.Method, "error", err)
		return
	}
	server.Command = resolved
}
