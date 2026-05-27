package processor

import (
	"context"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Compile-time interface check.
var _ Processor = (*ToolSearch)(nil)

// ToolDef describes a tool available for search.
type ToolDef struct {
	Name        string
	Description string
	Tags        []string
}

// ToolSearch resolves tool search queries using BM25 ranking over a registry
// of available tools. No LLM calls are required.
type ToolSearch struct {
	Tools []ToolDef
}

// ID returns the processor identifier.
func (ts *ToolSearch) ID() string { return "tool_search" }

// ProcessInput detects tool search queries in the input and runs BM25 scoring
// against the tool registry. Results are stored in State; messages are
// unchanged.
func (ts *ToolSearch) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	query := extractToolQuery(pctx.Input)
	if query == "" {
		return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
	}

	results := ts.search(query, 5)
	return ProcessorResult{
		Action:   ActionContinue,
		Messages: pctx.Messages,
		State: map[string]any{
			"tool_search_results": results,
			"query":               query,
		},
	}, nil
}

// ProcessOutputStream passes through unchanged.
func (ts *ToolSearch) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// ProcessOutputResult passes through unchanged.
func (ts *ToolSearch) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// ProcessAPIError passes through unchanged.
func (ts *ToolSearch) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// searchResult is a scored tool match.
type searchResult struct {
	Name        string  `json:"name"`
	Score       float64 `json:"score"`
	Description string  `json:"description"`
}

// search runs BM25 over the tool registry and returns the top-K results.
func (ts *ToolSearch) search(query string, topK int) []searchResult {
	if len(ts.Tools) == 0 {
		return nil
	}

	docs := make([][]string, len(ts.Tools))
	for i, tool := range ts.Tools {
		var b strings.Builder
		b.WriteString(tool.Name)
		b.WriteByte(' ')
		b.WriteString(tool.Description)
		b.WriteByte(' ')
		b.WriteString(strings.Join(tool.Tags, " "))
		docs[i] = tokenize(b.String())
	}

	terms := tokenize(query)
	N := float64(len(docs))
	avgDocLen := avgDocLength(docs)

	df := make(map[string]int)
	for _, term := range terms {
		for _, doc := range docs {
			if containsTerm(doc, term) {
				df[term]++
			}
		}
	}

	const k1 = 1.2
	const b = 0.75

	type scored struct {
		idx   int
		score float64
	}
	scores := make([]scored, len(docs))
	for i, doc := range docs {
		docLen := float64(len(doc))
		score := 0.0
		tfMap := termFreq(doc)
		for _, term := range terms {
			tf := float64(tfMap[term])
			dfVal := float64(df[term])
			idf := math.Log((N-dfVal+0.5)/(dfVal+0.5) + 1)
			denom := tf + k1*(1-b+b*docLen/avgDocLen)
			score += idf * tf * (k1 + 1) / denom
		}
		scores[i] = scored{idx: i, score: score}
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	results := make([]searchResult, 0, topK)
	for _, s := range scores {
		if s.score <= 0 || len(results) >= topK {
			break
		}
		tool := ts.Tools[s.idx]
		results = append(results, searchResult{
			Name:        tool.Name,
			Score:       math.Round(s.score*10000) / 10000,
			Description: tool.Description,
		})
	}
	return results
}

// termFreq returns a map from term to count in the document.
func termFreq(doc []string) map[string]int {
	m := make(map[string]int, len(doc))
	for _, t := range doc {
		m[t]++
	}
	return m
}

// containsTerm reports whether doc contains term.
func containsTerm(doc []string, term string) bool {
	return slices.Contains(doc, term)
}

// avgDocLength returns the average token count across documents.
func avgDocLength(docs [][]string) float64 {
	if len(docs) == 0 {
		return 0
	}
	total := 0
	for _, d := range docs {
		total += len(d)
	}
	return float64(total) / float64(len(docs))
}

// toolQueryPatterns matches common tool search phrasing.
var toolQueryPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)search\s+tools?\s+for\s+(.+)`),
	regexp.MustCompile(`(?i)find\s+tool\s+(.+)`),
	regexp.MustCompile(`(?i)which\s+tool\s+can\s+(.+)`),
	regexp.MustCompile(`(?i)what\s+tool\s+(.+)`),
	regexp.MustCompile(`(?i)look(?:ing)?\s+for\s+(?:a\s+)?tool\s+(?:that|to|for)\s+(.+)`),
}

// extractToolQuery checks input for tool search patterns and returns the
// captured query text, or an empty string if no pattern matches.
func extractToolQuery(input string) string {
	for _, re := range toolQueryPatterns {
		matches := re.FindStringSubmatch(input)
		if len(matches) >= 2 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}
