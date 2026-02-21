package explorer

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	require.Equal(t, "LLM enhanced Go summary", result.Summary)
	require.Contains(t, result.ExplorerUsed, "+llm")
}
