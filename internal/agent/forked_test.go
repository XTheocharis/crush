package agent

import (
	"testing"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/stretchr/testify/require"
)

func TestDeepCopyMessageBinaryContent(t *testing.T) {
	t.Parallel()

	msg := message.Message{
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "see attached"},
			message.BinaryContent{
				Path:     "/tmp/data.bin",
				MIMEType: "application/octet-stream",
				Data:     []byte{0x01, 0x02, 0x03, 0x04},
			},
		},
	}

	copied := deepCopyMessage(msg)

	require.Equal(t, msg.Parts, copied.Parts)

	bc, ok := copied.Parts[1].(message.BinaryContent)
	require.True(t, ok)
	bc.Data[0] = 0xFF

	origBC, ok := msg.Parts[1].(message.BinaryContent)
	require.True(t, ok)
	require.Equal(t, byte(0x01), origBC.Data[0], "modifying deep copy should not affect original")
}

func TestDeepCopyMessageEmptyParts(t *testing.T) {
	t.Parallel()

	msg := message.Message{
		Role:  message.Assistant,
		Parts: nil,
	}

	copied := deepCopyMessage(msg)
	require.Empty(t, copied.Parts)
}

func TestDeepCopyMessageMultipleBinaryParts(t *testing.T) {
	t.Parallel()

	msg := message.Message{
		Role: message.User,
		Parts: []message.ContentPart{
			message.BinaryContent{Path: "a.bin", MIMEType: "application/octet-stream", Data: []byte{0xAA}},
			message.TextContent{Text: "middle"},
			message.BinaryContent{Path: "b.bin", MIMEType: "image/png", Data: []byte{0xBB, 0xCC}},
		},
	}

	copied := deepCopyMessage(msg)

	for i, part := range copied.Parts {
		if bc, ok := part.(message.BinaryContent); ok {
			bc.Data[0] = 0x00

			origBC, ok := msg.Parts[i].(message.BinaryContent)
			require.True(t, ok)
			require.NotEqual(t, byte(0x00), origBC.Data[0], "clone at index %d should be independent", i)
		}
	}
}
