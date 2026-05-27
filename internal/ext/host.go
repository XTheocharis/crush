package ext

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	tea "charm.land/bubbletea/v2"
	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/session"
)

// globalMu protects compiledInExtensions for concurrent test access.
var globalMu sync.Mutex

var compiledInExtensions []Extension

// RegisterExtension adds an extension to the compiled-in registry.
func RegisterExtension(ext Extension) {
	globalMu.Lock()
	defer globalMu.Unlock()
	compiledInExtensions = append(compiledInExtensions, ext)
}

// ResetForTesting clears all state for test isolation.
func ResetForTesting() {
	globalMu.Lock()
	defer globalMu.Unlock()
	compiledInExtensions = nil
}

// ExtensionHost manages the lifecycle of all registered extensions and
// provides accessors for contributed tools, hooks, and prompt modifiers.
type ExtensionHost struct {
	deps         HostDeps
	extensions   []Extension
	tools        []fantasy.AgentTool
	toolNames    []string
	runHooks     []RunHook
	stepHooks    []StepHook
	promptHook   *PromptHook
	mu           sync.RWMutex
	bootstrapped bool
}

// NewExtensionHost creates a new host with the given dependencies. The host
// is not yet bootstrapped; call Bootstrap to initialize extensions.
func NewExtensionHost(deps HostDeps) *ExtensionHost {
	return &ExtensionHost{
		deps: deps,
	}
}

// Register adds an extension before bootstrap. Returns an error if called
// after Bootstrap.
func (h *ExtensionHost) Register(ext Extension) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.bootstrapped {
		return fmt.Errorf("cannot register extension %q after bootstrap", ext.Name())
	}
	h.extensions = append(h.extensions, ext)
	return nil
}

// Bootstrap moves compiled-in extensions into the host, calls Init on all
// extensions, and collects capabilities. Returns an error on duplicate tool
// names or Init failures.
func (h *ExtensionHost) Bootstrap(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.bootstrapped {
		return fmt.Errorf("already bootstrapped")
	}

	// Merge compiled-in extensions.
	globalMu.Lock()
	h.extensions = append(compiledInExtensions, h.extensions...)
	compiledInExtensions = nil
	globalMu.Unlock()

	hc := &hostContext{deps: h.deps, host: h}

	toolNameSet := make(map[string]string)

	for _, ext := range h.extensions {
		if err := safeCall("Init:"+ext.Name(), func() error {
			return ext.Init(ctx, hc)
		}); err != nil {
			return fmt.Errorf("extension %q Init failed: %w", ext.Name(), err)
		}

		if tp, ok := ext.(ToolProvider); ok {
			tools, names, err := CollectTools(ctx, tp)
			if err != nil {
				return fmt.Errorf("extension %q Tools failed: %w", ext.Name(), err)
			}
			for _, name := range names {
				if prev, exists := toolNameSet[name]; exists {
					return fmt.Errorf("duplicate tool name %q from %q (already registered by %q)", name, ext.Name(), prev)
				}
				toolNameSet[name] = ext.Name()
			}
			h.tools = append(h.tools, tools...)
			h.toolNames = append(h.toolNames, names...)
		}

		if rhp, ok := ext.(RunHookProvider); ok {
			hooks := safeCallRunHooks(rhp)
			h.runHooks = append(h.runHooks, hooks...)
		}

		if shp, ok := ext.(StepHookProvider); ok {
			hooks := safeCallStepHooks(shp)
			h.stepHooks = append(h.stepHooks, hooks...)
		}

		if php, ok := ext.(PromptHookProvider); ok {
			hook := safeCallPromptHook(php)
			if hook != nil {
				if h.promptHook != nil {
					return fmt.Errorf("multiple prompt hooks registered: %q conflicts with existing", ext.Name())
				}
				h.promptHook = hook
			}
		}
	}

	h.bootstrapped = true
	return nil
}

// Shutdown calls extension Shutdown in reverse order.
func (h *ExtensionHost) Shutdown(ctx context.Context) error {
	h.mu.RLock()
	exts := h.extensions
	h.mu.RUnlock()

	var firstErr error
	for _, ext := range slices.Backward(exts) {
		if err := safeCall("Shutdown:"+ext.Name(), func() error {
			return ext.Shutdown(ctx)
		}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ContributedTools returns a defensive copy of all contributed tools.
func (h *ExtensionHost) ContributedTools() []fantasy.AgentTool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]fantasy.AgentTool{}, h.tools...)
}

// ContributedToolNames returns a defensive copy of contributed tool names.
func (h *ExtensionHost) ContributedToolNames() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]string{}, h.toolNames...)
}

// RunHooks returns a defensive copy of run hooks.
func (h *ExtensionHost) RunHooks() []RunHook {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]RunHook{}, h.runHooks...)
}

// StepHooks returns a defensive copy of step hooks.
func (h *ExtensionHost) StepHooks() []StepHook {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]StepHook{}, h.stepHooks...)
}

// GetPromptHook returns the prompt hook, or nil if none registered.
func (h *ExtensionHost) GetPromptHook() *PromptHook {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.promptHook
}

// IsBootstrapped reports whether Bootstrap has been called.
func (h *ExtensionHost) IsBootstrapped() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.bootstrapped
}

type hostContext struct {
	deps HostDeps
	host *ExtensionHost
}

func (hc *hostContext) Config() *config.Config {
	return hc.deps.Config.Config()
}

func (hc *hostContext) WorkingDir() string {
	return hc.deps.WorkingDir
}

func (hc *hostContext) RegisterTools(provider ToolProvider) {
	// Registration happens during Init via Bootstrap's capability check.
	// This method is a placeholder for future dynamic registration.
}

func (hc *hostContext) RegisterRunHooks(provider RunHookProvider) {
	// Registration happens during Init via Bootstrap's capability check.
}

func (hc *hostContext) RegisterStepHooks(provider StepHookProvider) {
	// Registration happens during Init via Bootstrap's capability check.
}

func (hc *hostContext) RegisterPromptHook(provider PromptHookProvider) {
	// Registration happens during Init via Bootstrap's capability check.
}

func (hc *hostContext) PublishEvent(_ context.Context, eventType string, payload any) error {
	if hc.deps.Events == nil {
		return nil
	}
	evt := ExtensionEvent{
		Source:    "ext",
		EventType: eventType,
		Payload:   payload,
	}
	hc.deps.Events.Publish(pubsub.CreatedEvent, tea.Msg(evt))
	return nil
}

func (hc *hostContext) LSP() *lsp.Manager {
	return hc.deps.LSP
}

func (hc *hostContext) DB() *sql.DB {
	return hc.deps.DB
}

func (hc *hostContext) Sessions() session.Service {
	return hc.deps.Sessions
}

func (hc *hostContext) Messages() message.Service {
	return hc.deps.Messages
}

// safeCall executes fn with panic recovery, logging the panic and returning
// it as an error.
func safeCall(name string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Extension hook panicked", "hook", name, "panic", r)
			err = fmt.Errorf("extension hook %s panicked: %v", name, r)
		}
	}()
	return fn()
}

func safeCallRunHooks(p RunHookProvider) []RunHook {
	var hooks []RunHook
	safeCall("RunHooks:"+p.Name(), func() error {
		hooks = p.RunHooks()
		return nil
	})
	return hooks
}

func safeCallStepHooks(p StepHookProvider) []StepHook {
	var hooks []StepHook
	safeCall("StepHooks:"+p.Name(), func() error {
		hooks = p.StepHooks()
		return nil
	})
	return hooks
}

func safeCallPromptHook(p PromptHookProvider) *PromptHook {
	var hook *PromptHook
	safeCall("PromptHook:"+p.Name(), func() error {
		hook = p.PromptHook()
		return nil
	})
	return hook
}
