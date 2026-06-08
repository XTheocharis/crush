package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/crush/internal/message"
)

// CaptureMessage is a simplified message for capture output, compatible with
// the eval.Message type but with additional tool call context.
type CaptureMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CaptureToolCall represents a tool call extracted from an assistant message.
type CaptureToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"`
}

// CaptureToolResult represents a tool result from a tool message.
type CaptureToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

// CaptureTurn holds a single user-assistant exchange, including any tool
// calls and results in between.
type CaptureTurn struct {
	Index       int                 `json:"index"`
	Input       string              `json:"input"`
	Output      string              `json:"output"`
	ToolCalls   []CaptureToolCall   `json:"tool_calls,omitempty"`
	ToolResults []CaptureToolResult `json:"tool_results,omitempty"`
	Files       map[string]string   `json:"files,omitempty"`
}

// CaptureSession reads all messages for a session and converts them into a
// Dataset suitable for evaluation. It pairs user messages with the following
// assistant response, collecting any tool calls and results in between.
func CaptureSession(ctx context.Context, sessionID string, msgs []message.Message) (*Dataset, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages found for session %s", sessionID)
	}

	var filtered []message.Message
	for _, msg := range msgs {
		if msg.IsSummaryMessage {
			continue
		}
		if msg.Role == message.System {
			continue
		}
		filtered = append(filtered, msg)
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no usable messages found for session %s", sessionID)
	}

	turns := pairTurns(filtered)
	if len(turns) == 0 {
		return nil, fmt.Errorf("no complete user-assistant pairs found for session %s", sessionID)
	}

	examples := make([]DatasetExample, len(turns))
	for i, turn := range turns {
		examples[i] = DatasetExample{
			ID:   fmt.Sprintf("%s_turn_%d", sessionID, i),
			Name: fmt.Sprintf("Session %s turn %d", sessionID, i),
			Input: &EvalInput{
				SessionID: sessionID,
				Conversation: []Message{
					{Role: "user", Content: turn.Input},
				},
				Files: turn.Files,
			},
			Expected: &EvalInput{
				SessionID:    sessionID,
				Conversation: buildExpectedConversation(turn),
				Files:        turn.Files,
			},
		}
	}

	return &Dataset{
		Name:     fmt.Sprintf("Capture of session %s", sessionID),
		Version:  time.Now().Format("2006-01-02"),
		Examples: examples,
	}, nil
}

// WriteCaptureDataset writes a Dataset to the given output path as formatted
// JSON.
func WriteCaptureDataset(dataset *Dataset, outputPath string) error {
	data, err := json.MarshalIndent(dataset, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dataset: %w", err)
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("write dataset file: %w", err)
	}
	return nil
}

// pairTurns groups messages into user-assistant turns, collecting tool calls
// and results that appear between them.
func pairTurns(msgs []message.Message) []CaptureTurn {
	var turns []CaptureTurn

	i := 0
	for i < len(msgs) {
		if msgs[i].Role != message.User {
			i++
			continue
		}

		userContent := msgs[i].Content().Text
		i++

		var toolCalls []CaptureToolCall
		var toolResults []CaptureToolResult
		assistantContent := ""

		for i < len(msgs) {
			msg := msgs[i]

			switch msg.Role {
			case message.Assistant:
				for _, tc := range msg.ToolCalls() {
					toolCalls = append(toolCalls, CaptureToolCall{
						ID:    tc.ID,
						Name:  tc.Name,
						Input: tc.Input,
					})
				}
				if text := msg.Content().Text; text != "" {
					assistantContent = text
					i++
					goto nextTurn
				}
			case message.Tool:
				for _, tr := range msg.ToolResults() {
					toolResults = append(toolResults, CaptureToolResult{
						ToolCallID: tr.ToolCallID,
						Name:       tr.Name,
						Content:    tr.Content,
						IsError:    tr.IsError,
					})
				}
			}
			i++
		}

	nextTurn:
		if assistantContent != "" {
			turn := CaptureTurn{
				Index:       len(turns),
				Input:       userContent,
				Output:      assistantContent,
				ToolCalls:   toolCalls,
				ToolResults: toolResults,
			}
			turns = append(turns, turn)
		}
	}

	return turns
}

// buildExpectedConversation creates the expected conversation for an
// EvalInput, including the assistant response and any tool calls/results.
func buildExpectedConversation(turn CaptureTurn) []Message {
	msgs := []Message{
		{Role: "user", Content: turn.Input},
	}

	toolCallCount := 0
	for _, tc := range turn.ToolCalls {
		msgs = append(msgs, Message{
			Role:    "assistant",
			Content: fmt.Sprintf("[tool_call] %s(%s)", tc.Name, tc.Input),
		})

		for _, tr := range turn.ToolResults {
			if tr.ToolCallID == tc.ID {
				content := tr.Content
				if tr.IsError {
					content = "[ERROR] " + content
				}
				msgs = append(msgs, Message{
					Role:    "tool",
					Content: content,
				})
				break
			}
		}
		toolCallCount++
	}

	msgs = append(msgs, Message{
		Role:    "assistant",
		Content: turn.Output,
	})

	return msgs
}
