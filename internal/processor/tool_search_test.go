package processor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleTools() []ToolDef {
	return []ToolDef{
		{Name: "bash", Description: "Execute shell commands in a terminal", Tags: []string{"shell", "terminal", "execute"}},
		{Name: "edit", Description: "Edit file contents with string replacement", Tags: []string{"file", "write", "modify"}},
		{Name: "view", Description: "Read file contents from the filesystem", Tags: []string{"file", "read"}},
		{Name: "grep", Description: "Search file contents using regular expressions", Tags: []string{"search", "find", "regex"}},
		{Name: "glob", Description: "Find files matching a pattern", Tags: []string{"find", "files", "pattern"}},
		{Name: "lsp_diagnostics", Description: "Get errors and warnings from language server", Tags: []string{"lsp", "errors", "diagnostics"}},
	}
}

func TestToolSearchBM25Scoring(t *testing.T) {
	t.Parallel()
	ts := &ToolSearch{Tools: sampleTools()}

	results := ts.search("execute shell commands", 5)
	require.NotEmpty(t, results, "should return results for matching query")

	found := false
	for _, r := range results {
		if r.Name == "bash" {
			found = true
			require.Greater(t, r.Score, 0.0, "bash should have positive score")
			break
		}
	}
	require.True(t, found, "bash should be in results for 'execute shell commands'")
}

func TestToolSearchQueryDetection(t *testing.T) {
	t.Parallel()
	ts := &ToolSearch{Tools: sampleTools()}
	ctx := context.Background()

	cases := []struct {
		name       string
		input      string
		shouldFind bool
	}{
		{"search tools for", "search tools for file editing", true},
		{"find tool", "find tool that can read files", true},
		{"which tool can", "which tool can search for patterns", true},
		{"what tool", "what tool should I use for editing", true},
		{"looking for tool", "looking for a tool that executes commands", true},
		{"normal input", "please refactor the auth module", false},
		{"empty input", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pctx := ProcessorContext{
				Phase:    InputPhase,
				Input:    tc.input,
				Messages: []Message{UserMessage("hello")},
				State:    make(map[string]any),
			}
			result, err := ts.ProcessInput(ctx, pctx)
			require.NoError(t, err)
			require.Equal(t, ActionContinue, result.Action)

			if tc.shouldFind {
				require.NotNil(t, result.State, "should have state for search query")
				require.Contains(t, result.State, "tool_search_results")
				require.Contains(t, result.State, "query")
			} else {
				require.Nil(t, result.State, "should not have state for non-search input")
			}
		})
	}
}

func TestToolSearchNoQueryPassthrough(t *testing.T) {
	t.Parallel()
	ts := &ToolSearch{Tools: sampleTools()}
	msgs := []Message{UserMessage("refactor the code")}

	result, err := ts.ProcessInput(context.Background(), ProcessorContext{
		Phase:    InputPhase,
		Input:    "refactor the code",
		Messages: msgs,
		State:    make(map[string]any),
	})
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
	require.Nil(t, result.State)
}

func TestToolSearchEmptyRegistry(t *testing.T) {
	t.Parallel()
	ts := &ToolSearch{Tools: nil}

	result, err := ts.ProcessInput(context.Background(), ProcessorContext{
		Phase:    InputPhase,
		Input:    "search tools for bash",
		Messages: []Message{UserMessage("hello")},
		State:    make(map[string]any),
	})
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Contains(t, result.State, "tool_search_results")

	results := result.State["tool_search_results"]
	require.Nil(t, results, "empty registry should return nil results")
}

func TestToolSearchExactNameMatchRanksHighest(t *testing.T) {
	t.Parallel()
	ts := &ToolSearch{Tools: sampleTools()}

	results := ts.search("grep", 5)
	require.NotEmpty(t, results)
	require.Equal(t, "grep", results[0].Name, "exact name match should rank first")

	for _, r := range results[1:] {
		require.GreaterOrEqual(t, results[0].Score, r.Score,
			"grep should have highest score")
	}
}

func TestToolSearchStateContainsResults(t *testing.T) {
	t.Parallel()
	ts := &ToolSearch{Tools: sampleTools()}

	result, err := ts.ProcessInput(context.Background(), ProcessorContext{
		Phase:    InputPhase,
		Input:    "which tool can read files",
		Messages: []Message{UserMessage("hello")},
		State:    make(map[string]any),
	})
	require.NoError(t, err)

	require.Contains(t, result.State, "tool_search_results")
	require.Contains(t, result.State, "query")
	require.Equal(t, "read files", result.State["query"])

	searchResults, ok := result.State["tool_search_results"].([]searchResult)
	require.True(t, ok, "tool_search_results should be []searchResult")
	require.NotEmpty(t, searchResults)

	for _, r := range searchResults {
		require.NotEmpty(t, r.Name)
		require.Greater(t, r.Score, 0.0)
		require.NotEmpty(t, r.Description)
	}
}

func TestToolSearchRunAllPhases(t *testing.T) {
	t.Parallel()
	ts := &ToolSearch{Tools: sampleTools()}

	pctx := ProcessorContext{
		Phase:    InputPhase,
		Input:    "search tools for file editing",
		Messages: []Message{UserMessage("hello")},
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	final := RunAllPhases(t, ts, pctx)
	require.Contains(t, final.State, "tool_search_results")
	require.Contains(t, final.State, "query")
}

func TestToolSearchPassthroughPhases(t *testing.T) {
	t.Parallel()
	ts := &ToolSearch{Tools: sampleTools()}
	ctx := context.Background()
	msgs := []Message{AssistantMessage("response")}

	pctx := ProcessorContext{
		Phase:        OutputStreamPhase,
		OutputStream: "some output",
		Messages:     msgs,
		State:        make(map[string]any),
	}

	result, err := ts.ProcessOutputStream(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)

	pctx.Phase = OutputResultPhase
	result, err = ts.ProcessOutputResult(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)

	pctx.Phase = APIErrorPhase
	result, err = ts.ProcessAPIError(ctx, pctx)
	require.NoError(t, err)
	require.Equal(t, ActionContinue, result.Action)
	require.Equal(t, msgs, result.Messages)
}

func TestToolSearchTopKLimit(t *testing.T) {
	t.Parallel()
	ts := &ToolSearch{Tools: sampleTools()}

	results := ts.search("file", 3)
	require.LessOrEqual(t, len(results), 3, "should return at most top-K results")
}

func TestToolSearchMessagesUnchanged(t *testing.T) {
	t.Parallel()
	ts := &ToolSearch{Tools: sampleTools()}
	originalMsgs := []Message{UserMessage("test"), AssistantMessage("response")}

	result, err := ts.ProcessInput(context.Background(), ProcessorContext{
		Phase:    InputPhase,
		Input:    "search tools for bash",
		Messages: originalMsgs,
		State:    make(map[string]any),
	})
	require.NoError(t, err)
	require.Equal(t, originalMsgs, result.Messages, "messages should be unchanged")
}
