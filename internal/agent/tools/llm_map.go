package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"text/template"
	"time"

	"charm.land/fantasy"
)

const LlmMapToolName = "llm_map"

const (
	llmMapDefaultConcurrency = 16
	llmMapDefaultTimeout     = 120
	llmMapMaxRetries         = 3
)

// LLMCallFunc is the function signature for making LLM calls from the map
// tool. Implementations take a rendered prompt and return the raw LLM
// response text.
type LLMCallFunc func(ctx context.Context, prompt string) (string, error)

// LlmMapOption configures an LLM map tool.
type LlmMapOption func(*llmMapConfig)

type llmMapConfig struct {
	llmCall LLMCallFunc
}

// WithLLMCall sets the LLM call function for the map tool.
func WithLLMCall(fn LLMCallFunc) LlmMapOption {
	return func(c *llmMapConfig) {
		c.llmCall = fn
	}
}

type LlmMapParams struct {
	InputPath   string `json:"input_path" description:"Path to input JSONL file"`
	OutputPath  string `json:"output_path" description:"Path to write output JSONL file"`
	Prompt      string `json:"prompt" description:"Prompt template. Use {{.Input}} for the input item."`
	Schema      string `json:"schema,omitempty" description:"JSON Schema string for output validation (optional)"`
	Model       string `json:"model,omitempty" description:"Model to use: 'small', 'default', or explicit model name (default: 'default')"`
	Concurrency int    `json:"concurrency,omitempty" description:"Number of parallel workers (default 16)"`
	Timeout     int    `json:"timeout,omitempty" description:"Timeout per item in seconds (default 120)"`
}

type jsonlResult struct {
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  *string         `json:"error,omitempty"`
}

// NewLlmMapTool creates a tool that applies an LLM transformation to each item
// in a JSONL file and writes results to another JSONL file.
func NewLlmMapTool(opts ...LlmMapOption) fantasy.AgentTool {
	var cfg llmMapConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	return fantasy.NewAgentTool(
		LlmMapToolName,
		`Apply an LLM transformation to each item in a JSONL file and write results to another JSONL file.

This tool reads items from an input JSONL file, processes each item through an LLM with a prompt template,
validates the output against an optional JSON Schema, and writes the results to an output JSONL file.

Each output line has the format: {"input": ..., "output": ..., "error": null} for successful transformations,
or {"input": ..., "output": null, "error": "..."} for failures.

The prompt template can use {{.Input}} to reference the input item.`,
		func(ctx context.Context, params LlmMapParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.InputPath == "" {
				return fantasy.NewTextErrorResponse("input_path is required"), nil
			}
			if params.OutputPath == "" {
				return fantasy.NewTextErrorResponse("output_path is required"), nil
			}
			if params.Prompt == "" {
				return fantasy.NewTextErrorResponse("prompt is required"), nil
			}

			// Apply defaults.
			if params.Concurrency <= 0 {
				params.Concurrency = llmMapDefaultConcurrency
			}
			if params.Timeout <= 0 {
				params.Timeout = llmMapDefaultTimeout
			}
			if params.Model == "" {
				params.Model = "default"
			}

			if cfg.llmCall == nil {
				return fantasy.NewTextErrorResponse("llm_map tool: no LLM caller configured"), nil
			}

			// Read input JSONL.
			items, err := readJSONL(params.InputPath)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to read input file: %v", err)), nil
			}

			if len(items) == 0 {
				return fantasy.NewTextErrorResponse("Input file is empty"), nil
			}

			// Parse prompt template.
			tmpl, err := template.New("prompt").Parse(params.Prompt)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Invalid prompt template: %v", err)), nil
			}

			// Validate JSON Schema format if provided.
			if params.Schema != "" {
				var schemaObj any
				if err := json.Unmarshal([]byte(params.Schema), &schemaObj); err != nil {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("Invalid JSON Schema (not valid JSON): %v", err)), nil
				}
			}

			// Open output file.
			outFile, err := os.Create(params.OutputPath)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to create output file: %v", err)), nil
			}
			defer outFile.Close()

			// Process items with worker pool — order-preserving fan-in via
			// indexed slice.
			results := make([]jsonlResult, len(items))
			var wg sync.WaitGroup
			semaphore := make(chan struct{}, params.Concurrency)

			for i, item := range items {
				wg.Add(1)
				go func(idx int, inputItem json.RawMessage) {
					defer wg.Done()
					semaphore <- struct{}{}
					defer func() { <-semaphore }()

					itemCtx, cancel := context.WithTimeout(ctx, time.Duration(params.Timeout)*time.Second)
					defer cancel()

					results[idx] = processLlmMapItem(itemCtx, inputItem, tmpl, params.Schema, cfg.llmCall)
				}(i, item)
			}

			wg.Wait()

			// Write results to output file — preserves input order.
			encoder := json.NewEncoder(outFile)
			for _, result := range results {
				if err := encoder.Encode(result); err != nil {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to write output: %v", err)), nil
				}
			}

			successCount := 0
			errorCount := 0
			for _, result := range results {
				if result.Error == nil {
					successCount++
				} else {
					errorCount++
				}
			}

			summary := fmt.Sprintf("Processed %d items: %d succeeded, %d failed.\nOutput written to: %s",
				len(items), successCount, errorCount, params.OutputPath)
			return fantasy.NewTextResponse(summary), nil
		})
}

// processLlmMapItem processes a single item through the LLM with retries on
// schema validation failure.
func processLlmMapItem(ctx context.Context, input json.RawMessage, tmpl *template.Template, schemaStr string, llmCall LLMCallFunc) jsonlResult {
	// Render the prompt template with the input item.
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, struct{ Input string }{Input: string(input)}); err != nil {
		errMsg := fmt.Sprintf("template execution failed: %v", err)
		return jsonlResult{Input: input, Error: &errMsg}
	}

	prompt := rendered.String()
	maxRetries := llmMapMaxRetries
	if schemaStr == "" {
		maxRetries = 1 // No schema → no validation retries needed.
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			errMsg := ctx.Err().Error()
			return jsonlResult{Input: input, Error: &errMsg}
		default:
		}

		response, err := llmCall(ctx, prompt)
		if err != nil {
			errMsg := fmt.Sprintf("LLM call failed (attempt %d/%d): %v", attempt+1, maxRetries, err)
			// Only retry on validation failures, not on LLM call errors.
			return jsonlResult{Input: input, Error: &errMsg}
		}

		// Strip markdown code fences.
		cleaned := stripMarkdownFences(response)

		// Validate against schema if provided.
		if schemaStr != "" {
			if valid, validationErr := validateJSONSchema(cleaned, schemaStr); !valid {
				if attempt < maxRetries-1 {
					continue // Retry on validation failure.
				}
				errMsg := fmt.Sprintf("schema validation failed after %d attempts: %s", maxRetries, validationErr)
				return jsonlResult{Input: input, Error: &errMsg}
			}
		}

		return jsonlResult{Input: input, Output: json.RawMessage(cleaned)}
	}

	errMsg := "all retry attempts exhausted"
	return jsonlResult{Input: input, Error: &errMsg}
}

// readJSONL reads a JSONL file and returns each line as json.RawMessage.
func readJSONL(path string) ([]json.RawMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening input file: %w", err)
	}
	defer f.Close()

	var items []json.RawMessage
	scanner := bufio.NewScanner(f)
	// Increase buffer size for large JSON lines.
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		// Validate that it's valid JSON.
		var test any
		if err := json.Unmarshal(line, &test); err != nil {
			return nil, fmt.Errorf("invalid JSON at line %d: %w", lineNum, err)
		}

		// Make a copy of the bytes since scanner reuses the buffer.
		item := make([]byte, len(line))
		copy(item, line)
		items = append(items, json.RawMessage(item))
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return items, nil
}

// stripMarkdownFences removes markdown code fences from a string.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	// Strip ```json ... ``` or ``` ... ```.
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			lines = lines[1:] // Remove first line (```json etc).
			if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
				lines = lines[:len(lines)-1] // Remove last ```.
			}
		}
		s = strings.Join(lines, "\n")
	}
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// JSON Schema validation (basic, sufficient for LLM output checking).
// ---------------------------------------------------------------------------

// validateJSONSchema checks whether jsonString conforms to the given JSON
// Schema. It supports: type, required, properties, items, enum, and
// minLength/maxLength.
func validateJSONSchema(jsonString, schemaStr string) (bool, string) {
	var data any
	if err := json.Unmarshal([]byte(jsonString), &data); err != nil {
		return false, fmt.Sprintf("output is not valid JSON: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(schemaStr), &schema); err != nil {
		return false, fmt.Sprintf("invalid schema: %v", err)
	}

	return validateValue(data, schema)
}

// validateValue recursively validates data against a schema.
func validateValue(data any, schema map[string]any) (bool, string) {
	// Type check.
	if typeVal, ok := schema["type"].(string); ok {
		if !checkType(data, typeVal) {
			return false, fmt.Sprintf("expected type %q, got %T", typeVal, data)
		}
	}

	// Required properties (object).
	if required, ok := schema["required"].([]any); ok {
		obj, ok := data.(map[string]any)
		if !ok {
			return false, "required check: data is not an object"
		}
		for _, r := range required {
			key, ok := r.(string)
			if !ok {
				continue
			}
			if _, exists := obj[key]; !exists {
				return false, fmt.Sprintf("missing required property %q", key)
			}
		}
	}

	// Properties (object).
	if props, ok := schema["properties"].(map[string]any); ok {
		obj, ok := data.(map[string]any)
		if !ok {
			return false, "properties check: data is not an object"
		}
		for key, propSchema := range props {
			propMap, ok := propSchema.(map[string]any)
			if !ok {
				continue
			}
			val, exists := obj[key]
			if !exists {
				continue // Absent optional property.
			}
			if valid, msg := validateValue(val, propMap); !valid {
				return false, fmt.Sprintf("property %q: %s", key, msg)
			}
		}
	}

	// Items (array).
	if itemSchema, ok := schema["items"].(map[string]any); ok {
		arr, ok := data.([]any)
		if !ok {
			return false, "items check: data is not an array"
		}
		for i, item := range arr {
			if valid, msg := validateValue(item, itemSchema); !valid {
				return false, fmt.Sprintf("item[%d]: %s", i, msg)
			}
		}
	}

	// Enum.
	if enumVals, ok := schema["enum"].([]any); ok {
		found := false
		for _, ev := range enumVals {
			if jsonEqual(data, ev) {
				found = true
				break
			}
		}
		if !found {
			return false, fmt.Sprintf("value not in enum %v", enumVals)
		}
	}

	// String length constraints.
	if str, ok := data.(string); ok {
		if minLen, ok := schema["minLength"].(float64); ok && len(str) < int(minLen) {
			return false, fmt.Sprintf("string length %d < minLength %d", len(str), int(minLen))
		}
		if maxLen, ok := schema["maxLength"].(float64); ok && len(str) > int(maxLen) {
			return false, fmt.Sprintf("string length %d > maxLength %d", len(str), int(maxLen))
		}
	}

	return true, ""
}

// checkType verifies that data matches the expected JSON Schema type.
func checkType(data any, expected string) bool {
	switch expected {
	case "string":
		_, ok := data.(string)
		return ok
	case "number":
		switch data.(type) {
		case float64, float32, int, int64, int32:
			return true
		default:
			return false
		}
	case "integer":
		switch data := data.(type) {
		case float64:
			return data == float64(int(data))
		case int, int64, int32:
			return true
		default:
			return false
		}
	case "boolean":
		_, ok := data.(bool)
		return ok
	case "object":
		_, ok := data.(map[string]any)
		return ok
	case "array":
		_, ok := data.([]any)
		return ok
	case "null":
		return data == nil
	default:
		return true
	}
}

// jsonEqual does a simple equality check between two values decoded from JSON.
func jsonEqual(a, b any) bool {
	aj, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bj, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aj) == string(bj)
}
