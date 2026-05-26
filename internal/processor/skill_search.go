package processor

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Compile-time interface check.
var _ Processor = (*SkillSearch)(nil)

// SkillSearch searches across loaded skill definitions using BM25 scoring. It
// reads skills from State["loaded_skills"] (set by the Skills processor) and
// writes results to State["skill_search_results"]. It is pure computation —
// no LLM calls are required.
type SkillSearch struct{}

// ID returns the processor identifier.
func (SkillSearch) ID() string { return "skill_search" }

// ProcessInput checks the input for a skill search query, runs BM25 against
// loaded skills, and stores the top-K results in State.
func (s SkillSearch) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	query := extractSearchQuery(pctx.Input)
	if query == "" {
		return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
	}

	raw, ok := pctx.State["loaded_skills"]
	if !ok || raw == nil {
		return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
	}

	skills, ok := raw.([]map[string]any)
	if !ok || len(skills) == 0 {
		return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
	}

	results := bm25Search(query, skills, 5)

	return ProcessorResult{
		Action:   ActionContinue,
		Messages: pctx.Messages,
		State: map[string]any{
			"skill_search_results": results,
			"query":                query,
		},
	}, nil
}

// ProcessOutputStream passes through with ActionContinue.
func (s SkillSearch) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// ProcessOutputResult passes through with ActionContinue.
func (s SkillSearch) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// ProcessAPIError passes through with ActionContinue.
func (s SkillSearch) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

var searchPattern = regexp.MustCompile(`(?i)(?:search\s+skills?\s+(?:for\s+)?|find\s+skill\s+)(.+)`)

// extractSearchQuery checks if input contains a skill search trigger phrase
// and returns the query portion, or an empty string if no match.
func extractSearchQuery(input string) string {
	m := searchPattern.FindStringSubmatch(input)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// tokenize splits text into lowercase tokens on non-alphanumeric boundaries.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	tokens := nonAlphaNum.Split(text, -1)
	filtered := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if t != "" {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// bm25Search scores skills against the query using BM25 with k1=1.2, b=0.75
// and returns up to topK results sorted by descending score.
func bm25Search(query string, skills []map[string]any, topK int) []map[string]any {
	const k1, b = 1.2, 0.75

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	n := len(skills)

	// Build per-skill document texts: name + description + tags joined.
	docs := make([][]string, n)
	for i, sk := range skills {
		var sb strings.Builder
		sb.WriteString(strVal(sk, "name"))
		sb.WriteString(" ")
		sb.WriteString(strVal(sk, "description"))
		tags := tagsVal(sk)
		if len(tags) > 0 {
			sb.WriteString(" ")
			sb.WriteString(strings.Join(tags, " "))
		}
		docs[i] = tokenize(sb.String())
	}

	// Average document length.
	var totalLen float64
	for _, d := range docs {
		totalLen += float64(len(d))
	}
	avgDL := totalLen / float64(n)

	// Document frequency per token.
	df := make(map[string]int)
	for _, d := range docs {
		seen := make(map[string]bool, len(d))
		for _, t := range d {
			if !seen[t] {
				df[t]++
				seen[t] = true
			}
		}
	}

	type skillScored struct {
		idx   int
		score float64
	}

	scores := make([]skillScored, 0, n)
	for i, d := range docs {
		tf := make(map[string]int)
		for _, t := range d {
			tf[t]++
		}

		dl := float64(len(d))
		var score float64
		for _, qt := range queryTokens {
			f := float64(tf[qt])
			dfi := float64(df[qt])
			idf := math.Log(1 + (float64(n)-dfi+0.5)/(dfi+0.5))
			numerator := f * (k1 + 1)
			denominator := f + k1*(1-b+b*(dl/avgDL))
			score += idf * (numerator / denominator)
		}
		if score > 0 {
			scores = append(scores, skillScored{idx: i, score: score})
		}
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	if topK > len(scores) {
		topK = len(scores)
	}

	results := make([]map[string]any, topK)
	for i, s := range scores[:topK] {
		results[i] = map[string]any{
			"name":        skills[s.idx]["name"],
			"description": skills[s.idx]["description"],
			"score":       s.score,
		}
	}
	return results
}

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func tagsVal(m map[string]any) []string {
	raw, _ := m["tags"].([]string)
	return raw
}
