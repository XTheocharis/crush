package ext

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/filetracker"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/processor"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/rewind"
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
	deps               HostDeps
	extensions         []Extension
	tools              []fantasy.AgentTool
	toolNames          []string
	runHooks           []RunHook
	stepHooks          []StepHook
	promptHook         *PromptHook
	mu                 sync.RWMutex
	bootstrapped       bool
	stoppedByCondition atomic.Bool
	skipCompiledIn     bool
}

// NewExtensionHost creates a new host with the given dependencies. The host
// is not yet bootstrapped; call Bootstrap to initialize extensions.
func NewExtensionHost(deps HostDeps) *ExtensionHost {
	return &ExtensionHost{
		deps: deps,
	}
}

// NewLightweightHost creates a host for sub-agents that uses only the
// explicitly provided extensions. Compiled-in extensions are not merged,
// preventing heavy extensions (autofix, repomap, etc.) from running on
// sub-agents.
func NewLightweightHost(deps HostDeps, exts []Extension) *ExtensionHost {
	return &ExtensionHost{
		deps:           deps,
		extensions:     exts,
		skipCompiledIn: true,
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

	// Merge compiled-in extensions (skip for lightweight sub-agent hosts).
	if !h.skipCompiledIn {
		globalMu.Lock()
		h.extensions = append(compiledInExtensions, h.extensions...)
		compiledInExtensions = nil
		globalMu.Unlock()
	}

	hc := &hostContext{deps: h.deps, host: h}

	for _, ext := range h.extensions {
		if err := safeCall("Init:"+ext.Name(), func() error {
			return ext.Init(ctx, hc)
		}); err != nil {
			return fmt.Errorf("extension %q Init failed: %w", ext.Name(), err)
		}
	}

	if err := h.collectToolsFromProviders(ctx); err != nil {
		return err
	}

	h.bootstrapped = true
	return nil
}

// collectToolsFromProviders rebuilds the tool, hook, and prompt-hook slices
// from all registered extensions. The caller must hold h.mu (or be Bootstrap
// which already holds it).
func (h *ExtensionHost) collectToolsFromProviders(ctx context.Context) error {
	var (
		newTools      []fantasy.AgentTool
		newToolNames  []string
		newRunHooks   []RunHook
		newStepHooks  []StepHook
		newPromptHook *PromptHook
	)

	toolNameSet := make(map[string]string)

	for _, ext := range h.extensions {
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
			newTools = append(newTools, tools...)
			newToolNames = append(newToolNames, names...)
		}

		if rhp, ok := ext.(RunHookProvider); ok {
			hooks := safeCallRunHooks(rhp)
			newRunHooks = append(newRunHooks, hooks...)
		}

		if shp, ok := ext.(StepHookProvider); ok {
			hooks := safeCallStepHooks(shp)
			newStepHooks = append(newStepHooks, hooks...)
		}

		if php, ok := ext.(PromptHookProvider); ok {
			hook := safeCallPromptHook(php)
			if hook != nil {
				if newPromptHook != nil {
					return fmt.Errorf("multiple prompt hooks registered: %q conflicts with existing", ext.Name())
				}
				newPromptHook = hook
			}
		}
	}

	h.tools = newTools
	h.toolNames = newToolNames
	h.runHooks = newRunHooks
	h.stepHooks = newStepHooks
	h.promptHook = newPromptHook
	return nil
}

// RefreshContributedTools re-collects tools and hooks from all registered
// extensions, replacing the previously collected set. The host must already
// be bootstrapped. It is safe to call concurrently with read accessors.
func (h *ExtensionHost) RefreshContributedTools(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.bootstrapped {
		return fmt.Errorf("cannot refresh tools before bootstrap")
	}

	return h.collectToolsFromProviders(ctx)
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

// ExtensionByName returns the extension with the given name, or nil if
// not found.
func (h *ExtensionHost) ExtensionByName(name string) Extension {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, e := range h.extensions {
		if e.Name() == name {
			return e
		}
	}
	return nil
}

// MarkStoppedByCondition records that a stop condition was triggered.
func (h *ExtensionHost) MarkStoppedByCondition() {
	if h == nil {
		return
	}
	h.stoppedByCondition.Store(true)
}

// ClearStoppedByCondition resets the stop condition flag before a new run.
func (h *ExtensionHost) ClearStoppedByCondition() {
	if h == nil {
		return
	}
	h.stoppedByCondition.Store(false)
}

// WasStoppedByCondition reports whether a stop condition terminated the run.
func (h *ExtensionHost) WasStoppedByCondition() bool {
	if h == nil {
		return false
	}
	return h.stoppedByCondition.Load()
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

func (hc *hostContext) Completer() TextCompleter {
	return hc.deps.Completer
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

func (hc *hostContext) RewindService() rewind.Service {
	return hc.deps.RewindService
}

func (hc *hostContext) FileTracker() filetracker.Service {
	return hc.deps.FileTracker
}

func (hc *hostContext) ToolDefs() []processor.ToolDef {
	if hc.deps.ToolDefsFn == nil {
		return nil
	}
	return hc.deps.ToolDefsFn()
}

func (hc *hostContext) SkillDefs() []processor.SkillDef {
	if hc.deps.SkillDefsFn == nil {
		return nil
	}
	return hc.deps.SkillDefsFn()
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
