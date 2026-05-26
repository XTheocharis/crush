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

const (
	agenticMapDefaultConcurrency = 16
	agenticMapDefaultTimeout     = 900
	agenticMapDefaultMaxAttempts = 3
)

// SubAgentRunFunc is the function signature for running a sub-agent on a
// single task item. The readOnly flag indicates whether the sub-agent should
// be restricted to read-only tools.
type SubAgentRunFunc func(ctx context.Context, task string, readOnly bool) (string, error)

// AgenticMapOption configures an agentic map tool.
type AgenticMapOption func(*agenticMapConfig)

type agenticMapConfig struct {
	subAgentRun SubAgentRunFunc
}

// WithSubAgentRun sets the sub-agent runner function for the agentic map tool.
func WithSubAgentRun(fn SubAgentRunFunc) AgenticMapOption {
	return func(c *agenticMapConfig) {
		c.subAgentRun = fn
	}
}

type AgenticMapParams struct {
	InputPath   string `json:"input_path" description:"Path to input JSONL file"`
	OutputPath  string `json:"output_path" description:"Path to write output JSONL file"`
	Prompt      string `json:"prompt" description:"Task description for each sub-agent"`
	Schema      string `json:"schema,omitempty" description:"JSON Schema for output validation (optional)"`
	ReadOnly    bool   `json:"read_only,omitempty" description:"If true, sub-agent can't use write/edit/bash tools (default false)"`
	MaxAttempts int    `json:"max_attempts,omitempty" description:"Max attempts per item (retry on schema validation failure, default 3)"`
}

// NewAgenticMapTool creates a tool that runs a sub-agent on each item in a
// JSONL file and writes results to another JSONL file.
func NewAgenticMapTool(opts ...AgenticMapOption) fantasy.AgentTool {
	var cfg agenticMapConfig
	for _, opt := range opts {
		opt(&cfg)
	}

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
			if params.InputPath == "" {
				return fantasy.NewTextErrorResponse("input_path is required"), nil
			}
			if params.OutputPath == "" {
				return fantasy.NewTextErrorResponse("output_path is required"), nil
			}
			if params.Prompt == "" {
				return fantasy.NewTextErrorResponse("prompt is required"), nil
			}

			if params.MaxAttempts <= 0 {
				params.MaxAttempts = agenticMapDefaultMaxAttempts
			}

			if cfg.subAgentRun == nil {
				return fantasy.NewTextErrorResponse("agentic_map tool: no sub-agent runner configured"), nil
			}

			// Read input JSONL.
			items, err := readJSONL(params.InputPath)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to read input file: %v", err)), nil
			}

			if len(items) == 0 {
				return fantasy.NewTextErrorResponse("Input file is empty"), nil
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
			semaphore := make(chan struct{}, agenticMapDefaultConcurrency)

			for i, item := range items {
				wg.Add(1)
				go func(idx int, inputItem json.RawMessage) {
					defer wg.Done()
					semaphore <- struct{}{}
					defer func() { <-semaphore }()

					itemCtx, cancel := context.WithTimeout(ctx, agenticMapDefaultTimeout*time.Second)
					defer cancel()

					results[idx] = processAgenticMapItem(itemCtx, inputItem, params.Prompt, params.Schema, params.ReadOnly, params.MaxAttempts, cfg.subAgentRun)
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

// processAgenticMapItem processes a single item through a sub-agent with
// retries on schema validation failure.
func processAgenticMapItem(ctx context.Context, input json.RawMessage, prompt string, schemaStr string, readOnly bool, maxAttempts int, subAgentRun SubAgentRunFunc) jsonlResult {
	task := buildAgenticTask(prompt, input)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			errMsg := ctx.Err().Error()
			return jsonlResult{Input: input, Error: &errMsg}
		default:
		}

		response, err := subAgentRun(ctx, task, readOnly)
		if err != nil {
			if attempt < maxAttempts-1 {
				continue
			}
			errMsg := fmt.Sprintf("sub-agent failed after %d attempts: %v", maxAttempts, err)
			return jsonlResult{Input: input, Error: &errMsg}
		}

		cleaned := stripMarkdownFences(response)

		if schemaStr != "" {
			if valid, validationErr := validateJSONSchema(cleaned, schemaStr); !valid {
				if attempt < maxAttempts-1 {
					continue
				}
				errMsg := fmt.Sprintf("schema validation failed after %d attempts: %s", maxAttempts, validationErr)
				return jsonlResult{Input: input, Error: &errMsg}
			}
		}

		return jsonlResult{Input: input, Output: json.RawMessage(cleaned)}
	}

	errMsg := "all retry attempts exhausted"
	return jsonlResult{Input: input, Error: &errMsg}
}

// buildAgenticTask constructs the task prompt for a sub-agent by combining
// the user's prompt with the input item as context.
func buildAgenticTask(prompt string, input json.RawMessage) string {
	return fmt.Sprintf("%s\n\nInput item:\n%s", prompt, string(input))
}
