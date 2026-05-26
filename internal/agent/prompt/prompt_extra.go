package prompt

// WithExtraContextFiles injects additional context files (e.g. from LCM) into
// the system prompt alongside the standard context-path entries.
func WithExtraContextFiles(files []ContextFile) Option {
	return func(p *Prompt) {
		p.extraContextFiles = files
	}
}
