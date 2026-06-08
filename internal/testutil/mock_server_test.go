package testutil

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func postJSON(t *testing.T, url string, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	return resp
}

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return b
}

func TestMockServer_NonStreamingResponse(t *testing.T) {
	ms := NewMockServer(t)
	ms.SetResponse(MockResponse{
		Content: "Hello, world!",
	})

	resp := postJSON(t, ms.URL()+"/v1/chat/completions", `{
		"model": "mock-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": false
	}`)

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.Unmarshal(readBody(t, resp), &result))

	require.Equal(t, "chat.completion", result["object"])
	choices := result["choices"].([]any)
	require.Len(t, choices, 1)
	choice := choices[0].(map[string]any)
	require.Equal(t, "stop", choice["finish_reason"])

	msg := choice["message"].(map[string]any)
	require.Equal(t, "assistant", msg["role"])
	require.Equal(t, "Hello, world!", msg["content"])
}

func TestMockServer_SSEStreamingResponse(t *testing.T) {
	ms := NewMockServer(t)
	ms.SetResponse(MockResponse{
		Content:  "Hello!",
		IsStream: true,
	})

	resp := postJSON(t, ms.URL()+"/v1/chat/completions", `{
		"model": "mock-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()

	lines := ReadSSEStream(resp.Body)
	require.True(t, len(lines) >= 3, "expected at least 3 SSE lines, got %d", len(lines))

	// First chunk should have content in delta.
	var first map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	chunks := first["choices"].([]any)
	delta := chunks[0].(map[string]any)["delta"].(map[string]any)
	require.Equal(t, "assistant", delta["role"])

	// Verify the content was streamed across chunks.
	var assembled strings.Builder
	for _, line := range lines {
		if line == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}
		choices := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		d := choices[0].(map[string]any)["delta"].(map[string]any)
		if c, ok := d["content"].(string); ok && c != "" {
			assembled.WriteString(c)
		}
	}
	require.Equal(t, "Hello!", assembled.String())

	// Last line must be [DONE].
	require.Equal(t, "[DONE]", lines[len(lines)-1])
}

func TestMockServer_ToolUseResponse(t *testing.T) {
	ms := NewMockServer(t)
	tc := MockToolCall{
		ID:   "call_mock",
		Type: "function",
	}
	tc.Function.Name = "edit"
	tc.Function.Arguments = `{"file_path":"test.go","old_string":"old","new_string":"new"}`

	ms.SetResponse(MockResponse{
		ToolCalls: []MockToolCall{tc},
	})

	resp := postJSON(t, ms.URL()+"/v1/chat/completions", `{
		"model": "mock-model",
		"messages": [{"role": "user", "content": "edit the file"}],
		"stream": false
	}`)

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.Unmarshal(readBody(t, resp), &result))

	choices := result["choices"].([]any)
	choice := choices[0].(map[string]any)
	require.Equal(t, "tool_calls", choice["finish_reason"])

	msg := choice["message"].(map[string]any)
	require.Equal(t, "assistant", msg["role"])

	toolCalls := msg["tool_calls"].([]any)
	require.Len(t, toolCalls, 1)
	tcResult := toolCalls[0].(map[string]any)
	require.Equal(t, "call_mock", tcResult["id"])
	require.Equal(t, "function", tcResult["type"])

	fn := tcResult["function"].(map[string]any)
	require.Equal(t, "edit", fn["name"])
	require.Equal(t, `{"file_path":"test.go","old_string":"old","new_string":"new"}`, fn["arguments"])
}

func TestMockServer_ErrorInjection(t *testing.T) {
	ms := NewMockServer(t)
	ms.SetResponse(MockResponse{
		IsError:   true,
		ErrorCode: 429,
	})

	resp := postJSON(t, ms.URL()+"/v1/chat/completions", `{
		"model": "mock-model",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	require.Equal(t, 429, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(readBody(t, resp), &errResp))
	errObj := errResp["error"].(map[string]any)
	require.Equal(t, "mock error", errObj["message"])
}

func TestMockServer_SentinelEcho(t *testing.T) {
	ms := NewMockServer(t)
	// No explicit response set — sentinel echo mode kicks in.

	resp := postJSON(t, ms.URL()+"/v1/chat/completions", `{
		"model": "mock-model",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Please process SENTINEL_HELLO_123 and reply."}
		],
		"stream": false
	}`)

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.Unmarshal(readBody(t, resp), &result))

	choices := result["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	require.Equal(t, "SENTINEL_HELLO_123", msg["content"])
}

func TestMockServer_RequestRecording(t *testing.T) {
	ms := NewMockServer(t)
	ms.SetResponse(MockResponse{Content: "ok"})

	// Send two requests.
	postJSON(t, ms.URL()+"/v1/chat/completions", `{
		"model": "mock-model",
		"messages": [{"role": "user", "content": "first"}]
	}`)
	postJSON(t, ms.URL()+"/chat/completions", `{
		"model": "mock-model",
		"messages": [{"role": "user", "content": "second"}]
	}`)

	reqs := ms.Requests()
	require.Len(t, reqs, 2)

	msgs1 := reqs[0]["messages"].([]any)
	content1 := msgs1[0].(map[string]any)["content"].(string)
	require.Equal(t, "first", content1)

	msgs2 := reqs[1]["messages"].([]any)
	content2 := msgs2[0].(map[string]any)["content"].(string)
	require.Equal(t, "second", content2)
}

func TestMockServer_DynamicResponseFunc(t *testing.T) {
	ms := NewMockServer(t)
	ms.SetResponseFunc(func(reqBody map[string]any) MockResponse {
		msgs := reqBody["messages"].([]any)
		lastMsg := msgs[len(msgs)-1].(map[string]any)
		content := lastMsg["content"].(string)
		return MockResponse{
			Content: "echo: " + content,
		}
	})

	resp := postJSON(t, ms.URL()+"/v1/chat/completions", `{
		"model": "mock-model",
		"messages": [{"role": "user", "content": "ping"}],
		"stream": false
	}`)

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.Unmarshal(readBody(t, resp), &result))

	choices := result["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	require.Equal(t, "echo: ping", msg["content"])
}

func TestMockServer_SSEStreamingWithLatency(t *testing.T) {
	ms := NewMockServer(t)
	ms.SetResponse(MockResponse{
		Content:  "ABCDEFGHIJ",
		IsStream: true,
		Latency:  10 * time.Millisecond,
	})

	start := time.Now()
	resp := postJSON(t, ms.URL()+"/v1/chat/completions", `{
		"model": "mock-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Read all SSE data.
	scanner := bufio.NewScanner(resp.Body)
	var chunkCount int
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data: ") {
			chunkCount++
		}
	}
	resp.Body.Close()

	// Content "ABCDEFGHIJ" (10 chars) at 5 chars/chunk = 2 content chunks + 1 done chunk + 1 [DONE].
	require.Equal(t, 4, chunkCount)

	// Should have taken at least 20ms (2 chunks with 10ms delay each, skipping last).
	elapsed := time.Since(start)
	require.True(t, elapsed >= 15*time.Millisecond, "expected latency-induced delay, got %v", elapsed)
}

func TestMockServer_StreamingToolCalls(t *testing.T) {
	ms := NewMockServer(t)
	tc := MockToolCall{
		ID:   "call_stream_tool",
		Type: "function",
	}
	tc.Function.Name = "bash"
	tc.Function.Arguments = `{"command":"ls -la"}`

	ms.SetResponse(MockResponse{
		ToolCalls: []MockToolCall{tc},
		IsStream:  true,
	})

	resp := postJSON(t, ms.URL()+"/v1/chat/completions", `{
		"model": "mock-model",
		"messages": [{"role": "user", "content": "list files"}],
		"stream": true
	}`)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()

	lines := ReadSSEStream(resp.Body)
	require.True(t, len(lines) >= 2, "expected at least 2 SSE lines")

	// First chunk should contain tool_calls in delta.
	var first map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	choices := first["choices"].([]any)
	delta := choices[0].(map[string]any)["delta"].(map[string]any)
	toolCalls := delta["tool_calls"].([]any)
	require.Len(t, toolCalls, 1)

	fn := toolCalls[0].(map[string]any)["function"].(map[string]any)
	require.Equal(t, "bash", fn["name"])

	// Second chunk should have finish_reason tool_calls.
	var second map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &second))
	choices2 := second["choices"].([]any)
	require.Equal(t, "tool_calls", choices2[0].(map[string]any)["finish_reason"])

	// Last line must be [DONE].
	require.Equal(t, "[DONE]", lines[len(lines)-1])
}

func TestMockServer_MultipleSentinels(t *testing.T) {
	ms := NewMockServer(t)

	resp := postJSON(t, ms.URL()+"/v1/chat/completions", `{
		"model": "mock-model",
		"messages": [
			{"role": "user", "content": "Process SENTINEL_ALPHA and SENTINEL_BETA_42"}
		],
		"stream": false
	}`)

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.Unmarshal(readBody(t, resp), &result))

	choices := result["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	require.Equal(t, "SENTINEL_ALPHA SENTINEL_BETA_42", msg["content"])
}
