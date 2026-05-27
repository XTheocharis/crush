package lsp

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
	"github.com/stretchr/testify/require"
)

func TestHealthCheckerDetectsDead(t *testing.T) {
	t.Parallel()

	deadClient := &Client{
		name:        "dead-server",
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
		xrushClientFields: xrushClientFields{
			healthCheck: func() bool { return false },
		},
	}
	deadClient.serverState.Store(StateReady)

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		callback: func(string, *Client) {},
	}
	mgr.clients.Set("dead-server", deadClient)

	var detected atomic.Int32
	hc := NewHealthChecker(mgr, 100*time.Millisecond)
	hc.onRecover = func(name string, client *Client) {
		detected.Add(1)
	}

	hc.Start()
	defer hc.Stop()

	require.Eventually(t, func() bool {
		return detected.Load() >= 1
	}, 500*time.Millisecond, 50*time.Millisecond, "health check should detect dead client within 500ms")
}

func TestHealthCheckerShutdown(t *testing.T) {
	t.Parallel()

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		callback: func(string, *Client) {},
	}

	hc := NewHealthChecker(mgr, 100*time.Millisecond)
	hc.Start()

	done := make(chan struct{})
	go func() {
		hc.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("HealthChecker.Stop() should return within 1s of cancel")
	}
}

func TestHealthCheckerSkipsStoppedClients(t *testing.T) {
	t.Parallel()

	stoppedClient := &Client{
		name:        "stopped-server",
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
		xrushClientFields: xrushClientFields{
			healthCheck: func() bool { return false },
		},
	}
	stoppedClient.serverState.Store(StateStopped)

	disabledClient := &Client{
		name:        "disabled-server",
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
		xrushClientFields: xrushClientFields{
			healthCheck: func() bool { return false },
		},
	}
	disabledClient.serverState.Store(StateDisabled)

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		callback: func(string, *Client) {},
	}
	mgr.clients.Set("stopped-server", stoppedClient)
	mgr.clients.Set("disabled-server", disabledClient)

	var recoveries atomic.Int32
	hc := NewHealthChecker(mgr, 100*time.Millisecond)
	hc.onRecover = func(name string, client *Client) {
		recoveries.Add(1)
	}

	hc.Start()
	defer hc.Stop()

	time.Sleep(300 * time.Millisecond)
	require.Equal(t, int32(0), recoveries.Load(), "stopped/disabled clients should not trigger recovery")
}

func TestHealthCheckerDetectsUnexpectedState(t *testing.T) {
	t.Parallel()

	errorClient := &Client{
		name:        "error-server",
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
		xrushClientFields: xrushClientFields{
			healthCheck: func() bool { return true },
		},
	}
	errorClient.serverState.Store(StateError)

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		callback: func(string, *Client) {},
	}
	mgr.clients.Set("error-server", errorClient)

	var detected atomic.Int32
	hc := NewHealthChecker(mgr, 100*time.Millisecond)
	hc.onRecover = func(name string, client *Client) {
		detected.Add(1)
	}

	hc.Start()
	defer hc.Stop()

	require.Eventually(t, func() bool {
		return detected.Load() >= 1
	}, 500*time.Millisecond, 50*time.Millisecond, "health check should detect client in error state")
}

func TestHealthCheckerHealthyClientNoRecovery(t *testing.T) {
	t.Parallel()

	healthyClient := &Client{
		name:        "healthy-server",
		diagnostics: csync.NewVersionedMap[protocol.DocumentURI, []protocol.Diagnostic](),
		openFiles:   csync.NewMap[string, *OpenFileInfo](),
		xrushClientFields: xrushClientFields{
			healthCheck: func() bool { return true },
		},
	}
	healthyClient.serverState.Store(StateReady)

	mgr := &Manager{
		clients:  csync.NewMap[string, *Client](),
		callback: func(string, *Client) {},
	}
	mgr.clients.Set("healthy-server", healthyClient)

	var recoveries atomic.Int32
	hc := NewHealthChecker(mgr, 100*time.Millisecond)
	hc.onRecover = func(name string, client *Client) {
		recoveries.Add(1)
	}

	hc.Start()
	defer hc.Stop()

	time.Sleep(300 * time.Millisecond)
	require.Equal(t, int32(0), recoveries.Load(), "healthy client should not trigger recovery")
}
