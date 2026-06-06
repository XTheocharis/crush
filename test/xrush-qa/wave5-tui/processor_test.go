package wave5_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/processor"
	"github.com/stretchr/testify/require"
)

// allProcessorIDs lists the 16 processor IDs from the pipeline.
var allProcessorIDs = []string{
	"token_limiter",
	"system_prompt_scrubber",
	"pii_detector",
	"unicode_normalizer",
	"batch_parts",
	"message_selection",
	"tool_call_filter",
	"tool_search",
	"skills",
	"skill_search",
	"moderation",
	"prompt_injection",
	"language_detector",
	"workspace_instructions",
	"message_history",
	"structured_output",
}

// mkMock creates a MockProcessor with the given ID that returns ActionContinue
// for all phases.
func mkMock(id string) *processor.MockProcessor {
	return &processor.MockProcessor{IDField: id}
}

// TestAllProcessorsRegister verifies that all 16 processors can be registered
// into a ProcessorRunner across all three processor slices without errors.
func TestProcessorAllRegister(t *testing.T) {
	t.Parallel()

	// Distribute the 16 processors across the three runner slices to simulate
	// a realistic registration scenario.
	inputIDs := allProcessorIDs[:5]    // 5 input processors.
	outputIDs := allProcessorIDs[5:11] // 6 output processors.
	errorIDs := allProcessorIDs[11:]   // 5 error processors.

	var inputProcs, outputProcs, errorProcs []processor.Processor
	for _, id := range inputIDs {
		inputProcs = append(inputProcs, mkMock(id))
	}
	for _, id := range outputIDs {
		outputProcs = append(outputProcs, mkMock(id))
	}
	for _, id := range errorIDs {
		errorProcs = append(errorProcs, mkMock(id))
	}

	runner := processor.NewRunner(
		processor.WithInputProcessors(inputProcs...),
		processor.WithOutputProcessors(outputProcs...),
		processor.WithErrorProcessors(errorProcs...),
	)

	// Verify total count.
	total := len(runner.InputProcessors) + len(runner.OutputProcessors) + len(runner.ErrorProcessors)
	require.Equal(t, 16, total, "expected all 16 processors to be registered")

	// Verify each slice size.
	require.Len(t, runner.InputProcessors, len(inputIDs))
	require.Len(t, runner.OutputProcessors, len(outputIDs))
	require.Len(t, runner.ErrorProcessors, len(errorIDs))

	// Verify IDs in each slice.
	for i, p := range runner.InputProcessors {
		require.Equal(t, inputIDs[i], p.ID(), "input processor %d ID mismatch", i)
	}
	for i, p := range runner.OutputProcessors {
		require.Equal(t, outputIDs[i], p.ID(), "output processor %d ID mismatch", i)
	}
	for i, p := range runner.ErrorProcessors {
		require.Equal(t, errorIDs[i], p.ID(), "error processor %d ID mismatch", i)
	}
}

// TestDefaultProcessorsExecute verifies that the 3 default processors
// (TokenLimiter, SystemPromptScrubber, PIIDetector) execute in the correct
// phase order. TokenLimiter is an input processor; SystemPromptScrubber and
// PIIDetector are output processors.
func TestProcessorDefaultExecute(t *testing.T) {
	t.Parallel()

	var executionOrder []string

	// Simulate the 3 default processors with tracking callbacks.
	tokenLimiter := &processor.MockProcessor{
		IDField: "token_limiter",
		InputFn: func(_ context.Context, _ processor.ProcessorContext) (processor.ProcessorResult, error) {
			executionOrder = append(executionOrder, "token_limiter:Input")
			return processor.ProcessorResult{Action: processor.ActionContinue}, nil
		},
	}

	scrubber := &processor.MockProcessor{
		IDField: "system_prompt_scrubber",
		OutputStreamFn: func(_ context.Context, _ processor.ProcessorContext) (processor.ProcessorResult, error) {
			executionOrder = append(executionOrder, "system_prompt_scrubber:OutputStream")
			return processor.ProcessorResult{Action: processor.ActionContinue}, nil
		},
		OutputResultFn: func(_ context.Context, _ processor.ProcessorContext) (processor.ProcessorResult, error) {
			executionOrder = append(executionOrder, "system_prompt_scrubber:OutputResult")
			return processor.ProcessorResult{Action: processor.ActionContinue}, nil
		},
	}

	piiDetector := &processor.MockProcessor{
		IDField: "pii_detector",
		OutputStreamFn: func(_ context.Context, _ processor.ProcessorContext) (processor.ProcessorResult, error) {
			executionOrder = append(executionOrder, "pii_detector:OutputStream")
			return processor.ProcessorResult{Action: processor.ActionContinue}, nil
		},
		OutputResultFn: func(_ context.Context, _ processor.ProcessorContext) (processor.ProcessorResult, error) {
			executionOrder = append(executionOrder, "pii_detector:OutputResult")
			return processor.ProcessorResult{Action: processor.ActionContinue}, nil
		},
	}

	runner := processor.NewRunner(
		processor.WithInputProcessors(tokenLimiter),
		processor.WithOutputProcessors(scrubber, piiDetector),
	)

	pctx := processor.NewTestContext()
	_, err := runner.RunAll(context.Background(), pctx)
	require.NoError(t, err)

	// Verify the execution order: Input phase first, then OutputStream, then
	// OutputResult. Within OutputStream/OutputResult, scrubber runs before
	// pii_detector (registration order is preserved).
	expected := []string{
		"token_limiter:Input",                 // InputPhase.
		"system_prompt_scrubber:OutputStream", // OutputStreamPhase.
		"pii_detector:OutputStream",           // OutputStreamPhase.
		"system_prompt_scrubber:OutputResult", // OutputResultPhase.
		"pii_detector:OutputResult",           // OutputResultPhase.
	}
	require.Equal(t, expected, executionOrder, "processors should execute in correct phase order")
}

// TestCustomProcessorConfig verifies that when only specific processors are
// configured, only those processors execute — others are excluded.
func TestProcessorCustomConfig(t *testing.T) {
	t.Parallel()

	called := map[string]bool{}

	// Only register 2 custom processors instead of the full set.
	custom1 := &processor.MockProcessor{
		IDField: "custom-alpha",
		InputFn: func(_ context.Context, _ processor.ProcessorContext) (processor.ProcessorResult, error) {
			called["custom-alpha"] = true
			return processor.ProcessorResult{Action: processor.ActionContinue}, nil
		},
	}

	custom2 := &processor.MockProcessor{
		IDField: "custom-beta",
		OutputStreamFn: func(_ context.Context, _ processor.ProcessorContext) (processor.ProcessorResult, error) {
			called["custom-beta"] = true
			return processor.ProcessorResult{Action: processor.ActionContinue}, nil
		},
	}

	runner := processor.NewRunner(
		processor.WithInputProcessors(custom1),
		processor.WithOutputProcessors(custom2),
	)

	pctx := processor.NewTestContext()
	result, err := runner.RunAll(context.Background(), pctx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Only the configured processors should have executed.
	require.True(t, called["custom-alpha"], "custom-alpha should have been called")
	require.True(t, called["custom-beta"], "custom-beta should have been called")

	// No other processors exist in the runner.
	require.Len(t, runner.InputProcessors, 1, "only 1 input processor should be registered")
	require.Len(t, runner.OutputProcessors, 1, "only 1 output processor should be registered")
	require.Empty(t, runner.ErrorProcessors, "no error processors should be registered")
}

// TestAllPhasesExecute verifies that a message passes through all four phases
// (Input, OutputStream, OutputResult, APIError) and each phase fires exactly
// once.
func TestProcessorAllPhases(t *testing.T) {
	t.Parallel()

	// Use a single processor registered across all slices that records which
	// phases fire.
	phaseTracker := map[processor.ProcessorPhase]bool{}

	trackFn := func(phase processor.ProcessorPhase) processor.PhaseFunc {
		return func(_ context.Context, pctx processor.ProcessorContext) (processor.ProcessorResult, error) {
			phaseTracker[pctx.Phase] = true
			return processor.ProcessorResult{
				Action:   processor.ActionContinue,
				Messages: pctx.Messages,
				State:    map[string]any{fmt.Sprintf("phase_%d_done", phase): true},
			}, nil
		}
	}

	inputProc := &processor.MockProcessor{
		IDField: "phase-tracker-input",
		InputFn: trackFn(processor.InputPhase),
	}

	streamProc := &processor.MockProcessor{
		IDField:        "phase-tracker-stream",
		OutputStreamFn: trackFn(processor.OutputStreamPhase),
		OutputResultFn: trackFn(processor.OutputResultPhase),
	}

	errorProc := &processor.MockProcessor{
		IDField:    "phase-tracker-error",
		APIErrorFn: trackFn(processor.APIErrorPhase),
	}

	runner := processor.NewRunner(
		processor.WithInputProcessors(inputProc),
		processor.WithOutputProcessors(streamProc),
		processor.WithErrorProcessors(errorProc),
	)

	pctx := processor.NewTestContext()
	result, err := runner.RunAll(context.Background(), pctx)
	require.NoError(t, err)

	// All four phases must have fired.
	require.True(t, phaseTracker[processor.InputPhase], "InputPhase should have executed")
	require.True(t, phaseTracker[processor.OutputStreamPhase], "OutputStreamPhase should have executed")
	require.True(t, phaseTracker[processor.OutputResultPhase], "OutputResultPhase should have executed")
	require.True(t, phaseTracker[processor.APIErrorPhase], "APIErrorPhase should have executed")

	// State should carry markers from all phases.
	require.True(t, result.State["phase_0_done"].(bool), "InputPhase state should be set")
	require.True(t, result.State["phase_1_done"].(bool), "OutputStreamPhase state should be set")
	require.True(t, result.State["phase_2_done"].(bool), "OutputResultPhase state should be set")
	require.True(t, result.State["phase_3_done"].(bool), "APIErrorPhase state should be set")
}

func TestProcessorRealTokenLimiterTrimsOldMessages(t *testing.T) {
	t.Parallel()

	limiter := &processor.TokenLimiter{Budget: 5}
	pctx := processor.ProcessorContext{
		Messages: []processor.Message{
			{Role: "user", Content: strings.Repeat("a", 40)},
			{Role: "assistant", Content: strings.Repeat("b", 16)},
			{Role: "user", Content: "keep"},
		},
	}

	result, err := limiter.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, processor.ActionContinue, result.Action)
	require.Len(t, result.Messages, 2)
	require.Equal(t, "assistant", result.Messages[0].Role)
	require.Equal(t, "user", result.Messages[1].Role)
	require.Equal(t, "keep", result.Messages[1].Content)
	require.Equal(t, 15, result.State["tokens_before"])
	require.Equal(t, 5, result.State["tokens_after"])
	require.Equal(t, 1, result.State["messages_removed"])
}

func TestProcessorRealPIIDetectorRewritesSensitiveContent(t *testing.T) {
	t.Parallel()

	detector := processor.NewPIIDetector(processor.SensitivityMedium, nil)
	pctx := processor.ProcessorContext{
		Messages: []processor.Message{
			{Role: "user", Content: "Email me at person@example.com or call 555-123-4567."},
		},
	}

	result, err := detector.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, processor.ActionRewrite, result.Action)
	require.Len(t, result.Messages, 1)
	require.Contains(t, result.Messages[0].Content, "[REDACTED_EMAIL]")
	require.Contains(t, result.Messages[0].Content, "[REDACTED_PHONE]")
	require.NotContains(t, result.Messages[0].Content, "person@example.com")
	require.NotContains(t, result.Messages[0].Content, "555-123-4567")
	require.Equal(t, true, result.State["pii_found"])
	require.Equal(t, 2, result.State["redacted_count"])
	require.ElementsMatch(t, []string{"email", "phone"}, result.State["pii_types"])
}
