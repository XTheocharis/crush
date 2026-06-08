package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/stretchr/testify/require"
)

func TestCaptureSession_BasicConversation(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{
		newTextMessage("m1", "s1", message.User, "Hello, how are you?"),
		newTextMessage("m2", "s1", message.Assistant, "I'm doing well, thank you!"),
		newTextMessage("m3", "s1", message.User, "What is 2+2?"),
		newTextMessage("m4", "s1", message.Assistant, "2+2 equals 4."),
	}

	dataset, err := CaptureSession(context.Background(), "s1", msgs)
	require.NoError(t, err)
	require.NotNil(t, dataset)
	require.Len(t, dataset.Examples, 2)

	require.Equal(t, "Hello, how are you?", dataset.Examples[0].Input.Conversation[0].Content)
	require.Equal(t, "I'm doing well, thank you!", dataset.Examples[0].Expected.Conversation[len(dataset.Examples[0].Expected.Conversation)-1].Content)

	require.Equal(t, "What is 2+2?", dataset.Examples[1].Input.Conversation[0].Content)
	require.Equal(t, "2+2 equals 4.", dataset.Examples[1].Expected.Conversation[len(dataset.Examples[1].Expected.Conversation)-1].Content)
}

func TestCaptureSession_WithToolCalls(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{
		newTextMessage("m1", "s1", message.User, "Read the file main.go"),
		{
			ID: "m2", SessionID: "s1", Role: message.Assistant,
			Parts: []message.ContentPart{
				message.ToolCall{ID: "tc1", Name: "view", Input: `{"path":"main.go"}`, Finished: true},
			},
		},
		{
			ID: "m3", SessionID: "s1", Role: message.Tool,
			Parts: []message.ContentPart{
				message.ToolResult{ToolCallID: "tc1", Name: "view", Content: "package main\n\nfunc main() {}"},
			},
		},
		newTextMessage("m4", "s1", message.Assistant, "The file main.go contains a simple main function."),
	}

	dataset, err := CaptureSession(context.Background(), "s1", msgs)
	require.NoError(t, err)
	require.Len(t, dataset.Examples, 1)

	ex := dataset.Examples[0]
	require.Equal(t, "Read the file main.go", ex.Input.Conversation[0].Content)

	expected := ex.Expected.Conversation
	require.Equal(t, "user", expected[0].Role)
	require.Equal(t, "Read the file main.go", expected[0].Content)
	require.Equal(t, "assistant", expected[1].Role)
	require.Contains(t, expected[1].Content, "[tool_call] view")
	require.Equal(t, "tool", expected[2].Role)
	require.Contains(t, expected[2].Content, "package main")
	require.Equal(t, "assistant", expected[3].Role)
	require.Equal(t, "The file main.go contains a simple main function.", expected[3].Content)
}

func TestCaptureSession_EmptySession(t *testing.T) {
	t.Parallel()

	_, err := CaptureSession(context.Background(), "s1", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no messages found")
}

func TestCaptureSession_OnlyUserMessages(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{
		newTextMessage("m1", "s1", message.User, "Hello"),
		newTextMessage("m2", "s1", message.User, "World"),
	}

	_, err := CaptureSession(context.Background(), "s1", msgs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no complete user-assistant pairs")
}

func TestCaptureSession_SkipsSummaryMessages(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{
		newTextMessage("m1", "s1", message.User, "Hello"),
		newTextMessage("m2", "s1", message.Assistant, "Hi there!"),
		{ID: "m3", SessionID: "s1", Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "Summary text"}}, IsSummaryMessage: true},
		newTextMessage("m4", "s1", message.User, "How are you?"),
		newTextMessage("m5", "s1", message.Assistant, "Fine!"),
	}

	dataset, err := CaptureSession(context.Background(), "s1", msgs)
	require.NoError(t, err)
	require.Len(t, dataset.Examples, 2)
}

func TestCaptureSession_SkipsSystemMessages(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{
		newTextMessage("m0", "s1", message.System, "You are a helpful assistant."),
		newTextMessage("m1", "s1", message.User, "Hello"),
		newTextMessage("m2", "s1", message.Assistant, "Hi!"),
	}

	dataset, err := CaptureSession(context.Background(), "s1", msgs)
	require.NoError(t, err)
	require.Len(t, dataset.Examples, 1)
}

func TestCaptureSession_ToolResultWithError(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{
		newTextMessage("m1", "s1", message.User, "Read missing file"),
		{
			ID: "m2", SessionID: "s1", Role: message.Assistant,
			Parts: []message.ContentPart{
				message.ToolCall{ID: "tc1", Name: "view", Input: `{"path":"missing.go"}`, Finished: true},
			},
		},
		{
			ID: "m3", SessionID: "s1", Role: message.Tool,
			Parts: []message.ContentPart{
				message.ToolResult{ToolCallID: "tc1", Name: "view", Content: "file not found", IsError: true},
			},
		},
		newTextMessage("m4", "s1", message.Assistant, "The file does not exist."),
	}

	dataset, err := CaptureSession(context.Background(), "s1", msgs)
	require.NoError(t, err)
	require.Len(t, dataset.Examples, 1)

	expected := dataset.Examples[0].Expected.Conversation
	require.Contains(t, expected[2].Content, "[ERROR] file not found")
}

func TestCaptureSession_MultipleToolCalls(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{
		newTextMessage("m1", "s1", message.User, "Read both files"),
		{
			ID: "m2", SessionID: "s1", Role: message.Assistant,
			Parts: []message.ContentPart{
				message.ToolCall{ID: "tc1", Name: "view", Input: `{"path":"a.go"}`, Finished: true},
			},
		},
		{
			ID: "m3", SessionID: "s1", Role: message.Tool,
			Parts: []message.ContentPart{
				message.ToolResult{ToolCallID: "tc1", Name: "view", Content: "package a"},
			},
		},
		{
			ID: "m4", SessionID: "s1", Role: message.Assistant,
			Parts: []message.ContentPart{
				message.ToolCall{ID: "tc2", Name: "view", Input: `{"path":"b.go"}`, Finished: true},
			},
		},
		{
			ID: "m5", SessionID: "s1", Role: message.Tool,
			Parts: []message.ContentPart{
				message.ToolResult{ToolCallID: "tc2", Name: "view", Content: "package b"},
			},
		},
		newTextMessage("m6", "s1", message.Assistant, "Both files read."),
	}

	dataset, err := CaptureSession(context.Background(), "s1", msgs)
	require.NoError(t, err)
	require.Len(t, dataset.Examples, 1)

	expected := dataset.Examples[0].Expected.Conversation
	require.Len(t, expected, 6) // user + tool_call + result + tool_call + result + assistant
}

func TestWriteCaptureDataset(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "dataset.json")

	dataset := &Dataset{
		Name:    "test dataset",
		Version: "2024-01-01",
		Examples: []DatasetExample{
			{
				ID:   "ex1",
				Name: "Example 1",
				Input: &EvalInput{
					SessionID:    "s1",
					Conversation: []Message{{Role: "user", Content: "hello"}},
				},
				Expected: &EvalInput{
					SessionID:    "s1",
					Conversation: []Message{{Role: "assistant", Content: "hi"}},
				},
			},
		},
	}

	err := WriteCaptureDataset(dataset, outputPath)
	require.NoError(t, err)

	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var loaded Dataset
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	require.Equal(t, "test dataset", loaded.Name)
	require.Len(t, loaded.Examples, 1)
	require.Equal(t, "hello", loaded.Examples[0].Input.Conversation[0].Content)
}

func TestWriteCaptureDataset_CompatibleWithLoadDataset(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "dataset.json")

	dataset := &Dataset{
		Name:    "compat test",
		Version: "2024-01-01",
		Examples: []DatasetExample{
			{
				ID:   "ex1",
				Name: "Example 1",
				Input: &EvalInput{
					SessionID:    "s1",
					Conversation: []Message{{Role: "user", Content: "hello"}},
				},
				Expected: &EvalInput{
					SessionID:    "s1",
					Conversation: []Message{{Role: "assistant", Content: "hi there"}},
				},
			},
		},
	}

	err := WriteCaptureDataset(dataset, outputPath)
	require.NoError(t, err)

	loaded, err := LoadDataset(outputPath)
	require.NoError(t, err)
	require.Equal(t, dataset.Name, loaded.Name)
	require.Len(t, loaded.Examples, 1)
	require.Equal(t, dataset.Examples[0].ID, loaded.Examples[0].ID)
}

func TestCaptureSession_ExampleIDs(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{
		newTextMessage("m1", "my-session", message.User, "Hello"),
		newTextMessage("m2", "my-session", message.Assistant, "Hi"),
		newTextMessage("m3", "my-session", message.User, "Bye"),
		newTextMessage("m4", "my-session", message.Assistant, "Goodbye"),
	}

	dataset, err := CaptureSession(context.Background(), "my-session", msgs)
	require.NoError(t, err)
	require.Equal(t, "my-session_turn_0", dataset.Examples[0].ID)
	require.Equal(t, "my-session_turn_1", dataset.Examples[1].ID)
}

func newTextMessage(id, sessionID string, role message.MessageRole, text string) message.Message {
	return message.Message{
		ID:        id,
		SessionID: sessionID,
		Role:      role,
		Parts:     []message.ContentPart{message.TextContent{Text: text}},
	}
}
