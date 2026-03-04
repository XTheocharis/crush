package lcm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetSystemPromptFile(t *testing.T) {
	t.Parallel()
	file := GetSystemPromptFile()
	require.Equal(t, "LCM Instructions", file.Name)
	require.Contains(t, file.Content, "Lossless Context Management")
	require.Contains(t, file.Content, "lcm_grep")
	require.Contains(t, file.Content, "lcm_describe")
	require.Contains(t, file.Content, "lcm_expand")
}

func TestLCMSystemPrompt_ContainsAllTools(t *testing.T) {
	t.Parallel()
	require.Contains(t, LCMSystemPrompt, "lcm_grep")
	require.Contains(t, LCMSystemPrompt, "lcm_describe")
	require.Contains(t, LCMSystemPrompt, "lcm_expand")
	require.Contains(t, LCMSystemPrompt, "summary_id")
	require.Contains(t, LCMSystemPrompt, "file_")
	require.Contains(t, LCMSystemPrompt, "sum_")
}

func TestLCMSystemPrompt_ContainsMapTools(t *testing.T) {
	t.Parallel()
	require.Contains(t, LCMSystemPrompt, "llm_map")
	require.Contains(t, LCMSystemPrompt, "agentic_map")
}

func TestConstants(t *testing.T) {
	t.Parallel()
	require.Equal(t, 4, CharsPerToken)
	require.Equal(t, 10000, LargeOutputThreshold)
	require.Equal(t, 10, MaxCompactionRounds)
	require.Equal(t, 3, MinMessagesToSummarize)
	require.Equal(t, 2048, FallbackMaxChars)
	require.Equal(t, "sum_", SummaryIDPrefix)
	require.Equal(t, "file_", FileIDPrefix)
	require.Equal(t, "leaf", KindLeaf)
	require.Equal(t, "condensed", KindCondensed)
}
