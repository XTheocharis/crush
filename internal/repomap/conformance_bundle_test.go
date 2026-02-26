package repomap

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildSignOffBundle(t *testing.T) {
	t.Parallel()

	bundle, err := BuildSignOffBundle(".")
	require.NoError(t, err)
	require.NotNil(t, bundle)

	require.Equal(t, "1", bundle.Version)
	require.NotEmpty(t, bundle.GeneratedAt)
	require.True(t, bundle.GateAPassed)
	require.True(t, bundle.GateBPassed)
	require.True(t, bundle.Phase5Passed)

	require.NotEmpty(t, bundle.RepoMap.AiderCommitSHA)
	require.NotEmpty(t, bundle.RepoMap.ComparatorPath)
	require.NotEmpty(t, bundle.RepoMap.FixturesSHA256)
	require.NotEmpty(t, bundle.Explorer.VoltCommitSHA)
	require.NotEmpty(t, bundle.Explorer.ComparatorPath)
	require.NotEmpty(t, bundle.Explorer.FixturesSHA256)
}

func TestValidateSignOffBundle(t *testing.T) {
	t.Parallel()

	bundle, err := BuildSignOffBundle(".")
	require.NoError(t, err)
	require.NoError(t, ValidateSignOffBundle(bundle))

	bad := *bundle
	bad.Phase5Passed = false
	err = ValidateSignOffBundle(&bad)
	require.Error(t, err)
	require.Contains(t, err.Error(), "phase5_passed")
}

func TestWriteAndLoadSignOffBundleManifest(t *testing.T) {
	t.Parallel()

	bundle, err := BuildSignOffBundle(".")
	require.NoError(t, err)

	manifestPath := filepath.Join(t.TempDir(), "conformance", "signoff_bundle.json")
	require.NoError(t, WriteSignOffBundleManifest(manifestPath, bundle))

	loaded, err := LoadSignOffBundleManifest(manifestPath)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, bundle.Version, loaded.Version)
	require.Equal(t, bundle.GateAPassed, loaded.GateAPassed)
	require.Equal(t, bundle.GateBPassed, loaded.GateBPassed)
	require.Equal(t, bundle.Phase5Passed, loaded.Phase5Passed)
	require.Equal(t, bundle.RepoMap.AiderCommitSHA, loaded.RepoMap.AiderCommitSHA)
	require.Equal(t, bundle.Explorer.VoltCommitSHA, loaded.Explorer.VoltCommitSHA)
}

func TestValidateSignOffBundleRejectsPlaceholderProvenance(t *testing.T) {
	t.Parallel()

	bundle, err := BuildSignOffBundle(".")
	require.NoError(t, err)

	badSHA := *bundle
	badSHA.RepoMap.AiderCommitSHA = strings.Repeat("0", 40)
	err = ValidateSignOffBundle(&badSHA)
	require.Error(t, err)
	require.Contains(t, err.Error(), "placeholder comparator commit sha")

	badHash := *bundle
	badHash.Explorer.FixturesSHA256 = strings.Repeat("d", 64)
	err = ValidateSignOffBundle(&badHash)
	require.Error(t, err)
	require.Contains(t, err.Error(), "placeholder fixture hash")

	badPath := *bundle
	badPath.RepoMap.ComparatorPath = "https://github.com/Aider-AI/aider/tree/" + strings.Repeat("0", 40)
	err = ValidateSignOffBundle(&badPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "placeholder comparator path")
}
