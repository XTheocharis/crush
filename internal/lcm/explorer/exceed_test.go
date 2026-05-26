package explorer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExplorerExceedProfileAssertions_Exceed(t *testing.T) {
	t.Parallel()

	cfg := NewDefaultParityFixtureConfig(".")
	loader := NewParityFixtureLoader(cfg)
	index, err := loader.LoadIndex()
	require.NoError(t, err)

	registry := NewRegistry(WithOutputProfile(OutputProfileEnhancement))

	for lang, fixtureName := range index.Language {
		content, err := LoadFixtureFile(cfg, fixtureName)
		require.NoError(t, err)
		result, err := registry.exploreStatic(t.Context(), ExploreInput{Path: fixtureName, Content: content})
		require.NoError(t, err)
		require.NotEmpty(t, strings.TrimSpace(result.Summary), "language=%s summary must be non-empty", lang)
	}
}
