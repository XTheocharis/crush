package tools

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsKnownRole(t *testing.T) {
	t.Parallel()
	require.True(t, isKnownRole("researcher"))
	require.True(t, isKnownRole("tester"))
	require.True(t, isKnownRole("reviewer"))
	require.False(t, isKnownRole("unknown"))
	require.False(t, isKnownRole("Researcher"))
	require.False(t, isKnownRole(""))
}

func TestSortedRoleNames(t *testing.T) {
	t.Parallel()
	names := sortedRoleNames()
	require.Equal(t, []string{"researcher", "reviewer", "tester"}, names)
}
