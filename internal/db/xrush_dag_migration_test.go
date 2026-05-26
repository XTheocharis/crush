package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

var dagTestGooseOnce sync.Once

func dagTestInitGoose(t *testing.T) {
	t.Helper()
	dagTestGooseOnce.Do(func() {
		goose.SetBaseFS(FS)
		if err := goose.SetDialect("sqlite3"); err != nil {
			t.Fatalf("goose.SetDialect: %v", err)
		}
	})
}

func dagTestOpenDB(t *testing.T) *sql.DB {
	t.Helper()
	dagTestInitGoose(t)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=recursive_triggers(ON)", dbPath)
	sqlDB, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	err = sqlDB.PingContext(context.Background())
	require.NoError(t, err)

	return sqlDB
}

func TestXrushDAGMigration_Up(t *testing.T) {
	t.Parallel()
	sqlDB := dagTestOpenDB(t)

	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err, "all migrations should apply cleanly")

	ctx := context.Background()

	// Create a session to satisfy FK constraints.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('dag-test-session', 'test', 0, 0)",
	)
	require.NoError(t, err)

	// Insert a summary row to verify the metadata column exists.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO lcm_summaries (summary_id, session_id, kind, content, metadata) VALUES ('check-meta', 'dag-test-session', 'leaf', '', '{\"test\":true}')",
	)
	require.NoError(t, err, "lcm_summaries should have a metadata column")

	var metadataVal string
	err = sqlDB.QueryRowContext(ctx,
		"SELECT metadata FROM lcm_summaries WHERE summary_id = 'check-meta'",
	).Scan(&metadataVal)
	require.NoError(t, err)
	require.Equal(t, `{"test":true}`, metadataVal)

	testCases := []string{"leaf", "condensed", "observation", "auto_memory", "session"}
	for _, kind := range testCases {
		_, err = sqlDB.ExecContext(ctx,
			"INSERT INTO lcm_summaries (summary_id, session_id, kind, content) VALUES (?, ?, ?, '')",
			fmt.Sprintf("sum-%s", kind), "dag-test-session", kind,
		)
		require.NoError(t, err, "kind=%q should be accepted by CHECK constraint", kind)
	}

	// Verify lcm_reversible_state table exists.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO lcm_reversible_state (id, summary_id, original_messages) VALUES ('rs-1', 'sum-leaf', '[]')",
	)
	require.NoError(t, err, "lcm_reversible_state should accept inserts")

	// Verify lcm_observation_buffer table exists and buffer_type constraint.
	bufTypes := []string{"observation", "insight", "summary"}
	for _, bt := range bufTypes {
		_, err = sqlDB.ExecContext(ctx,
			"INSERT INTO lcm_observation_buffer (id, session_id, buffer_type, content) VALUES (?, ?, ?, '')",
			fmt.Sprintf("buf-%s", bt), "dag-test-session", bt,
		)
		require.NoError(t, err, "buffer_type=%q should be accepted", bt)
	}

	// Verify lcm_auto_memory table exists and memory_type constraint.
	memTypes := []string{"fact", "decision", "preference", "lesson"}
	for _, mt := range memTypes {
		_, err = sqlDB.ExecContext(ctx,
			"INSERT INTO lcm_auto_memory (id, session_id, memory_type, content, source_message_ids) VALUES (?, ?, ?, '', '[]')",
			fmt.Sprintf("mem-%s", mt), "dag-test-session", mt,
		)
		require.NoError(t, err, "memory_type=%q should be accepted", mt)
	}

	// Verify FTS5 virtual table still exists.
	var ftsCount int
	err = sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM lcm_summaries_fts").Scan(&ftsCount)
	require.NoError(t, err, "lcm_summaries_fts should be queryable")
}

func TestXrushDAGMigration_Down(t *testing.T) {
	t.Parallel()
	sqlDB := dagTestOpenDB(t)

	// Apply all migrations.
	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	// Roll back only the xrush_dag migration.
	err = goose.DownTo(sqlDB, "migrations", 20260222000000)
	require.NoError(t, err, "rolling back xrush_dag migration should succeed")

	ctx := context.Background()

	// Verify new tables are gone.
	for _, table := range []string{"lcm_auto_memory", "lcm_observation_buffer", "lcm_reversible_state"} {
		_, err = sqlDB.ExecContext(ctx, fmt.Sprintf("SELECT * FROM %s LIMIT 0", table))
		require.Error(t, err, "table %s should not exist after rollback", table)
	}

	// Verify lcm_summaries exists with original kind constraint.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('dag-down-test', 'test', 0, 0)",
	)
	require.NoError(t, err)

	// Original kinds should work.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO lcm_summaries (summary_id, session_id, kind, content) VALUES ('s1', 'dag-down-test', 'leaf', '')",
	)
	require.NoError(t, err, "kind='leaf' should be accepted after rollback")

	// New kinds should be rejected.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO lcm_summaries (summary_id, session_id, kind, content) VALUES ('s2', 'dag-down-test', 'observation', '')",
	)
	require.Error(t, err, "kind='observation' should be rejected after rollback (original constraint)")

	// Verify metadata column is gone.
	var colCount int
	err = sqlDB.QueryRowContext(ctx,
		"SELECT count(*) FROM pragma_table_info('lcm_summaries') WHERE name = 'metadata'",
	).Scan(&colCount)
	require.NoError(t, err)
	require.Equal(t, 0, colCount, "metadata column should not exist after rollback")
}

func TestXrushDAGMigration_RoundTrip(t *testing.T) {
	t.Parallel()
	sqlDB := dagTestOpenDB(t)

	// Apply all migrations.
	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	// Roll back to before xrush_dag.
	err = goose.DownTo(sqlDB, "migrations", 20260222000000)
	require.NoError(t, err)

	// Re-apply all migrations.
	err = goose.Up(sqlDB, "migrations")
	require.NoError(t, err, "re-applying xrush_dag migration should succeed")

	ctx := context.Background()

	// Verify tables are back.
	for _, table := range []string{"lcm_auto_memory", "lcm_observation_buffer", "lcm_reversible_state"} {
		_, err = sqlDB.ExecContext(ctx, fmt.Sprintf("SELECT * FROM %s LIMIT 0", table))
		require.NoError(t, err, "table %s should exist after round-trip", table)
	}

	// Verify expanded kind still works.
	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('dag-rt-test', 'test', 0, 0)",
	)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO lcm_summaries (summary_id, session_id, kind, content) VALUES ('rt-1', 'dag-rt-test', 'session', '')",
	)
	require.NoError(t, err, "kind='session' should work after round-trip")
}
