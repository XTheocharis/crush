package message

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func makeTestAttachments(n int, contentSize int) []Attachment {
	attachments := make([]Attachment, n)
	content := []byte(strings.Repeat("x", contentSize))
	for i := range n {
		attachments[i] = Attachment{
			FilePath: fmt.Sprintf("/path/to/file%d.txt", i),
			MimeType: "text/plain",
			Content:  content,
		}
	}
	return attachments
}

func TestMessageCloneDeepCopy_BinaryContent(t *testing.T) {
	t.Parallel()

	original := Message{
		Role: User,
		Parts: []ContentPart{
			TextContent{Text: "hello"},
			BinaryContent{Path: "file.bin", MIMEType: "application/octet-stream", Data: []byte{0xDE, 0xAD, 0xBE, 0xEF}},
			TextContent{Text: "world"},
		},
	}

	cloned := original.Clone()

	require.Equal(t, original.Parts, cloned.Parts)

	bc, ok := cloned.Parts[1].(BinaryContent)
	require.True(t, ok)
	bc.Data[0] = 0xFF

	origBC, ok := original.Parts[1].(BinaryContent)
	require.True(t, ok)
	require.Equal(t, byte(0xDE), origBC.Data[0], "modifying clone should not affect original")
}

func TestMessageCloneDeepCopy_NilBinaryData(t *testing.T) {
	t.Parallel()

	original := Message{
		Role: User,
		Parts: []ContentPart{
			BinaryContent{Path: "empty.bin", MIMEType: "application/octet-stream", Data: nil},
		},
	}

	cloned := original.Clone()

	bc, ok := cloned.Parts[0].(BinaryContent)
	require.True(t, ok)
	require.Nil(t, bc.Data)
}

func TestMessageCloneDeepCopy_OtherPartTypes(t *testing.T) {
	t.Parallel()

	original := Message{
		Role: Assistant,
		Parts: []ContentPart{
			TextContent{Text: "text"},
			ToolCall{ID: "tc1", Name: "bash", Input: `{"cmd":"ls"}`},
			ToolResult{ToolCallID: "tc1", Name: "bash", Content: "file.go"},
			Finish{Reason: FinishReasonEndTurn, Time: 12345},
		},
	}

	cloned := original.Clone()

	require.Equal(t, original.Parts, cloned.Parts)
	require.Equal(t, len(original.Parts), len(cloned.Parts))
}

func TestToAIMessage_CorruptedMediaData(t *testing.T) {
	t.Parallel()

	msg := &Message{
		Role: Tool,
		Parts: []ContentPart{
			ToolResult{
				ToolCallID: "call_123",
				Name:       "screenshot",
				Content:    "Loaded image/png content",
				Data:       "abc\x80def",
				MIMEType:   "image/png",
			},
		},
	}

	messages := msg.ToAIMessage()
	require.Len(t, messages, 1)
	require.Len(t, messages[0].Content, 1)

	part, ok := messages[0].Content[0].(fantasy.ToolResultPart)
	require.True(t, ok)

	require.Equal(t, "call_123", part.ToolCallID)

	textContent, ok := part.Output.(fantasy.ToolResultOutputContentText)
	require.True(t, ok, "corrupted media should be downgraded to text")
	require.Equal(t, mediaLoadFailedPlaceholder, textContent.Text)
}

func TestToAIMessage_ValidMediaData(t *testing.T) {
	t.Parallel()

	validBase64 := base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4E, 0x47})

	msg := &Message{
		Role: Tool,
		Parts: []ContentPart{
			ToolResult{
				ToolCallID: "call_456",
				Name:       "screenshot",
				Content:    "Loaded image/png content",
				Data:       validBase64,
				MIMEType:   "image/png",
			},
		},
	}

	messages := msg.ToAIMessage()
	require.Len(t, messages, 1)
	require.Len(t, messages[0].Content, 1)

	part, ok := messages[0].Content[0].(fantasy.ToolResultPart)
	require.True(t, ok)

	require.Equal(t, "call_456", part.ToolCallID)

	mediaContent, ok := part.Output.(fantasy.ToolResultOutputContentMedia)
	require.True(t, ok, "valid media should remain as media")
	require.Equal(t, validBase64, mediaContent.Data)
	require.Equal(t, "image/png", mediaContent.MediaType)
}

func TestToAIMessage_ASCIIButInvalidBase64(t *testing.T) {
	t.Parallel()

	msg := &Message{
		Role: Tool,
		Parts: []ContentPart{
			ToolResult{
				ToolCallID: "call_789",
				Name:       "screenshot",
				Content:    "Loaded image/png content",
				Data:       "not-valid-base64!!!",
				MIMEType:   "image/png",
			},
		},
	}

	messages := msg.ToAIMessage()
	require.Len(t, messages, 1)
	require.Len(t, messages[0].Content, 1)

	part, ok := messages[0].Content[0].(fantasy.ToolResultPart)
	require.True(t, ok)

	require.Equal(t, "call_789", part.ToolCallID)

	textContent, ok := part.Output.(fantasy.ToolResultOutputContentText)
	require.True(t, ok, "ASCII but invalid base64 should be downgraded to text")
	require.Equal(t, mediaLoadFailedPlaceholder, textContent.Text)
}

func BenchmarkPromptWithTextAttachments(b *testing.B) {
	cases := []struct {
		name        string
		numFiles    int
		contentSize int
	}{
		{"1file_100bytes", 1, 100},
		{"5files_1KB", 5, 1024},
		{"10files_10KB", 10, 10 * 1024},
		{"20files_50KB", 20, 50 * 1024},
	}

	for _, tc := range cases {
		attachments := makeTestAttachments(tc.numFiles, tc.contentSize)
		prompt := "Process these files"

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_ = PromptWithTextAttachments(prompt, attachments)
			}
		})
	}
}
