package lsp

import (
	"context"
	"log/slog"
	"time"
)

// DefaultHealthCheckInterval is the default interval between health checks.
const DefaultHealthCheckInterval = 30 * time.Second

// HealthChecker periodically checks the health of all managed LSP clients.
// If a client is found to be unresponsive, it logs a warning and triggers
// the recovery path.
type HealthChecker struct {
	manager  *Manager
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}

	// onRecover is called when a client needs recovery. Defaults to
	// the manager's RestartLanguageServer when crashRecovery is enabled.
	onRecover func(name string, client *Client)
}

// NewHealthChecker creates a new HealthChecker for the given manager.
// The interval controls how often clients are checked.
func NewHealthChecker(manager *Manager, interval time.Duration) *HealthChecker {
	ctx, cancel := context.WithCancel(context.Background())
	return &HealthChecker{
		manager:  manager,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
}

// Start launches the background health check goroutine.
func (h *HealthChecker) Start() {
	go h.run()
}

// Stop cancels the health check context and waits for the goroutine to exit.
func (h *HealthChecker) Stop() {
	h.cancel()
	<-h.done
}

// run is the main health check loop. It iterates all clients at the configured
// interval and checks whether they are still responsive.
func (h *HealthChecker) run() {
	defer close(h.done)

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.checkAll()
		}
	}
}

// checkAll iterates all managed clients and checks their health.
func (h *HealthChecker) checkAll() {
	for name, client := range h.manager.clients.Seq2() {
		h.checkClient(name, client)
	}
}

// checkClient performs a health check on a single client. If the client is not
// in the Ready state or the underlying connection is dead, it logs a warning
// and triggers the recovery path.
func (h *HealthChecker) checkClient(name string, client *Client) {
	state := client.GetServerState()

	if state == StateStopped || state == StateDisabled || state == StateUnstarted {
		return
	}

	if !client.IsAlive() {
		slog.Warn("LSP health check: server process is not running",
			"name", name, "state", state)
		h.triggerRecovery(name, client)
		return
	}

	if state != StateReady && state != StateStarting {
		slog.Warn("LSP health check: server in unexpected state",
			"name", name, "state", state)
		h.triggerRecovery(name, client)
	}
}

// triggerRecovery handles an unhealthy client by logging the issue and
// attempting to restart it. If crash recovery is enabled on the manager,
// the client is restarted via RestartLanguageServer.
func (h *HealthChecker) triggerRecovery(name string, client *Client) {
	if h.onRecover != nil {
		h.onRecover(name, client)
		return
	}

	if !h.manager.crashRecovery {
		slog.Warn("LSP health check: crash recovery disabled, skipping restart",
			"name", name)
		return
	}

	slog.Warn("LSP health check: attempting recovery",
		"name", name)

	ctx, cancel := context.WithTimeout(h.ctx, 30*time.Second)
	defer cancel()

	if err := h.manager.RestartLanguageServer(ctx, name); err != nil {
		slog.Error("LSP health check: recovery failed",
			"name", name, "error", err)
		return
	}
	slog.Info("LSP health check: recovery succeeded", "name", name)
}
