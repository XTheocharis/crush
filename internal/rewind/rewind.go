package rewind

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/charmbracelet/crush/internal/db"
)

const maxSeqBound = 999999

type rewinder struct {
	q              db.Querier
	workingDir     string
	snapshots      Snapshotter
	postRewindHook PostRewindHook
}

// WithPostRewindHook sets a callback that runs after messages are deleted
// during rewind. Hook errors are logged but do not fail the rewind.
func WithPostRewindHook(h PostRewindHook) RewinderOption {
	return func(r *rewinder) { r.postRewindHook = h }
}

// NewRewinder creates a new Rewinder backed by the given db.Querier,
// Snapshotter, and working directory.
func NewRewinder(q db.Querier, snapshots Snapshotter, workingDir string, opts ...RewinderOption) Rewinder {
	r := &rewinder{
		q:          q,
		workingDir: workingDir,
		snapshots:  snapshots,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Rewind reverts the session to the state at the given sequence number,
// according to the specified mode.
func (r *rewinder) Rewind(ctx context.Context, sessionID string, seq int, mode RewindMode) (*RewindResult, error) {
	switch mode {
	case RewindCodeOnly:
		return r.rewindCode(ctx, sessionID, seq)
	case RewindConvoOnly:
		return r.rewindConvo(ctx, sessionID, seq)
	case RewindBoth:
		return r.rewindBoth(ctx, sessionID, seq)
	default:
		return nil, fmt.Errorf("unknown rewind mode: %d", mode)
	}
}

func (r *rewinder) rewindCode(ctx context.Context, sessionID string, seq int) (*RewindResult, error) {
	snap, err := r.snapshots.GetSnapshotAtOrBeforeSeq(ctx, sessionID, seq)
	if err != nil {
		return nil, fmt.Errorf("getting snapshot for rewind: %w", err)
	}

	files, err := r.snapshots.GetSnapshotFiles(ctx, snap.ID)
	if err != nil {
		return nil, fmt.Errorf("getting snapshot files: %w", err)
	}

	restored := 0
	for _, f := range files {
		fullPath := filepath.Join(r.workingDir, f.Path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating directory %s: %w", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(f.Content), 0o644); err != nil {
			return nil, fmt.Errorf("writing file %s: %w", fullPath, err)
		}
		restored++
	}

	var extractedText string
	msg, err := r.q.GetMessageBySessionAndSeq(ctx, db.GetMessageBySessionAndSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq),
	})
	if err == nil {
		extractedText, _ = extractTextFromParts(msg.Parts)
	}

	return &RewindResult{
		FilesRestored: restored,
		Snapshot:      snap,
		ExtractedText: extractedText,
	}, nil
}

func (r *rewinder) rewindConvo(ctx context.Context, sessionID string, seq int) (*RewindResult, error) {
	msg, err := r.q.GetMessageBySessionAndSeq(ctx, db.GetMessageBySessionAndSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq),
	})
	var extractedText string
	if err == nil {
		extractedText, _ = extractTextFromParts(msg.Parts)
	}

	toDelete, err := r.q.ListMessagesInSeqRange(ctx, db.ListMessagesInSeqRangeParams{
		SessionID: sessionID,
		Seq:       int64(seq),
		Seq_2:     maxSeqBound,
	})
	if err != nil {
		return nil, fmt.Errorf("counting messages in range: %w", err)
	}

	// Use seq-1 to remove the user message at seq, the assistant response at
	// seq+1, and all subsequent messages.
	if err := r.q.DeleteMessagesAfterSeq(ctx, db.DeleteMessagesAfterSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq - 1),
	}); err != nil {
		return nil, fmt.Errorf("deleting messages after seq %d: %w", seq-1, err)
	}

	if err := r.snapshots.DeleteSnapshotsAfterSeq(ctx, sessionID, seq-1); err != nil {
		return nil, fmt.Errorf("deleting snapshots after seq %d: %w", seq-1, err)
	}

	r.runPostRewindHook(ctx, sessionID)

	return &RewindResult{
		MessagesDeleted: len(toDelete),
		ExtractedText:   extractedText,
	}, nil
}

func (r *rewinder) rewindBoth(ctx context.Context, sessionID string, seq int) (*RewindResult, error) {
	codeResult, err := r.rewindCode(ctx, sessionID, seq)
	if err != nil {
		return nil, err
	}

	convoResult, err := r.rewindConvo(ctx, sessionID, seq)
	if err != nil {
		return nil, err
	}

	return &RewindResult{
		FilesRestored:   codeResult.FilesRestored,
		MessagesDeleted: convoResult.MessagesDeleted,
		Snapshot:        codeResult.Snapshot,
		ExtractedText:   convoResult.ExtractedText,
	}, nil
}

func (r *rewinder) runPostRewindHook(ctx context.Context, sessionID string) {
	if r.postRewindHook == nil {
		return
	}
	if err := r.postRewindHook(ctx, sessionID); err != nil {
		slog.Error("Post rewind hook failed", "sessionID", sessionID, "error", err)
	}
}
