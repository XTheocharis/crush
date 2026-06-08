package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestToolSurfaceInfluencesToolSelection(t *testing.T) {
	t.Parallel()

	t.Run("surface_includes_edit_tool", func(t *testing.T) {
		t.Parallel()

		surface := NewToolSurface()
		surface.UpdateCapabilities(SurfaceContext{
			HasLSP:    true,
			HasLCM:    true,
			BetaTools: false,
		})

		visible := surface.GetVisibleTools()
		require.Contains(t, visible, "edit")

		descriptions := buildToolDescriptions(visible)
		require.True(t, strings.Contains(descriptions, "edit"))

		mock := testutil.NewMockServer(t)
		mock.SetResponse(testutil.MockResponse{
			ToolCalls: []testutil.MockToolCall{
				newToolCall("call_edit_1", "edit", `{"file_path":"/tmp/test.go","old_string":"foo","new_string":"bar"}`),
			},
		})

		resp := postMockCompletion(t, mock, map[string]any{
			"messages": []map[string]any{
				{"role": "system", "content": descriptions},
				{"role": "user", "content": "fix the typo"},
			},
			"tools": namesToToolDefs(visible),
		})

		toolCalls := parseToolCalls(t, resp)
		require.Len(t, toolCalls, 1)
		require.Equal(t, "edit", toolCalls[0].Name)
	})

	t.Run("no_lsp_hides_code_intelligence_tools", func(t *testing.T) {
		t.Parallel()

		surface := NewToolSurface()
		surface.UpdateCapabilities(SurfaceContext{
			HasLSP: false,
			HasLCM: true,
		})

		visible := surface.GetVisibleTools()
		for _, name := range visible {
			require.False(t, surface.HasCapability(name, CapabilityCodeIntelligence),
				"%s should not be visible when HasLSP=false", name)
		}

		hidden := surface.GetHiddenTools()
		for _, name := range []string{"lsp_diagnostics", "lsp_references", "lsp_definition"} {
			require.Contains(t, hidden, name, "%s should be hidden without LSP", name)
		}
	})

	t.Run("planning_phase_hides_edit_tools", func(t *testing.T) {
		t.Parallel()

		surface := NewToolSurface()
		surface.UpdateCapabilities(SurfaceContext{})

		visible := surface.GetVisibleTools()
		require.Contains(t, visible, "edit")

		phaseFiltered := PhaseFilteredTools(visible, PhasePlanning)
		require.NotContains(t, phaseFiltered, "edit")
		require.NotContains(t, phaseFiltered, "multiedit")
		require.NotContains(t, phaseFiltered, "write")
		require.Contains(t, phaseFiltered, "view")
		require.Contains(t, phaseFiltered, "bash")
		require.Contains(t, phaseFiltered, "grep")

		mock := testutil.NewMockServer(t)
		mock.SetResponse(testutil.MockResponse{
			ToolCalls: []testutil.MockToolCall{
				newToolCall("call_view_1", "view", `{"file_path":"/tmp/test.go"}`),
			},
		})

		resp := postMockCompletion(t, mock, map[string]any{
			"messages": []map[string]any{
				{"role": "user", "content": "plan the architecture"},
			},
			"tools": namesToToolDefs(phaseFiltered),
		})

		toolCalls := parseToolCalls(t, resp)
		require.Len(t, toolCalls, 1)
		require.Equal(t, "view", toolCalls[0].Name)
	})
}

func TestSurfaceDescriptionContainsVisibleTools(t *testing.T) {
	t.Parallel()

	surface := NewToolSurface()
	surface.UpdateCapabilities(SurfaceContext{
		HasLSP:    true,
		HasLCM:    true,
		HasMCP:    true,
		BetaTools: false,
	})

	visible := surface.GetVisibleTools()
	hidden := surface.GetHiddenTools()
	descriptions := buildToolDescriptions(visible)

	for _, name := range visible {
		require.True(t, strings.Contains(descriptions, name),
			"visible tool %q must appear in surface descriptions", name)
	}

	for _, name := range hidden {
		require.False(t, strings.Contains(descriptions, name),
			"hidden tool %q must NOT appear in surface descriptions", name)
	}
}

func TestMockServerDeterministicToolSelection(t *testing.T) {
	t.Parallel()

	mock := testutil.NewMockServer(t)
	mock.SetResponse(testutil.MockResponse{
		ToolCalls: []testutil.MockToolCall{
			newToolCall("call_bash_1", "bash", `{"command":"echo hello"}`),
			newToolCall("call_view_1", "view", `{"file_path":"test.go"}`),
		},
	})

	prompts := []string{"list files", "read the code", "show me the project"}
	for i, prompt := range prompts {
		resp := postMockCompletion(t, mock, map[string]any{
			"messages": []map[string]any{
				{"role": "user", "content": prompt},
			},
		})

		toolCalls := parseToolCalls(t, resp)
		require.Len(t, toolCalls, 2, "request %d: expected 2 tool calls", i)
		require.Equal(t, "bash", toolCalls[0].Name, "request %d", i)
		require.Equal(t, "view", toolCalls[1].Name, "request %d", i)
	}

	require.Len(t, mock.Requests(), len(prompts))
}

func newToolCall(id, name, arguments string) testutil.MockToolCall {
	return testutil.MockToolCall{
		ID:   id,
		Type: "function",
		Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{
			Name:      name,
			Arguments: arguments,
		},
	}
}

func buildToolDescriptions(visible []string) string {
	var b strings.Builder
	b.WriteString("Available tools:\n")
	for _, name := range visible {
		b.WriteString("  - ")
		b.WriteString(name)
		b.WriteByte('\n')
	}
	return b.String()
}

func namesToToolDefs(names []string) []map[string]any {
	defs := make([]map[string]any, len(names))
	for i, name := range names {
		defs[i] = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": name,
			},
		}
	}
	return defs
}

func postMockCompletion(t *testing.T, mock *testutil.MockServer, payload map[string]any) map[string]any {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	resp, err := http.Post(mock.URL()+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "mock server returned non-200: %s", string(respBody))

	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	return result
}

type mockToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func parseToolCalls(t *testing.T, resp map[string]any) []mockToolCall {
	t.Helper()

	choices, ok := resp["choices"].([]any)
	require.True(t, ok, "response should have choices")
	require.NotEmpty(t, choices, "choices should not be empty")

	first, ok := choices[0].(map[string]any)
	require.True(t, ok)

	message, ok := first["message"].(map[string]any)
	require.True(t, ok)

	rawToolCalls, ok := message["tool_calls"].([]any)
	require.True(t, ok, "message should have tool_calls")

	var result []mockToolCall
	for _, raw := range rawToolCalls {
		tc, ok := raw.(map[string]any)
		require.True(t, ok)

		fn, ok := tc["function"].(map[string]any)
		require.True(t, ok)

		name, _ := fn["name"].(string)
		args, _ := fn["arguments"].(string)
		result = append(result, mockToolCall{Name: name, Arguments: args})
	}
	return result
}
