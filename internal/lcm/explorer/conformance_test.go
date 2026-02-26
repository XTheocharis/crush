package explorer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildConformanceSnapshot(t *testing.T) {
	t.Parallel()

	snapshot, err := BuildConformanceSnapshot(".")
	require.NoError(t, err)
	require.NotNil(t, snapshot)

	require.Equal(t, "1", snapshot.Version)
	require.Len(t, snapshot.VoltCommitSHA, 40)
	require.Regexp(t, `^[0-9a-fA-F]{40}$`, snapshot.VoltCommitSHA)
	require.NotEqual(t, strings.Repeat("0", 40), snapshot.VoltCommitSHA)
	require.NotEqual(t, strings.Repeat("c", 40), strings.ToLower(snapshot.VoltCommitSHA))
	require.Contains(t, snapshot.ComparatorPath, "../volt/tree/")
	require.Contains(t, snapshot.ComparatorPath, snapshot.VoltCommitSHA)
	require.Len(t, snapshot.FixturesSHA256, 64)
	require.Regexp(t, `^[0-9a-fA-F]{64}$`, snapshot.FixturesSHA256)
	require.NotEqual(t, strings.Repeat("d", 64), strings.ToLower(snapshot.FixturesSHA256))
	require.Equal(t, "1", snapshot.FixtureIndexVersion)
	require.Equal(t, "1", snapshot.RuntimeInventoryVersion)
	require.Equal(t, "1", snapshot.TokenizerSupportVersion)
	require.Equal(t, "1", snapshot.ExplorerFamilyMatrixVersion)
	require.Equal(t, "parity", snapshot.Profile)
	require.True(t, snapshot.DeterministicMode)
	require.Equal(t, "none", snapshot.EnhancementTiersEnabled)
	require.Equal(t, "tokenizer_backed", snapshot.TokenCounterMode)
	require.EqualValues(t, 1337, snapshot.FixedSeed)
	require.True(t, snapshot.GateBPassed)
}

func TestBuildConformanceSnapshot_EmptyBasePathDefaultsToDot(t *testing.T) {
	t.Parallel()

	snapshot, err := BuildConformanceSnapshot("")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.True(t, snapshot.GateBPassed)
}
