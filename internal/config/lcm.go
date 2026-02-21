package config

// LCMOptions configures the Lossless Context Management system.
// When non-nil, LCM is active for all sessions.
type LCMOptions struct {
	// CtxCutoffThreshold is the fraction of the context window at which
	// soft compaction is triggered (default: 0.6 = 60%).
	CtxCutoffThreshold float64 `json:"ctx_cutoff_threshold,omitempty"`

	// DisableLargeToolOutput disables automatic storage of large tool outputs
	// in LCM when true (default: false).
	DisableLargeToolOutput bool `json:"disable_large_tool_output,omitempty"`

	// LargeToolOutputTokenThreshold is the token count threshold above which
	// tool output is stored in LCM instead of passed inline (default: 10000).
	LargeToolOutputTokenThreshold int `json:"large_tool_output_token_threshold,omitempty"`
}

// DefaultLCMOptions returns LCMOptions with default values applied.
func DefaultLCMOptions() LCMOptions {
	return LCMOptions{
		CtxCutoffThreshold:            0.6,
		LargeToolOutputTokenThreshold: 10000,
	}
}
