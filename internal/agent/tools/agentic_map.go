package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"charm.land/fantasy"
)

const AgenticMapToolName = "agentic_map"

type AgenticMapParams struct {
	InputPath   string `json:"input_path" description:"Path to input JSONL file"`
	OutputPath  string `json:"output_path" description:"Path to write output JSONL file"`
	Prompt      string `json:"prompt" description:"Task description for each sub-agent"`
	Schema      string `json:"schema,omitempty" description:"JSON Schema for output validation (optional)"`
	ReadOnly    bool   `json:"read_only,omitempty" description:"If true, sub-agent can't use write/edit/bash tools (default false)"`
	MaxAttempts int    `json:"max_attempts,omitempty" description:"Max attempts per item (retry on schema validation failure, default 3)"`
}

// NewAgenticMapTool creates a tool that runs a sub-agent on each item in a JSONL file
// and writes results to another JSONL file.
func NewAgenticMapTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		AgenticMapToolName,
		`Run a sub-agent on each item in a JSONL file and write results to another JSONL file.

This tool reads items from an input JSONL file, runs a sub-agent for each item with the item as context,
validates the output against an optional JSON Schema, and writes the results to an output JSONL file.

Each output line has the format: {"input": ..., "output": ..., "error": null} for successful executions,
or {"input": ..., "output": null, "error": "..."} for failures.

The tool uses a hardcoded concurrency of 16 workers and a timeout of 900 seconds per item.

If read_only is true, the sub-agent will not have access to write, edit, or bash tools.`,
		func(ctx context.Context, params AgenticMapParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
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
			if params.MaxAttempts <= 0 {
				params.MaxAttempts = 3
			}

			// Hardcoded concurrency and timeout as per spec
			const concurrency = 16
			const workerTimeout = 900 // seconds

			// Read input JSONL
			items, err := readJSONL(params.InputPath)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to read input file: %v", err)), nil
			}

			if len(items) == 0 {
				return fantasy.NewTextErrorResponse("Input file is empty"), nil
			}

			// Validate JSON Schema format if provided
			// Actual schema compilation and validation will be implemented
			// when the sub-agent integration is complete
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
			semaphore := make(chan struct{}, concurrency)

			for i, item := range items {
				wg.Add(1)
				go func(idx int, inputItem json.RawMessage) {
					defer wg.Done()
					semaphore <- struct{}{}
					defer func() { <-semaphore }()

					// Create timeout context for this item
					itemCtx, cancel := context.WithTimeout(ctx, time.Duration(workerTimeout)*time.Second)
					defer cancel()

					result := processAgenticMapItem(itemCtx, inputItem, params.Prompt, params.Schema, params.ReadOnly, params.MaxAttempts)
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

// processAgenticMapItem processes a single item through a sub-agent with retries.
func processAgenticMapItem(ctx context.Context, input json.RawMessage, prompt string, schemaStr string, readOnly bool, maxAttempts int) jsonlResult {
	// This is a simplified implementation that returns a placeholder.
	// In a full implementation, this would:
	// 1. Create a new sub-agent instance
	// 2. Configure it with read-only restrictions if needed
	// 3. Run the sub-agent with the prompt and input item as context
	// 4. Validate the output against schema if provided
	// 5. Retry up to maxAttempts times on validation failures
	// 6. Return the result or error

	// For now, return a placeholder indicating sub-agent integration is needed
	errMsg := "Sub-agent integration not yet fully implemented - this tool requires agent coordinator access"
	return jsonlResult{
		Input:  input,
		Output: nil,
		Error:  &errMsg,
	}
}
