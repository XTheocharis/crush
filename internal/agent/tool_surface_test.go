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

	// Both should be visible by default.
	require.True(t, s.IsVisible("batch_edit"),
		"batch_edit should be visible by default")
	require.True(t, s.IsVisible("synthetic_output"),
		"synthetic_output should be visible by default")
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
