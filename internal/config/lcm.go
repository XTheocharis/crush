package config

// LCMOptions configures the Lossless Context Management system.
// When non-nil, LCM is active for all sessions.
type LCMOptions struct {
	// CtxCutoffThreshold is the fraction of the context window at which
	// soft compaction is triggered (default: 0.6 = 60%).
	CtxCutoffThreshold float64 `json:"ctx_cutoff_threshold,omitempty"`

	// SummarizerModel optionally overrides the model used for LCM summary and
	// condensation calls. When nil, Crush uses the configured large model. If a
	// custom summarizer model has a smaller context window than the large model,
	// Crush ignores it and keeps using the large model.
	SummarizerModel *SelectedModel `json:"summarizer_model,omitempty" jsonschema:"description=Optional dedicated model configuration for LCM summarization; defaults to the configured large model, and is ignored when its context window is smaller than the large model"`

	// DisableLargeToolOutput disables automatic storage of large tool outputs
	// in LCM when true (default: false).
	DisableLargeToolOutput bool `json:"disable_large_tool_output,omitempty"`

	// LargeToolOutputTokenThreshold is the token count threshold above which
	// tool output is stored in LCM instead of passed inline (default: 10000).
	LargeToolOutputTokenThreshold int `json:"large_tool_output_token_threshold,omitempty"`

	// ExplorerOutputProfile controls runtime formatter profile for large-output
	// exploration summaries. Accepted values: "enhancement" (default) or
	// "parity".
	ExplorerOutputProfile string `json:"explorer_output_profile,omitempty"`

	// OperationalMemoryEnabled wires the session operational memory store into
	// the LCM manager so that PostCompactionHook and PostTurnHook persist
	// extracted observations. Defaults to false (off, zero overhead).
	OperationalMemoryEnabled bool `json:"operational_memory_enabled,omitempty" jsonschema:"description=Enable operational memory persistence from LCM lifecycle hooks,default=false"`

	// Observation configures the observation strategy used by the LCM observer.
	// When nil, the default strategy (always observe, JSON output) is used.
	Observation *ObservationOptions `json:"observation,omitempty" jsonschema:"description=Observation strategy configuration for the LCM observer"`

	// Nudge configures the context-limit nudge injection system. When nil,
	// default nudge values are used.
	Nudge *NudgeOptions `json:"nudge,omitempty" jsonschema:"description=Nudge injection configuration for context-limit warnings"`
}

// NudgeOptions configures the nudge injection system.
type NudgeOptions struct {
	// MinContextLimit is the minimum token count below which nudges are never
	// injected. Default: 50000.
	MinContextLimit int64 `json:"min_context_limit,omitempty"`

	// MaxContextLimit is the token count above which context-limit nudges are
	// injected when pressure is high. Default: 100000.
	MaxContextLimit int64 `json:"max_context_limit,omitempty"`

	// NudgeFrequency controls how often nudges are injected (every N turns).
	// Default: 5.
	NudgeFrequency int `json:"nudge_frequency,omitempty"`

	// NudgeForce controls nudge intensity: "soft" or "hard". Default: "soft".
	NudgeForce string `json:"nudge_force,omitempty"`
}

// ObservationOptions configures the observation strategy for the LCM observer.
type ObservationOptions struct {
	// Strategy selects the observation strategy. Accepted values:
	//   - "default" (always observe, JSON output)
	//   - "resource-scoped" (skip observations under high memory pressure)
	// The default is "default".
	Strategy string `json:"strategy,omitempty" jsonschema:"description=Observation strategy name,enum=default,enum=resource-scoped,default=default"`
}

// DefaultLCMOptions returns LCMOptions with default values applied.
func DefaultLCMOptions() LCMOptions {
	return LCMOptions{
		CtxCutoffThreshold:            0.6,
		LargeToolOutputTokenThreshold: 10000,
		ExplorerOutputProfile:         "enhancement",
	}
}
