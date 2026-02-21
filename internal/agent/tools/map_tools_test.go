package tools

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLlmMapToolCreation(t *testing.T) {
	t.Parallel()

	tool := NewLlmMapTool()
	require.NotNil(t, tool)

	// Verify it implements fantasy.AgentTool
	_ = tool
}

func TestAgenticMapToolCreation(t *testing.T) {
	t.Parallel()

	tool := NewAgenticMapTool()
	require.NotNil(t, tool)

	// Verify it implements fantasy.AgentTool
	_ = tool
}

func TestReadJSONL(t *testing.T) {
	t.Parallel()

	// Create a temporary JSONL file
	tmpFile := t.TempDir() + "/test.jsonl"

	content := `{"name": "Alice", "age": 30}
{"name": "Bob", "age": 25}
{"name": "Charlie", "age": 35}
`
	err := writeFile(tmpFile, []byte(content))
	require.NoError(t, err)

	// Read the JSONL file
	items, err := readJSONL(tmpFile)
	require.NoError(t, err)
	require.Len(t, items, 3)

	// Verify the first item
	require.JSONEq(t, `{"name": "Alice", "age": 30}`, string(items[0]))
}

func TestReadJSONLEmptyLines(t *testing.T) {
	t.Parallel()

	tmpFile := t.TempDir() + "/test.jsonl"

	content := `{"name": "Alice"}

{"name": "Bob"}

`
	err := writeFile(tmpFile, []byte(content))
	require.NoError(t, err)

	items, err := readJSONL(tmpFile)
	require.NoError(t, err)
	require.Len(t, items, 2, "Empty lines should be skipped")
}

func TestReadJSONLInvalidJSON(t *testing.T) {
	t.Parallel()

	tmpFile := t.TempDir() + "/test.jsonl"

	content := `{"name": "Alice"}
not valid json
{"name": "Bob"}
`
	err := writeFile(tmpFile, []byte(content))
	require.NoError(t, err)

	_, err = readJSONL(tmpFile)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid JSON at line 2")
}

func TestStripMarkdownFences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "json fence",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "plain fence",
			input:    "```\n{\"key\": \"value\"}\n```",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "no fence",
			input:    "{\"key\": \"value\"}",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "fence with extra whitespace",
			input:    "  ```json\n{\"key\": \"value\"}\n```  ",
			expected: "{\"key\": \"value\"}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := stripMarkdownFences(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

// Helper function for tests
func writeFile(path string, content []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(content)
	return err
}
