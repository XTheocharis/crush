// Package processor defines the core interfaces and types for the message
// processing pipeline. Processors intercept LLM input and output at four
// phases: input, output stream, output result, and API error.
package processor

import "context"

// ProcessorPhase identifies which processing phase is active.
type ProcessorPhase int

const (
	InputPhase ProcessorPhase = iota
	OutputStreamPhase
	OutputResultPhase
	APIErrorPhase
)

// ProcessorAction controls what happens after a processor runs.
type ProcessorAction int

const (
	// ActionContinue passes control to the next processor in the chain.
	ActionContinue ProcessorAction = iota
	// ActionAbort stops processing and returns an error.
	ActionAbort
	// ActionRewrite rewrites the message and continues processing.
	ActionRewrite
)

// Message represents a single message in the conversation.
type Message struct {
	Role    string
	Content string
	Meta    map[string]any
}

// ProcessorContext carries all state through the processor chain.
type ProcessorContext struct {
	Phase        ProcessorPhase
	Input        string
	OutputStream string
	OutputResult string
	APIError     string
	Messages     []Message
	State        map[string]any
	Metadata     map[string]any
}

// ProcessorResult is what a processor returns after execution.
type ProcessorResult struct {
	Messages []Message
	State    map[string]any
	Action   ProcessorAction
	Error    error
}

// TripWire aborts the processor chain when a threshold is exceeded.
type TripWire struct {
	Name      string
	Threshold int
	Counter   int
	Message   string
}

// ShouldAbort increments the counter and returns true when it exceeds the
// threshold.
func (tw *TripWire) ShouldAbort() bool {
	tw.Counter++
	return tw.Counter > tw.Threshold
}

// Processor is the interface all processors must implement. Each method
// handles a specific processing phase; implementations that do not need to
// act on a phase should return a result with ActionContinue.
type Processor interface {
	// ID returns a unique identifier for the processor.
	ID() string
	// ProcessInput handles the input phase.
	ProcessInput(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error)
	// ProcessOutputStream handles the output stream phase.
	ProcessOutputStream(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error)
	// ProcessOutputResult handles the output result phase.
	ProcessOutputResult(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error)
	// ProcessAPIError handles the API error phase.
	ProcessAPIError(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error)
}
