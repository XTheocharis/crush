package processor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockProcessor is a minimal implementation that satisfies the Processor
// interface for testing purposes.
type mockProcessor struct {
	id string
}

func (m *mockProcessor) ID() string { return m.id }

func (m *mockProcessor) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

func (m *mockProcessor) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

func (m *mockProcessor) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

func (m *mockProcessor) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

var _ Processor = (*mockProcessor)(nil)

func TestMockSatisfiesProcessorInterface(t *testing.T) {
	t.Parallel()
	p := &mockProcessor{id: "test"}
	require.Equal(t, "test", p.ID())

	ctx := context.Background()
	pctx := ProcessorContext{Phase: InputPhase, Input: "hello"}

	result, err := p.ProcessInput(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
}

func TestProcessorContextStateAccumulation(t *testing.T) {
	t.Parallel()

	pctx := ProcessorContext{
		Phase:    InputPhase,
		State:    make(map[string]any),
		Messages: []Message{},
	}

	processors := []Processor{
		&stateProcessor{key: "step1", value: "a"},
		&stateProcessor{key: "step2", value: "b"},
		&stateProcessor{key: "step3", value: "c"},
	}

	for _, p := range processors {
		result, err := p.ProcessInput(context.Background(), pctx)
		require.NoError(t, err)
		for k, v := range result.State {
			pctx.State[k] = v
		}
	}

	require.Equal(t, "a", pctx.State["step1"])
	require.Equal(t, "b", pctx.State["step2"])
	require.Equal(t, "c", pctx.State["step3"])
	require.Len(t, pctx.State, 3)
}

func TestTripWireAbortsAtThreshold(t *testing.T) {
	t.Parallel()

	tw := &TripWire{Name: "retry", Threshold: 3, Message: "too many retries"}

	require.False(t, tw.ShouldAbort()) // counter=1
	require.False(t, tw.ShouldAbort()) // counter=2
	require.False(t, tw.ShouldAbort()) // counter=3
	require.True(t, tw.ShouldAbort())  // counter=4 > 3
	require.Equal(t, 4, tw.Counter)
}

func TestProcessorActionValues(t *testing.T) {
	t.Parallel()

	require.Equal(t, ProcessorAction(0), ActionContinue)
	require.Equal(t, ProcessorAction(1), ActionAbort)
	require.Equal(t, ProcessorAction(2), ActionRewrite)
}

func TestProcessorPhaseValues(t *testing.T) {
	t.Parallel()

	require.Equal(t, ProcessorPhase(0), InputPhase)
	require.Equal(t, ProcessorPhase(1), OutputStreamPhase)
	require.Equal(t, ProcessorPhase(2), OutputResultPhase)
	require.Equal(t, ProcessorPhase(3), APIErrorPhase)
}

// stateProcessor is a test helper that adds a key-value pair to result state.
type stateProcessor struct {
	key   string
	value string
}

func (s *stateProcessor) ID() string { return "state-" + s.key }

func (s *stateProcessor) ProcessInput(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Action:   ActionContinue,
		State:    map[string]any{s.key: s.value},
		Messages: []Message{},
	}, nil
}

func (s *stateProcessor) ProcessOutputStream(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: []Message{}}, nil
}

func (s *stateProcessor) ProcessOutputResult(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: []Message{}}, nil
}

func (s *stateProcessor) ProcessAPIError(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: []Message{}}, nil
}
