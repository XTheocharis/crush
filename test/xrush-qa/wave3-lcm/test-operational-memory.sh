#!/usr/bin/env bash
# Test: Operational memory is populated when enabled.
# Verifies that Crush writes entries to session_operational_memory when
# operational_memory_enabled is true (wave3 config).
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
# Scenario 1: Operational memory populated when enabled
# ---------------------------------------------------------------------------
test_operational_memory_populated() {
  echo "=== Scenario 1: Operational memory populated when enabled ==="

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
  }
  trap restore_on_exit EXIT

  # Wave 3 config has operational_memory_enabled: true.
  start_crush 3

  # Send first message with a preference that should be captured as memory.
  send_prompt "I prefer using table-driven tests in Go."
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_evidence 14 "operational-memory"
    return
  fi

  # Send second message to give the auto-memory bridge more material.
  send_prompt "I also like parallel tests."
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle after second prompt"
    capture_evidence 14 "operational-memory"
    return
  fi

  # Get the session ID.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 14 "operational-memory"
    return
  fi

  # Query session_operational_memory for entries with key starting with 'memory:'.
  local om_rows
  om_rows=$(query_db "SELECT key, value, priority FROM session_operational_memory WHERE session_id = '$SID' AND key LIKE 'memory:%'")
  if [[ -z "$om_rows" ]] || [[ "$om_rows" == "[]" ]]; then
    fail "Scenario 1: No session_operational_memory rows with key LIKE 'memory:%' for session $SID"
    capture_evidence 14 "operational-memory"
    return
  fi

  # Assert: at least 1 entry.
  local om_count
  om_count=$(echo "$om_rows" | jq 'length')
  if [[ "$om_count" -ge 1 ]]; then
    pass "Scenario 1: Found $om_count operational memory entry/entries with key LIKE 'memory:%'"
  else
    fail "Scenario 1: Expected >= 1 operational memory entries, got $om_count"
    capture_evidence 14 "operational-memory"
    return
  fi

  # Assert: value is non-empty for all entries.
  local empty_values
  empty_values=$(echo "$om_rows" | jq '[.[] | select(.value == null or .value == "")] | length')
  if [[ "$empty_values" -eq 0 ]]; then
    pass "Scenario 1: All operational memory entries have non-empty values"
  else
    fail "Scenario 1: $empty_values entries have empty values"
  fi

  # Assert: priority is 'medium' (default for auto-memory bridge).
  local medium_count
  medium_count=$(echo "$om_rows" | jq '[.[] | select(.priority == "medium")] | length')
  if [[ "$medium_count" -ge 1 ]]; then
    pass "Scenario 1: At least 1 entry has priority 'medium' (found $medium_count)"
  else
    fail "Scenario 1: No entries with priority 'medium' found"
  fi

  local relevant_values
  relevant_values=$(echo "$om_rows" | jq '[.[] | select((.value | ascii_downcase) | test("test|parallel|table"))] | length')
  if [[ "$relevant_values" -ge 1 ]]; then
    pass "Scenario 1: Operational memory values capture test-related preferences"
  else
    fail "Scenario 1: Operational memory values did not capture test-related preferences"
  fi

  local thread_id_count
  thread_id_count=$(query_db "SELECT COUNT(*) as count FROM session_operational_memory WHERE session_id = '$SID' AND key LIKE 'memory:%' AND thread_id IS NOT NULL" | jq '.[0].count')
  if [[ "$thread_id_count" -ge "$om_count" ]]; then
    pass "Scenario 1: Operational memory rows include thread_id column values"
  else
    fail "Scenario 1: Some operational memory rows are missing thread_id values"
  fi

  capture_evidence 14 "operational-memory"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_operational_memory_populated

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
