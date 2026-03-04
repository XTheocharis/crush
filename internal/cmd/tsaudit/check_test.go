package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompareDriftNoDriftWithPrimaryFallbackPrecedence(t *testing.T) {
	t.Parallel()

	manifest := set("go", "typescript")
	queries := set("go", "typescript")
	primary := set("go", "typescript")
	fallback := set("typescript", "php") // typescript is ignored from fallback when in primary.

	report := compareDrift(manifest, queries, primary, fallback)

	require.Equal(t, []string{"php"}, report.MissingInManifest)
	require.Empty(t, report.UnexpectedInManifest)
	require.Empty(t, report.MissingQueryFiles)
	require.Empty(t, report.UnexpectedQueryFiles)
	require.True(t, report.HasDrift())
}

func TestCompareDriftManifestVsQueriesOnly(t *testing.T) {
	t.Parallel()

	manifest := set("go", "python")
	queries := set("go", "rust")

	report := compareDrift(manifest, queries, nil, nil)

	require.Empty(t, report.MissingInManifest)
	require.Empty(t, report.UnexpectedInManifest)
	require.Equal(t, []string{"python"}, report.MissingQueryFiles)
	require.Equal(t, []string{"rust"}, report.UnexpectedQueryFiles)
	require.True(t, report.HasDrift())
}

func TestCompareDriftNoDrift(t *testing.T) {
	t.Parallel()

	manifest := set("go", "python")
	queries := set("go", "python")
	primary := set("go")
	fallback := set("python")

	report := compareDrift(manifest, queries, primary, fallback)

	require.False(t, report.HasDrift())
	require.Empty(t, report.MissingInManifest)
	require.Empty(t, report.UnexpectedInManifest)
	require.Empty(t, report.MissingQueryFiles)
	require.Empty(t, report.UnexpectedQueryFiles)
}

func TestExpectedNames(t *testing.T) {
	t.Parallel()

	result := expectedNames(set("go", "python"), set("python", "rust"))
	require.Equal(t, set("go", "python", "rust"), result)
}

func set(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		out[v] = struct{}{}
	}
	return out
}
