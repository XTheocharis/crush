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
}

// DefaultLCMOptions returns LCMOptions with default values applied.
func DefaultLCMOptions() LCMOptions {
	return LCMOptions{
		CtxCutoffThreshold:            0.6,
		LargeToolOutputTokenThreshold: 10000,
		ExplorerOutputProfile:         "enhancement",
	}
}
