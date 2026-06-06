package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func mpTestOpenDB(t *testing.T) *sql.DB {
	t.Helper()
	testInitGoose(t)

	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("file:%s/test.db?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=recursive_triggers(ON)", tmpDir)
	sqlDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	sqlDB.SetMaxOpenConns(1)

	err = sqlDB.PingContext(context.Background())
	require.NoError(t, err)

	return sqlDB
}

func mpTestSetup(t *testing.T) (*sql.DB, *Queries) {
	t.Helper()
	sqlDB := mpTestOpenDB(t)

	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	return sqlDB, New(sqlDB)
}

func TestMessageParts_InsertAndGetByMessageID(t *testing.T) {
	t.Parallel()
	_, q := mpTestSetup(t)
	ctx := context.Background()

	_, err := q.CreateSession(ctx, CreateSessionParams{
		ID:    "mp-session-1",
		Title: "test",
	})
	require.NoError(t, err)

	_, err = q.CreateMessage(ctx, CreateMessageParams{
		ID:        "mp-msg-1",
		SessionID: "mp-session-1",
		Role:      "user",
		Parts:     "[]",
	})
	require.NoError(t, err)

	part1, err := q.InsertMessagePart(ctx, InsertMessagePartParams{
		PartID:      "part-1",
		MessageID:   "mp-msg-1",
		SessionID:   "mp-session-1",
		PartType:    "text",
		PartIndex:   0,
		ContentJson: `{"text":"hello"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "part-1", part1.PartID)
	require.Equal(t, "text", part1.PartType)

	part2, err := q.InsertMessagePart(ctx, InsertMessagePartParams{
		PartID:      "part-2",
		MessageID:   "mp-msg-1",
		SessionID:   "mp-session-1",
		PartType:    "tool_call",
		PartIndex:   1,
		ContentJson: `{"id":"tc-1","name":"bash","input":"ls"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "part-2", part2.PartID)

	parts, err := q.GetMessagePartsByMessageID(ctx, "mp-msg-1")
	require.NoError(t, err)
	require.Len(t, parts, 2)
	require.Equal(t, int64(0), parts[0].PartIndex)
	require.Equal(t, int64(1), parts[1].PartIndex)
	require.Equal(t, "text", parts[0].PartType)
	require.Equal(t, "tool_call", parts[1].PartType)
}

func TestMessageParts_GetBySessionAndType(t *testing.T) {
	t.Parallel()
	_, q := mpTestSetup(t)
	ctx := context.Background()

	_, err := q.CreateSession(ctx, CreateSessionParams{
		ID:    "mp-session-2",
		Title: "test",
	})
	require.NoError(t, err)

	_, err = q.CreateMessage(ctx, CreateMessageParams{
		ID:        "mp-msg-2",
		SessionID: "mp-session-2",
		Role:      "assistant",
		Parts:     "[]",
	})
	require.NoError(t, err)

	for i, pt := range []string{"text", "reasoning", "tool_result"} {
		_, err = q.InsertMessagePart(ctx, InsertMessagePartParams{
			PartID:      fmt.Sprintf("part-st-%d", i),
			MessageID:   "mp-msg-2",
			SessionID:   "mp-session-2",
			PartType:    pt,
			PartIndex:   int64(i),
			ContentJson: fmt.Sprintf(`{"type":"%s"}`, pt),
		})
		require.NoError(t, err)
	}

	reasoningParts, err := q.GetMessagePartsBySessionAndType(ctx, GetMessagePartsBySessionAndTypeParams{
		SessionID: "mp-session-2",
		PartType:  "reasoning",
	})
	require.NoError(t, err)
	require.Len(t, reasoningParts, 1)
	require.Equal(t, "reasoning", reasoningParts[0].PartType)

	textParts, err := q.GetMessagePartsBySessionAndType(ctx, GetMessagePartsBySessionAndTypeParams{
		SessionID: "mp-session-2",
		PartType:  "text",
	})
	require.NoError(t, err)
	require.Len(t, textParts, 1)
}

func TestMessageParts_CountBySession(t *testing.T) {
	t.Parallel()
	_, q := mpTestSetup(t)
	ctx := context.Background()

	_, err := q.CreateSession(ctx, CreateSessionParams{
		ID:    "mp-session-3",
		Title: "test",
	})
	require.NoError(t, err)

	_, err = q.CreateMessage(ctx, CreateMessageParams{
		ID:        "mp-msg-3",
		SessionID: "mp-session-3",
		Role:      "assistant",
		Parts:     "[]",
	})
	require.NoError(t, err)

	count, err := q.CountMessagePartsBySession(ctx, "mp-session-3")
	require.NoError(t, err)
	require.Equal(t, int64(0), count)

	for i := range 3 {
		_, err = q.InsertMessagePart(ctx, InsertMessagePartParams{
			PartID:      fmt.Sprintf("part-cnt-%d", i),
			MessageID:   "mp-msg-3",
			SessionID:   "mp-session-3",
			PartType:    "text",
			PartIndex:   int64(i),
			ContentJson: "{}",
		})
		require.NoError(t, err)
	}

	count, err = q.CountMessagePartsBySession(ctx, "mp-session-3")
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
}

func TestMessageParts_DeleteByMessageID(t *testing.T) {
	t.Parallel()
	_, q := mpTestSetup(t)
	ctx := context.Background()

	_, err := q.CreateSession(ctx, CreateSessionParams{
		ID:    "mp-session-4",
		Title: "test",
	})
	require.NoError(t, err)

	_, err = q.CreateMessage(ctx, CreateMessageParams{
		ID:        "mp-msg-4",
		SessionID: "mp-session-4",
		Role:      "user",
		Parts:     "[]",
	})
	require.NoError(t, err)

	_, err = q.InsertMessagePart(ctx, InsertMessagePartParams{
		PartID:      "part-del-1",
		MessageID:   "mp-msg-4",
		SessionID:   "mp-session-4",
		PartType:    "text",
		PartIndex:   0,
		ContentJson: "{}",
	})
	require.NoError(t, err)

	parts, err := q.GetMessagePartsByMessageID(ctx, "mp-msg-4")
	require.NoError(t, err)
	require.Len(t, parts, 1)

	err = q.DeleteMessagePartsByMessageID(ctx, "mp-msg-4")
	require.NoError(t, err)

	parts, err = q.GetMessagePartsByMessageID(ctx, "mp-msg-4")
	require.NoError(t, err)
	require.Len(t, parts, 0)
}

func TestMessageParts_CheckConstraint(t *testing.T) {
	t.Parallel()
	_, q := mpTestSetup(t)
	ctx := context.Background()

	_, err := q.CreateSession(ctx, CreateSessionParams{
		ID:    "mp-session-5",
		Title: "test",
	})
	require.NoError(t, err)

	_, err = q.CreateMessage(ctx, CreateMessageParams{
		ID:        "mp-msg-5",
		SessionID: "mp-session-5",
		Role:      "user",
		Parts:     "[]",
	})
	require.NoError(t, err)

	for _, pt := range []string{"text", "reasoning", "tool_call", "tool_result", "finish", "image_url", "binary"} {
		_, err = q.InsertMessagePart(ctx, InsertMessagePartParams{
			PartID:      fmt.Sprintf("part-check-%s", pt),
			MessageID:   "mp-msg-5",
			SessionID:   "mp-session-5",
			PartType:    pt,
			PartIndex:   0,
			ContentJson: "{}",
		})
		require.NoError(t, err, "part_type=%q should be accepted", pt)
	}
}
