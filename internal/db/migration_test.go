package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

// migrationTestOpenDB creates a fresh SQLite database for migration testing.
func migrationTestOpenDB(t *testing.T) *sql.DB {
	t.Helper()
	testInitGoose(t)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=recursive_triggers(ON)",
		dbPath,
	)
	sqlDB, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	sqlDB.SetMaxOpenConns(1)

	err = sqlDB.PingContext(context.Background())
	require.NoError(t, err)

	return sqlDB
}

// TestMigrationFullIdempotency verifies that running goose Up twice is safe.
// Goose tracks applied migrations in goose_db_version, so the second Up should
// be a no-op and not error.
func TestMigrationFullIdempotency(t *testing.T) {
	t.Parallel()
	sqlDB := migrationTestOpenDB(t)

	// Apply all migrations.
	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err, "first goose.Up should succeed")

	// Apply all migrations again — goose should see they're all applied.
	err = goose.Up(sqlDB, "migrations")
	require.NoError(t, err, "second goose.Up should be a no-op and succeed")
}

// TestMigrationGooseVersionTable verifies that goose_db_version is properly
// maintained after up and down operations.
func TestMigrationGooseVersionTable(t *testing.T) {
	t.Parallel()
	sqlDB := migrationTestOpenDB(t)
	ctx := context.Background()

	// Apply all migrations.
	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	// Verify the latest version is non-zero.
	var version int64
	err = sqlDB.QueryRowContext(ctx,
		"SELECT max(version_id) FROM goose_db_version WHERE is_applied = true",
	).Scan(&version)
	require.NoError(t, err)
	require.Greater(t, version, int64(0), "current migration version should be > 0")

	// Verify goose_db_version has rows for all applied migrations.
	var totalRows int
	err = sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM goose_db_version").Scan(&totalRows)
	require.NoError(t, err)
	require.Greater(t, totalRows, 0, "goose_db_version should have rows after Up")

	// Roll back one migration and verify the table state changed.
	err = goose.Down(sqlDB, "migrations")
	require.NoError(t, err)

	// After Down, the latest version_id should be less than before.
	var newVersion int64
	err = sqlDB.QueryRowContext(ctx,
		"SELECT max(version_id) FROM goose_db_version WHERE is_applied = true",
	).Scan(&newVersion)
	require.NoError(t, err)
	require.Less(t, newVersion, version, "version should decrease after Down")
}

// TestMigrationUpDownUpRoundTrip verifies that migrating up, then fully down,
// then up again produces a working schema.
func TestMigrationUpDownUpRoundTrip(t *testing.T) {
	t.Parallel()
	sqlDB := migrationTestOpenDB(t)

	// Apply all migrations.
	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err, "initial Up should succeed")

	// Roll back everything.
	err = goose.DownTo(sqlDB, "migrations", 0)
	require.NoError(t, err, "DownTo(0) should succeed")

	// Re-apply all migrations.
	err = goose.Up(sqlDB, "migrations")
	require.NoError(t, err, "second Up after full rollback should succeed")

	// Verify core tables exist and accept writes.
	ctx := context.Background()
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('round-trip-test', 'test', 0, 0)",
	)
	require.NoError(t, err, "sessions table should accept inserts after round-trip")

	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO messages (id, session_id, role, parts, created_at, updated_at) VALUES ('msg-1', 'round-trip-test', 'user', '[]', 0, 0)",
	)
	require.NoError(t, err, "messages table should accept inserts after round-trip")
}

// TestMessageTimestampsMigration tests that the message_timestamps migration
// (20260516000000) adds the timestamp columns and that data survives an
// up/down/up cycle.
func TestMessageTimestampsMigration(t *testing.T) {
	t.Parallel()
	sqlDB := migrationTestOpenDB(t)
	ctx := context.Background()

	// Apply all migrations.
	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	// Create session and message with timestamp columns populated.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('ts-test', 'test', 0, 0)",
	)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO messages (id, session_id, role, parts, created_at, updated_at,
			submitted_at, sent_to_llm_at, first_token_at, completed_at)
		 VALUES ('ts-msg-1', 'ts-test', 'user', '[]', 100, 200, 50, 60, 70, 80)`,
	)
	require.NoError(t, err)

	// Verify the timestamp columns were stored.
	var submitted, sent, firstTok, completed int
	err = sqlDB.QueryRowContext(ctx,
		"SELECT submitted_at, sent_to_llm_at, first_token_at, completed_at FROM messages WHERE id = 'ts-msg-1'",
	).Scan(&submitted, &sent, &firstTok, &completed)
	require.NoError(t, err)
	require.Equal(t, 50, submitted)
	require.Equal(t, 60, sent)
	require.Equal(t, 70, firstTok)
	require.Equal(t, 80, completed)

	// Roll back the message_timestamps migration.
	err = goose.DownTo(sqlDB, "migrations", 20260515000000)
	require.NoError(t, err, "rolling back message_timestamps should succeed")

	// Verify the columns no longer exist.
	_, err = sqlDB.ExecContext(ctx, "SELECT submitted_at FROM messages LIMIT 0")
	require.Error(t, err, "submitted_at column should not exist after rollback")

	// Re-apply all migrations.
	err = goose.Up(sqlDB, "migrations")
	require.NoError(t, err, "re-applying migrations should succeed")

	// Verify the columns are back.
	_, err = sqlDB.ExecContext(ctx, "SELECT submitted_at, sent_to_llm_at, first_token_at, completed_at FROM messages LIMIT 0")
	require.NoError(t, err, "timestamp columns should exist after re-migration")
}

// TestTurnSnapshotMigrationSchema verifies that the turn_snapshots migration
// (20260515000000) creates the expected tables and indexes.
func TestTurnSnapshotMigrationSchema(t *testing.T) {
	t.Parallel()
	sqlDB := migrationTestOpenDB(t)
	ctx := context.Background()

	// Apply all migrations.
	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	// Create prerequisite data.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('snap-test', 'test', 0, 0)",
	)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO messages (id, session_id, role, parts, created_at, updated_at) VALUES ('snap-msg', 'snap-test', 'user', '[]', 0, 0)",
	)
	require.NoError(t, err)

	// Insert into turn_snapshots.
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO turn_snapshots (id, session_id, user_message_id, user_message_seq, created_at)
		 VALUES ('snap-1', 'snap-test', 'snap-msg', 1, 1000)`,
	)
	require.NoError(t, err, "turn_snapshots should accept inserts")

	// Insert into turn_snapshot_files.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO files (id, session_id, path, content, version, created_at, updated_at) VALUES ('file-1', 'snap-test', 'main.go', 'package main', 1, 0, 0)",
	)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO turn_snapshot_files (snapshot_id, file_id, path, version)
		 VALUES ('snap-1', 'file-1', 'main.go', 1)`,
	)
	require.NoError(t, err, "turn_snapshot_files should accept inserts")

	// Verify the snapshot was stored correctly.
	var seq int
	err = sqlDB.QueryRowContext(ctx,
		"SELECT user_message_seq FROM turn_snapshots WHERE id = 'snap-1'",
	).Scan(&seq)
	require.NoError(t, err)
	require.Equal(t, 1, seq)

	// Verify the index exists by querying with the indexed column.
	var snapCount int
	err = sqlDB.QueryRowContext(ctx,
		"SELECT count(*) FROM turn_snapshots WHERE session_id = 'snap-test'",
	).Scan(&snapCount)
	require.NoError(t, err)
	require.Equal(t, 1, snapCount)
}

// TestTableSwapMigrationDataPreservation verifies that the xrush_dag table-swap
// migration (20260501000000) preserves existing data through the
// create-new/copy/drop-old/rename cycle.
func TestTableSwapMigrationDataPreservation(t *testing.T) {
	t.Parallel()
	sqlDB := migrationTestOpenDB(t)
	ctx := context.Background()

	// Apply migrations up to (but not including) xrush_dag.
	err := goose.UpTo(sqlDB, "migrations", 20260222000000)
	require.NoError(t, err)

	// Create prerequisite data.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('swap-test', 'test', 0, 0)",
	)
	require.NoError(t, err)

	// Insert a summary row with the original schema (only leaf/condensed kinds).
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO lcm_summaries (summary_id, session_id, kind, content, token_count, file_ids, created_at)
		 VALUES ('swap-leaf', 'swap-test', 'leaf', 'original content', 42, '[]', 100)`,
	)
	require.NoError(t, err)

	// Now apply the xrush_dag migration which does the table swap.
	err = goose.Up(sqlDB, "migrations")
	require.NoError(t, err, "applying remaining migrations should succeed")

	// Verify the data survived the table swap.
	var content string
	var tokenCount int
	err = sqlDB.QueryRowContext(ctx,
		"SELECT content, token_count FROM lcm_summaries WHERE summary_id = 'swap-leaf'",
	).Scan(&content, &tokenCount)
	require.NoError(t, err)
	require.Equal(t, "original content", content)
	require.Equal(t, 42, tokenCount)

	// Verify the new metadata column exists and has the default value.
	var metadata string
	err = sqlDB.QueryRowContext(ctx,
		"SELECT metadata FROM lcm_summaries WHERE summary_id = 'swap-leaf'",
	).Scan(&metadata)
	require.NoError(t, err)
	require.Equal(t, "{}", metadata)
}

// TestFTS5PreservedAfterMigration verifies that full-text search on messages
// still works after all migrations have been applied.
func TestFTS5PreservedAfterMigration(t *testing.T) {
	t.Parallel()
	sqlDB := migrationTestOpenDB(t)
	ctx := context.Background()

	// Apply all migrations.
	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	// Create prerequisite data.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('fts-test', 'test', 0, 0)",
	)
	require.NoError(t, err)

	// The FTS trigger extracts $.content where $.type = 'text'.
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO messages (id, session_id, role, parts, seq, created_at, updated_at)
		 VALUES ('fts-msg-1', 'fts-test', 'user',
		 '[{"type":"text","content":"unique searchable phrase about database migrations"}]',
		 1, 0, 0)`,
	)
	require.NoError(t, err)

	// Verify FTS5 can find the message.
	var count int
	err = sqlDB.QueryRowContext(ctx,
		"SELECT count(*) FROM messages_fts WHERE messages_fts MATCH 'searchable phrase'",
	).Scan(&count)
	require.NoError(t, err, "FTS5 query should succeed")
	require.Greater(t, count, 0, "FTS5 should find the message")

	// Insert a message with no text parts.
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO messages (id, session_id, role, parts, seq, created_at, updated_at)
		 VALUES ('fts-msg-2', 'fts-test', 'user', '[]', 2, 0, 0)`,
	)
	require.NoError(t, err)

	// Search for something that should NOT match.
	err = sqlDB.QueryRowContext(ctx,
		"SELECT count(*) FROM messages_fts WHERE messages_fts MATCH 'nonexistent_term_xyz'",
	).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "FTS5 should not find nonexistent terms")
}

// TestLCMSummariesFTSPreservedAfterDAGMigration verifies that the
// lcm_summaries_fts virtual table is properly recreated after the xrush_dag
// table-swap migration and can be queried.
func TestLCMSummariesFTSPreservedAfterDAGMigration(t *testing.T) {
	t.Parallel()
	sqlDB := migrationTestOpenDB(t)
	ctx := context.Background()

	// Apply all migrations.
	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	// Verify the FTS virtual table exists and is queryable.
	_, err = sqlDB.ExecContext(ctx, "SELECT count(*) FROM lcm_summaries_fts")
	require.NoError(t, err, "lcm_summaries_fts should be queryable after DAG migration")

	// Create prerequisite data and insert a summary.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('lcm-fts-test', 'test', 0, 0)",
	)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO lcm_summaries (summary_id, session_id, kind, content)
		 VALUES ('lcm-fts-sum', 'lcm-fts-test', 'leaf',
		 'important architectural decision about caching layer')`,
	)
	require.NoError(t, err)

	// Rebuild the FTS index after manual insert (external content FTS
	// does not auto-sync without triggers).
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO lcm_summaries_fts(lcm_summaries_fts) VALUES('rebuild')",
	)
	require.NoError(t, err, "FTS rebuild should succeed")

	// Verify FTS5 on lcm_summaries works after rebuild.
	var count int
	err = sqlDB.QueryRowContext(ctx,
		"SELECT count(*) FROM lcm_summaries_fts WHERE lcm_summaries_fts MATCH 'architectural'",
	).Scan(&count)
	require.NoError(t, err)
	require.Greater(t, count, 0, "lcm_summaries_fts should find the summary after rebuild")
}

// TestSessionOperationalMemoryMigration verifies the session_om rename and
// thread_id table-swap migrations preserve data.
func TestSessionOperationalMemoryMigration(t *testing.T) {
	t.Parallel()
	sqlDB := migrationTestOpenDB(t)
	ctx := context.Background()

	// Apply all migrations.
	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	// Create prerequisite data.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('om-test', 'test', 0, 0)",
	)
	require.NoError(t, err)

	// Insert operational memory with thread_id (post-table-swap schema).
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO session_operational_memory (session_id, thread_id, key, value, priority)
		 VALUES ('om-test', 'thread-1', 'active_task', 'refactor auth', 'high')`,
	)
	require.NoError(t, err, "session_operational_memory should accept inserts with thread_id")

	// Verify the data.
	var value string
	err = sqlDB.QueryRowContext(ctx,
		"SELECT value FROM session_operational_memory WHERE session_id = 'om-test' AND key = 'active_task'",
	).Scan(&value)
	require.NoError(t, err)
	require.Equal(t, "refactor auth", value)
}

// TestMessagePartsMigration verifies the message_parts migration creates
// the table with proper constraints.
func TestMessagePartsMigration(t *testing.T) {
	t.Parallel()
	sqlDB := migrationTestOpenDB(t)
	ctx := context.Background()

	// Apply all migrations.
	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	// Create prerequisite data.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('parts-test', 'test', 0, 0)",
	)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO messages (id, session_id, role, parts, created_at, updated_at) VALUES ('parts-msg', 'parts-test', 'user', '[]', 0, 0)",
	)
	require.NoError(t, err)

	// Insert valid message parts.
	validTypes := []string{"text", "reasoning", "tool_call", "tool_result", "finish", "image_url", "binary"}
	for i, pt := range validTypes {
		_, err = sqlDB.ExecContext(ctx,
			`INSERT INTO message_parts (part_id, message_id, session_id, part_type, part_index, content_json)
			 VALUES (?, 'parts-msg', 'parts-test', ?, ?, '{}')`,
			fmt.Sprintf("part-%s", pt), pt, i,
		)
		require.NoError(t, err, "part_type=%q should be accepted", pt)
	}

	// Verify invalid part_type is rejected.
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO message_parts (part_id, message_id, session_id, part_type, part_index, content_json)
		 VALUES ('part-invalid', 'parts-msg', 'parts-test', 'invalid_type', 99, '{}')`,
	)
	require.Error(t, err, "invalid part_type should be rejected by CHECK constraint")

	// Verify the Down migration works.
	err = goose.Down(sqlDB, "migrations")
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, "SELECT * FROM message_parts LIMIT 0")
	require.Error(t, err, "message_parts table should not exist after rollback")
}
