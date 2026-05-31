// Package types defines shared type definitions, interfaces, and data
// structures used across the tools ecosystem. It exists to break upward
// dependency cycles: lower-level packages (e.g. LCM) can depend on this
// leaf package instead of importing agent/tools directly.
//
// This package contains ONLY type definitions and interfaces — no business
// logic, no side effects, no concrete tool implementations.
package types

import (
	"context"
	"database/sql"

	"charm.land/fantasy"
)

// LLMCallFunc is the function signature for making LLM calls from the
// llm_map tool. Implementations take a rendered prompt and return the raw
// LLM response text.
type LLMCallFunc func(ctx context.Context, prompt string) (string, error)

// SubAgentRunFunc is the function signature for running a sub-agent on a
// single task item. The readOnly flag indicates whether the sub-agent
// should be restricted to read-only tools.
type SubAgentRunFunc func(ctx context.Context, task string, readOnly bool) (string, error)

// LCMToolFactory provides constructors for LCM-related agent tools.
// The concrete implementation lives in the parent tools package. Consumers
// (e.g., internal/lcm) depend on this interface to avoid importing
// agent/tools directly, which would create a layering violation where a
// "lower" package depends on a "higher" one.
type LCMToolFactory interface {
	// NewLcmGrepTool creates a full-text search tool for conversation
	// history.
	NewLcmGrepTool(sqlDB *sql.DB) fantasy.AgentTool

	// NewLcmDescribeTool creates a tool for describing LCM files and
	// summaries by their identifiers.
	NewLcmDescribeTool(sqlDB *sql.DB) fantasy.AgentTool

	// NewLcmExpandTool creates a tool for expanding compressed summaries
	// back into their original messages.
	NewLcmExpandTool(sqlDB *sql.DB) fantasy.AgentTool

	// NewLlmMapTool creates a tool that applies an LLM transformation to
	// each item in a JSONL file.
	NewLlmMapTool(sqlDB *sql.DB) fantasy.AgentTool

	// NewAgenticMapTool creates a tool that runs a sub-agent on each item
	// in a JSONL file.
	NewAgenticMapTool(sqlDB *sql.DB) fantasy.AgentTool
}
