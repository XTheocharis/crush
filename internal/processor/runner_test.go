package processor

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// NewRunner
// ---------------------------------------------------------------------------

func TestNewRunner_DefaultOptions(t *testing.T) {
	t.Parallel()
	r := NewRunner()
	require.NotNil(t, r)
	require.Empty(t, r.InputProcessors)
	require.Empty(t, r.OutputProcessors)
	require.Empty(t, r.ErrorProcessors)
	require.Nil(t, r.TripWire)
}

func TestNewRunner_WithInputProcessors(t *testing.T) {
	t.Parallel()
	p1 := &MockProcessor{IDField: "in-1"}
	p2 := &MockProcessor{IDField: "in-2"}
	r := NewRunner(WithInputProcessors(p1, p2))
	require.Len(t, r.InputProcessors, 2)
	require.Equal(t, "in-1", r.InputProcessors[0].ID())
}

func TestNewRunner_WithOutputProcessors(t *testing.T) {
	t.Parallel()
	p := &MockProcessor{IDField: "out-1"}
	r := NewRunner(WithOutputProcessors(p))
	require.Len(t, r.OutputProcessors, 1)
}

func TestNewRunner_WithErrorProcessors(t *testing.T) {
	t.Parallel()
	p := &MockProcessor{IDField: "err-1"}
	r := NewRunner(WithErrorProcessors(p))
	require.Len(t, r.ErrorProcessors, 1)
}

func TestNewRunner_WithTripWire(t *testing.T) {
	t.Parallel()
	tw := &TripWire{Name: "test", Threshold: 5}
	r := NewRunner(WithTripWire(tw))
	require.NotNil(t, r.TripWire)
	require.Equal(t, 5, r.TripWire.Threshold)
}

func TestNewRunner_CombinedOptions(t *testing.T) {
	t.Parallel()
	r := NewRunner(
		WithInputProcessors(&MockProcessor{IDField: "in"}),
		WithOutputProcessors(&MockProcessor{IDField: "out"}),
		WithErrorProcessors(&MockProcessor{IDField: "err"}),
		WithTripWire(&TripWire{Name: "tw", Threshold: 3}),
	)
	require.Len(t, r.InputProcessors, 1)
	require.Len(t, r.OutputProcessors, 1)
	require.Len(t, r.ErrorProcessors, 1)
	require.NotNil(t, r.TripWire)
}

// ---------------------------------------------------------------------------
// Execute — single-phase tests
// ---------------------------------------------------------------------------

func TestExecute_InputPhase(t *testing.T) {
	t.Parallel()
	called := false
	p := &MockProcessor{
		IDField: "input-proc",
		InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			called = true
			require.Equal(t, InputPhase, pctx.Phase)
			return ProcessorResult{Action: ActionContinue, State: map[string]any{"k": "v"}}, nil
		},
	}
	r := NewRunner(WithInputProcessors(p))
	pctx := NewTestContext()
	pctx.Phase = InputPhase

	result, err := r.Execute(context.Background(), InputPhase, pctx)
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "v", result.State["k"])
}

func TestExecute_OutputStreamPhase(t *testing.T) {
	t.Parallel()
	called := false
	p := &MockProcessor{
		IDField: "stream-proc",
		OutputStreamFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			called = true
			return ProcessorResult{Action: ActionContinue}, nil
		},
	}
	r := NewRunner(WithOutputProcessors(p))
	pctx := NewTestContext()
	pctx.Phase = OutputStreamPhase

	_, err := r.Execute(context.Background(), OutputStreamPhase, pctx)
	require.NoError(t, err)
	require.True(t, called)
}

func TestExecute_OutputResultPhase(t *testing.T) {
	t.Parallel()
	called := false
	p := &MockProcessor{
		IDField: "result-proc",
		OutputResultFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			called = true
			return ProcessorResult{Action: ActionContinue}, nil
		},
	}
	r := NewRunner(WithOutputProcessors(p))
	pctx := NewTestContext()
	pctx.Phase = OutputResultPhase

	_, err := r.Execute(context.Background(), OutputResultPhase, pctx)
	require.NoError(t, err)
	require.True(t, called)
}

func TestExecute_APIErrorPhase(t *testing.T) {
	t.Parallel()
	called := false
	p := &MockProcessor{
		IDField: "error-proc",
		APIErrorFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			called = true
			return ProcessorResult{Action: ActionContinue}, nil
		},
	}
	r := NewRunner(WithErrorProcessors(p))
	pctx := NewTestContext()
	pctx.Phase = APIErrorPhase

	_, err := r.Execute(context.Background(), APIErrorPhase, pctx)
	require.NoError(t, err)
	require.True(t, called)
}

func TestExecute_EmptyProcessors(t *testing.T) {
	t.Parallel()
	r := NewRunner()
	pctx := NewTestContext()
	result, err := r.Execute(context.Background(), InputPhase, pctx)
	require.NoError(t, err)
	require.Equal(t, pctx.Input, result.Input)
}

// ---------------------------------------------------------------------------
// Execute — state accumulation
// ---------------------------------------------------------------------------

func TestExecute_StateAccumulatesAcrossProcessors(t *testing.T) {
	t.Parallel()
	p1 := &MockProcessor{
		IDField: "p1",
		InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action: ActionContinue,
				State:  map[string]any{"step1": true},
			}, nil
		},
	}
	p2 := &MockProcessor{
		IDField: "p2",
		InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			// p2 sees state from p1.
			require.True(t, pctx.State["step1"].(bool))
			return ProcessorResult{
				Action: ActionContinue,
				State:  map[string]any{"step2": true},
			}, nil
		},
	}
	p3 := &MockProcessor{
		IDField: "p3",
		InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			// p3 sees state from p1 and p2.
			require.True(t, pctx.State["step1"].(bool))
			require.True(t, pctx.State["step2"].(bool))
			return ProcessorResult{
				Action: ActionContinue,
				State:  map[string]any{"step3": true},
			}, nil
		},
	}

	r := NewRunner(WithInputProcessors(p1, p2, p3))
	pctx := NewTestContext()
	result, err := r.Execute(context.Background(), InputPhase, pctx)
	require.NoError(t, err)
	require.True(t, result.State["step1"].(bool))
	require.True(t, result.State["step2"].(bool))
	require.True(t, result.State["step3"].(bool))
}

// ---------------------------------------------------------------------------
// Execute — message accumulation
// ---------------------------------------------------------------------------

func TestExecute_MessagesPropagate(t *testing.T) {
	t.Parallel()
	p1 := &MockProcessor{
		IDField: "msg-1",
		InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action:   ActionContinue,
				Messages: []Message{UserMessage("from-p1")},
			}, nil
		},
	}
	p2 := &MockProcessor{
		IDField: "msg-2",
		InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			require.Len(t, pctx.Messages, 1)
			require.Equal(t, "from-p1", pctx.Messages[0].Content)
			return ProcessorResult{
				Action: ActionContinue,
				Messages: []Message{
					UserMessage("from-p1"),
					AssistantMessage("from-p2"),
				},
			}, nil
		},
	}

	r := NewRunner(WithInputProcessors(p1, p2))
	pctx := NewTestContext()
	result, err := r.Execute(context.Background(), InputPhase, pctx)
	require.NoError(t, err)
	require.Len(t, result.Messages, 2)
	require.Equal(t, "from-p1", result.Messages[0].Content)
	require.Equal(t, "from-p2", result.Messages[1].Content)
}

// ---------------------------------------------------------------------------
// Execute — abort handling
// ---------------------------------------------------------------------------

func TestExecute_ProcessorReturnsAbort(t *testing.T) {
	t.Parallel()
	p1 := &MockProcessor{
		IDField: "abort-proc",
		InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{Action: ActionAbort}, nil
		},
	}
	p2 := &MockProcessor{
		IDField: "should-not-run",
		InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			t.Fatal("p2 should not run after abort")
			return ProcessorResult{}, nil
		},
	}

	r := NewRunner(WithInputProcessors(p1, p2))
	pctx := NewTestContext()
	_, err := r.Execute(context.Background(), InputPhase, pctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "abort")
	require.Contains(t, err.Error(), "abort-proc")
}

func TestExecute_ProcessorReturnsError(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("processor exploded")
	p := &MockProcessor{
		IDField: "error-proc",
		InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{}, expectedErr
		},
	}

	r := NewRunner(WithInputProcessors(p))
	pctx := NewTestContext()
	_, err := r.Execute(context.Background(), InputPhase, pctx)
	require.ErrorIs(t, err, expectedErr)
}

// ---------------------------------------------------------------------------
// Execute — rewrite action
// ---------------------------------------------------------------------------

func TestExecute_ProcessorRewriteContinues(t *testing.T) {
	t.Parallel()
	p1 := &MockProcessor{
		IDField: "rewrite-proc",
		InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action:   ActionRewrite,
				Messages: []Message{UserMessage("rewritten")},
				State:    map[string]any{"rewritten": true},
			}, nil
		},
	}
	p2 := &MockProcessor{
		IDField: "next-proc",
		InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			require.Len(t, pctx.Messages, 1)
			require.Equal(t, "rewritten", pctx.Messages[0].Content)
			return ProcessorResult{Action: ActionContinue}, nil
		},
	}

	r := NewRunner(WithInputProcessors(p1, p2))
	pctx := NewTestContext()
	result, err := r.Execute(context.Background(), InputPhase, pctx)
	require.NoError(t, err)
	require.True(t, result.State["rewritten"].(bool))
}

// ---------------------------------------------------------------------------
// Execute — TripWire
// ---------------------------------------------------------------------------

func TestExecute_TripWireAborts(t *testing.T) {
	t.Parallel()
	tw := &TripWire{Name: "limit", Threshold: 2}
	callCount := 0

	mkProc := func(id string) *MockProcessor {
		return &MockProcessor{
			IDField: id,
			InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
				callCount++
				return ProcessorResult{Action: ActionContinue}, nil
			},
		}
	}

	// 4 processors, but tripwire threshold is 2 → only first 2 run, 3rd triggers
	// abort.
	r := NewRunner(
		WithInputProcessors(mkProc("p1"), mkProc("p2"), mkProc("p3"), mkProc("p4")),
		WithTripWire(tw),
	)
	pctx := NewTestContext()
	_, err := r.Execute(context.Background(), InputPhase, pctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "tripwire")
	require.Contains(t, err.Error(), "limit")
	// p1 and p2 executed normally; p3 triggers the tripwire check before
	// execution.
	require.Equal(t, 2, callCount)
}

func TestExecute_TripWireExactlyAtThreshold(t *testing.T) {
	t.Parallel()
	tw := &TripWire{Name: "exact", Threshold: 2}
	callCount := 0

	mkProc := func(id string) *MockProcessor {
		return &MockProcessor{
			IDField: id,
			InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
				callCount++
				return ProcessorResult{Action: ActionContinue}, nil
			},
		}
	}

	// 2 processors, threshold 2: both should run (abort triggers when counter >
	// threshold, not >=).
	r := NewRunner(
		WithInputProcessors(mkProc("p1"), mkProc("p2")),
		WithTripWire(tw),
	)
	pctx := NewTestContext()
	_, err := r.Execute(context.Background(), InputPhase, pctx)
	require.NoError(t, err)
	require.Equal(t, 2, callCount)
}

func TestExecute_NoTripWire(t *testing.T) {
	t.Parallel()
	callCount := 0
	mkProc := func(id string) *MockProcessor {
		return &MockProcessor{
			IDField: id,
			InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
				callCount++
				return ProcessorResult{Action: ActionContinue}, nil
			},
		}
	}

	r := NewRunner(WithInputProcessors(mkProc("p1"), mkProc("p2"), mkProc("p3")))
	pctx := NewTestContext()
	_, err := r.Execute(context.Background(), InputPhase, pctx)
	require.NoError(t, err)
	require.Equal(t, 3, callCount)
}

// ---------------------------------------------------------------------------
// RunAll — sequential phase execution
// ---------------------------------------------------------------------------

func TestRunAll_FourPhases(t *testing.T) {
	t.Parallel()
	var phases []ProcessorPhase

	inputProc := &MockProcessor{
		IDField: "in",
		InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			phases = append(phases, pctx.Phase)
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"input_done": true},
				Messages: pctx.Messages,
			}, nil
		},
	}

	streamProc := &MockProcessor{
		IDField: "stream",
		OutputStreamFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			phases = append(phases, pctx.Phase)
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"stream_done": true},
				Messages: pctx.Messages,
			}, nil
		},
	}

	resultProc := &MockProcessor{
		IDField: "result",
		OutputResultFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			phases = append(phases, pctx.Phase)
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"result_done": true},
				Messages: pctx.Messages,
			}, nil
		},
	}

	errorProc := &MockProcessor{
		IDField: "err",
		APIErrorFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			phases = append(phases, pctx.Phase)
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"error_done": true},
				Messages: pctx.Messages,
			}, nil
		},
	}

	r := NewRunner(
		WithInputProcessors(inputProc),
		WithOutputProcessors(streamProc, resultProc),
		WithErrorProcessors(errorProc),
	)

	pctx := NewTestContext()
	result, err := r.RunAll(context.Background(), pctx)
	require.NoError(t, err)

	// All 4 phases ran in order.
	require.Equal(t, []ProcessorPhase{InputPhase, OutputStreamPhase, OutputResultPhase, APIErrorPhase}, phases)

	// State from all phases accumulated.
	require.True(t, result.State["input_done"].(bool))
	require.True(t, result.State["stream_done"].(bool))
	require.True(t, result.State["result_done"].(bool))
	require.True(t, result.State["error_done"].(bool))
}

func TestRunAll_StateCarriesBetweenPhases(t *testing.T) {
	t.Parallel()
	inputProc := &MockProcessor{
		IDField: "in",
		InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"from_input": 42},
				Messages: []Message{UserMessage("hello")},
			}, nil
		},
	}

	streamProc := &MockProcessor{
		IDField: "stream",
		OutputStreamFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			// OutputStreamPhase sees state from InputPhase.
			val, ok := pctx.State["from_input"]
			require.True(t, ok)
			require.Equal(t, 42, val)
			require.Len(t, pctx.Messages, 1)
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"from_stream": "yes"},
				Messages: pctx.Messages,
			}, nil
		},
	}

	r := NewRunner(
		WithInputProcessors(inputProc),
		WithOutputProcessors(streamProc),
	)

	pctx := NewTestContext()
	result, err := r.RunAll(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, 42, result.State["from_input"])
	require.Equal(t, "yes", result.State["from_stream"])
}

func TestRunAll_StopsOnPhaseError(t *testing.T) {
	t.Parallel()
	inputProc := &MockProcessor{
		IDField: "in",
		InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{}, fmt.Errorf("input failed")
		},
	}

	streamProc := &MockProcessor{
		IDField: "stream",
		OutputStreamFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			t.Fatal("stream should not run after input failure")
			return ProcessorResult{}, nil
		},
	}

	r := NewRunner(
		WithInputProcessors(inputProc),
		WithOutputProcessors(streamProc),
	)

	pctx := NewTestContext()
	_, err := r.RunAll(context.Background(), pctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "input failed")
}

func TestRunAll_TripWireAcrossPhases(t *testing.T) {
	t.Parallel()
	tw := &TripWire{Name: "global", Threshold: 2}

	mkProc := func(id string, phaseFn *PhaseFunc) *MockProcessor {
		return &MockProcessor{
			IDField: id,
			InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
				return ProcessorResult{Action: ActionContinue}, nil
			},
		}
	}

	// 1 input processor + 2 output processors = 3 total. Tripwire at 2 means
	// third execution is blocked.
	_ = mkProc // suppress unused variable warning — using inline procs below.

	inputProc := &MockProcessor{
		IDField: "in-1",
		InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{Action: ActionContinue}, nil
		},
	}
	out1 := &MockProcessor{
		IDField: "out-1",
		OutputStreamFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{Action: ActionContinue}, nil
		},
	}
	out2 := &MockProcessor{
		IDField: "out-2",
		OutputResultFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{Action: ActionContinue}, nil
		},
	}

	r := NewRunner(
		WithInputProcessors(inputProc),
		WithOutputProcessors(out1, out2),
		WithTripWire(tw),
	)

	pctx := NewTestContext()
	_, err := r.RunAll(context.Background(), pctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "tripwire")
	// Counter should be 3: p1 incremented to 1, p2 to 2, p3 triggers abort at
	// 3 > 2.
	require.Equal(t, 3, tw.Counter)
}

func TestRunAll_EmptyRunner(t *testing.T) {
	t.Parallel()
	r := NewRunner()
	pctx := NewTestContext()
	pctx.State["original"] = true

	result, err := r.RunAll(context.Background(), pctx)
	require.NoError(t, err)
	require.True(t, result.State["original"].(bool))
}

// ---------------------------------------------------------------------------
// Execute — context cancellation
// ---------------------------------------------------------------------------

func TestExecute_ContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	p := &MockProcessor{
		IDField: "cancelled",
		InputFn: func(ctx context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{Action: ActionContinue}, ctx.Err()
		},
	}

	r := NewRunner(WithInputProcessors(p))
	pctx := NewTestContext()
	_, err := r.Execute(ctx, InputPhase, pctx)
	require.ErrorIs(t, err, context.Canceled)
}
