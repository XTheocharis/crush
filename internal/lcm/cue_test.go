package lcm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCueInjector_DefaultTemplates(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	require.NotNil(t, ci)
	require.Len(t, ci.templates, 3)
	_, hasSummary := ci.templates[CueTypeSummaryID]
	_, hasLineage := ci.templates[CueTypeLineagePointer]
	_, hasArchive := ci.templates[CueTypeArchiveStub]
	require.True(t, hasSummary, "should have summary_id template")
	require.True(t, hasLineage, "should have lineage_pointer template")
	require.True(t, hasArchive, "should have archive_stub template")
}

func TestCueTypeConstants(t *testing.T) {
	t.Parallel()
	require.Equal(t, "summary_id", CueTypeSummaryID)
	require.Equal(t, "lineage_pointer", CueTypeLineagePointer)
	require.Equal(t, "archive_stub", CueTypeArchiveStub)
}

func TestRender_SummaryID(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	result := ci.Render(CueTypeSummaryID, map[string]string{
		"SummaryID": "sum_a1b2c3d4e5f6a7b8",
		"Snippet":   "discussion about caching",
	})
	require.Equal(t, "[sum_a1b2c3d4e5f6a7b8] discussion about caching", result)
}

func TestRender_LineagePointer(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	result := ci.Render(CueTypeLineagePointer, map[string]string{
		"ParentIDs": "sum_aaa,sum_bbb",
		"Depth":     "3",
	})
	require.Equal(t, "[Lineage: sum_aaa,sum_bbb, depth=3]", result)
}

func TestRender_ArchiveStub(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	result := ci.Render(CueTypeArchiveStub, map[string]string{
		"SummaryID":  "sum_f1e2d3c4b5a69788",
		"TokenCount": "4096",
	})
	require.Equal(t, "[Archived: sum_f1e2d3c4b5a69788, tokens=4096]", result)
}

func TestRender_UnknownType_Fallback(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	result := ci.Render("unknown_type", map[string]string{
		"Foo": "bar",
		"Baz": "qux",
	})
	require.Contains(t, result, "bar")
	require.Contains(t, result, "qux")
}

func TestRegisterTemplate_Override(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	ci.RegisterTemplate(CueTypeSummaryID, CueTemplate{
		Template: "Summary {{.ID}} ({{.Kind}})",
	})
	result := ci.Render(CueTypeSummaryID, map[string]string{
		"ID":   "sum_abc",
		"Kind": "leaf",
	})
	require.Equal(t, "Summary sum_abc (leaf)", result)
}

func TestRegisterTemplate_NewType(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	ci.RegisterTemplate("custom_type", CueTemplate{
		Template: "Custom: {{.Val}}",
	})
	result := ci.Render("custom_type", map[string]string{"Val": "hello"})
	require.Equal(t, "Custom: hello", result)
}

func TestNewCue_IDPrefix(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	cue := ci.NewCue(CueTypeSummaryID, 5, map[string]string{
		"SummaryID": "sum_abc123",
		"Snippet":   "test snippet",
	})
	require.True(t, strings.HasPrefix(cue.ID, "cue_"), "cue ID should have cue_ prefix")
	require.Len(t, cue.ID, 20, "cue_ prefix + 16 hex chars = 20 chars")
}

func TestNewCue_Fields(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	cue := ci.NewCue(CueTypeSummaryID, 10, map[string]string{
		"SummaryID": "sum_deadbeef",
		"Snippet":   "some content here",
	})
	require.Equal(t, CueTypeSummaryID, cue.Type)
	require.Equal(t, 10, cue.Priority)
	require.Equal(t, "[sum_deadbeef] some content here", cue.Content)
}

func TestNewCue_UniqueIDs(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	cue1 := ci.NewCue(CueTypeSummaryID, 1, map[string]string{"SummaryID": "a", "Snippet": "s1"})
	cue2 := ci.NewCue(CueTypeSummaryID, 1, map[string]string{"SummaryID": "a", "Snippet": "s1"})
	require.NotEqual(t, cue1.ID, cue2.ID, "each cue should get a unique ID")
}

func TestInjectIntoPrompt_EmptyCues(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	prompt := "<system>base prompt</system>"
	result := ci.InjectIntoPrompt(prompt, nil, 1000)
	require.Equal(t, prompt, result)
}

func TestInjectIntoPrompt_SingleCue(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	prompt := "<system>base prompt</system>"
	cues := []GhostCue{
		{ID: "cue_aaa", Type: CueTypeSummaryID, Priority: 5, Content: "[sum_abc] snippet text"},
	}
	result := ci.InjectIntoPrompt(prompt, cues, 1000)
	require.Contains(t, result, "<system>base prompt</system>")
	require.Contains(t, result, "[sum_abc] snippet text")
}

func TestInjectIntoPrompt_PriorityOrdering(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	prompt := "system-prompt"
	cues := []GhostCue{
		{ID: "cue_low", Type: CueTypeSummaryID, Priority: 1, Content: "LOW_PRIORITY"},
		{ID: "cue_high", Type: CueTypeSummaryID, Priority: 10, Content: "HIGH_PRIORITY"},
		{ID: "cue_mid", Type: CueTypeSummaryID, Priority: 5, Content: "MID_PRIORITY"},
	}
	result := ci.InjectIntoPrompt(prompt, cues, 1000)

	highIdx := strings.Index(result, "HIGH_PRIORITY")
	midIdx := strings.Index(result, "MID_PRIORITY")
	lowIdx := strings.Index(result, "LOW_PRIORITY")

	require.True(t, highIdx < midIdx, "high priority should appear before mid")
	require.True(t, midIdx < lowIdx, "mid priority should appear before low")
}

func TestInjectIntoPrompt_BudgetConstraint(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	prompt := "system"

	// Each cue content is ~15 chars = ~4 tokens.
	cues := []GhostCue{
		{ID: "cue_a", Type: CueTypeSummaryID, Priority: 10, Content: "AAAAAAAAAAAAAAA"},
		{ID: "cue_b", Type: CueTypeSummaryID, Priority: 5, Content: "BBBBBBBBBBBBBBB"},
		{ID: "cue_c", Type: CueTypeSummaryID, Priority: 1, Content: "CCCCCCCCCCCCCCC"},
	}

	// Budget only fits the first cue (~4 tokens).
	result := ci.InjectIntoPrompt(prompt, cues, 5)
	require.Contains(t, result, "AAAAAAAAAAAAAAA")
	require.NotContains(t, result, "BBBBBBBBBBBBBBB", "should drop cue_b due to budget")
	require.NotContains(t, result, "CCCCCCCCCCCCCCC", "should drop cue_c due to budget")
}

func TestInjectIntoPrompt_BudgetZero(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	prompt := "system"
	cues := []GhostCue{
		{ID: "cue_a", Type: CueTypeSummaryID, Priority: 10, Content: "SOME_CONTENT"},
	}
	result := ci.InjectIntoPrompt(prompt, cues, 0)
	require.Equal(t, prompt, result, "zero budget should produce no injection")
}

func TestInjectIntoPrompt_StableSort(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	prompt := "system"
	cues := []GhostCue{
		{ID: "cue_first", Type: CueTypeSummaryID, Priority: 5, Content: "FIRST"},
		{ID: "cue_second", Type: CueTypeLineagePointer, Priority: 5, Content: "SECOND"},
		{ID: "cue_third", Type: CueTypeArchiveStub, Priority: 5, Content: "THIRD"},
	}
	result := ci.InjectIntoPrompt(prompt, cues, 100)

	firstIdx := strings.Index(result, "FIRST")
	secondIdx := strings.Index(result, "SECOND")
	thirdIdx := strings.Index(result, "THIRD")
	require.True(t, firstIdx < secondIdx, "equal priority should preserve insertion order")
	require.True(t, secondIdx < thirdIdx, "equal priority should preserve insertion order")
}

func TestInjectIntoToolResult_EmptyCues(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	result := ci.InjectIntoToolResult("tool output", nil, 1000)
	require.Equal(t, "tool output", result)
}

func TestInjectIntoToolResult_SingleCue(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	cues := []GhostCue{
		{ID: "cue_a", Type: CueTypeArchiveStub, Priority: 3, Content: "[Archived: sum_abc, tokens=500]"},
	}
	result := ci.InjectIntoToolResult("tool output", cues, 1000)
	require.Contains(t, result, "tool output")
	require.Contains(t, result, "[Archived: sum_abc, tokens=500]")
}

func TestInjectIntoToolResult_BudgetConstraint(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	cues := []GhostCue{
		{ID: "cue_a", Type: CueTypeSummaryID, Priority: 10, Content: "HIGH_CUE"},
		{ID: "cue_b", Type: CueTypeSummaryID, Priority: 1, Content: "LOW_CUE_SHOULD_BE_DROPPED"},
	}
	// "HIGH_CUE" is 8 chars = 2 tokens. Budget of 3 only fits the first.
	result := ci.InjectIntoToolResult("result", cues, 3)
	require.Contains(t, result, "HIGH_CUE")
	require.NotContains(t, result, "LOW_CUE_SHOULD_BE_DROPPED")
}

func TestGenerateCueID_Format(t *testing.T) {
	t.Parallel()
	id := generateCueID(CueTypeSummaryID)
	require.True(t, strings.HasPrefix(id, "cue_"))
	require.Len(t, id, 20)

	// All hex after prefix.
	hexPart := id[4:]
	for _, c := range hexPart {
		require.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"cue ID hex part should be lowercase hex")
	}
}

func TestGhostCue_AllTypes(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()

	summaryCue := ci.NewCue(CueTypeSummaryID, 10, map[string]string{
		"SummaryID": "sum_1111222233334444",
		"Snippet":   "a condensed summary",
	})
	require.Equal(t, CueTypeSummaryID, summaryCue.Type)
	require.Contains(t, summaryCue.Content, "sum_1111222233334444")

	lineageCue := ci.NewCue(CueTypeLineagePointer, 7, map[string]string{
		"ParentIDs": "sum_aaa,sum_bbb",
		"Depth":     "2",
	})
	require.Equal(t, CueTypeLineagePointer, lineageCue.Type)
	require.Contains(t, lineageCue.Content, "sum_aaa,sum_bbb")
	require.Contains(t, lineageCue.Content, "2")

	archiveCue := ci.NewCue(CueTypeArchiveStub, 3, map[string]string{
		"SummaryID":  "sum_9999",
		"TokenCount": "2048",
	})
	require.Equal(t, CueTypeArchiveStub, archiveCue.Type)
	require.Contains(t, archiveCue.Content, "sum_9999")
	require.Contains(t, archiveCue.Content, "2048")
}

func TestInjectIntoPrompt_CuesAfterPrompt(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	prompt := "<system>instructions</system>"
	cues := []GhostCue{
		{ID: "cue_a", Type: CueTypeSummaryID, Priority: 5, Content: "[sum_xyz] snippet"},
	}
	result := ci.InjectIntoPrompt(prompt, cues, 100)

	promptEnd := strings.LastIndex(result, "</system>")
	cueStart := strings.Index(result, "[sum_xyz]")
	require.True(t, cueStart > promptEnd, "cue should be injected after the prompt boundary")
}

func TestInjectIntoToolResult_PriorityOrdering(t *testing.T) {
	t.Parallel()
	ci := NewCueInjector()
	cues := []GhostCue{
		{ID: "cue_low", Type: CueTypeSummaryID, Priority: 1, Content: "LOW"},
		{ID: "cue_high", Type: CueTypeSummaryID, Priority: 10, Content: "HIGH"},
		{ID: "cue_mid", Type: CueTypeSummaryID, Priority: 5, Content: "MID"},
	}
	result := ci.InjectIntoToolResult("result", cues, 100)

	highIdx := strings.Index(result, "HIGH")
	midIdx := strings.Index(result, "MID")
	lowIdx := strings.Index(result, "LOW")
	require.True(t, highIdx < midIdx, "high priority should come first")
	require.True(t, midIdx < lowIdx, "mid priority should come before low")
}
