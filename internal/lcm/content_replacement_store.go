package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/crush/internal/db"
)

type contentReplacementStore struct {
	q       db.Querier
	queries *db.Queries
	rawDB   *sql.DB
}

func newContentReplacementStore(queries *db.Queries, rawDB *sql.DB) *contentReplacementStore {
	return &contentReplacementStore{q: queries, queries: queries, rawDB: rawDB}
}

func (s *contentReplacementStore) RecordReplacement(ctx context.Context, r ContentReplacement) (int64, error) {
	id, err := s.q.RecordContentReplacement(ctx, db.RecordContentReplacementParams{
		SessionID:             r.SessionID,
		Position:              r.Position,
		MessageID:             r.MessageID,
		FileID:                r.FileID,
		State:                 string(r.State),
		Round:                 int64(r.Round),
		OriginalTokenCount:    int64(r.OriginalTokenCount),
		ReplacementTokenCount: int64(r.ReplacementTokenCount),
	})
	if err != nil {
		slog.Warn("Failed to record content replacement",
			slog.String("session_id", r.SessionID),
			slog.Int64("position", r.Position),
			slog.String("error", err.Error()),
		)
		return 0, fmt.Errorf("recording content replacement: %w", err)
	}
	return id, nil
}

func (s *contentReplacementStore) GetBySessionPosition(ctx context.Context, sessionID string, position int64) ([]ContentReplacement, error) {
	rows, err := s.q.GetContentReplacementsBySessionPosition(ctx, db.GetContentReplacementsBySessionPositionParams{
		SessionID: sessionID,
		Position:  position,
	})
	if err != nil {
		return nil, fmt.Errorf("getting replacements by session position: %w", err)
	}
	return convertDBReplacements(rows), nil
}

func (s *contentReplacementStore) GetByFileID(ctx context.Context, sessionID string, fileID string) ([]ContentReplacement, error) {
	rows, err := s.q.GetContentReplacementsByFileID(ctx, db.GetContentReplacementsByFileIDParams{
		SessionID: sessionID,
		FileID:    sql.NullString{String: fileID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("getting replacements by file ID: %w", err)
	}
	return convertDBReplacements(rows), nil
}

func (s *contentReplacementStore) ListByState(ctx context.Context, sessionID string, state ReplacementState) ([]ContentReplacement, error) {
	rows, err := s.q.ListContentReplacementsByState(ctx, db.ListContentReplacementsByStateParams{
		SessionID: sessionID,
		State:     string(state),
	})
	if err != nil {
		return nil, fmt.Errorf("listing replacements by state: %w", err)
	}
	return convertDBReplacements(rows), nil
}

func (s *contentReplacementStore) UpdateState(ctx context.Context, id int64, newState ReplacementState) error {
	current, err := s.q.GetContentReplacement(ctx, id)
	if err != nil {
		return fmt.Errorf("getting replacement %d: %w", id, err)
	}

	if err := ValidateTransition(ReplacementState(current.State), newState); err != nil {
		return err
	}

	return s.q.UpdateContentReplacementState(ctx, db.UpdateContentReplacementStateParams{
		State: string(newState),
		ID:    id,
	})
}

func (s *contentReplacementStore) ListByRound(ctx context.Context, sessionID string, round int) ([]ContentReplacement, error) {
	rows, err := s.q.ListContentReplacementsByRound(ctx, db.ListContentReplacementsByRoundParams{
		SessionID: sessionID,
		Round:     int64(round),
	})
	if err != nil {
		return nil, fmt.Errorf("listing replacements by round: %w", err)
	}
	return convertDBReplacements(rows), nil
}

func convertDBReplacements(rows []db.LcmContentReplacement) []ContentReplacement {
	result := make([]ContentReplacement, len(rows))
	for i, r := range rows {
		result[i] = ContentReplacement{
			ID:                    r.ID,
			SessionID:             r.SessionID,
			Position:              r.Position,
			MessageID:             r.MessageID,
			FileID:                r.FileID,
			State:                 ReplacementState(r.State),
			Round:                 int(r.Round),
			OriginalTokenCount:    int(r.OriginalTokenCount),
			ReplacementTokenCount: int(r.ReplacementTokenCount),
			CreatedAt:             r.CreatedAt,
			UpdatedAt:             r.UpdatedAt,
		}
	}
	return result
}
