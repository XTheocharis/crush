package rewind

import (
	"context"
	"fmt"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/google/uuid"
)

type forker struct {
	q        db.Querier
	sessions session.Service
}

func NewForker(q db.Querier, sessions session.Service) Forker {
	return &forker{q: q, sessions: sessions}
}

func (f *forker) Fork(ctx context.Context, sessionID string, seq int) (*ForkResult, error) {
	orig, err := f.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get original session: %w", err)
	}

	title := orig.Title + " (fork)"
	sessions, err := f.sessions.List(ctx)
	if err == nil {
		prefix := orig.Title + " (fork"
		count := 0
		for _, s := range sessions {
			if len(s.Title) >= len(prefix) && s.Title[:len(prefix)] == prefix {
				count++
			}
		}
		if count > 0 {
			title = fmt.Sprintf("%s (fork #%d)", orig.Title, count+1)
		}
	}

	newSession, err := f.sessions.Create(ctx, title)
	if err != nil {
		return nil, fmt.Errorf("create forked session: %w", err)
	}

	newSession.ParentSessionID = orig.ID
	newSession, err = f.sessions.Save(ctx, newSession)
	if err != nil {
		return nil, fmt.Errorf("set parent session id: %w", err)
	}

	clonePrefix := "fork-" + uuid.NewString() + "-"

	err = f.q.CloneSessionMessages(ctx, db.CloneSessionMessagesParams{
		ID:          clonePrefix,
		SessionID:   newSession.ID,
		SessionID_2: newSession.ID,
		SessionID_3: orig.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("clone session messages: %w", err)
	}

	err = f.q.CloneSessionFiles(ctx, db.CloneSessionFilesParams{
		ID:          clonePrefix,
		SessionID:   newSession.ID,
		SessionID_2: orig.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("clone session files: %w", err)
	}

	err = f.q.DeleteMessagesAfterSeq(ctx, db.DeleteMessagesAfterSeqParams{
		SessionID: newSession.ID,
		Seq:       int64(seq),
	})
	if err != nil {
		return nil, fmt.Errorf("trim messages after seq: %w", err)
	}

	return &ForkResult{
		NewSessionID:    newSession.ID,
		NewSessionTitle: title,
		MessagesCloned:  seq,
	}, nil
}
