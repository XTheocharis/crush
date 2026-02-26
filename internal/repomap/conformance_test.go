package repomap

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
	require.Equal(t, strings.Repeat("0", 40), snapshot.AiderCommitSHA)
	require.Contains(t, snapshot.ComparatorPath, "https://github.com/Aider-AI/aider/tree/")
	require.Equal(t, "2d2737a92d8132a0dc50c461a0bcbae7900470d71b32b9b18da83a2964ffba80", snapshot.FixturesSHA256)
	require.Equal(t, "1", snapshot.ComparatorConfigVersion)
	require.Equal(t, "1", snapshot.TokenizerSupportVersion)
	require.Equal(t, "1", snapshot.ExplorerFamilyMatrixVersion)
	require.NotEmpty(t, snapshot.Profile)
	require.True(t, snapshot.DeterministicMode)
	require.Equal(t, "none", snapshot.EnhancementTiersEnabled)
	require.Equal(t, "tokenizer_backed", snapshot.TokenCounterMode)
	require.EqualValues(t, 1337, snapshot.FixedSeed)
	require.True(t, snapshot.GateAPassed)
}

func TestBuildConformanceSnapshot_EmptyBasePathDefaultsToDot(t *testing.T) {
	t.Parallel()

	snapshot, err := BuildConformanceSnapshot("")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.True(t, snapshot.GateAPassed)
}
