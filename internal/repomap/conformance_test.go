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
	require.Len(t, snapshot.AiderCommitSHA, 40)
	require.Regexp(t, `^[0-9a-fA-F]{40}$`, snapshot.AiderCommitSHA)
	require.NotEqual(t, strings.Repeat("0", 40), snapshot.AiderCommitSHA)
	require.Contains(t, snapshot.ComparatorPath, snapshot.AiderCommitSHA)
	require.Contains(t, snapshot.ComparatorPath, "https://github.com/Aider-AI/aider/tree/")
	require.Len(t, snapshot.FixturesSHA256, 64)
	require.Regexp(t, `^[0-9a-fA-F]{64}$`, snapshot.FixturesSHA256)
	require.NotEqual(t, strings.Repeat("d", 64), strings.ToLower(snapshot.FixturesSHA256))
	require.Equal(t, "1", snapshot.ComparatorConfigVersion)
	require.Equal(t, "1", snapshot.TokenizerSupportVersion)
	require.Equal(t, "1", snapshot.ExplorerFamilyMatrixVersion)
	require.NotEmpty(t, snapshot.Profile)
	require.True(t, snapshot.DeterministicMode)
	require.Equal(t, "none", snapshot.EnhancementTiersEnabled)
	require.Equal(t, "tokenizer_backed", snapshot.TokenCounterMode)
	require.EqualValues(t, 1337, snapshot.FixedSeed)
	require.True(t, snapshot.GateAPassed)
	require.NotEmpty(t, snapshot.RunID)
	require.NotEmpty(t, snapshot.GateAEvidencePath)
	require.FileExists(t, snapshot.GateAEvidencePath)
}

func TestBuildConformanceSnapshot_EmptyBasePathDefaultsToDot(t *testing.T) {
	t.Parallel()

	snapshot, err := BuildConformanceSnapshot("")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.True(t, snapshot.GateAPassed)
}

func TestBuildConformanceSnapshotWithRunID(t *testing.T) {
	t.Parallel()

	runID := "run-test-repomap"
	snapshot, err := BuildConformanceSnapshotWithRunID(".", runID)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Equal(t, runID, snapshot.RunID)
	require.True(t, snapshot.GateAPassed)
	require.NotEmpty(t, snapshot.GateAEvidencePath)
	require.FileExists(t, snapshot.GateAEvidencePath)
}
