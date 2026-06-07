#!/usr/bin/env bash
# Test: DB migrations through TUI startup (TUI-first).
# Scenario 1: Create minimal old DB schema → start TUI → verify migration succeeded.
# Strategy: Create a SQLite DB with only the initial (v1) schema tables, then start
# Crush TUI against it. Crush auto-migrates on startup. After TUI reaches ready state
# and responds to a sentinel prompt, verify DB schema version and expected migrated tables.
set -euo pipefail

WAVE=5

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup_test() {
    cleanup_tui
  local _bak
  _bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  [[ -n "$_bak" ]] && mv "$_bak" crush.json
  restore_crush
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Helper: Create a minimal "old" database with only the initial schema.
# This simulates a database from before any LCM, repo_map, or other migrations.
# ---------------------------------------------------------------------------
create_old_db() {
  local db_path="$1"
  mkdir -p "$(dirname "$db_path")"

  sqlite3 "$db_path" <<'INITIAL_SCHEMA'
-- Mimic goose initial migration (20250424200609_initial.sql)
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    parent_session_id TEXT,
    title TEXT NOT NULL,
    message_count INTEGER NOT NULL DEFAULT 0 CHECK (message_count >= 0),
    prompt_tokens  INTEGER NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens  INTEGER NOT NULL DEFAULT 0 CHECK (completion_tokens>= 0),
    cost REAL NOT NULL DEFAULT 0.0 CHECK (cost >= 0.0),
    updated_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TRIGGER IF NOT EXISTS update_sessions_updated_at
AFTER UPDATE ON sessions
BEGIN
UPDATE sessions SET updated_at = strftime('%s', 'now')
WHERE id = new.id;
END;

CREATE TABLE IF NOT EXISTS files (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    path TEXT NOT NULL,
    content TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions (id) ON DELETE CASCADE,
    UNIQUE(path, session_id, version)
);

CREATE INDEX IF NOT EXISTS idx_files_session_id ON files (session_id);
CREATE INDEX IF NOT EXISTS idx_files_path ON files (path);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    parts TEXT NOT NULL default '[]',
    model TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    finished_at INTEGER,
    FOREIGN KEY (session_id) REFERENCES sessions (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages (session_id);

-- Simulate goose_db_version with only the initial migration applied.
-- goose uses (id, version_id, is_applied) columns.
CREATE TABLE IF NOT EXISTS goose_db_version (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version_id INTEGER NOT NULL,
    is_applied BOOLEAN NOT NULL,
    tstamp TEXT DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now'))
);
INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1);
INSERT INTO goose_db_version (version_id, is_applied) VALUES (20250424200609, 1);
INITIAL_SCHEMA
}

# ---------------------------------------------------------------------------
# Scenario 1: Old DB → TUI startup auto-migrates → verify schema + sentinel
# ---------------------------------------------------------------------------
test_old_db_migration() {
  SCENARIO="old-db-migration"
  echo "=== Scenario 1: Old DB schema auto-migrates through TUI startup ==="

  # Step 1: Set up a clean .crush directory (backs up existing).
  setup_clean_crush

  # Step 2: Create a minimal old DB before Crush starts.
  # Crush will detect the incomplete schema and auto-migrate.
  local db_path=".crush/crush.db"
  create_old_db "$db_path"

  # Verify our old DB has only the initial tables.
  local old_table_count
  old_table_count=$(sqlite3 "$db_path" \
    "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE '%_fts%';")
  echo "  Old DB table count (before TUI): $old_table_count"

  # Step 3: Start Crush TUI — it should auto-migrate the DB on startup.
  start_crush_tui 5
  focus_editor

  # Step 4: Wait for TUI to be ready, then send sentinel prompt.
  # Give Crush a moment to complete migrations and initialize.
  sleep 5

  send_tui_prompt "Please respond with exactly: DB_MIGRATION_SENTINEL_42"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle within 120s"
    capture_tui_evidence "migration-timeout"
    return
  fi

  # Step 5: Primary gate — TUI pane shows sentinel response.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "DB_MIGRATION_SENTINEL_42"; then
    pass "Scenario 1: TUI contains DB_MIGRATION_SENTINEL_42"
  else
    fail "Scenario 1: TUI does not contain DB_MIGRATION_SENTINEL_42"
  fi

  # Step 6: Secondary gate — DB schema version check.
  # Goose tracks the latest applied migration version_id in goose_db_version.
  local migration_version
  migration_version=$(sqlite3 "$db_path" \
    "SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1;" 2>/dev/null || echo "0")

  # The initial migration is 20250424200609 (version 20250424200609).
  # After full migration, version should be at least 20260522000000 (latest migration file).
  if [[ "$migration_version" != "0" && "$migration_version" -ge 20260522000000 ]]; then
    pass "Scenario 1: DB migration version is $migration_version (>= 20260522000000)"
  else
    fail "Scenario 1: DB migration version is $migration_version, expected >= 20260522000000"
  fi

  # Step 7: Verify key migrated tables exist.
  local tables_to_check=(
    "sessions"
    "messages"
    "files"
    "lcm_session_config"
    "lcm_summaries"
    "lcm_context_items"
    "repo_map_file_cache"
    "repo_map_tags"
  )

  local table
  for table in "${tables_to_check[@]}"; do
    local exists
    exists=$(sqlite3 "$db_path" \
      "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='$table';" 2>/dev/null || echo "0")
    if [[ "$exists" -eq 1 ]]; then
      pass "Scenario 1: Migrated table '$table' exists"
    else
      fail "Scenario 1: Migrated table '$table' NOT found"
    fi
  done

  # Step 8: Verify messages table has migration-added columns (seq, token_count).
  local has_seq_col
  has_seq_col=$(sqlite3 "$db_path" \
    "SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name='seq';" 2>/dev/null || echo "0")
  if [[ "$has_seq_col" -eq 1 ]]; then
    pass "Scenario 1: messages table has 'seq' column (migrated from LCM migration)"
  else
    fail "Scenario 1: messages table missing 'seq' column"
  fi

  local has_token_count_col
  has_token_count_col=$(sqlite3 "$db_path" \
    "SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name='token_count';" 2>/dev/null || echo "0")
  if [[ "$has_token_count_col" -eq 1 ]]; then
    pass "Scenario 1: messages table has 'token_count' column (migrated from LCM migration)"
  else
    fail "Scenario 1: messages table missing 'token_count' column"
  fi

  # Step 9: Log evidence for migration activity.
  local log_file=".crush/logs/crush.log"
  if [[ -f "$log_file" ]]; then
    local migration_log_matches
    migration_log_matches=$(grep -ciE "migration|migrate|goose|schema" "$log_file" 2>/dev/null ) || migration_log_matches=0
    if [[ "$migration_log_matches" -ge 1 ]]; then
      pass "Scenario 1: Crush log contains migration evidence ($migration_log_matches matches)"
    else
      echo "  NOTE: No migration log entries found (may be at expected schema already)"
    fi
  fi

  capture_tui_evidence "old-db-migration"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_old_db_migration

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-db-migrations-live" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
