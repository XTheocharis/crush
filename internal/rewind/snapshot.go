package rewind

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/google/uuid"
)

const defaultMaxPerSession = 50

type snapshotter struct {
	q             db.Querier
	maxPerSession int
	workingDir    string
}

// SnapshotterOption configures a snapshotter.
type SnapshotterOption func(*snapshotter)

// WithMaxPerSession sets the maximum number of snapshots to retain per session.
func WithMaxPerSession(n int) SnapshotterOption {
	return func(s *snapshotter) { s.maxPerSession = n }
}

// NewSnapshotter creates a new Snapshotter backed by the given Querier.
func NewSnapshotter(q db.Querier, opts ...SnapshotterOption) Snapshotter {
	s := &snapshotter{
		q:             q,
		maxPerSession: defaultMaxPerSession,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *snapshotter) CaptureSnapshot(ctx context.Context, sessionID string, userMessageSeq int) error {
	slog.Debug("Rewind: CaptureSnapshot invoked",
		"session_id", sessionID,
		"user_message_seq", userMessageSeq,
	)

	msg, err := s.q.GetLatestUserMessage(ctx, sessionID)
	if err != nil {
		slog.Debug("Rewind: CaptureSnapshot failed to get latest user message",
			"session_id", sessionID,
			"error", err,
		)
		return fmt.Errorf("getting latest user message: %w", err)
	}

	snapshot, err := s.q.CreateTurnSnapshot(ctx, db.CreateTurnSnapshotParams{
		ID:             uuid.NewString(),
		SessionID:      sessionID,
		UserMessageID:  msg.ID,
		UserMessageSeq: int64(userMessageSeq),
	})
	if err != nil {
		slog.Debug("Rewind: CaptureSnapshot failed to create snapshot",
			"session_id", sessionID,
			"error", err,
		)
		return fmt.Errorf("creating turn snapshot: %w", err)
	}

	files, err := s.q.ListLatestSessionFiles(ctx, sessionID)
	if err != nil {
		slog.Debug("Rewind: CaptureSnapshot failed to list session files",
			"session_id", sessionID,
			"error", err,
		)
		return fmt.Errorf("listing session files: %w", err)
	}

	for _, f := range files {
		if err := s.q.AddSnapshotFile(ctx, db.AddSnapshotFileParams{
			SnapshotID: snapshot.ID,
			FileID:     f.ID,
			Path:       f.Path,
			Version:    f.Version,
		}); err != nil {
			slog.Debug("Rewind: CaptureSnapshot failed to add snapshot file",
				"session_id", sessionID,
				"path", f.Path,
				"error", err,
			)
			return fmt.Errorf("adding snapshot file %s: %w", f.Path, err)
		}
	}

	slog.Debug("Rewind: CaptureSnapshot completed",
		"session_id", sessionID,
		"snapshot_id", snapshot.ID,
		"file_count", len(files),
	)

	return nil
}

func (s *snapshotter) GetSnapshotAtOrBeforeSeq(ctx context.Context, sessionID string, seq int) (*TurnSnapshot, error) {
	ts, err := s.q.GetTurnSnapshotAtOrBeforeSeq(ctx, db.GetTurnSnapshotAtOrBeforeSeqParams{
		SessionID:      sessionID,
		UserMessageSeq: int64(seq),
	})
	if err != nil {
		return nil, fmt.Errorf("querying snapshot at or before seq %d: %w", seq, err)
	}
	return &TurnSnapshot{
		ID:             ts.ID,
		SessionID:      ts.SessionID,
		UserMessageID:  ts.UserMessageID,
		UserMessageSeq: int(ts.UserMessageSeq),
		CreatedAt:      time.Unix(ts.CreatedAt, 0),
	}, nil
}

func (s *snapshotter) GetSnapshotFiles(ctx context.Context, snapshotID string) ([]SnapshotFile, error) {
	rows, err := s.q.ListSnapshotFiles(ctx, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("listing snapshot files: %w", err)
	}

	files := make([]SnapshotFile, len(rows))
	for i, row := range rows {
		files[i] = SnapshotFile{
			FileID:  row.FileID,
			Path:    row.Path,
			Version: strconv.FormatInt(row.Version, 10),
			Content: row.Content,
		}
	}
	return files, nil
}

func (s *snapshotter) DeleteSnapshotsAfterSeq(ctx context.Context, sessionID string, seq int) error {
	if err := s.q.DeleteSnapshotsAfterSeq(ctx, db.DeleteSnapshotsAfterSeqParams{
		SessionID:      sessionID,
		UserMessageSeq: int64(seq),
	}); err != nil {
		return fmt.Errorf("deleting snapshots after seq %d: %w", seq, err)
	}
	return nil
}

func (s *snapshotter) CleanupOldSnapshots(ctx context.Context, sessionID string) error {
	_, err := s.q.DeleteOldTurnSnapshots(ctx, db.DeleteOldTurnSnapshotsParams{
		SessionID:   sessionID,
		Column2:     int64(s.maxPerSession),
		SessionID_2: sessionID,
	})
	if err != nil {
		return fmt.Errorf("cleaning up old snapshots: %w", err)
	}
	return nil
}
