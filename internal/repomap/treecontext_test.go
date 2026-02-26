package repomap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderTreeContextBasicGapsAndPrefixes(t *testing.T) {
	t.Parallel()

	lines := []string{
		"package main",
		"",
		"func a() {",
		"\tprintln(1)",
		"}",
		"func b() {",
		"\tprintln(2)",
		"}",
	}
	show := map[int]struct{}{0: {}, 3: {}, 6: {}}

	got := RenderTreeContext(lines, show)
	require.Equal(t, "│package main\n│\n⋮\n│\tprintln(1)\n⋮\n│\tprintln(2)\n", got)
}

func TestRenderTreeContextClosesSingleLineGap(t *testing.T) {
	t.Parallel()

	lines := []string{"a", "mid", "b"}
	show := map[int]struct{}{0: {}, 2: {}}

	got := RenderTreeContext(lines, show)
	require.Equal(t, "│a\n│mid\n│b\n", got)
}

func TestRenderTreeContextBlankLineAdjacency(t *testing.T) {
	t.Parallel()

	lines := []string{"x", "", "", "y"}
	show := map[int]struct{}{0: {}, 3: {}}

	got := RenderTreeContext(lines, show)
	require.Equal(t, "│x\n│\n│\n│y\n", got)
}

func TestRenderTreeContextOutOfRangeAndEmpty(t *testing.T) {
	t.Parallel()

	require.Empty(t, RenderTreeContext(nil, map[int]struct{}{0: {}}))
	require.Empty(t, RenderTreeContext([]string{"a"}, nil))
	require.Empty(t, RenderTreeContext([]string{"a"}, map[int]struct{}{5: {}}))
}
