package processor

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Config Wiring Integration Tests
//
// These tests verify that the ProcessorRunner correctly wires and activates
// only the configured processors, and that the phase ordering matches
// expectations: InputPhase → OutputStreamPhase → OutputResultPhase →
// APIErrorPhase.
// ---------------------------------------------------------------------------

// TestConfigWiring_DefaultProcessors verifies that the three default
// processors (TokenLimiter, SystemPromptScrubber, PIIDetector) are
// correctly wired when constructing a runner with the default set.
func TestConfigWiring_DefaultProcessors(t *testing.T) {
	t.Parallel()

	// Build a runner with the three default processors.
	tl := &TokenLimiter{Budget: 10000}
	scrubber := NewSystemPromptScrubber(nil)
	pii := NewPIIDetector(SensitivityMedium, nil)

	r := NewRunner(
		WithInputProcessors(tl, pii),
		WithOutputProcessors(scrubber, pii),
		WithErrorProcessors(),
	)

	// Verify input processors are registered in order.
	require.Len(t, r.InputProcessors, 2)
	require.Equal(t, "token_limiter", r.InputProcessors[0].ID())
	require.Equal(t, "pii_detector", r.InputProcessors[1].ID())

	// Verify output processors are registered.
	require.Len(t, r.OutputProcessors, 2)
	require.Equal(t, "system_prompt_scrubber", r.OutputProcessors[0].ID())
	require.Equal(t, "pii_detector", r.OutputProcessors[1].ID())

	// No error processors in default config.
	require.Empty(t, r.ErrorProcessors)
}

// TestConfigWiring_OnlyConfiguredProcessorsActive verifies that only
// processors explicitly registered in a runner are invoked during
// execution. Unregistered processors must not appear in the pipeline.
func TestConfigWiring_OnlyConfiguredProcessorsActive(t *testing.T) {
	t.Parallel()

	// Register only a single input processor.
	var calledIDs []string
	trackProc := func(id string) *MockProcessor {
		return &MockProcessor{
			IDField: id,
			InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
				calledIDs = append(calledIDs, id)
				return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
			},
		}
	}

	r := NewRunner(
		WithInputProcessors(trackProc("only-me")),
	)

	pctx := NewTestContext()
	_, err := r.RunAll(context.Background(), pctx)
	require.NoError(t, err)

	// Only the single input processor was invoked.
	require.Equal(t, []string{"only-me"}, calledIDs)
}

// TestConfigWiring_PhaseOrdering verifies that RunAll executes phases in
// the correct order: InputPhase → OutputStreamPhase → OutputResultPhase →
// APIErrorPhase.
func TestConfigWiring_PhaseOrdering(t *testing.T) {
	t.Parallel()

	var phaseOrder []ProcessorPhase

	inputProc := &MockProcessor{
		IDField: "input-tracker",
		InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			phaseOrder = append(phaseOrder, pctx.Phase)
			return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
		},
	}

	outputStreamProc := &MockProcessor{
		IDField: "stream-tracker",
		OutputStreamFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			phaseOrder = append(phaseOrder, pctx.Phase)
			return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
		},
	}

	outputResultProc := &MockProcessor{
		IDField: "result-tracker",
		OutputResultFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			phaseOrder = append(phaseOrder, pctx.Phase)
			return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
		},
	}

	errorProc := &MockProcessor{
		IDField: "error-tracker",
		APIErrorFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			phaseOrder = append(phaseOrder, pctx.Phase)
			return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
		},
	}

	r := NewRunner(
		WithInputProcessors(inputProc),
		WithOutputProcessors(outputStreamProc, outputResultProc),
		WithErrorProcessors(errorProc),
	)

	pctx := NewTestContext()
	_, err := r.RunAll(context.Background(), pctx)
	require.NoError(t, err)

	require.Equal(t, []ProcessorPhase{InputPhase, OutputStreamPhase, OutputResultPhase, APIErrorPhase}, phaseOrder,
		"phases must execute in order: Input → OutputStream → OutputResult → APIError")
}

// TestConfigWiring_PhaseMapping verifies that the runner correctly maps
// phases to processor slices: Input → InputProcessors,
// OutputStream/OutputResult → OutputProcessors, APIError → ErrorProcessors.
func TestConfigWiring_PhaseMapping(t *testing.T) {
	t.Parallel()

	var inputCalled, streamCalled, resultCalled, errorCalled bool

	inputProc := &MockProcessor{
		IDField: "in",
		InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			inputCalled = true
			return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
		},
	}

	streamProc := &MockProcessor{
		IDField: "stream",
		OutputStreamFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			streamCalled = true
			return ProcessorResult{Action: ActionContinue}, nil
		},
		OutputResultFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			resultCalled = true
			return ProcessorResult{Action: ActionContinue}, nil
		},
	}

	errorProc := &MockProcessor{
		IDField: "err",
		APIErrorFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			errorCalled = true
			return ProcessorResult{Action: ActionContinue}, nil
		},
	}

	r := NewRunner(
		WithInputProcessors(inputProc),
		WithOutputProcessors(streamProc),
		WithErrorProcessors(errorProc),
	)

	pctx := NewTestContext()
	_, err := r.RunAll(context.Background(), pctx)
	require.NoError(t, err)

	require.True(t, inputCalled, "InputPhase should dispatch to InputProcessors")
	require.True(t, streamCalled, "OutputStreamPhase should dispatch to OutputProcessors")
	require.True(t, resultCalled, "OutputResultPhase should dispatch to OutputProcessors")
	require.True(t, errorCalled, "APIErrorPhase should dispatch to ErrorProcessors")
}

// TestConfigWiring_PIIDetectorIntegration verifies that the PIIDetector
// processor correctly redacts PII within a full pipeline run.
func TestConfigWiring_PIIDetectorIntegration(t *testing.T) {
	t.Parallel()

	pii := NewPIIDetector(SensitivityMedium, nil)
	tl := &TokenLimiter{Budget: 10000}

	r := NewRunner(
		WithInputProcessors(tl, pii),
		WithOutputProcessors(pii),
	)

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			UserMessage("Contact qa-person@example.com or call 555-123-4567"),
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := r.RunAll(context.Background(), pctx)
	require.NoError(t, err)

	// PII should be redacted in the input messages.
	require.Len(t, result.Messages, 1)
	content := result.Messages[0].Content
	require.Contains(t, content, "[REDACTED_EMAIL]", "email should be redacted")
	require.Contains(t, content, "[REDACTED_PHONE]", "phone should be redacted")
	require.NotContains(t, content, "qa-person@example.com", "raw email must not appear")
	require.NotContains(t, content, "555-123-4567", "raw phone must not appear")
}

// TestConfigWiring_TokenLimiterIntegration verifies that the TokenLimiter
// correctly trims messages when the token budget is exceeded within a full
// pipeline run.
func TestConfigWiring_TokenLimiterIntegration(t *testing.T) {
	t.Parallel()

	// "SENTINEL_A_" is 11 chars. 8 repeats = 88 chars = 22 tokens (88/4).
	// Budget of 30 means both messages (44 tokens total) exceed it,
	// so the oldest is dropped, leaving 22 tokens.
	tl := &TokenLimiter{Budget: 30}

	r := NewRunner(
		WithInputProcessors(tl),
	)

	pctx := ProcessorContext{
		Phase: InputPhase,
		Messages: []Message{
			UserMessage(strings.Repeat("SENTINEL_A_", 8)),      // 88 chars = 22 tokens
			AssistantMessage(strings.Repeat("SENTINEL_B_", 8)), // 88 chars = 22 tokens
		},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := r.RunAll(context.Background(), pctx)
	require.NoError(t, err)

	// Oldest message should have been removed.
	require.Len(t, result.Messages, 1)
	require.Equal(t, "assistant", result.Messages[0].Role)
	require.Contains(t, result.Messages[0].Content, "SENTINEL_B_")

	// State should record the trimming.
	require.Equal(t, 44, result.State["tokens_before"])
	require.Equal(t, 22, result.State["tokens_after"])
	require.Equal(t, 1, result.State["messages_removed"])
}

// TestConfigWiring_OutputProcessorsSharedBetweenStreamAndResult verifies
// that OutputProcessors are invoked for both OutputStreamPhase and
// OutputResultPhase.
func TestConfigWiring_OutputProcessorsSharedBetweenStreamAndResult(t *testing.T) {
	t.Parallel()

	var streamCount, resultCount int

	sharedProc := &MockProcessor{
		IDField: "shared-output",
		OutputStreamFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			streamCount++
			return ProcessorResult{Action: ActionContinue}, nil
		},
		OutputResultFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			resultCount++
			return ProcessorResult{Action: ActionContinue}, nil
		},
	}

	r := NewRunner(WithOutputProcessors(sharedProc))

	pctx := NewTestContext()
	_, err := r.RunAll(context.Background(), pctx)
	require.NoError(t, err)

	require.Equal(t, 1, streamCount, "OutputStreamPhase should invoke OutputProcessors once")
	require.Equal(t, 1, resultCount, "OutputResultPhase should invoke OutputProcessors once")
}

// TestConfigWiring_EmptyConfigWiresNothing verifies that a runner with no
// configured processors passes all phases through without modification.
func TestConfigWiring_EmptyConfigWiresNothing(t *testing.T) {
	t.Parallel()

	r := NewRunner()
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Input:    "original",
		Messages: []Message{UserMessage("hello")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := r.RunAll(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, "original", result.Input)
	require.Len(t, result.Messages, 1)
	require.Equal(t, "hello", result.Messages[0].Content)
}
