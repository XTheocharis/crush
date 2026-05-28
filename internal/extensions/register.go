// Package extensions registers compiled-in extensions that wrap fork-ported
// features as extension host providers. Each extension implements at least
// the Extension interface plus one capability interface (ToolProvider,
// RunHookProvider, or StepHookProvider).
package extensions

import "github.com/charmbracelet/crush/internal/ext"

func init() {
	ext.RegisterExtension(&LSPToolsExtension{})
	ext.RegisterExtension(&TreesitterExtension{})
	ext.RegisterExtension(&AutofixExtension{})
	ext.RegisterExtension(&DiagGateExtension{})
	ext.RegisterExtension(&EditExtension{})
	ext.RegisterExtension(&RewindExtension{})
	ext.RegisterExtension(&DoomExtension{})
	ext.RegisterExtension(&OperatorExtension{})
	ext.RegisterExtension(&SwarmExtension{})
	ext.RegisterExtension(&ToolSurfaceExtension{})
	ext.RegisterExtension(&ResourceLimitsExtension{})
	ext.RegisterExtension(&XrushExtension{})
	ext.RegisterExtension(TheLCMExtension) // [XRUSH: wire compaction event to pill]
	ext.RegisterExtension(TheRepomapExtension)
	ext.RegisterExtension(&PromptAssemblyExtension{})
	ext.RegisterExtension(&OrchestrationExtension{})
	ext.RegisterExtension(&ModelRouterExtension{})
	ext.RegisterExtension(&ProcessorExtension{})
	ext.RegisterExtension(&StepAdapter{})
}
