package processor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegrationFullLifecycle(t *testing.T) {
	t.Parallel()

	suite := NewProcessorTestSuite(t)

	// Register mock processors for all 4 phases.
	suite.RegisterProcessor(InputPhase, &MockProcessor{
		IDField: "input-logger",
		InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"input_seen": pctx.Input},
				Messages: pctx.Messages,
			}, nil
		},
	})

	suite.RegisterProcessor(OutputStreamPhase, &MockProcessor{
		IDField: "stream-logger",
		OutputStreamFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"stream_ran": true},
				Messages: pctx.Messages,
			}, nil
		},
	})

	suite.RegisterProcessor(OutputResultPhase, &MockProcessor{
		IDField: "result-logger",
		OutputResultFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"result_ran": true},
				Messages: pctx.Messages,
			}, nil
		},
	})

	suite.RegisterProcessor(APIErrorPhase, &MockProcessor{
		IDField: "error-logger",
		APIErrorFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"error_ran": true},
				Messages: pctx.Messages,
			}, nil
		},
	})

	final, err := suite.RunLifecycle(context.Background(), "integration test input")
	require.NoError(t, err)

	// All phases executed.
	suite.AssertPhaseExecuted(t, InputPhase)
	suite.AssertPhaseExecuted(t, OutputStreamPhase)
	suite.AssertPhaseExecuted(t, OutputResultPhase)
	suite.AssertPhaseExecuted(t, APIErrorPhase)

	// State accumulated from all phases.
	require.Equal(t, "integration test input", final.State["input_seen"])
	require.True(t, final.State["stream_ran"].(bool))
	require.True(t, final.State["result_ran"].(bool))
	require.True(t, final.State["error_ran"].(bool))
}

func TestIntegrationTripWireAbort(t *testing.T) {
	t.Parallel()

	suite := NewProcessorTestSuite(t)
	suite.SetTripWire(&TripWire{Name: "early-stop", Threshold: 1})

	// InputPhase processor runs (counter=1, within threshold).
	suite.RegisterProcessor(InputPhase, &MockProcessor{
		IDField: "input-ok",
		InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"input_ok": true},
				Messages: pctx.Messages,
			}, nil
		},
	})

	// OutputStreamPhase processor triggers tripwire check (counter=2 > 1).
	suite.RegisterProcessor(OutputStreamPhase, &MockProcessor{
		IDField: "stream-abort",
		OutputStreamFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"stream_ran": true},
				Messages: pctx.Messages,
			}, nil
		},
	})

	// These should not run.
	suite.RegisterProcessor(OutputResultPhase, &MockProcessor{
		IDField: "result-skip",
		OutputResultFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			t.Fatal("result processor should not run after tripwire")
			return ProcessorResult{}, nil
		},
	})

	suite.RegisterProcessor(APIErrorPhase, &MockProcessor{
		IDField: "error-skip",
		APIErrorFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			t.Fatal("error processor should not run after tripwire")
			return ProcessorResult{}, nil
		},
	})

	_, err := suite.RunLifecycle(context.Background(), "tripwire test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "tripwire")
	require.Contains(t, err.Error(), "early-stop")

	// InputPhase ran; later phases did not.
	suite.AssertPhaseExecuted(t, InputPhase)
	suite.AssertPhaseNotExecuted(t, OutputResultPhase)
	suite.AssertPhaseNotExecuted(t, APIErrorPhase)
}

func TestIntegrationStateAccumulation(t *testing.T) {
	t.Parallel()

	suite := NewProcessorTestSuite(t)

	// Three processors each add distinct state.
	suite.RegisterProcessor(InputPhase, &MockProcessor{
		IDField: "step-1",
		InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"alpha": 1},
				Messages: []Message{},
			}, nil
		},
	})

	suite.RegisterProcessor(OutputStreamPhase, &MockProcessor{
		IDField: "step-2",
		OutputStreamFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			// Verify alpha from step-1 is visible.
			require.Equal(t, 1, pctx.State["alpha"])
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"beta": 2},
				Messages: pctx.Messages,
			}, nil
		},
	})

	suite.RegisterProcessor(OutputResultPhase, &MockProcessor{
		IDField: "step-3",
		OutputResultFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			// Verify alpha and beta from prior steps.
			require.Equal(t, 1, pctx.State["alpha"])
			require.Equal(t, 2, pctx.State["beta"])
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"gamma": 3},
				Messages: pctx.Messages,
			}, nil
		},
	})

	suite.RegisterProcessor(APIErrorPhase, &MockProcessor{
		IDField: "step-4",
		APIErrorFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
			// Final phase sees all accumulated state.
			require.Equal(t, 1, pctx.State["alpha"])
			require.Equal(t, 2, pctx.State["beta"])
			require.Equal(t, 3, pctx.State["gamma"])
			return ProcessorResult{
				Action:   ActionContinue,
				State:    map[string]any{"delta": 4},
				Messages: pctx.Messages,
			}, nil
		},
	})

	final, err := suite.RunLifecycle(context.Background(), "state test")
	require.NoError(t, err)

	// Final state has all four keys.
	require.Equal(t, 1, final.State["alpha"])
	require.Equal(t, 2, final.State["beta"])
	require.Equal(t, 3, final.State["gamma"])
	require.Equal(t, 4, final.State["delta"])
	require.Len(t, final.State, 4)
}

func TestIntegrationFixtures(t *testing.T) {
	t.Parallel()

	fixtures := TestFixtures()

	for name, input := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			suite := NewProcessorTestSuite(t)

			// Register a processor that tags the input.
			suite.RegisterProcessor(InputPhase, &MockProcessor{
				IDField: "tagger",
				InputFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
					return ProcessorResult{
						Action: ActionContinue,
						State:  map[string]any{"fixture": name, "input_len": len(pctx.Input)},
						Messages: []Message{
							UserMessage(pctx.Input),
						},
					}, nil
				},
			})

			suite.RegisterProcessor(OutputStreamPhase, &MockProcessor{
				IDField: "passthrough",
				OutputStreamFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
					return ProcessorResult{
						Action:   ActionContinue,
						Messages: pctx.Messages,
					}, nil
				},
			})

			suite.RegisterProcessor(OutputResultPhase, &MockProcessor{
				IDField: "passthrough-result",
				OutputResultFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
					return ProcessorResult{
						Action:   ActionContinue,
						Messages: pctx.Messages,
					}, nil
				},
			})

			suite.RegisterProcessor(APIErrorPhase, &MockProcessor{
				IDField: "passthrough-error",
				APIErrorFn: func(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
					return ProcessorResult{
						Action:   ActionContinue,
						Messages: pctx.Messages,
					}, nil
				},
			})

			final, err := suite.RunLifecycle(context.Background(), input)
			require.NoError(t, err)

			// The fixture name is recorded in state.
			require.Equal(t, name, final.State["fixture"])
			require.Equal(t, len(input), final.State["input_len"])

			// All phases ran.
			suite.AssertPhaseExecuted(t, InputPhase)
			suite.AssertPhaseExecuted(t, OutputStreamPhase)
			suite.AssertPhaseExecuted(t, OutputResultPhase)
			suite.AssertPhaseExecuted(t, APIErrorPhase)
		})
	}
}

func TestIntegrationMessageFactoryConversation(t *testing.T) {
	t.Parallel()

	factory := NewMessageFactory()
	msgs := factory.Conversation(
		"refactor auth",
		"bash",
		"grep -r 'session' auth/",
		"auth/session.go:42: func NewSession",
		"Found the session file.",
	)

	require.Len(t, msgs, 5)
	require.Equal(t, "user", msgs[0].Role)
	require.Equal(t, "assistant", msgs[1].Role)
	require.Equal(t, "tool_use", msgs[2].Role)
	require.Equal(t, "tool_result", msgs[3].Role)
	require.Equal(t, "assistant", msgs[4].Role)

	// Tool result references the tool use ID.
	require.Equal(t, msgs[2].Meta["id"], msgs[3].Meta["tool_use_id"])
}

func TestIntegrationProcessorAbortReturnsError(t *testing.T) {
	t.Parallel()

	suite := NewProcessorTestSuite(t)
	wantErr := errors.New("unsafe content detected")

	suite.RegisterProcessor(InputPhase, &MockProcessor{
		IDField: "safety-check",
		InputFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			return ProcessorResult{
				Action: ActionAbort,
				Error:  wantErr,
			}, nil
		},
	})

	suite.RegisterProcessor(OutputStreamPhase, &MockProcessor{
		IDField: "should-not-run",
		OutputStreamFn: func(_ context.Context, _ ProcessorContext) (ProcessorResult, error) {
			t.Fatal("stream processor should not run after abort")
			return ProcessorResult{}, nil
		},
	})

	_, err := suite.RunLifecycle(context.Background(), "dangerous input")
	require.Error(t, err)
	require.Contains(t, err.Error(), "safety-check")
	require.Contains(t, err.Error(), "abort")

	suite.AssertPhaseExecuted(t, InputPhase)
	suite.AssertPhaseNotExecuted(t, OutputStreamPhase)
	suite.AssertPhaseNotExecuted(t, OutputResultPhase)
	suite.AssertPhaseNotExecuted(t, APIErrorPhase)
}
