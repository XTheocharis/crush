package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtensionToolNamesDefault(t *testing.T) {
	ResetExtensionToolNamesForTesting()
	t.Parallel()

	names := allToolNames()
	require.Len(t, names, 47)
	require.Contains(t, names, "bash")
	require.Contains(t, names, "edit")
	require.Contains(t, names, "view")
	require.Contains(t, names, "write")
}

func TestExtensionToolNamesContributed(t *testing.T) {
	ResetExtensionToolNamesForTesting()
	t.Cleanup(func() { ResetExtensionToolNamesForTesting() })

	RegisterExtensionToolNames(func() []string {
		return []string{"ext_tool_a", "ext_tool_b"}
	})

	names := allToolNames()
	require.Len(t, names, 49)
	require.Contains(t, names, "bash")
	require.Contains(t, names, "ext_tool_a")
	require.Contains(t, names, "ext_tool_b")

	last := names[len(names)-2:]
	require.Equal(t, []string{"ext_tool_a", "ext_tool_b"}, last)
}

func TestExtensionToolNamesReset(t *testing.T) {
	ResetExtensionToolNamesForTesting()

	RegisterExtensionToolNames(func() []string {
		return []string{"ext_tool_x"}
	})

	namesBefore := allToolNames()
	require.Contains(t, namesBefore, "ext_tool_x")

	ResetExtensionToolNamesForTesting()

	namesAfter := allToolNames()
	require.NotContains(t, namesAfter, "ext_tool_x")
	require.Len(t, namesAfter, 47)
}

func TestExtensionToolNamesEmptyFunction(t *testing.T) {
	ResetExtensionToolNamesForTesting()
	t.Cleanup(func() { ResetExtensionToolNamesForTesting() })

	RegisterExtensionToolNames(func() []string {
		return nil
	})

	names := allToolNames()
	require.Len(t, names, 47)
}
