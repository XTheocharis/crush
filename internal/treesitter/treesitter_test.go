package treesitter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTagString(t *testing.T) {
	tag := Tag{
		RelPath:  "internal/config/config.go",
		Line:     42,
		Kind:     "def",
		Name:     "Options",
		NodeType: "struct",
	}
	require.Equal(t, "internal/config/config.go:42 def Options [struct]", tag.String())
}

func TestSymbolInfoFields(t *testing.T) {
	s := SymbolInfo{
		Name:       "Analyze",
		Kind:       "function",
		Line:       10,
		EndLine:    20,
		Params:     "ctx context.Context",
		ReturnType: "error",
		Modifiers:  []string{"public"},
		Decorators: []string{"trace"},
		Parent:     "Parser",
		DocComment: "Analyze parses content.",
	}

	require.Equal(t, "Analyze", s.Name)
	require.Equal(t, "function", s.Kind)
	require.Equal(t, 10, s.Line)
	require.Equal(t, 20, s.EndLine)
	require.Equal(t, "ctx context.Context", s.Params)
	require.Equal(t, "error", s.ReturnType)
	require.Equal(t, []string{"public"}, s.Modifiers)
	require.Equal(t, []string{"trace"}, s.Decorators)
	require.Equal(t, "Parser", s.Parent)
	require.Equal(t, "Analyze parses content.", s.DocComment)
}

func TestImportCategoryValues(t *testing.T) {
	require.Equal(t, "stdlib", ImportCategoryStdlib)
	require.Equal(t, "third_party", ImportCategoryThirdParty)
	require.Equal(t, "local", ImportCategoryLocal)
	require.Equal(t, "unknown", ImportCategoryUnknown)
}
