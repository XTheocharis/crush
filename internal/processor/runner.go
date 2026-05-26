package processor

import (
	"context"
	"fmt"
	"maps"
)

// ProcessorRunner chains processors through sequential phases with state
// accumulation and optional TripWire abort.
type ProcessorRunner struct {
	InputProcessors  []Processor
	OutputProcessors []Processor
	ErrorProcessors  []Processor
	TripWire         *TripWire
}

// RunnerOption configures a ProcessorRunner.
type RunnerOption func(*ProcessorRunner)

// WithInputProcessors registers processors for the InputPhase.
func WithInputProcessors(ps ...Processor) RunnerOption {
	return func(r *ProcessorRunner) { r.InputProcessors = append(r.InputProcessors, ps...) }
}

// WithOutputProcessors registers processors for the OutputStreamPhase and
// OutputResultPhase.
func WithOutputProcessors(ps ...Processor) RunnerOption {
	return func(r *ProcessorRunner) { r.OutputProcessors = append(r.OutputProcessors, ps...) }
}

// WithErrorProcessors registers processors for the APIErrorPhase.
func WithErrorProcessors(ps ...Processor) RunnerOption {
	return func(r *ProcessorRunner) { r.ErrorProcessors = append(r.ErrorProcessors, ps...) }
}

// WithTripWire sets the TripWire that aborts the chain when its threshold is
// exceeded.
func WithTripWire(tw *TripWire) RunnerOption {
	return func(r *ProcessorRunner) { r.TripWire = tw }
}

// NewRunner creates a ProcessorRunner with the given options.
func NewRunner(opts ...RunnerOption) *ProcessorRunner {
	r := &ProcessorRunner{}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// processorsFor returns the processor list for a given phase.
func (r *ProcessorRunner) processorsFor(phase ProcessorPhase) []Processor {
	switch phase {
	case InputPhase:
		return r.InputProcessors
	case OutputStreamPhase, OutputResultPhase:
		return r.OutputProcessors
	case APIErrorPhase:
		return r.ErrorProcessors
	default:
		return nil
	}
}

// invoke calls the appropriate phase method on a processor.
func invoke(p Processor, phase ProcessorPhase, ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	switch phase {
	case InputPhase:
		return p.ProcessInput(ctx, pctx)
	case OutputStreamPhase:
		return p.ProcessOutputStream(ctx, pctx)
	case OutputResultPhase:
		return p.ProcessOutputResult(ctx, pctx)
	case APIErrorPhase:
		return p.ProcessAPIError(ctx, pctx)
	default:
		return ProcessorResult{Action: ActionContinue}, nil
	}
}

// Execute runs all processors registered for the given phase, accumulating
// state and messages. It stops on processor error, ActionAbort, or TripWire
// threshold exceeded.
func (r *ProcessorRunner) Execute(ctx context.Context, phase ProcessorPhase, pctx ProcessorContext) (ProcessorContext, error) {
	pctx.Phase = phase
	for _, p := range r.processorsFor(phase) {
		if r.TripWire != nil && r.TripWire.ShouldAbort() {
			return pctx, fmt.Errorf("tripwire %q: threshold exceeded", r.TripWire.Name)
		}
		result, err := invoke(p, phase, ctx, pctx)
		if err != nil {
			return pctx, fmt.Errorf("processor %q: %w", p.ID(), err)
		}
		if result.State != nil {
			if pctx.State == nil {
				pctx.State = make(map[string]any)
			}
			maps.Copy(pctx.State, result.State)
		}
		if result.Messages != nil {
			pctx.Messages = result.Messages
		}
		if result.Action == ActionAbort {
			return pctx, fmt.Errorf("processor %q: abort", p.ID())
		}
	}
	return pctx, nil
}

// RunAll executes all four phases in sequence: InputPhase, OutputStreamPhase,
// OutputResultPhase, APIErrorPhase. State accumulates across phases. It stops
// on the first error from any phase.
func (r *ProcessorRunner) RunAll(ctx context.Context, pctx ProcessorContext) (ProcessorContext, error) {
	phases := []ProcessorPhase{InputPhase, OutputStreamPhase, OutputResultPhase, APIErrorPhase}
	var err error
	for _, phase := range phases {
		pctx, err = r.Execute(ctx, phase, pctx)
		if err != nil {
			return pctx, err
		}
	}
	return pctx, nil
}
