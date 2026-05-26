package lsp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNamePathMatcher_SimpleGlob(t *testing.T) {
	t.Parallel()
	m := NewNamePathMatcher([]string{"*.proto"})
	require.True(t, m.Match("api/foo.proto"))
	require.True(t, m.Match("bar.proto"))
	require.False(t, m.Match("api/foo.go"))
}

func TestNamePathMatcher_DoubleStar(t *testing.T) {
	t.Parallel()
	m := NewNamePathMatcher([]string{"**/graphql/**"})
	require.True(t, m.Match("src/graphql/schema.graphql"))
	require.True(t, m.Match("pkg/internal/graphql/resolver.go"))
	require.False(t, m.Match("src/api/rest/handler.go"))
}

func TestNamePathMatcher_PrefixGlob(t *testing.T) {
	t.Parallel()
	m := NewNamePathMatcher([]string{"src/generated/**"})
	require.True(t, m.Match("src/generated/foo.go"))
	require.True(t, m.Match("src/generated/bar/baz.ts"))
	require.False(t, m.Match("src/main.go"))
}

func TestNamePathMatcher_EmptyPatterns(t *testing.T) {
	t.Parallel()
	m := NewNamePathMatcher(nil)
	require.False(t, m.Match("any/file.go"))

	m = NewNamePathMatcher([]string{})
	require.False(t, m.Match("any/file.go"))
}

func TestNamePathMatcher_MultiplePatterns(t *testing.T) {
	t.Parallel()
	m := NewNamePathMatcher([]string{"*.proto", "*.graphql"})
	require.True(t, m.Match("api/foo.proto"))
	require.True(t, m.Match("schema.graphql"))
	require.False(t, m.Match("main.go"))
}

func TestNamePathMatcher_Patterns(t *testing.T) {
	t.Parallel()
	m := NewNamePathMatcher([]string{"*.proto"})
	require.Equal(t, []string{"*.proto"}, m.Patterns())
}
