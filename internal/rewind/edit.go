package rewind

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/crush/internal/db"
)

const maxSeqBound = 999999

type editor struct {
	q db.Querier
}

// NewEditor returns an Editor backed by the given Querier.
func NewEditor(q db.Querier) Editor {
	return &editor{q: q}
}

func (e *editor) EditMessage(ctx context.Context, sessionID string, seq int) (*EditResult, error) {
	msg, err := e.q.GetMessageBySessionAndSeq(ctx, db.GetMessageBySessionAndSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq),
	})
	if err != nil {
		return nil, fmt.Errorf("get message at seq %d: %w", seq, err)
	}

	if msg.Role != "user" {
		return nil, fmt.Errorf("not a user message: role=%q at seq %d", msg.Role, seq)
	}

	extractedText, err := extractTextFromParts(msg.Parts)
	if err != nil {
		return nil, fmt.Errorf("extract text from parts: %w", err)
	}

	toDelete, err := e.q.ListMessagesInSeqRange(ctx, db.ListMessagesInSeqRangeParams{
		SessionID: sessionID,
		Seq:       int64(seq),
		Seq_2:     maxSeqBound,
	})
	if err != nil {
		return nil, fmt.Errorf("count messages in range: %w", err)
	}

	err = e.q.DeleteMessagesAfterSeq(ctx, db.DeleteMessagesAfterSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq - 1),
	})
	if err != nil {
		return nil, fmt.Errorf("delete messages after seq %d: %w", seq, err)
	}

	return &EditResult{
		ExtractedText:   extractedText,
		MessagesDeleted: len(toDelete),
		NewMessageID:    msg.ID,
	}, nil
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
