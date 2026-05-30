package agent

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestProductiveLoopNoLoopDetected(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	toolNames := []string{"read", "edit", "bash", "grep", "glob"}
	steps := make([]fantasy.StepResult, 10)
	for i := range 10 {
		steps[i] = makeToolStep(toolNames[i%len(toolNames)], fmt.Sprintf(`{"arg":"%d"}`, i), fmt.Sprintf("content-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationNone, result.Level)
	require.False(t, result.IsProductive)
}

func TestProductiveLoopDoomLoopSameOutput(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 5 {
		steps[i] = makeToolStep("edit", `{"file":"auth.go"}`, "error: permission denied")
	}
	for i := 5; i < 10; i++ {
		steps[i] = makeToolStep("bash", fmt.Sprintf(`{"command":"cmd%d"}`, i), "ok")
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationMedium, result.Level, "should escalate when outputs are identical")
	require.False(t, result.IsProductive)
	require.Equal(t, "edit", result.ToolName)
	require.Equal(t, 5, result.RepeatCount)
}

func TestProductiveLoopProductiveDifferentOutputs(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 5 {
		steps[i] = makeToolStep("edit", fmt.Sprintf(`{"file":"file%d.go"}`, i), fmt.Sprintf("fixed-bug-%d", i))
	}
	for i := 5; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), fmt.Sprintf("content-%d", i))
	}

	result := d.Detect(steps)
	require.True(t, result.IsProductive, "should detect productivity when outputs differ")
	require.Equal(t, EscalationMedium, result.Level, "productive loop with 5 repeats should escalate to medium")
	require.Equal(t, "edit", result.ToolName)
	require.True(t, result.UniqueOutputCnt > 1, "should have multiple unique outputs")
}

func TestProductiveLoopProductiveSameToolVaryingResults(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 7 {
		steps[i] = makeToolStep("bash", `{"command":"ls"}`, fmt.Sprintf("file-%d.txt file-%d.txt", i, i+1))
	}
	for i := 7; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "data")
	}

	result := d.Detect(steps)
	require.True(t, result.IsProductive)
	require.Equal(t, EscalationMedium, result.Level, "productive loop with high repeat count should downgrade hard to medium")
	require.Equal(t, "bash", result.ToolName)
	require.Equal(t, 7, result.UniqueOutputCnt)
}

func TestProductiveLoopMixedCallsSameToolDifferentOutput(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 4 {
		steps[i] = makeToolStep("bash", `{"command":"go test"}`, fmt.Sprintf("PASS %d/10 tests", i+1))
	}
	for i := 4; i < 10; i++ {
		steps[i] = makeToolStep("bash", fmt.Sprintf(`{"command":"build %d"}`, i), "ok")
	}

	result := d.Detect(steps)
	require.True(t, result.IsProductive)
	require.Equal(t, EscalationMedium, result.Level, "productive loop with high repeat count should downgrade hard to medium")
}

func TestProductiveLoopFewerThanWindowSize(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 5)
	for i := range 5 {
		steps[i] = makeToolStep("edit", `{"file":"a.go"}`, fmt.Sprintf("result-%d", i))
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationNone, result.Level)
	require.False(t, result.IsProductive)
}

func TestProductiveLoopExactOutputRatio50Percent(t *testing.T) {
	t.Parallel()

	thresholds := DoomLoopThresholds{Soft: 3, Medium: 5, Hard: 7}
	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(thresholds, 6),
	)

	steps := make([]fantasy.StepResult, 6)
	steps[0] = makeToolStep("bash", `{"command":"run"}`, "output-a")
	steps[1] = makeToolStep("bash", `{"command":"run"}`, "output-b")
	steps[2] = makeToolStep("bash", `{"command":"run"}`, "output-a")
	steps[3] = makeToolStep("bash", `{"command":"run"}`, "output-c")
	steps[4] = makeToolStep("read", `{"file":"x.go"}`, "data")
	steps[5] = makeToolStep("read", `{"file":"y.go"}`, "data")

	result := d.Detect(steps)
	require.True(t, result.IsProductive, "50% unique ratio should be productive")
	require.Equal(t, EscalationSoft, result.Level, "productive loop with 4 repeats should escalate to soft")
	require.Equal(t, "bash", result.ToolName)
}

func TestProductiveLoopBelowOutputRatioNotProductive(t *testing.T) {
	t.Parallel()

	thresholds := DoomLoopThresholds{Soft: 3, Medium: 5, Hard: 7}
	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(thresholds, 6),
	)

	steps := make([]fantasy.StepResult, 6)
	steps[0] = makeToolStep("bash", `{"command":"run"}`, "same-output")
	steps[1] = makeToolStep("bash", `{"command":"run"}`, "same-output")
	steps[2] = makeToolStep("bash", `{"command":"run"}`, "same-output")
	steps[3] = makeToolStep("bash", `{"command":"run"}`, "same-output")
	steps[4] = makeToolStep("read", `{"file":"x.go"}`, "data")
	steps[5] = makeToolStep("read", `{"file":"y.go"}`, "data")

	result := d.Detect(steps)
	require.False(t, result.IsProductive, "1 unique output out of 4 calls should not be productive")
}

func TestProductiveLoopMultipleToolsInWindow(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 5 {
		steps[i] = makeToolStep("edit", fmt.Sprintf(`{"file":"file%d.go"}`, i), fmt.Sprintf("result-%d", i))
	}
	for i := 5; i < 10; i++ {
		steps[i] = makeToolStep("bash", fmt.Sprintf(`{"command":"cmd%d"}`, i), "done")
	}

	result := d.Detect(steps)
	require.True(t, result.IsProductive)
}

func TestProductiveLoopResultPreservesDoomInfoWhenNotProductive(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range 3 {
		steps[i] = makeToolStep("bash", `{"command":"ls"}`, "file1.go file2.go")
	}
	for i := 3; i < 10; i++ {
		steps[i] = makeToolStep("read", fmt.Sprintf(`{"file":"%d.go"}`, i), "data")
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationSoft, result.Level)
	require.False(t, result.IsProductive)
	require.Equal(t, "bash", result.ToolName)
	require.Contains(t, result.Message, "SOFT LOOP")
}

func TestProductiveLoopEmptySteps(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	result := d.Detect(nil)
	require.Equal(t, EscalationNone, result.Level)
	require.False(t, result.IsProductive)
}

func TestProductiveLoopAllEmptySteps(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 10),
	)

	steps := make([]fantasy.StepResult, 10)
	for i := range steps {
		steps[i] = makeEmptyStep()
	}

	result := d.Detect(steps)
	require.Equal(t, EscalationNone, result.Level)
	require.False(t, result.IsProductive)
}

func TestHashOutputDeterministic(t *testing.T) {
	t.Parallel()

	h1 := hashOutput("bash", "output-1")
	h2 := hashOutput("bash", "output-1")
	h3 := hashOutput("bash", "output-2")

	require.Equal(t, h1, h2, "same inputs should produce same hash")
	require.NotEqual(t, h1, h3, "different outputs should produce different hashes")
}

func TestHashOutputDifferentTools(t *testing.T) {
	t.Parallel()

	h1 := hashOutput("bash", "output")
	h2 := hashOutput("edit", "output")

	require.NotEqual(t, h1, h2, "different tools with same output should produce different hashes")
}

func TestProductiveLoopMultipleStepsPerCall(t *testing.T) {
	t.Parallel()

	d := NewProductiveLoopDetector(
		NewDoomLoopDetector(DefaultDoomLoopThresholds, 6),
	)

	steps := make([]fantasy.StepResult, 6)
	for i := range 4 {
		callID1 := fmt.Sprintf("call_edit_%d_1", i)
		callID2 := fmt.Sprintf("call_edit_%d_2", i)
		steps[i] = makeStep(
			[]fantasy.ToolCallContent{
				{ToolCallID: callID1, ToolName: "edit", Input: fmt.Sprintf(`{"file":"a%d.go"}`, i)},
				{ToolCallID: callID2, ToolName: "edit", Input: fmt.Sprintf(`{"file":"b%d.go"}`, i)},
			},
			[]fantasy.ToolResultContent{
				{ToolCallID: callID1, ToolName: "edit", Result: fantasy.ToolResultOutputContentText{Text: fmt.Sprintf("ok-%d-a", i)}},
				{ToolCallID: callID2, ToolName: "edit", Result: fantasy.ToolResultOutputContentText{Text: fmt.Sprintf("ok-%d-b", i)}},
			},
		)
	}
	steps[4] = makeToolStep("read", `{"file":"x.go"}`, "data")
	steps[5] = makeToolStep("read", `{"file":"y.go"}`, "data")

	result := d.Detect(steps)
	require.True(t, result.IsProductive)
	require.Equal(t, EscalationMedium, result.Level, "productive loop should downgrade hard to medium")
	require.Equal(t, "edit", result.ToolName)
}

// ---------------------------------------------------------------------------
// Productive orchestration pattern tests
// ---------------------------------------------------------------------------

type productiveMockFactory struct {
	responses []StructuredResponse
	callCount atomic.Int32
}

func (f *productiveMockFactory) NewStructuredSubagent(_ context.Context, _ string) (StructuredSubagent, error) {
	return &productiveMockSubagent{factory: f}, nil
}

type productiveMockSubagent struct {
	factory *productiveMockFactory
}

func (s *productiveMockSubagent) Execute(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
	idx := int(s.factory.callCount.Add(1)) - 1
	if idx < len(s.factory.responses) {
		return s.factory.responses[idx], nil
	}
	return StructuredResponse{Success: true, Result: "default-result"}, nil
}

func (s *productiveMockSubagent) Capabilities() []string {
	return []string{"view", "edit", "bash"}
}

func newTestProductive(factory StructuredSubagentFactory) *Productive {
	cache := NewSharedCache()
	return NewProductive(ProductiveConfig{
		MaxIterations: 5,
		CacheTTL:      time.Minute,
	}, cache, factory, nil)
}

func TestProductivePatternAccessible(t *testing.T) {
	t.Parallel()

	factory := &productiveMockFactory{
		responses: []StructuredResponse{
			{Success: true, Result: "task completed"},
		},
	}
	p := newTestProductive(factory)

	result := p.Run(context.Background(), "session-1", "refactor the auth module")

	require.True(t, result.Success)
	require.Equal(t, 1, result.Iterations)
	require.Contains(t, result.Result, "task completed")
	require.False(t, result.Stalled)
	require.Empty(t, result.Error)
}

func TestProductivePatternIteratesUntilSuccess(t *testing.T) {
	t.Parallel()

	factory := &productiveMockFactory{
		responses: []StructuredResponse{
			{Success: false, Result: "partial-1"},
			{Success: false, Result: "partial-2"},
			{Success: true, Result: "final-result"},
		},
	}
	p := newTestProductive(factory)

	result := p.Run(context.Background(), "session-1", "incremental task that needs refinement")

	require.True(t, result.Success)
	require.Equal(t, 3, result.Iterations)
	require.Contains(t, result.Result, "partial-1")
	require.Contains(t, result.Result, "final-result")
}

func TestProductivePatternMaxIterations(t *testing.T) {
	t.Parallel()

	factory := &productiveMockFactory{}
	for i := range 10 {
		_ = i
		factory.responses = append(factory.responses, StructuredResponse{Success: false, Result: fmt.Sprintf("step-%d", i)})
	}

	p := newTestProductive(factory)

	result := p.Run(context.Background(), "session-1", "task that never completes")

	require.False(t, result.Success)
	require.Equal(t, 5, result.Iterations)
	require.Contains(t, result.Error, "max iterations")
}

func TestProductivePatternStallDetection(t *testing.T) {
	t.Parallel()

	factory := &productiveMockFactory{
		responses: []StructuredResponse{
			{Success: false, Result: "same-output"},
			{Success: false, Result: "same-output"},
			{Success: false, Result: "same-output"},
		},
	}
	p := newTestProductive(factory)

	result := p.Run(context.Background(), "session-1", "task that stalls")

	require.True(t, result.Stalled)
	require.Contains(t, result.Result, "same-output")
}

func TestProductivePatternNoStallOnDifferentOutput(t *testing.T) {
	t.Parallel()

	factory := &productiveMockFactory{
		responses: []StructuredResponse{
			{Success: false, Result: "progress-1"},
			{Success: false, Result: "progress-2"},
			{Success: true, Result: "progress-3"},
		},
	}
	p := newTestProductive(factory)

	result := p.Run(context.Background(), "session-1", "task making steady progress")

	require.True(t, result.Success)
	require.False(t, result.Stalled)
	require.Equal(t, 3, result.Iterations)
}

func TestProductivePatternEmptyTask(t *testing.T) {
	t.Parallel()

	factory := &productiveMockFactory{}
	p := newTestProductive(factory)

	result := p.Run(context.Background(), "session-1", "")

	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "empty task")
}

func TestProductivePatternNilFactory(t *testing.T) {
	t.Parallel()

	cache := NewSharedCache()
	p := NewProductive(ProductiveConfig{}, cache, nil, nil)

	result := p.Run(context.Background(), "session-1", "some task")

	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "no structured subagent factory")
}

func TestProductivePatternCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	factory := &productiveMockFactory{}
	p := newTestProductive(factory)

	result := p.Run(ctx, "session-1", "task with cancelled context")

	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "context cancelled")
}

func TestProductivePatternCaching(t *testing.T) {
	t.Parallel()

	factory := &productiveMockFactory{
		responses: []StructuredResponse{
			{Success: true, Result: "cached-result"},
		},
	}
	p := newTestProductive(factory)

	result1 := p.Run(context.Background(), "session-1", "cacheable task")
	require.True(t, result1.Success)
	callsAfterFirst := factory.callCount.Load()

	result2 := p.Run(context.Background(), "session-1", "cacheable task")
	require.True(t, result2.Success)

	callsAfterSecond := factory.callCount.Load()
	require.Equal(t, callsAfterFirst, callsAfterSecond, "cache should prevent re-execution")
}

func TestProductivePatternConfigDefaults(t *testing.T) {
	t.Parallel()

	cfg := ProductiveConfig{}
	require.Equal(t, defaultMaxIterations, cfg.maxIterations())
	require.Equal(t, defaultStallThreshold, cfg.stallThreshold())
	require.Equal(t, DefaultRepoMapTTL, cfg.cacheTTL())
}

func TestProductivePatternConfigCustomValues(t *testing.T) {
	t.Parallel()

	cfg := ProductiveConfig{
		MaxIterations:  20,
		StallThreshold: 5,
		CacheTTL:       10 * time.Minute,
	}
	require.Equal(t, 20, cfg.maxIterations())
	require.Equal(t, 5, cfg.stallThreshold())
	require.Equal(t, 10*time.Minute, cfg.cacheTTL())
}

func TestProductivePatternSubagentError(t *testing.T) {
	t.Parallel()

	factory := &productiveErrorFactory{}
	p := newTestProductive(factory)

	result := p.Run(context.Background(), "session-1", "task that triggers subagent error")

	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "execute iteration")
}

type productiveErrorFactory struct{}

func (f *productiveErrorFactory) NewStructuredSubagent(_ context.Context, _ string) (StructuredSubagent, error) {
	return &productiveErrorSubagent{}, nil
}

type productiveErrorSubagent struct{}

func (s *productiveErrorSubagent) Execute(_ context.Context, _ StructuredRequest) (StructuredResponse, error) {
	return StructuredResponse{}, fmt.Errorf("subagent exploded")
}

func (s *productiveErrorSubagent) Capabilities() []string { return nil }

func TestProductivePatternCreateSubagentError(t *testing.T) {
	t.Parallel()

	factory := &productiveCreateErrorFactory{}
	p := newTestProductive(factory)

	result := p.Run(context.Background(), "session-1", "task that triggers create error")

	require.NotEmpty(t, result.Error)
	require.Contains(t, result.Error, "create subagent")
}

type productiveCreateErrorFactory struct{}

func (f *productiveCreateErrorFactory) NewStructuredSubagent(_ context.Context, _ string) (StructuredSubagent, error) {
	return nil, fmt.Errorf("factory unavailable")
}

func TestProductivePatternResultAccumulation(t *testing.T) {
	t.Parallel()

	factory := &productiveMockFactory{
		responses: []StructuredResponse{
			{Success: false, Result: "step-1-done"},
			{Success: false, Result: "step-2-done"},
			{Success: true, Result: "step-3-final"},
		},
	}
	p := newTestProductive(factory)

	result := p.Run(context.Background(), "session-1", "multi-step task")

	require.True(t, result.Success)
	require.Contains(t, result.Result, "step-1-done")
	require.Contains(t, result.Result, "step-2-done")
	require.Contains(t, result.Result, "step-3-final")
}

func TestProductivePatternSubagentReceivesAccumulatedContext(t *testing.T) {
	t.Parallel()

	var captured []StructuredRequest
	factory := &productiveCapturingFactory{requests: &captured}
	p := newTestProductive(factory)

	result := p.Run(context.Background(), "session-1", "task that needs context passing")

	require.True(t, result.Success)
	require.NotEmpty(t, captured)

	if len(captured) >= 2 {
		require.Contains(t, captured[1].Task, "Previous progress",
			"second iteration should receive accumulated context")
	}
}

type productiveCapturingFactory struct {
	requests  *[]StructuredRequest
	callCount atomic.Int32
}

func (f *productiveCapturingFactory) NewStructuredSubagent(_ context.Context, _ string) (StructuredSubagent, error) {
	return &productiveCapturingSubagent{factory: f}, nil
}

type productiveCapturingSubagent struct {
	factory *productiveCapturingFactory
}

func (s *productiveCapturingSubagent) Execute(_ context.Context, req StructuredRequest) (StructuredResponse, error) {
	idx := s.factory.callCount.Add(1)
	*s.factory.requests = append(*s.factory.requests, req)
	if idx <= 2 {
		return StructuredResponse{Success: false, Result: fmt.Sprintf("progress-%d", idx)}, nil
	}
	return StructuredResponse{Success: true, Result: "done"}, nil
}

func (s *productiveCapturingSubagent) Capabilities() []string { return nil }

func TestFingerprintOutput(t *testing.T) {
	t.Parallel()

	fp1 := fingerprintOutput("same")
	fp2 := fingerprintOutput("same")
	fp3 := fingerprintOutput("different")

	require.Equal(t, fp1, fp2, "same input should produce same fingerprint")
	require.NotEqual(t, fp1, fp3, "different inputs should produce different fingerprints")

	fpEmpty := fingerprintOutput("")
	require.NotEmpty(t, fpEmpty, "empty string should still produce a fingerprint")
}
