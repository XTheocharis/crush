//go:build treesitter

package extensions

import (
	"context"
	"log/slog"
	"sync"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/treesitter"
)

// TreesitterExtension wires the tree-sitter validation pipeline as
// post-edit infrastructure. Only compiled when the "treesitter" build tag
// is set.
type TreesitterExtension struct {
	mu      sync.RWMutex
	host    ext.HostContext
	handler *tools.ValidationHandler
	active  bool
}

func (e *TreesitterExtension) Name() string { return "treesitter-validation" }

func (e *TreesitterExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	cfg := host.Config()
	if cfg == nil || cfg.Options == nil || cfg.Options.Validation == nil {
		e.active = false
		return nil
	}

	vcfg := cfg.Options.Validation
	handlerCfg := tools.ValidationHandlerConfig{
		Enabled: vcfg.Enabled,
		AutoFix: vcfg.AutoFix,
	}

	// Create the tree-sitter parser. Stages 5-7 (ParseCheck,
	// SymbolConsistency, ImportConsistency) skip gracefully when nil.
	var parser interface{}
	if vcfg.Enabled {
		parser = treesitter.NewParser()
		if parser == nil {
			slog.Warn("TreesitterExtension: NewParser returned nil, parser-dependent stages will be skipped")
		}
	}

	var diagGate *tools.DiagnosticGate
	if lspMgr := host.LSP(); lspMgr != nil {
		diagGate = tools.NewDiagnosticGate(lspMgr, tools.WithSeverityFilter(
			tools.ParseSeverityFilter(vcfg.SeverityFilter),
		))
	}

	e.handler = tools.NewValidationHandler(parser, diagGate, handlerCfg)
	e.active = true
	return nil
}

func (e *TreesitterExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handler = nil
	e.active = false
	return nil
}

// Handler returns the wired ValidationHandler for use as post-edit
// infrastructure by the coordinator.
func (e *TreesitterExtension) Handler() *tools.ValidationHandler {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.handler
}

var _ ext.Extension = (*TreesitterExtension)(nil)
