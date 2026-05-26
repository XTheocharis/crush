package processor

import "context"

// Compile-time interface check.
var _ Processor = (*Skills)(nil)

// SkillDef describes a single skill definition available to the processor
// pipeline.
type SkillDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	Tags        []string `json:"tags"`
}

// Skills is a processor that loads skill definitions into State so that
// downstream processors (e.g. SkillSearch) can access them. It is pure
// computation — no LLM calls are required.
type Skills struct {
	Skills []SkillDef
}

// ID returns the processor identifier.
func (s *Skills) ID() string { return "skills" }

// ProcessInput stores the loaded skill definitions in State under the
// "loaded_skills" key and sets "skill_count" to the number of skills. It
// returns ActionContinue with messages unchanged.
func (s *Skills) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	skills := s.skillsList()
	return ProcessorResult{
		Action:   ActionContinue,
		Messages: pctx.Messages,
		State: map[string]any{
			"loaded_skills": skills,
			"skill_count":   len(skills),
		},
	}, nil
}

// ProcessOutputStream passes through with ActionContinue.
func (s *Skills) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// ProcessOutputResult passes through with ActionContinue.
func (s *Skills) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// ProcessAPIError passes through with ActionContinue.
func (s *Skills) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// skillsList returns a slice of maps suitable for State storage. Each map
// contains the skill name, description, content, and tags.
func (s *Skills) skillsList() []map[string]any {
	out := make([]map[string]any, len(s.Skills))
	for i, sk := range s.Skills {
		tags := make([]string, len(sk.Tags))
		copy(tags, sk.Tags)
		out[i] = map[string]any{
			"name":        sk.Name,
			"description": sk.Description,
			"content":     sk.Content,
			"tags":        tags,
		}
	}
	return out
}
