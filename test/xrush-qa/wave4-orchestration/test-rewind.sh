#!/usr/bin/env bash
# Test: Turn snapshot creation and RollbackManager bug detection.
# Verifies that Crush creates turn_snapshots rows after file-editing turns,
# and detects the known "RollbackManager failed to persist snapshot" bug.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: Turn snapshots created after file edit
# ---------------------------------------------------------------------------
test_turn_snapshots() {
  echo "=== Scenario 1: Turn snapshots created after file edit ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    restore_crush
    rm -f /tmp/qa-rewind-test.txt
  }
  trap restore_on_exit EXIT

  start_crush 4
  send_prompt "Create a new file /tmp/qa-rewind-test.txt with content 'rewind test'"
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 41 "rewind-snapshots"
    return
  fi

  # Get the session ID.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 41 "rewind-snapshots"
    return
  fi

  if [[ -f /tmp/qa-rewind-test.txt ]] && grep -q "rewind test" /tmp/qa-rewind-test.txt; then
    pass "Scenario 1: /tmp/qa-rewind-test.txt exists with expected content"
  else
    fail "Scenario 1: /tmp/qa-rewind-test.txt missing or has unexpected content"
  fi

  # Query turn_snapshots count for this session.
  local snapshot_count
  snapshot_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM turn_snapshots WHERE session_id = '$SID'")
  echo "  Snapshot count for session $SID: $snapshot_count"

  # Check logs for RollbackManager errors.
  local error_count
  error_count=$(grep -c "RollbackManager failed" .crush/logs/crush.log 2>/dev/null || echo 0)
  echo "  RollbackManager error count: $error_count"

  if [[ "$snapshot_count" -gt 0 ]]; then
    pass "Scenario 1: $snapshot_count turn snapshot(s) recorded — rewind feature active"
  else
    fail "Scenario 1: Expected at least one turn snapshot after file-editing turn; errors=$error_count"
  fi

  local snapshot_file_count
  snapshot_file_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM turn_snapshot_files WHERE snapshot_id IN (SELECT id FROM turn_snapshots WHERE session_id = '$SID')")
  if [[ "$snapshot_file_count" -gt 0 ]]; then
    pass "Scenario 1: $snapshot_file_count turn snapshot file row(s) recorded"
  else
    fail "Scenario 1: Expected turn_snapshot_files rows for recorded snapshots"
  fi

  capture_evidence 41 "rewind-snapshots"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_turn_snapshots

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
