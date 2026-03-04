package agent

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestEstimateToolTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		toolName    string
		description string
		wantMin     int64
	}{
		{
			name:        "short tool",
			toolName:    "read",
			description: "Read a file",
			wantMin:     1,
		},
		{
			name:        "tool with long description",
			toolName:    "bash",
			description: "Execute a bash command in the user's shell environment. Supports timeouts, background execution, and working directory configuration.",
			wantMin:     20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tool := &stubAgentTool{
				name:        tt.toolName,
				description: tt.description,
			}
			tokens := estimateToolTokens(tool)
			require.GreaterOrEqual(t, tokens, tt.wantMin)
			require.Greater(t, tokens, int64(0))
		})
	}
}

func TestEstimateToolTokens_IncludesSchemaOverhead(t *testing.T) {
	t.Parallel()

	// A tool with parameters should have more estimated tokens than one without.
	noParams := &stubAgentTool{
		name:        "simple",
		description: "A simple tool",
	}
	withParams := &stubAgentTool{
		name:        "complex",
		description: "A complex tool",
		parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "File path to read"},
				"content": map[string]any{"type": "string", "description": "Content to write"},
			},
		},
		required: []string{"path", "content"},
	}

	tokensNoParams := estimateToolTokens(noParams)
	tokensWithParams := estimateToolTokens(withParams)
	require.Greater(t, tokensWithParams, tokensNoParams)
}

// stubAgentTool implements fantasy.AgentTool for testing.
type stubAgentTool struct {
	name        string
	description string
	parameters  map[string]any
	required    []string
}

func (s *stubAgentTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        s.name,
		Description: s.description,
		Parameters:  s.parameters,
		Required:    s.required,
	}
}

func (s *stubAgentTool) Run(_ context.Context, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return fantasy.ToolResponse{}, nil
}

func (s *stubAgentTool) ProviderOptions() fantasy.ProviderOptions { return nil }

func (s *stubAgentTool) SetProviderOptions(_ fantasy.ProviderOptions) {}
