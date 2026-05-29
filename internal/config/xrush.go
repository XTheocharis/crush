package config

// RoutingTier defines a single tier in the multi-tier model router. Each tier
// specifies a token threshold and the model type to use for prompts at or
// below that threshold. Tiers are sorted ascending by UpToTokens before use.
type RoutingTier struct {
	// UpToTokens is the token count threshold for this tier. Prompts with
	// token counts at or below this value are routed to ModelType.
	UpToTokens int `json:"up_to_tokens,omitempty" jsonschema:"description=Token count threshold for this tier"`
	// ModelType is the SelectedModelType to use for prompts within this tier.
	ModelType SelectedModelType `json:"model_type,omitempty" jsonschema:"description=Model type for this tier,example=small,example=large"`
}

// ModelRole identifies the purpose a model serves within the
// architect/editor split. The architect role handles planning and
// high-level decisions; the editor role handles focused coding work.
type ModelRole string

const (
	RoleArchitect ModelRole = "architect"
	RoleEditor    ModelRole = "editor"
)

// ArchitectOptions controls the two-phase architect→editor planning flow.
type ArchitectOptions struct {
	ApprovalRequired bool `json:"approval_required,omitempty" jsonschema:"description=Require user approval before executing architect plans,default=false"`
}

// ValidationOptions controls the edit validation pipeline. When Enabled is
// false (the default), the ValidationHandler is inert and no post-edit
// diagnostics or auto-fixes run.
type ValidationOptions struct {
	Enabled            bool `json:"enabled,omitempty" jsonschema:"description=Enable post-edit validation pipeline,default=false"`
	AutoFix            bool `json:"auto_fix,omitempty" jsonschema:"description=Enable automatic fix attempts when validation fails,default=false"`
	AutoFixLoopEnabled bool `json:"autofix_loop_enabled,omitempty" jsonschema:"description=Enable post-turn auto-fix quality cycle,default=false"`
}

// SnapshotConfig configures snapshot retention for the rewind system.
type SnapshotConfig struct {
	MaxPerSession int `json:"max_per_session,omitempty" jsonschema:"description=Maximum snapshots to retain per session (older ones are cleaned up),default=50"`
}

// ProcessorConfig holds per-processor configuration. Keys are processor
// names and values are arbitrary config objects read by each processor.
type ProcessorConfig map[string]any

// ProcessorsOptions controls the processor pipeline. When Enabled is nil
// (not explicitly set), the pipeline is enabled by default. Set Enabled to
// false to disable.
type ProcessorsOptions struct {
	Enabled *bool           `json:"enabled,omitempty" jsonschema:"description=Enable the message processing pipeline,default=true"`
	List    []string        `json:"list,omitempty" jsonschema:"description=List of processor names to enable. Safe defaults: token_limiter, system_prompt_scrubber, pii_detector"`
	Config  ProcessorConfig `json:"config,omitempty" jsonschema:"description=Per-processor configuration keyed by processor name"`
}

// AutoDownloadConfig specifies where to download an LSP server binary from
// and how to verify its integrity.
type AutoDownloadConfig struct {
	URL    string `json:"url,omitempty" jsonschema:"description=Download URL for the LSP server binary"`
	SHA256 string `json:"sha256,omitempty" jsonschema:"description=Expected SHA256 hash of the downloaded binary"`
}
