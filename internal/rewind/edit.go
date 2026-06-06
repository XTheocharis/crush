package rewind

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/crush/internal/db"
)

// editorStringParts builds the JSON parts string for a text-only user message.
func editorStringParts(text string) string {
	type pw struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	type td struct {
		Text string `json:"text"`
	}
	data, _ := json.Marshal(td{Text: text})
	// Preserve the finish part that message.Service always appends.
	finishData, _ := json.Marshal(map[string]any{
		"reason":  "stop",
		"time":    float64(0),
		"message": "",
		"details": "",
	})
	parts, _ := json.Marshal([]pw{
		{Type: "text", Data: data},
		{Type: "finish", Data: finishData},
	})
	return string(parts)
}

type editor struct {
	q db.Querier
}

// NewEditor returns an Editor backed by the given Querier.
func NewEditor(q db.Querier) Editor {
	return &editor{q: q}
}

func (e *editor) ExtractMessageText(ctx context.Context, sessionID string, seq int) (*EditResult, error) {
	msg, err := e.q.GetMessageBySessionAndSeq(ctx, db.GetMessageBySessionAndSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq),
	})
	if err != nil {
		return nil, fmt.Errorf("get message at seq %d: %w", seq, err)
	}

	extractedText, err := extractTextFromParts(msg.Parts)
	if err != nil {
		return nil, fmt.Errorf("extract text from parts: %w", err)
	}

	return &EditResult{
		ExtractedText: extractedText,
		NewMessageID:  msg.ID,
	}, nil
}

func (e *editor) UpdateMessageText(ctx context.Context, sessionID string, seq int, newText string) error {
	msg, err := e.q.GetMessageBySessionAndSeq(ctx, db.GetMessageBySessionAndSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq),
	})
	if err != nil {
		return fmt.Errorf("get message at seq %d: %w", seq, err)
	}

	parts := editorStringParts(newText)

	return e.q.UpdateMessage(ctx, db.UpdateMessageParams{
		Parts: parts,
		ID:    msg.ID,
	})
}

// partWrapper matches the JSON shape stored in db.Message.Parts.
type partWrapper struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func extractTextFromParts(partsJSON string) (string, error) {
	if partsJSON == "" {
		return "", nil
	}

	var wrappers []partWrapper
	if err := json.Unmarshal([]byte(partsJSON), &wrappers); err != nil {
		return "", fmt.Errorf("unmarshal parts: %w", err)
	}

	var texts []string
	for _, w := range wrappers {
		if w.Type != "text" {
			continue
		}
		var data struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(w.Data, &data); err != nil {
			continue
		}
		if data.Text != "" {
			texts = append(texts, data.Text)
		}
	}

	return strings.Join(texts, ""), nil
}
