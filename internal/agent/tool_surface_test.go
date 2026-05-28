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
