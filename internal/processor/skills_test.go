package processor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSkills_LoadsSkillsIntoState(t *testing.T) {
	t.Parallel()

	skills := []SkillDef{
		{Name: "git-master", Description: "Git operations skill", Content: "Use for git", Tags: []string{"git", "vcs"}},
		{Name: "code-review", Description: "Review code changes", Content: "Use for reviews", Tags: []string{"review"}},
	}

	p := &Skills{Skills: skills}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Input:    "hello",
		Messages: []Message{UserMessage("hello")},
		State:    make(map[string]any),
	}

	result, err := p.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Len(t, result.Messages, 1)

	loaded, ok := result.State["loaded_skills"].([]map[string]any)
	require.True(t, ok, "loaded_skills should be []map[string]any")
	require.Len(t, loaded, 2)
	require.Equal(t, "git-master", loaded[0]["name"])
	require.Equal(t, "Git operations skill", loaded[0]["description"])
	require.Equal(t, "Use for git", loaded[0]["content"])
	require.Equal(t, []string{"git", "vcs"}, loaded[0]["tags"])

	require.Equal(t, 2, result.State["skill_count"])
}

func TestSkills_EmptySkillList(t *testing.T) {
	t.Parallel()

	p := &Skills{Skills: nil}
	pctx := ProcessorContext{
		Phase:    InputPhase,
		Input:    "hello",
		Messages: []Message{UserMessage("hello")},
		State:    make(map[string]any),
	}

	result, err := p.ProcessInput(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)

	loaded, ok := result.State["loaded_skills"].([]map[string]any)
	require.True(t, ok)
	require.Empty(t, loaded)
	require.Equal(t, 0, result.State["skill_count"])
}

func TestSkills_PassThroughPhases(t *testing.T) {
	t.Parallel()

	p := &Skills{Skills: []SkillDef{{Name: "test"}}}
	pctx := ProcessorContext{
		Phase:    OutputStreamPhase,
		Messages: []Message{UserMessage("msg")},
		State:    make(map[string]any),
	}

	result, err := p.ProcessOutputStream(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Len(t, result.Messages, 1)

	result, err = p.ProcessOutputResult(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)

	result, err = p.ProcessAPIError(context.Background(), pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
}

func TestSkills_ID(t *testing.T) {
	t.Parallel()
	p := &Skills{}
	require.Equal(t, "skills", p.ID())
}

func TestSkills_RunAllPhases(t *testing.T) {
	t.Parallel()

	p := &Skills{
		Skills: []SkillDef{
			{Name: "review", Description: "Code review", Content: "content", Tags: []string{"review"}},
		},
	}

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Input:    "test",
		Messages: []Message{UserMessage("hi")},
		State:    make(map[string]any),
	}

	final := RunAllPhases(t, p, pctx)
	require.Equal(t, 1, final.State["skill_count"])

	loaded, ok := final.State["loaded_skills"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, loaded, 1)
	require.Equal(t, "review", loaded[0]["name"])
}
