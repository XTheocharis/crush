package processor

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSkillSearch_WithKnownSkillsAndQuery(t *testing.T) {
	t.Parallel()

	p := SkillSearch{}
	loadedSkills := []map[string]any{
		{"name": "git-master", "description": "Git operations", "content": "git help", "tags": []string{"git", "vcs"}},
		{"name": "code-review", "description": "Review pull requests", "content": "review help", "tags": []string{"review"}},
		{"name": "deploy", "description": "Deploy to production", "content": "deploy help", "tags": []string{"deploy", "ci"}},
	}

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Input:    "search skills for git",
		Messages: []Message{UserMessage("search skills for git")},
		State: map[string]any{
			"loaded_skills": loadedSkills,
			"skill_count":   3,
		},
	}

	result, err := p.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)

	results, ok := result.State["skill_search_results"].([]map[string]any)
	require.True(t, ok, "skill_search_results should be []map[string]any")
	require.NotEmpty(t, results, "should find at least one result")
	require.Equal(t, "git-master", results[0]["name"])

	query, ok := result.State["query"].(string)
	require.True(t, ok)
	require.Equal(t, "git", query)
}

func TestSkillSearch_NoSkillsLoaded(t *testing.T) {
	t.Parallel()

	p := SkillSearch{}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Input:    "search skills for git",
		Messages: []Message{UserMessage("search skills for git")},
		State:    make(map[string]any),
	}

	result, err := p.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Nil(t, result.State, "no state changes without loaded skills")
}

func TestSkillSearch_NoSearchQuery(t *testing.T) {
	t.Parallel()

	p := SkillSearch{}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Input:    "just a normal input",
		Messages: []Message{UserMessage("just a normal input")},
		State: map[string]any{
			"loaded_skills": []map[string]any{
				{"name": "test", "description": "test skill", "content": "", "tags": []string{}},
			},
		},
	}

	result, err := p.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Nil(t, result.State, "no state changes without search query")
}

func TestSkillSearch_FindSkillVariant(t *testing.T) {
	t.Parallel()

	p := SkillSearch{}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Input:    "find skill deploy",
		Messages: []Message{UserMessage("find skill deploy")},
		State: map[string]any{
			"loaded_skills": []map[string]any{
				{"name": "deploy", "description": "Deploy to prod", "content": "", "tags": []string{"deploy"}},
			},
		},
	}

	result, err := p.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	results, ok := result.State["skill_search_results"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, results, 1)
	require.Equal(t, "deploy", results[0]["name"])
}

func TestSkillSearch_BM25ScoringCorrectness(t *testing.T) {
	t.Parallel()

	skills := []map[string]any{
		{"name": "git-master", "description": "Git operations skill", "content": "", "tags": []string{"git"}},
		{"name": "github-actions", "description": "GitHub CI/CD pipelines", "content": "", "tags": []string{"ci", "github"}},
		{"name": "code-review", "description": "Review code changes", "content": "", "tags": []string{"review"}},
		{"name": "git-rebase", "description": "Interactive git rebase", "content": "", "tags": []string{"git", "rebase"}},
	}

	results := bm25Search("git", skills, 5)
	require.NotEmpty(t, results)

	// git-master and git-rebase should rank higher than github-actions
	// because "git" matches their names directly.
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r["name"].(string)
	}
	require.Contains(t, names, "git-master")
	require.Contains(t, names, "git-rebase")

	// git-master should score highest (name + tag both match).
	require.Equal(t, "git-master", results[0]["name"])
}

func TestSkillSearch_TopKLimit(t *testing.T) {
	t.Parallel()

	skills := []map[string]any{
		{"name": "git-1", "description": "Git tool one", "content": "", "tags": []string{"git"}},
		{"name": "git-2", "description": "Git tool two", "content": "", "tags": []string{"git"}},
		{"name": "git-3", "description": "Git tool three", "content": "", "tags": []string{"git"}},
		{"name": "git-4", "description": "Git tool four", "content": "", "tags": []string{"git"}},
		{"name": "git-5", "description": "Git tool five", "content": "", "tags": []string{"git"}},
		{"name": "git-6", "description": "Git tool six", "content": "", "tags": []string{"git"}},
	}

	results := bm25Search("git", skills, 5)
	require.Len(t, results, 5, "should return at most topK results")
}

func TestSkillSearch_NoMatch(t *testing.T) {
	t.Parallel()

	skills := []map[string]any{
		{"name": "deploy", "description": "Deploy to prod", "content": "", "tags": []string{"deploy"}},
		{"name": "review", "description": "Code review", "content": "", "tags": []string{"review"}},
	}

	results := bm25Search("kubernetes", skills, 5)
	require.Empty(t, results, "no results for non-matching query")
}

func TestSkillSearch_ID(t *testing.T) {
	t.Parallel()
	p := SkillSearch{}
	require.Equal(t, "skill_search", p.ID())
}

func TestSkillSearch_PassThroughPhases(t *testing.T) {
	t.Parallel()

	p := SkillSearch{}
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{UserMessage("msg")},
		State:    make(map[string]any),
	}

	result, err := p.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)

	result, err = p.ProcessOutputResult(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)

	result, err = p.ProcessAPIError(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
}

func TestSkillSearch_SequentialWithSkillsProcessor(t *testing.T) {
	t.Parallel()

	skillsProc := &Skills{
		Skills: []SkillDef{
			{Name: "git-master", Description: "Git operations", Content: "git help", Tags: []string{"git"}},
			{Name: "deploy", Description: "Deploy to production", Content: "deploy help", Tags: []string{"deploy", "ci"}},
			{Name: "review", Description: "Code review helper", Content: "review help", Tags: []string{"review"}},
		},
	}
	searchProc := SkillSearch{}

	r := NewRunner(WithInputProcessors(skillsProc, searchProc))
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Input:    "search skills for git",
		Messages: []Message{UserMessage("search skills for git")},
		State:    make(map[string]any),
	}

	result, err := r.Execute(context.Background(), InputPhase, pctx)
	require.NoError(t, err)

	results, ok := result.State["skill_search_results"].([]map[string]any)
	require.True(t, ok, "skill_search_results should exist after sequential run")
	require.NotEmpty(t, results)
	require.Equal(t, "git-master", results[0]["name"])
	require.Equal(t, 3, result.State["skill_count"])
}

func TestSkillSearch_BM25ScoreValues(t *testing.T) {
	t.Parallel()

	// Single document containing "hello" — verify score uses BM25 formula.
	skills := []map[string]any{
		{"name": "greeting", "description": "Say hello world", "content": "", "tags": []string{"hello"}},
	}

	results := bm25Search("hello", skills, 5)
	require.Len(t, results, 1)

	score, ok := results[0]["score"].(float64)
	require.True(t, ok)
	require.False(t, math.IsNaN(score), "score should not be NaN")
	require.False(t, math.IsInf(score, 0), "score should not be Inf")
	require.Greater(t, score, 0.0, "BM25 score should be positive for a match")
}

func TestSkillSearch_ExtractSearchQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"search skills for git rebase", "git rebase"},
		{"find skill deploy", "deploy"},
		{"Search Skills For testing", "testing"},
		{"search skill for foo bar", "foo bar"},
		{"just normal text", ""},
		{"search something else", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := extractSearchQuery(tt.input)
		require.Equal(t, tt.expected, got, "input: %q", tt.input)
	}
}

func TestSkillSearch_Tokenize(t *testing.T) {
	t.Parallel()

	tokens := tokenize("Hello, World! 123")
	require.Equal(t, []string{"hello", "world", "123"}, tokens)

	tokens = tokenize("")
	require.Empty(t, tokens)

	tokens = tokenize("  ---  ")
	require.Empty(t, tokens)
}
