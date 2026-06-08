package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

// --- llm_map tests ---

func TestLlmMapBasicTransformation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	items := []map[string]string{
		{"text": "hello"},
		{"text": "world"},
		{"text": "test"},
	}
	require.NoError(t, writeJSONL(inputPath, items))

	callCount := atomic.Int64{}
	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		callCount.Add(1)
		return `{"result": "MOCK_OUTPUT"}`, nil
	}

	tool := NewLlmMapTool(WithLLMCall(mockLLM))
	resp, err := runLlmMap(tool, LlmMapParams{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Prompt:     "Transform: {{.Input}}",
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "Processed 3 items")
	require.Contains(t, resp.Content, "3 succeeded")
	require.Contains(t, resp.Content, "0 failed")

	require.Equal(t, int64(3), callCount.Load())

	results := readJSONLResults(t, outputPath)
	require.Len(t, results, 3)
	for _, r := range results {
		require.Nil(t, r.Error)
		require.Equal(t, `{"result":"MOCK_OUTPUT"}`, string(r.Output))
	}
}

func TestLlmMapWithSchemaValidation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	items := []map[string]string{{"name": "alice"}}
	require.NoError(t, writeJSONL(inputPath, items))

	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		return `{"name": "alice", "score": 42}`, nil
	}

	schema := `{
		"type": "object",
		"required": ["name", "score"],
		"properties": {
			"name": {"type": "string"},
			"score": {"type": "number"}
		}
	}`

	tool := NewLlmMapTool(WithLLMCall(mockLLM))
	resp, err := runLlmMap(tool, LlmMapParams{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Prompt:     "Analyze: {{.Input}}",
		Schema:     schema,
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "1 succeeded")

	results := readJSONLResults(t, outputPath)
	require.Len(t, results, 1)
	require.Nil(t, results[0].Error)

	var output map[string]any
	require.NoError(t, json.Unmarshal(results[0].Output, &output))
	require.Equal(t, "alice", output["name"])
}

func TestLlmMapSchemaValidationFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	items := []map[string]string{{"name": "alice"}}
	require.NoError(t, writeJSONL(inputPath, items))

	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		return `{"name": "alice"}`, nil
	}

	schema := `{
		"type": "object",
		"required": ["name", "score"],
		"properties": {
			"name": {"type": "string"},
			"score": {"type": "number"}
		}
	}`

	tool := NewLlmMapTool(WithLLMCall(mockLLM))
	resp, err := runLlmMap(tool, LlmMapParams{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Prompt:     "Analyze: {{.Input}}",
		Schema:     schema,
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "1 failed")

	results := readJSONLResults(t, outputPath)
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Error)
	require.Contains(t, *results[0].Error, "schema validation failed")
}

func TestLlmMapMarkdownFenceStripping(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	items := []map[string]string{{"text": "fence"}}
	require.NoError(t, writeJSONL(inputPath, items))

	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		return "```json\n{\"cleaned\": true}\n```", nil
	}

	tool := NewLlmMapTool(WithLLMCall(mockLLM))
	resp, err := runLlmMap(tool, LlmMapParams{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Prompt:     "Process: {{.Input}}",
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "1 succeeded")

	results := readJSONLResults(t, outputPath)
	require.Len(t, results, 1)
	require.Nil(t, results[0].Error)

	var output map[string]bool
	require.NoError(t, json.Unmarshal(results[0].Output, &output))
	require.True(t, output["cleaned"])
}

func TestLlmMapMissingInputPath(t *testing.T) {
	t.Parallel()

	tool := NewLlmMapTool(WithLLMCall(func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}))
	resp, err := runLlmMap(tool, LlmMapParams{
		OutputPath: "/tmp/out.jsonl",
		Prompt:     "test",
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "input_path is required")
}

func TestLlmMapMissingPrompt(t *testing.T) {
	t.Parallel()

	tool := NewLlmMapTool(WithLLMCall(func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}))
	resp, err := runLlmMap(tool, LlmMapParams{
		InputPath:  "/tmp/in.jsonl",
		OutputPath: "/tmp/out.jsonl",
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "prompt is required")
}

func TestLlmMapNoLLMCallConfigured(t *testing.T) {
	t.Parallel()

	tool := NewLlmMapTool()
	resp, err := runLlmMap(tool, LlmMapParams{
		InputPath:  "/tmp/in.jsonl",
		OutputPath: "/tmp/out.jsonl",
		Prompt:     "test",
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "no LLM caller configured")
}

func TestLlmMapEmptyInput(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "empty.jsonl")
	require.NoError(t, os.WriteFile(inputPath, []byte(""), 0o644))

	tool := NewLlmMapTool(WithLLMCall(func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}))
	resp, err := runLlmMap(tool, LlmMapParams{
		InputPath:  inputPath,
		OutputPath: filepath.Join(tmpDir, "out.jsonl"),
		Prompt:     "test",
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "Input file is empty")
}

func TestLlmMapConcurrency(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	var items []map[string]int
	for i := range 10 {
		items = append(items, map[string]int{"id": i})
	}
	require.NoError(t, writeJSONL(inputPath, items))

	var currentConcurrent, maxConcurrent atomic.Int64
	mockLLM := func(ctx context.Context, prompt string) (string, error) {
		cur := currentConcurrent.Add(1)
		defer currentConcurrent.Add(-1)
		for {
			old := maxConcurrent.Load()
			if cur <= old {
				break
			}
			if maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}
		// Small delay to allow goroutines to overlap.
		time.Sleep(10 * time.Millisecond)
		return `{"done":true}`, nil
	}

	tool := NewLlmMapTool(WithLLMCall(mockLLM))
	resp, err := runLlmMap(tool, LlmMapParams{
		InputPath:   inputPath,
		OutputPath:  outputPath,
		Prompt:      "Process: {{.Input}}",
		Concurrency: 5,
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "10 succeeded")

	require.LessOrEqual(t, maxConcurrent.Load(), int64(5))
	require.Greater(t, maxConcurrent.Load(), int64(1))
}

// --- agentic_map tests ---

func TestAgenticMapBasicRun(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	items := []map[string]string{
		{"task": "review file A"},
		{"task": "review file B"},
	}
	require.NoError(t, writeJSONL(inputPath, items))

	callCount := atomic.Int64{}
	mockSubAgent := func(ctx context.Context, task string, readOnly bool) (string, error) {
		callCount.Add(1)
		return `{"status": "completed"}`, nil
	}

	tool := NewAgenticMapTool(WithSubAgentRun(mockSubAgent))
	resp, err := runAgenticMap(tool, AgenticMapParams{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Prompt:     "Review this code",
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "Processed 2 items")
	require.Contains(t, resp.Content, "2 succeeded")
	require.Contains(t, resp.Content, "0 failed")

	require.Equal(t, int64(2), callCount.Load())

	results := readJSONLResults(t, outputPath)
	require.Len(t, results, 2)
	for _, r := range results {
		require.Nil(t, r.Error)
		require.Equal(t, `{"status":"completed"}`, string(r.Output))
	}
}

func TestAgenticMapWithSchema(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	items := []map[string]string{{"task": "analyze"}}
	require.NoError(t, writeJSONL(inputPath, items))

	mockSubAgent := func(ctx context.Context, task string, readOnly bool) (string, error) {
		return `{"score": 95, "issues": []}`, nil
	}

	schema := `{
		"type": "object",
		"required": ["score"],
		"properties": {
			"score": {"type": "number"},
			"issues": {"type": "array"}
		}
	}`

	tool := NewAgenticMapTool(WithSubAgentRun(mockSubAgent))
	resp, err := runAgenticMap(tool, AgenticMapParams{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Prompt:     "Analyze code",
		Schema:     schema,
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "1 succeeded")

	results := readJSONLResults(t, outputPath)
	require.Len(t, results, 1)
	require.Nil(t, results[0].Error)
}

func TestAgenticMapSubAgentError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	items := []map[string]string{{"task": "fail-me"}}
	require.NoError(t, writeJSONL(inputPath, items))

	mockSubAgent := func(ctx context.Context, task string, readOnly bool) (string, error) {
		return "", context.Canceled
	}

	tool := NewAgenticMapTool(WithSubAgentRun(mockSubAgent))
	resp, err := runAgenticMap(tool, AgenticMapParams{
		InputPath:   inputPath,
		OutputPath:  outputPath,
		Prompt:      "Do something",
		MaxAttempts: 2,
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "1 failed")

	results := readJSONLResults(t, outputPath)
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Error)
	require.Contains(t, *results[0].Error, "sub-agent failed")
}

func TestAgenticMapReadOnlyFlag(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	items := []map[string]string{{"task": "read-only test"}}
	require.NoError(t, writeJSONL(inputPath, items))

	var receivedReadOnly bool
	mockSubAgent := func(ctx context.Context, task string, readOnly bool) (string, error) {
		receivedReadOnly = readOnly
		return `{"ok": true}`, nil
	}

	tool := NewAgenticMapTool(WithSubAgentRun(mockSubAgent))
	resp, err := runAgenticMap(tool, AgenticMapParams{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Prompt:     "Read-only analysis",
		ReadOnly:   true,
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "1 succeeded")
	require.True(t, receivedReadOnly, "readOnly flag should be true")
}

func TestAgenticMapMissingInputPath(t *testing.T) {
	t.Parallel()

	tool := NewAgenticMapTool(WithSubAgentRun(func(ctx context.Context, task string, readOnly bool) (string, error) {
		return "", nil
	}))
	resp, err := runAgenticMap(tool, AgenticMapParams{
		OutputPath: "/tmp/out.jsonl",
		Prompt:     "test",
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "input_path is required")
}

func TestAgenticMapNoSubAgentConfigured(t *testing.T) {
	t.Parallel()

	tool := NewAgenticMapTool()
	resp, err := runAgenticMap(tool, AgenticMapParams{
		InputPath:  "/tmp/in.jsonl",
		OutputPath: "/tmp/out.jsonl",
		Prompt:     "test",
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "no sub-agent runner configured")
}

func TestAgenticMapTaskBuildsPrompt(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	items := []map[string]string{{"task": "verify this"}}
	require.NoError(t, writeJSONL(inputPath, items))

	var receivedTask string
	mockSubAgent := func(ctx context.Context, task string, readOnly bool) (string, error) {
		receivedTask = task
		return `{"verified": true}`, nil
	}

	tool := NewAgenticMapTool(WithSubAgentRun(mockSubAgent))
	_, err := runAgenticMap(tool, AgenticMapParams{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Prompt:     "Verify the code",
	})
	require.NoError(t, err)

	require.Contains(t, receivedTask, "Verify the code")
	require.Contains(t, receivedTask, "verify this")
}

func TestAgenticMapMixedResults(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jsonl")
	outputPath := filepath.Join(tmpDir, "output.jsonl")

	items := []map[string]string{
		{"task": "succeed"},
		{"task": "fail"},
		{"task": "succeed-again"},
	}
	require.NoError(t, writeJSONL(inputPath, items))

	var failCount atomic.Int64
	mockSubAgent := func(ctx context.Context, task string, readOnly bool) (string, error) {
		if strings.Contains(task, `"fail"`) {
			failCount.Add(1)
			return "", context.Canceled
		}
		return `{"result":"ok"}`, nil
	}

	tool := NewAgenticMapTool(WithSubAgentRun(mockSubAgent))
	resp, err := runAgenticMap(tool, AgenticMapParams{
		InputPath:   inputPath,
		OutputPath:  outputPath,
		Prompt:      "Process",
		MaxAttempts: 1,
	})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "2 succeeded")
	require.Contains(t, resp.Content, "1 failed")

	results := readJSONLResults(t, outputPath)
	require.Len(t, results, 3)

	var successCount, errorCount int
	for _, r := range results {
		if r.Error != nil {
			errorCount++
		} else {
			successCount++
		}
	}
	require.Equal(t, 2, successCount)
	require.Equal(t, 1, errorCount)
}

// --- stripMarkdownFences unit tests ---

func TestStripMarkdownFences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no fences", `{"a": 1}`, `{"a": 1}`},
		{"json fence", "```json\n{\"a\": 1}\n```", `{"a": 1}`},
		{"bare fence", "```\n{\"a\": 1}\n```", `{"a": 1}`},
		{"with whitespace", "  ```json\n{\"a\": 1}\n```  ", `{"a": 1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := stripMarkdownFences(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

// --- buildAgenticTask unit test ---

func TestBuildAgenticTask(t *testing.T) {
	t.Parallel()

	task := buildAgenticTask("Review code", json.RawMessage(`{"file": "main.go"}`))
	require.Contains(t, task, "Review code")
	require.Contains(t, task, `"file": "main.go"`)
	require.Contains(t, task, "Input item:")
}

// --- test helpers ---

func runLlmMap(tool fantasy.AgentTool, params LlmMapParams) (fantasy.ToolResponse, error) {
	input, _ := json.Marshal(params)
	return tool.Run(context.Background(), fantasy.ToolCall{Input: string(input)})
}

func runAgenticMap(tool fantasy.AgentTool, params AgenticMapParams) (fantasy.ToolResponse, error) {
	input, _ := json.Marshal(params)
	return tool.Run(context.Background(), fantasy.ToolCall{Input: string(input)})
}

func writeJSONL(path string, items any) error {
	var buf bytes.Buffer
	val := reflect.ValueOf(items)
	for i := range val.Len() {
		data, err := json.Marshal(val.Index(i).Interface())
		if err != nil {
			return err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func readJSONLResults(t *testing.T, path string) []jsonlResult {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var results []jsonlResult
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r jsonlResult
		require.NoError(t, json.Unmarshal([]byte(line), &r), "failed to parse: %s", line)
		results = append(results, r)
	}
	return results
}
