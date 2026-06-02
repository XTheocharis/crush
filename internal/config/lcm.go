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

	// PostCompactMaxFiles limits how many files are re-injected after
	// compaction. Default: 0 (unlimited).
	PostCompactMaxFiles int `json:"post_compact_max_files,omitempty" jsonschema:"description=Maximum number of files re-injected after compaction,default=0"`

	// PostCompactTokenBudget is the token budget for post-compaction
	// re-injection. Default: 0 (unlimited).
	PostCompactTokenBudget int64 `json:"post_compact_token_budget,omitempty" jsonschema:"description=Token budget for post-compaction re-injection,default=0"`

	// DeduplicationEnabled enables deduplication of conversation entries
	// during compaction. Default: false.
	DeduplicationEnabled bool `json:"deduplication_enabled,omitempty" jsonschema:"description=Enable deduplication of conversation entries during compaction,default=false"`

	// PurgeErrorsEnabled enables purging of error entries during compaction.
	// Default: false.
	PurgeErrorsEnabled bool `json:"purge_errors_enabled,omitempty" jsonschema:"description=Enable purging of error entries during compaction,default=false"`

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

	// TokenBudget is the maximum number of tokens allocated for observation
	// text injected into agent prompts. Default: 2000.
	TokenBudget int64 `json:"token_budget,omitempty" jsonschema:"description=Maximum tokens for observation prompt injection,default=2000"`

	// ObserverMessageTokens is the token budget for observer message output.
	// Default: 0 (use TokenBudget).
	ObserverMessageTokens int `json:"observer_message_tokens,omitempty" jsonschema:"description=Token budget for observer message output,default=0"`

	// ObserverBufferRatio is the buffer ratio for observer activation.
	// Default: 0.
	ObserverBufferRatio float64 `json:"observer_buffer_ratio,omitempty" jsonschema:"description=Buffer ratio for observer activation,default=0"`

	// ObserverModel optionally overrides the model used for observation.
	ObserverModel string `json:"observer_model,omitempty" jsonschema:"description=Optional model override for observer"`

	// ReflectorObservationTokens is the token budget for reflector output.
	// Default: 0.
	ReflectorObservationTokens int `json:"reflector_observation_tokens,omitempty" jsonschema:"description=Token budget for reflector output,default=0"`

	// ReflectorBufferActivation is the buffer activation threshold for the
	// reflector. Default: 0.
	ReflectorBufferActivation float64 `json:"reflector_buffer_activation,omitempty" jsonschema:"description=Buffer activation threshold for reflector,default=0"`

	// ReflectorModel optionally overrides the model used for reflection.
	ReflectorModel string `json:"reflector_model,omitempty" jsonschema:"description=Optional model override for reflector"`
}

// DefaultLCMOptions returns LCMOptions with default values applied.
func DefaultLCMOptions() LCMOptions {
	return LCMOptions{
		CtxCutoffThreshold:            0.6,
		LargeToolOutputTokenThreshold: 10000,
		ExplorerOutputProfile:         "enhancement",
	}
}
