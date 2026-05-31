package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisterDefaults(t *testing.T) {
	t.Parallel()

	s := NewToolSurface()

	// Verify batch_edit is registered with CapabilityFS.
	require.True(t, s.HasCapability("batch_edit", CapabilityFS),
		"batch_edit should have CapabilityFS")
	require.Greater(t, s.GetToolCapabilities("batch_edit"), Capability(0),
		"batch_edit should be registered")

	// Verify synthetic_output is registered with CapabilityObservation.
	require.True(t, s.HasCapability("synthetic_output", CapabilityObservation),
		"synthetic_output should have CapabilityObservation")
	require.Greater(t, s.GetToolCapabilities("synthetic_output"), Capability(0),
		"synthetic_output should be registered")
	require.True(t, s.HasToolMarker("synthetic_output", MarkerBeta),
		"synthetic_output should have MarkerBeta")

	// batch_edit is visible by default (not beta).
	require.True(t, s.IsVisible("batch_edit"),
		"batch_edit should be visible by default")
	// synthetic_output is hidden by default (beta, BetaTools=false).
	require.True(t, s.IsVisible("synthetic_output"),
		"synthetic_output should be visible before UpdateCapabilities")

	// After UpdateCapabilities with BetaTools=false, beta tools are hidden.
	s.UpdateCapabilities(SurfaceContext{})
	require.False(t, s.IsVisible("synthetic_output"),
		"synthetic_output should be hidden when BetaTools=false")
	require.True(t, s.IsVisible("batch_edit"),
		"batch_edit should remain visible when BetaTools=false")
}

func TestToolMarkers(t *testing.T) {
	t.Parallel()

	// All 6 markers are defined at bits 6–11.
	markers := AllMarkers()
	require.Len(t, markers, 6)
	require.Equal(t, ToolMarker(0x0040), MarkerCanEdit)
	require.Equal(t, ToolMarker(0x0080), MarkerSymbolicRead)
	require.Equal(t, ToolMarker(0x0100), MarkerSymbolicEdit)
	require.Equal(t, ToolMarker(0x0200), MarkerOptional)
	require.Equal(t, ToolMarker(0x0400), MarkerBeta)
	require.Equal(t, ToolMarker(0x0800), MarkerDoesNotRequireActiveProject)

	// Markers do not collide with Capability bits (bits 0–5).
	allMarkers := MarkerCanEdit | MarkerSymbolicRead | MarkerSymbolicEdit |
		MarkerOptional | MarkerBeta | MarkerDoesNotRequireActiveProject
	allCaps := CapabilityFS | CapabilityNetwork | CapabilityCodeIntelligence |
		CapabilityExecution | CapabilityMemory | CapabilityObservation
	require.Equal(t, uint16(0x0FC0), uint16(allMarkers),
		"markers should occupy bits 6–11 only")
	require.Equal(t, uint8(0x3F), uint8(allCaps),
		"capabilities should occupy bits 0–5 only")

	// HasMarker works on combined values.
	combined := MarkerCanEdit | MarkerBeta
	require.True(t, combined.HasMarker(MarkerCanEdit))
	require.True(t, combined.HasMarker(MarkerBeta))
	require.False(t, combined.HasMarker(MarkerOptional))

	// HasMarker on zero value returns false.
	require.False(t, ToolMarker(0).HasMarker(MarkerCanEdit))
}

func TestLSPToolVisibility(t *testing.T) {
	t.Parallel()

	s := NewToolSurface()

	// All 14 LSP tools must be registered with CapabilityCodeIntelligence.
	expectedLSPTools := []string{
		"lsp_diagnostics",
		"lsp_references",
		"lsp_restart",
		"lsp_definition",
		"lsp_rename",
		"lsp_code_action",
		"lsp_safe_delete",
		"lsp_replace_symbol",
		"lsp_insert_before",
		"lsp_insert_after",
		"lsp_formatting",
		"lsp_hover",
		"lsp_completion",
		"lsp_signature_help",
	}

	for _, name := range expectedLSPTools {
		require.True(t, s.HasCapability(name, CapabilityCodeIntelligence),
			"%s should have CapabilityCodeIntelligence", name)
		require.True(t, s.IsVisible(name),
			"%s should be visible by default", name)
	}

	// When LSP is unavailable, all pure CodeIntelligence tools are hidden.
	s.UpdateCapabilities(SurfaceContext{HasLSP: false})
	for _, name := range expectedLSPTools {
		require.False(t, s.IsVisible(name),
			"%s should be hidden when HasLSP=false", name)
	}

	// When LSP is available, all tools become visible again.
	s.UpdateCapabilities(SurfaceContext{HasLSP: true})
	for _, name := range expectedLSPTools {
		require.True(t, s.IsVisible(name),
			"%s should be visible when HasLSP=true", name)
	}
}

func TestToolMarkerRegistration(t *testing.T) {
	t.Parallel()

	s := NewToolSurface()

	// Existing tools have no markers (backward compat).
	require.Equal(t, ToolMarker(0), s.GetToolMarkers("edit"))
	require.False(t, s.HasToolMarker("edit", MarkerCanEdit))

	// Unknown tool returns zero markers.
	require.Equal(t, ToolMarker(0), s.GetToolMarkers("nonexistent"))
	require.False(t, s.HasToolMarker("nonexistent", MarkerCanEdit))

	// Register a tool with markers.
	s.RegisterWithMarkers("test_tool", CapabilityFS, MarkerCanEdit|MarkerBeta)
	require.True(t, s.HasToolMarker("test_tool", MarkerCanEdit))
	require.True(t, s.HasToolMarker("test_tool", MarkerBeta))
	require.False(t, s.HasToolMarker("test_tool", MarkerOptional))
	require.Equal(t, CapabilityFS, s.GetToolCapabilities("test_tool"))

	// Register without markers still works.
	s.Register("plain_tool", CapabilityExecution)
	require.Equal(t, ToolMarker(0), s.GetToolMarkers("plain_tool"))
	require.Equal(t, CapabilityExecution, s.GetToolCapabilities("plain_tool"))
}

func TestBetaToolVisibility(t *testing.T) {
	t.Parallel()

	s := NewToolSurface()
	s.RegisterWithMarkers("beta_experimental", CapabilityFS, MarkerBeta)
	s.Register("stable_tool", CapabilityFS)

	// Before any UpdateCapabilities call, all tools are visible (default).
	require.True(t, s.IsVisible("beta_experimental"))
	require.True(t, s.IsVisible("stable_tool"))

	// With BetaTools=false, beta tools are hidden.
	s.UpdateCapabilities(SurfaceContext{BetaTools: false})
	require.False(t, s.IsVisible("beta_experimental"))
	require.True(t, s.IsVisible("stable_tool"))

	// With BetaTools=true, beta tools become visible.
	s.UpdateCapabilities(SurfaceContext{BetaTools: true})
	require.True(t, s.IsVisible("beta_experimental"))
	require.True(t, s.IsVisible("stable_tool"))

	// Toggling back hides beta tools again.
	s.UpdateCapabilities(SurfaceContext{BetaTools: false})
	require.False(t, s.IsVisible("beta_experimental"))

	// synthetic_output is registered as beta by default.
	s2 := NewToolSurface()
	s2.UpdateCapabilities(SurfaceContext{})
	require.False(t, s2.IsVisible("synthetic_output"))
	s2.UpdateCapabilities(SurfaceContext{BetaTools: true})
	require.True(t, s2.IsVisible("synthetic_output"))
}

func TestBetaToolsHiddenFromGetVisibleTools(t *testing.T) {
	t.Parallel()

	s := NewToolSurface()
	s.RegisterWithMarkers("beta_a", CapabilityObservation, MarkerBeta)
	s.Register("stable_b", CapabilityObservation)

	s.UpdateCapabilities(SurfaceContext{})

	hidden := s.GetHiddenTools()
	visible := s.GetVisibleTools()

	require.Contains(t, hidden, "beta_a")
	require.NotContains(t, visible, "beta_a")
	require.Contains(t, visible, "stable_b")
	require.NotContains(t, hidden, "stable_b")
}
