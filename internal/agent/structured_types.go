package agent

import (
	"context"
	"time"
)

// StructuredRequest is a typed, in-process request for a subagent execution.
type StructuredRequest struct {
	Task     string            `json:"task"`
	Context  map[string]string `json:"context,omitempty"`
	Tools    []string          `json:"tools,omitempty"`
	MaxSteps int               `json:"max_steps,omitempty"`
	Timeout  time.Duration     `json:"timeout,omitempty"`
}

// StructuredResponse is the typed result from a structured subagent run.
type StructuredResponse struct {
	Result     string  `json:"result"`
	Success    bool    `json:"success"`
	StepsTaken int     `json:"steps_taken"`
	Cost       float64 `json:"cost"`
	Error      string  `json:"error,omitempty"`
}

// StructuredSubagentFactory creates StructuredSubagent instances scoped to a
// parent session. Implementations wrap existing subagent creation — they do
// not replace it.
type StructuredSubagentFactory interface {
	NewStructuredSubagent(ctx context.Context, parentSessionID string) (StructuredSubagent, error)
}

// StructuredSubagent runs a single subagent task with typed request/response
// semantics, wrapping the existing subagent execution path.
type StructuredSubagent interface {
	Execute(ctx context.Context, req StructuredRequest) (StructuredResponse, error)
	Capabilities() []string
}
