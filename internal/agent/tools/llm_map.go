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
func NewLlmMapTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		LlmMapToolName,
		`Apply an LLM transformation to each item in a JSONL file and write results to another JSONL file.

This tool reads items from an input JSONL file, processes each item through an LLM with a prompt template,
validates the output against an optional JSON Schema, and writes the results to an output JSONL file.

Each output line has the format: {"input": ..., "output": ..., "error": null} for successful transformations,
or {"input": ..., "output": null, "error": "..."} for failures.

The prompt template can use {{.Input}} to reference the input item.`,
		func(ctx context.Context, params LlmMapParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			// Validate required parameters
			if params.InputPath == "" {
				return fantasy.NewTextErrorResponse("input_path is required"), nil
			}
			if params.OutputPath == "" {
				return fantasy.NewTextErrorResponse("output_path is required"), nil
			}
			if params.Prompt == "" {
				return fantasy.NewTextErrorResponse("prompt is required"), nil
			}

			// Set defaults
			if params.Concurrency <= 0 {
				params.Concurrency = 16
			}
			if params.Timeout <= 0 {
				params.Timeout = 120
			}
			if params.Model == "" {
				params.Model = "default"
			}

			// Read input JSONL
			items, err := readJSONL(params.InputPath)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to read input file: %v", err)), nil
			}

			if len(items) == 0 {
				return fantasy.NewTextErrorResponse("Input file is empty"), nil
			}

			// Parse prompt template
			tmpl, err := template.New("prompt").Parse(params.Prompt)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Invalid prompt template: %v", err)), nil
			}

			// Validate JSON Schema format if provided
			// Actual schema compilation and validation will be implemented
			// when the LLM integration is complete
			if params.Schema != "" {
				// Basic check that it's valid JSON
				var schemaObj any
				if err := json.Unmarshal([]byte(params.Schema), &schemaObj); err != nil {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("Invalid JSON Schema (not valid JSON): %v", err)), nil
				}
			}

			// Open output file
			outFile, err := os.Create(params.OutputPath)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to create output file: %v", err)), nil
			}
			defer outFile.Close()

			// Process items with worker pool
			results := make([]jsonlResult, len(items))
			var wg sync.WaitGroup
			semaphore := make(chan struct{}, params.Concurrency)

			for i, item := range items {
				wg.Add(1)
				go func(idx int, inputItem json.RawMessage) {
					defer wg.Done()
					semaphore <- struct{}{}
					defer func() { <-semaphore }()

					// Create timeout context for this item
					itemCtx, cancel := context.WithTimeout(ctx, time.Duration(params.Timeout)*time.Second)
					defer cancel()

					result := processLlmMapItem(itemCtx, inputItem, tmpl, params.Schema, params.Model)
					results[idx] = result
				}(i, item)
			}

			wg.Wait()

			// Write results to output file
			encoder := json.NewEncoder(outFile)
			for _, result := range results {
				if err := encoder.Encode(result); err != nil {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to write output: %v", err)), nil
				}
			}

			// Count successes and failures
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

// processLlmMapItem processes a single item through the LLM with retries.
func processLlmMapItem(ctx context.Context, input json.RawMessage, tmpl *template.Template, schemaStr string, model string) jsonlResult {
	// This is a simplified implementation that returns a placeholder.
	// In a full implementation, this would:
	// 1. Execute the template with the input
	// 2. Call the LLM with the rendered prompt
	// 3. Parse the LLM response as JSON (stripping markdown fences)
	// 4. Validate against schema if provided
	// 5. Retry up to 3 times on validation failures

	// For now, return a placeholder indicating LLM integration is needed
	errMsg := "LLM integration not yet fully implemented - this tool requires LLM client access"
	return jsonlResult{
		Input:  input,
		Output: nil,
		Error:  &errMsg,
	}
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
	// Increase buffer size for large JSON lines
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		// Validate that it's valid JSON
		var test any
		if err := json.Unmarshal(line, &test); err != nil {
			return nil, fmt.Errorf("invalid JSON at line %d: %w", lineNum, err)
		}

		// Make a copy of the bytes since scanner reuses the buffer
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
	// Strip ```json ... ``` or ``` ... ```
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			lines = lines[1:] // Remove first line (```json etc)
			if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
				lines = lines[:len(lines)-1] // Remove last ```
			}
		}
		s = strings.Join(lines, "\n")
	}
	return strings.TrimSpace(s)
}
