package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockSymbolParser implements symbolParser for testing without tree-sitter.
type mockSymbolParser struct {
	analysis *symbolAnalysis
	err      error
}

func (m *mockSymbolParser) Analyze(_ context.Context, _ string, _ []byte) (*symbolAnalysis, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.analysis, nil
}

func TestFuzzyMatchScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		query  string
		target string
		want   int
	}{
		{"exact match", "UserHandler", "UserHandler", -1},
		{"camelCase prefix", "UsrH", "UserHandler", -1},
		{"subsequence match", "UsrHndlr", "UserHandler", -1},
		{"case insensitive", "usrhndlr", "UserHandler", -1},
		{"no match", "xyz", "UserHandler", 0},
		{"empty query", "", "UserHandler", 0},
		{"empty target", "test", "", 0},
		{"underscore boundary", "uh", "user_handler", -1},
		{"single char", "U", "UserHandler", -1},
		{"partial camelCase", "UH", "UserHandler", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fuzzyMatchScore(tt.query, tt.target)
			if tt.want > 0 {
				require.Equal(t, tt.want, got, "score for %q vs %q", tt.query, tt.target)
			} else if tt.want == 0 {
				require.Equal(t, 0, got, "expected zero score for %q vs %q", tt.query, tt.target)
			}
			// tt.want < 0 means "any positive score is fine" — just verify > 0.
			if tt.want < 0 {
				require.Greater(t, got, 0, "expected positive score for %q vs %q", tt.query, tt.target)
			}
		})
	}
}

func TestFuzzyMatchScoreRanking(t *testing.T) {
	t.Parallel()

	// "UserHandler" should score higher than "UserHandlerFactory" for query "UH".
	short := fuzzyMatchScore("UH", "UserHandler")
	long := fuzzyMatchScore("UH", "UserHandlerFactory")
	require.Greater(t, short, long, "shorter name should score higher")
}

func TestFuzzySymbolLookupWithMock(t *testing.T) {
	orig := globalSymbolParser
	defer func() { globalSymbolParser = orig }()

	globalSymbolParser = &mockSymbolParser{
		analysis: &symbolAnalysis{
			Symbols: []symbolDef{
				{Name: "UserHandler", Kind: "struct", Line: 10},
				{Name: "HandleUser", Kind: "function", Line: 25},
				{Name: "UserHandlerFactory", Kind: "function", Line: 40},
				{Name: "ProcessRequest", Kind: "function", Line: 55},
			},
		},
	}

	matches, err := fuzzySymbolLookup(context.Background(), "UsrHndlr", "test.go", nil)
	require.NoError(t, err)
	require.Len(t, matches, 2, "expected 2 matches for 'UsrHndlr'")

	// UserHandler should rank higher than UserHandlerFactory.
	require.Equal(t, "UserHandler", matches[0].Name)
	require.Equal(t, "struct", matches[0].Kind)
	require.Equal(t, 10, matches[0].Line)
	require.Equal(t, "test.go", matches[0].FilePath)

	require.Equal(t, "UserHandlerFactory", matches[1].Name)

	// Verify descending score order.
	require.GreaterOrEqual(t, matches[0].Score, matches[1].Score)
}

func TestFuzzySymbolLookupNoParser(t *testing.T) {
	orig := globalSymbolParser
	defer func() { globalSymbolParser = orig }()

	globalSymbolParser = nil

	matches, err := fuzzySymbolLookup(context.Background(), "test", "file.go", nil)
	require.NoError(t, err)
	require.Nil(t, matches)
}

func TestFuzzySymbolLookupEmptyQuery(t *testing.T) {
	orig := globalSymbolParser
	defer func() { globalSymbolParser = orig }()

	globalSymbolParser = &mockSymbolParser{}

	matches, err := fuzzySymbolLookup(context.Background(), "", "file.go", nil)
	require.NoError(t, err)
	require.Nil(t, matches)
}

func TestFuzzySymbolLookupParseError(t *testing.T) {
	orig := globalSymbolParser
	defer func() { globalSymbolParser = orig }()

	globalSymbolParser = &mockSymbolParser{
		err: context.Canceled,
	}

	matches, err := fuzzySymbolLookup(context.Background(), "test", "file.go", nil)
	require.NoError(t, err)
	require.Nil(t, matches, "parse errors should be handled gracefully")
}

func TestFuzzySymbolLookupNilAnalysis(t *testing.T) {
	orig := globalSymbolParser
	defer func() { globalSymbolParser = orig }()

	globalSymbolParser = &mockSymbolParser{
		analysis: nil,
	}

	matches, err := fuzzySymbolLookup(context.Background(), "test", "file.go", nil)
	require.NoError(t, err)
	require.Nil(t, matches)
}

func TestFuzzySymbolLookupNoMatches(t *testing.T) {
	orig := globalSymbolParser
	defer func() { globalSymbolParser = orig }()

	globalSymbolParser = &mockSymbolParser{
		analysis: &symbolAnalysis{
			Symbols: []symbolDef{
				{Name: "CompletelyDifferent", Kind: "function", Line: 1},
			},
		},
	}

	matches, err := fuzzySymbolLookup(context.Background(), "XYZ", "file.go", nil)
	require.NoError(t, err)
	require.Empty(t, matches)
}

func TestFuzzyLookupStringMatchPreferred(t *testing.T) {
	orig := globalSymbolParser
	defer func() { globalSymbolParser = orig }()

	globalSymbolParser = &mockSymbolParser{
		analysis: &symbolAnalysis{
			Symbols: []symbolDef{
				{Name: "testFunc", Kind: "function", Line: 5},
			},
		},
	}

	content := []byte("package main\n\nfunc testFunc() {}\n")
	matched, found, symbols := FuzzyLookup(context.Background(), "testFunc", "test.go", content)
	require.True(t, found)
	require.NotEmpty(t, matched)
	require.Nil(t, symbols, "symbols should be nil when string match succeeds")
}

func TestFuzzyLookupFallsBackToSymbols(t *testing.T) {
	orig := globalSymbolParser
	defer func() { globalSymbolParser = orig }()

	globalSymbolParser = &mockSymbolParser{
		analysis: &symbolAnalysis{
			Symbols: []symbolDef{
				{Name: "UserHandler", Kind: "struct", Line: 10},
			},
		},
	}

	content := []byte("package main\n\ntype UserHandler struct{}\n")
	matched, found, symbols := FuzzyLookup(context.Background(), "UsrHndlr", "test.go", content)
	require.False(t, found)
	require.Empty(t, matched)
	require.Len(t, symbols, 1)
	require.Equal(t, "UserHandler", symbols[0].Name)
}

func TestFuzzyLookupNoMatchAtAll(t *testing.T) {
	orig := globalSymbolParser
	defer func() { globalSymbolParser = orig }()

	globalSymbolParser = nil

	content := []byte("package main\n")
	matched, found, symbols := FuzzyLookup(context.Background(), "nonexistent", "test.go", content)
	require.False(t, found)
	require.Empty(t, matched)
	require.Nil(t, symbols)
}

func TestSetSymbolParser(t *testing.T) {
	orig := globalSymbolParser
	defer func() { globalSymbolParser = orig }()

	SetSymbolParser(nil)
	require.Nil(t, globalSymbolParser)

	mock := &mockSymbolParser{}
	SetSymbolParser(mock)
	require.Equal(t, mock, globalSymbolParser)
}

func TestIsWordBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		runes string
		idx   int
		want  bool
	}{
		{"start of string", "hello", 0, true},
		{"underscore", "hello_world", 6, true},
		{"hyphen", "hello-world", 6, true},
		{"camelCase", "helloWorld", 5, true},
		{"mid lowercase", "hello", 2, false},
		{"uppercase no transition", "HELLO", 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isWordBoundary([]rune(tt.runes), tt.idx)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNewSymbolParserStub(t *testing.T) {
	// Without treesitter build tag, newSymbolParser should return nil.
	p := newSymbolParser(nil)
	require.Nil(t, p)
}
