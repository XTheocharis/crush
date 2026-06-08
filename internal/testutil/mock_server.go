// Package testutil provides shared helpers for integration tests.
package testutil

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// MockToolCall represents a single tool call in an OpenAI-format response.
type MockToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// MockResponse configures what the mock server returns for the next request.
type MockResponse struct {
	Content   string         // Text content for the assistant message.
	ToolCalls []MockToolCall // Tool calls to include in the response.
	IsError   bool           // Inject an error response.
	ErrorCode int            // HTTP status code for error (e.g. 429, 500).
	IsStream  bool           // Use SSE streaming instead of JSON.
	Latency   time.Duration  // Delay between SSE chunks (streaming only).
}

// MockServer is an OpenAI-compatible HTTP test server for deterministic
// testing. It handles /v1/chat/completions and /chat/completions endpoints,
// supporting non-streaming JSON, SSE streaming, tool_use responses, error
// injection, and sentinel echo mode.
type MockServer struct {
	t      *testing.T
	server *httptest.Server

	mu           sync.Mutex
	response     MockResponse
	responseFunc func(reqBody map[string]any) MockResponse
	requests     []map[string]any
}

// NewMockServer creates and starts a new mock server. The server is
// automatically closed when the test finishes.
func NewMockServer(t *testing.T) *MockServer {
	t.Helper()
	ms := &MockServer{
		t: t,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", ms.handleCompletions)
	mux.HandleFunc("/chat/completions", ms.handleCompletions)
	ms.server = httptest.NewServer(mux)
	t.Cleanup(ms.server.Close)
	return ms
}

// Server returns the underlying httptest.Server.
func (ms *MockServer) Server() *httptest.Server {
	return ms.server
}

// URL returns the base URL of the mock server for use as baseURL in provider
// config (e.g. openai.WithBaseURL(ms.URL())).
func (ms *MockServer) URL() string {
	return ms.server.URL
}

// SetResponse configures the response for the next request(s).
func (ms *MockServer) SetResponse(resp MockResponse) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.response = resp
	ms.responseFunc = nil
}

// SetResponseFunc sets a dynamic response function. The function receives the
// parsed request body and returns the response to send. Overrides
// SetResponse.
func (ms *MockServer) SetResponseFunc(fn func(reqBody map[string]any) MockResponse) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.responseFunc = fn
}

// Requests returns a copy of all recorded request bodies.
func (ms *MockServer) Requests() []map[string]any {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	out := make([]map[string]any, len(ms.requests))
	copy(out, ms.requests)
	return out
}

// handleCompletions is the main HTTP handler for chat completion requests.
func (ms *MockServer) handleCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Record the request.
	ms.mu.Lock()
	ms.requests = append(ms.requests, reqBody)

	// Determine response.
	var resp MockResponse
	if ms.responseFunc != nil {
		resp = ms.responseFunc(reqBody)
	} else {
		resp = ms.response
	}
	ms.mu.Unlock()

	// Inject errors.
	if resp.IsError {
		code := resp.ErrorCode
		if code == 0 {
			code = http.StatusInternalServerError
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		fmt.Fprintf(w, `{"error":{"message":"mock error","type":"server_error","code":%d}}`, code)
		return
	}

	// Sentinel echo mode: scan request messages for SENTINEL_* patterns.
	echoContent := extractSentinel(reqBody)

	// If sentinel found and no explicit content set, use echo.
	if echoContent != "" && resp.Content == "" && len(resp.ToolCalls) == 0 {
		resp.Content = echoContent
	}

	// Check if streaming is requested.
	isStream := false
	if v, ok := reqBody["stream"].(bool); ok && v {
		isStream = true
	}

	// Force streaming if the response is configured for it.
	if resp.IsStream {
		isStream = true
	}

	if isStream {
		ms.writeSSE(w, resp)
	} else {
		ms.writeJSON(w, resp)
	}
}

// writeJSON writes a non-streaming OpenAI-format JSON response.
func (ms *MockServer) writeJSON(w http.ResponseWriter, resp MockResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	content := resp.Content
	finishReason := "stop"
	var rawToolCalls any

	if len(resp.ToolCalls) > 0 {
		finishReason = "tool_calls"
		content = "" // Tool call responses set content to empty.
		rawToolCalls = resp.ToolCalls
	}

	response := map[string]any{
		"id":      "mock-id",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "mock-model",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":       "assistant",
					"content":    strPtr(content),
					"tool_calls": rawToolCalls,
				},
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	}

	enc := json.NewEncoder(w)
	if err := enc.Encode(response); err != nil {
		ms.t.Logf("mock server: failed to encode JSON response: %v", err)
	}
}

// writeSSE writes a streaming OpenAI-format SSE response.
func (ms *MockServer) writeSSE(w http.ResponseWriter, resp MockResponse) {
	flusher, canFlush := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	content := resp.Content
	latency := resp.Latency

	if len(resp.ToolCalls) > 0 {
		// Stream tool calls as a single chunk.
		chunk := map[string]any{
			"id":      "mock-id",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   "mock-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"role":       "assistant",
						"content":    nil,
						"tool_calls": resp.ToolCalls,
					},
					"finish_reason": nil,
				},
			},
		}
		ms.writeSSEChunk(w, chunk, canFlush, flusher)
		if latency > 0 {
			time.Sleep(latency)
		}

		// Final chunk with finish_reason.
		doneChunk := map[string]any{
			"id":      "mock-id",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   "mock-model",
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         map[string]any{},
					"finish_reason": "tool_calls",
				},
			},
		}
		ms.writeSSEChunk(w, doneChunk, canFlush, flusher)
	} else if content != "" {
		chunkSize := min(5, len(content))
		for i := 0; i < len(content); i += chunkSize {
			end := min(i+chunkSize, len(content))
			chunk := content[i:end]
			sseChunk := map[string]any{
				"id":      "mock-id",
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   "mock-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role":    "assistant",
							"content": chunk,
						},
						"finish_reason": nil,
					},
				},
			}
			ms.writeSSEChunk(w, sseChunk, canFlush, flusher)
			if latency > 0 {
				time.Sleep(latency)
			}
		}

		// Final chunk with finish_reason.
		doneChunk := map[string]any{
			"id":      "mock-id",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   "mock-model",
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         map[string]any{},
					"finish_reason": "stop",
				},
			},
		}
		ms.writeSSEChunk(w, doneChunk, canFlush, flusher)
	}

	// Write [DONE] terminator.
	fmt.Fprint(w, "data: [DONE]\n\n")
	if canFlush {
		flusher.Flush()
	}
}

// writeSSEChunk writes a single SSE data chunk.
func (ms *MockServer) writeSSEChunk(w io.Writer, data any, canFlush bool, flusher http.Flusher) {
	b, err := json.Marshal(data)
	if err != nil {
		ms.t.Logf("mock server: failed to marshal SSE chunk: %v", err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
	if canFlush {
		flusher.Flush()
	}
}

// strPtr returns a string pointer for the given value, or nil if empty.
func strPtr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// sentinelPattern matches SENTINEL_<alphanumeric> tokens in message content.
var sentinelPattern = regexp.MustCompile(`SENTINEL_[A-Za-z0-9_]+`)

// extractSentinel scans request messages for SENTINEL_* patterns and returns
// all matches joined by spaces.
func extractSentinel(reqBody map[string]any) string {
	messages, ok := reqBody["messages"].([]any)
	if !ok {
		return ""
	}
	var matches []string
	for _, msg := range messages {
		m, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		content, ok := m["content"].(string)
		if !ok {
			continue
		}
		found := sentinelPattern.FindAllString(content, -1)
		matches = append(matches, found...)
	}
	if len(matches) == 0 {
		return ""
	}
	return strings.Join(matches, " ")
}

// ReadSSEStream reads an SSE response body and returns the parsed data lines.
// Returns a slice of raw strings (the data content, excluding "data: " prefix).
// Useful in tests for validating streaming responses.
func ReadSSEStream(body io.Reader) []string {
	scanner := bufio.NewScanner(body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			lines = append(lines, data)
		}
	}
	return lines
}
