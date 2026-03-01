package explorer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Complete(_ context.Context, _, _ string) (string, error) {
	return m.response, m.err
}

func TestTruncateForLLM_Short(t *testing.T) {
	t.Parallel()
	content := "short content"
	result := truncateForLLM(content)
	require.Equal(t, content, result)
}

func TestTruncateForLLM_ExactLimit(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("x", llmTruncateMax)
	result := truncateForLLM(content)
	require.Equal(t, content, result)
}

func TestTruncateForLLM_OverLimit(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("a", llmTruncateHead) + strings.Repeat("b", 20_000) + strings.Repeat("c", llmTruncateMax-llmTruncateHead)
	result := truncateForLLM(content)
	require.Contains(t, result, "...[TRUNCATED]...")
	require.True(t, strings.HasPrefix(result, strings.Repeat("a", 100)))
	require.True(t, strings.HasSuffix(result, strings.Repeat("c", 100)))
}

func TestDetectLanguage_Extension(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.js", "javascript"},
		{"lib.ts", "typescript"},
		{"lib.rs", "rust"},
		{"App.java", "java"},
		{"main.cpp", "cpp"},
		{"main.c", "c"},
		{"script.rb", "ruby"},
		{"app.swift", "swift"},
		{"unknown.xyz", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			got := detectLanguage(tt.path, nil)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDetectLanguage_Shebang(t *testing.T) {
	t.Parallel()
	content := []byte("#!/usr/bin/env python3\nimport sys\n")
	got := detectLanguage("script", content)
	require.Equal(t, "python", got)
}

func TestGetLanguagePrompt_Known(t *testing.T) {
	t.Parallel()
	for lang := range languagePrompts {
		t.Run(lang, func(t *testing.T) {
			t.Parallel()
			prompt := getLanguagePrompt(lang)
			require.NotEmpty(t, prompt)
			require.Contains(t, prompt, "Read tool")
		})
	}
}

func TestGetLanguagePrompt_Unknown(t *testing.T) {
	t.Parallel()
	prompt := getLanguagePrompt("brainfuck")
	require.NotEmpty(t, prompt)
	require.Contains(t, prompt, "Read tool")
}

func TestGenerateLLMSummary_Success(t *testing.T) {
	t.Parallel()
	llm := &mockLLM{response: "A Go file that handles HTTP routing."}
	result, err := generateLLMSummary(context.Background(), llm, "server.go", []byte("package main"))
	require.NoError(t, err)
	require.Equal(t, "A Go file that handles HTTP routing.", result)
}

func TestGenerateLLMSummary_Error(t *testing.T) {
	t.Parallel()
	llm := &mockLLM{err: errors.New("rate limited")}
	_, err := generateLLMSummary(context.Background(), llm, "server.go", []byte("package main"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "server.go")
}

func TestGenerateAgentSummary_Success(t *testing.T) {
	t.Parallel()
	agentFn := func(_ context.Context, _, _, _ string) (string, error) {
		return "Agent analysis of Go file.", nil
	}
	result, err := generateAgentSummary(context.Background(), agentFn, "main.go", "go")
	require.NoError(t, err)
	require.Equal(t, "Agent analysis of Go file.", result)
}

func TestGenerateAgentSummary_Error(t *testing.T) {
	t.Parallel()
	agentFn := func(_ context.Context, _, _, _ string) (string, error) {
		return "", errors.New("agent timeout")
	}
	_, err := generateAgentSummary(context.Background(), agentFn, "main.go", "go")
	require.Error(t, err)
	require.Contains(t, err.Error(), "main.go")
}

func TestExploreLLMEnhanced_Tier1_NoLLM(t *testing.T) {
	t.Parallel()
	static := ExploreResult{Summary: "static", ExplorerUsed: "go", TokenEstimate: 10}
	result := exploreLLMEnhanced(context.Background(), nil, nil, ExploreInput{Path: "main.go"}, static)
	require.Equal(t, static, result)
}

func TestExploreLLMEnhanced_Tier2_LLM(t *testing.T) {
	t.Parallel()
	llm := &mockLLM{response: "LLM summary"}
	static := ExploreResult{Summary: "static", ExplorerUsed: "go", TokenEstimate: 10}
	result := exploreLLMEnhanced(context.Background(), llm, nil, ExploreInput{Path: "main.go", Content: []byte("package main")}, static)
	require.Equal(t, "LLM summary", result.Summary)
	require.Equal(t, "go+llm", result.ExplorerUsed)
}

func TestExploreLLMEnhanced_Tier3_Agent(t *testing.T) {
	t.Parallel()
	agentFn := func(_ context.Context, _, _, _ string) (string, error) {
		return "Agent summary", nil
	}
	static := ExploreResult{Summary: "static", ExplorerUsed: "go", TokenEstimate: 10}
	input := ExploreInput{Path: "main.go", Content: []byte("package main"), SessionID: "sess-123"}
	result := exploreLLMEnhanced(context.Background(), nil, agentFn, input, static)
	require.Equal(t, "Agent summary", result.Summary)
	require.Equal(t, "go+agent", result.ExplorerUsed)
}

func TestExploreLLMEnhanced_PythonSkipsTier2(t *testing.T) {
	t.Parallel()
	llm := &mockLLM{response: "should not be called"}
	static := ExploreResult{Summary: "static python", ExplorerUsed: "python", TokenEstimate: 10}
	input := ExploreInput{Path: "app.py", Content: []byte("import os")}
	result := exploreLLMEnhanced(context.Background(), llm, nil, input, static)
	require.Equal(t, "static python", result.Summary)
}

func TestExploreLLMEnhanced_AgentFallbackToLLM(t *testing.T) {
	t.Parallel()
	llm := &mockLLM{response: "LLM fallback"}
	agentFn := func(_ context.Context, _, _, _ string) (string, error) {
		return "", errors.New("agent failed")
	}
	static := ExploreResult{Summary: "static", ExplorerUsed: "go", TokenEstimate: 10}
	input := ExploreInput{Path: "main.go", Content: []byte("package main"), SessionID: "sess-123"}
	result := exploreLLMEnhanced(context.Background(), llm, agentFn, input, static)
	require.Equal(t, "LLM fallback", result.Summary)
	require.Equal(t, "go+llm", result.ExplorerUsed)
}

func TestNewRegistryWithLLM(t *testing.T) {
	t.Parallel()
	llm := &mockLLM{response: "enhanced"}
	agentFn := func(_ context.Context, _, _, _ string) (string, error) {
		return "agent", nil
	}
	r := NewRegistryWithLLM(llm, agentFn)
	require.NotNil(t, r)
	require.NotNil(t, r.llm)
	require.NotNil(t, r.agentFn)
	require.True(t, len(r.explorers) > 0)
}

func TestRegistryExplore_WithLLM(t *testing.T) {
	t.Parallel()
	llm := &mockLLM{response: "LLM enhanced Go summary"}
	r := NewRegistryWithLLM(llm, nil)
	result, err := r.Explore(context.Background(), ExploreInput{
		Path:    "main.go",
		Content: []byte("package main\n\nfunc main() {}"),
	})
	require.NoError(t, err)
	require.Contains(t, result.Summary, "## LLM enhanced Go summary")
	require.Contains(t, result.ExplorerUsed, "+llm")
}

// mockParser is a test implementation of treesitter.Parser.
type mockParser struct {
	closeErr  error
	analyzeFn func(ctx context.Context, path string, content []byte) (*treesitter.FileAnalysis, error)
}

func (m *mockParser) Analyze(ctx context.Context, path string, content []byte) (*treesitter.FileAnalysis, error) {
	if m.analyzeFn != nil {
		return m.analyzeFn(ctx, path, content)
	}
	return &treesitter.FileAnalysis{
		Language: "go",
		Symbols: []treesitter.SymbolInfo{
			{Name: "main", Kind: "function", Line: 1},
		},
		Imports: []treesitter.ImportInfo{
			{Path: "fmt", Category: treesitter.ImportCategoryStdlib},
		},
	}, nil
}

func (m *mockParser) Languages() []string {
	return []string{"go", "python", "javascript", "typescript", "rust", "java"}
}

func (m *mockParser) SupportsLanguage(lang string) bool {
	switch lang {
	case "go", "python", "javascript", "typescript", "rust", "java":
		return true
	default:
		return false
	}
}

func (m *mockParser) HasTags(lang string) bool {
	return m.SupportsLanguage(lang)
}

func (m *mockParser) ParseTree(_ context.Context, _ string, _ []byte) (*tree_sitter.Tree, error) {
	return nil, fmt.Errorf("ParseTree not implemented in mock")
}

func (m *mockParser) Close() error {
	return m.closeErr
}

func TestWithTreeSitter_OptionWiring(t *testing.T) {
	t.Parallel()
	parser := &mockParser{}
	r := NewRegistry(WithTreeSitter(parser))
	require.NotNil(t, r)
	require.NotNil(t, r.tsParser)
	require.Same(t, parser, r.tsParser)
}

func TestNewRegistry_WithTreeSitter_ExplorerChain(t *testing.T) {
	t.Parallel()
	parser := &mockParser{}
	r := NewRegistry(WithTreeSitter(parser))

	// Verify TreeSitterExplorer is in the chain.
	var treesitterExp *TreeSitterExplorer
	found := false
	for _, e := range r.explorers {
		if ts, ok := e.(*TreeSitterExplorer); ok {
			treesitterExp = ts
			found = true
			break
		}
	}
	require.True(t, found, "TreeSitterExplorer should be in the chain when parser is provided")
	require.NotNil(t, treesitterExp)
	require.Equal(t, OutputProfileEnhancement, treesitterExp.formatterProfile)

	// Check that it comes before TextExplorer data-format explorers.
	var tsIndex, textIndex int
	for i, e := range r.explorers {
		if _, ok := e.(*TreeSitterExplorer); ok {
			tsIndex = i
		}
		if _, ok := e.(*TextExplorer); ok {
			textIndex = i
		}
	}
	require.Less(t, tsIndex, textIndex, "TreeSitterExplorer should come before TextExplorer")
}

func TestNewRegistry_WithTreeSitter_DataFormatFirstOrdering(t *testing.T) {
	t.Parallel()
	parser := &mockParser{}
	r := NewRegistry(WithTreeSitter(parser))

	// Build a list of explorer type names in order.
	var types []string
	for _, e := range r.explorers {
		types = append(types, explorerTypeName(e))
	}

	// Verify data format explorers come before TreeSitterExplorer.
	jsonIdx, csvIdx, yamlIdx, tomlIdx, xmlIdx, htmlIdx := -1, -1, -1, -1, -1, -1
	tsIdx := -1

	for i, typ := range types {
		switch typ {
		case "JSONExplorer":
			jsonIdx = i
		case "CSVExplorer":
			csvIdx = i
		case "YAMLExplorer":
			yamlIdx = i
		case "TOMLExplorer":
			tomlIdx = i
		case "XMLExplorer":
			xmlIdx = i
		case "HTMLExplorer":
			htmlIdx = i
		case "TreeSitterExplorer":
			tsIdx = i
		}
	}

	// All data format explorers should come before TreeSitterExplorer.
	for _, idx := range []int{jsonIdx, csvIdx, yamlIdx, tomlIdx, xmlIdx, htmlIdx} {
		require.GreaterOrEqual(t, tsIdx, 0, "TreeSitterExplorer should be in the chain")
		if idx >= 0 {
			require.Less(t, idx, tsIdx, "%s should come before TreeSitterExplorer", types[idx])
		}
	}
}

func TestNewRegistry_BackwardCompatibility(t *testing.T) {
	t.Parallel()

	// No options provided - should work as before.
	r := NewRegistry()
	require.NotNil(t, r)
	require.Nil(t, r.tsParser)
	require.Greater(t, len(r.explorers), 0)

	// Verify TreeSitterExplorer is NOT in the chain without option.
	for _, e := range r.explorers {
		_, ok := e.(*TreeSitterExplorer)
		require.False(t, ok, "TreeSitterExplorer should NOT be in the chain without WithTreeSitter option")
	}

	// Empty options should be fine.
	r2 := NewRegistry()
	require.NotNil(t, r2)
	require.Nil(t, r2.tsParser)
}

func TestNewRegistryWithLLM_WithTreeSitter_Passthrough(t *testing.T) {
	t.Parallel()
	parser := &mockParser{}
	llm := &mockLLM{response: "test"}
	agentFn := func(_ context.Context, _, _, _ string) (string, error) {
		return "agent", nil
	}

	r := NewRegistryWithLLM(llm, agentFn, WithTreeSitter(parser))
	require.NotNil(t, r)
	require.NotNil(t, r.llm)
	require.NotNil(t, r.agentFn)
	require.NotNil(t, r.tsParser)
	require.Same(t, parser, r.tsParser)

	// Verify TreeSitterExplorer is in the chain.
	var found bool
	for _, e := range r.explorers {
		if _, ok := e.(*TreeSitterExplorer); ok {
			found = true
			break
		}
	}
	require.True(t, found, "TreeSitterExplorer should be in the chain with option passthrough")
}

func TestNewRegistryWithLLM_BackwardCompatibility(t *testing.T) {
	t.Parallel()
	llm := &mockLLM{response: "test"}

	// No TreeSitter option provided - should preserve old signature.
	r := NewRegistryWithLLM(llm, nil)
	require.NotNil(t, r)
	require.NotNil(t, r.llm)
	require.Nil(t, r.agentFn)
	require.Nil(t, r.tsParser)
}

func TestNewRegistryWithLLM_NoOptions(t *testing.T) {
	t.Parallel()
	llm := &mockLLM{response: "test"}

	// Explicitly passing no options.
	r := NewRegistryWithLLM(llm, nil)
	require.NotNil(t, r)
	require.NotNil(t, r.llm)
	require.Nil(t, r.tsParser)

	// Verify TreeSitterExplorer is NOT in the chain.
	for _, e := range r.explorers {
		_, ok := e.(*TreeSitterExplorer)
		require.False(t, ok, "TreeSitterExplorer should not be in chain with no options")
	}
}

func TestTreeSitterExplorer_InChain_HandlesSupportedFiles(t *testing.T) {
	t.Parallel()
	parser := &mockParser{}
	r := NewRegistry(WithTreeSitter(parser))

	content := []byte(`package main

func main() {
	fmt.Println("hello")
}
`)

	result, err := r.Explore(context.Background(), ExploreInput{
		Path:    "main.go",
		Content: content,
	})
	require.NoError(t, err)

	// TreeSitterExplorer handles code files when a parser is configured.
	// This test just verifies that the chain works.
	require.NotEmpty(t, result.Summary)
}

// explorerTypeName returns the name of an explorer type for testing.
func explorerTypeName(e Explorer) string {
	switch e.(type) {
	case *BinaryExplorer:
		return "BinaryExplorer"
	case *ShellExplorer:
		return "ShellExplorer"
	case *JSONExplorer:
		return "JSONExplorer"
	case *CSVExplorer:
		return "CSVExplorer"
	case *YAMLExplorer:
		return "YAMLExplorer"
	case *TOMLExplorer:
		return "TOMLExplorer"
	case *INIExplorer:
		return "INIExplorer"
	case *XMLExplorer:
		return "XMLExplorer"
	case *HTMLExplorer:
		return "HTMLExplorer"
	case *MarkdownExplorer:
		return "MarkdownExplorer"
	case *LatexExplorer:
		return "LatexExplorer"
	case *SQLiteExplorer:
		return "SQLiteExplorer"
	case *LogsExplorer:
		return "LogsExplorer"
	case *TextExplorer:
		return "TextExplorer"
	case *FallbackExplorer:
		return "FallbackExplorer"
	case *TreeSitterExplorer:
		return "TreeSitterExplorer"
	default:
		return "Unknown"
	}
}
