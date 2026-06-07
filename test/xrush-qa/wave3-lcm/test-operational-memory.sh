#!/usr/bin/env bash
# Test: Operational memory population and session-scoped context (TUI-first).
# Verifies that Crush writes entries to session_operational_memory when
# operational_memory_enabled is true (wave3 config), and that the memory
# content is visible in TUI responses.
set -euo pipefail

WAVE=3
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# Shared session ID from Scenario 1.
SID=""

cleanup_test() { cleanup_tui; restore_crush; }
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Operational memory populated with sentinel content
# ---------------------------------------------------------------------------
test_operational_memory_populated() {
  echo "=== Scenario 1: Operational memory populated with sentinel content ==="
  SCENARIO="op-memory-populated"

  setup_clean_crush
  start_crush_tui 3
  focus_editor

  # First message with a preference containing a deterministic sentinel.
  send_tui_prompt "I prefer using table-driven tests in Go. My operational context identifier is OP_MEM_STORE_SENTINEL_88."
  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_tui_evidence "idle-timeout-p1"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI must echo the sentinel back.
  if assert_tui_contains "OP_MEM_STORE_SENTINEL_88"; then
    pass "Scenario 1: TUI shows OP_MEM_STORE_SENTINEL_88 sentinel"
  else
    fail "Scenario 1: TUI does not show OP_MEM_STORE_SENTINEL_88 sentinel"
    capture_tui_evidence "sentinel-missing-p1"
    return
  fi

  # Second message to give the auto-memory bridge more material.
  send_tui_prompt "I also like parallel tests. Please confirm OP_MEM_STORE_SENTINEL_88 in your response."
  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle after second prompt"
    capture_tui_evidence "idle-timeout-p2"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "OP_MEM_STORE_SENTINEL_88"; then
    pass "Scenario 1: TUI confirms OP_MEM_STORE_SENTINEL_88 on second turn"
  else
    fail "Scenario 1: TUI lost OP_MEM_STORE_SENTINEL_88 on second turn"
    capture_tui_evidence "sentinel-missing-p2"
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary DB checks ---
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Query session_operational_memory for entries with key starting with 'memory:'.
  local om_rows
  om_rows=$(query_db "SELECT key, value, priority FROM session_operational_memory WHERE session_id = '$SID' AND key LIKE 'memory:%'")
  if [[ -z "$om_rows" ]] || [[ "$om_rows" == "[]" ]]; then
    fail "Scenario 1: No session_operational_memory rows with key LIKE 'memory:%' for session $SID"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Assert: at least 1 entry.
  local om_count
  om_count=$(echo "$om_rows" | jq 'length')
  if [[ "$om_count" -ge 1 ]]; then
    pass "Scenario 1: Found $om_count operational memory entry/entries with key LIKE 'memory:%'"
  else
    fail "Scenario 1: Expected >= 1 operational memory entries, got $om_count"
    tmux send-keys -t "$TMUX_SESSION" C-c
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
  relevant_values=$(echo "$om_rows" | jq '[.[] | select((.value | ascii_downcase) | test("test|parallel|table|sentinel"))] | length')
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

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Scenario 2: Operational memory integration — recall within session
# ---------------------------------------------------------------------------
test_operational_memory_recall() {
  echo "=== Scenario 2: Operational memory recall within session ==="
  SCENARIO="op-memory-recall"

  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID from Scenario 1, skipping"
    return
  fi

  # Reuse the same session — operational memory is session-scoped.
  start_crush_tui 3 --session "$SID"
  focus_editor

  # Ask about the stored operational context using recall sentinel.
  send_tui_prompt "What is my operational context identifier? Reply with OP_MEM_RECALL_SENTINEL_88 if you remember it from our conversation."
  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle after recall prompt"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI must show the recall sentinel.
  if assert_tui_contains "OP_MEM_RECALL_SENTINEL_88"; then
    pass "Scenario 2: TUI shows OP_MEM_RECALL_SENTINEL_88 — operational memory recalled"
  else
    fail "Scenario 2: TUI does not show OP_MEM_RECALL_SENTINEL_88 — operational memory not recalled"
    capture_tui_evidence "recall-sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  capture_tui_evidence "tui-recall-response"

  # --- Secondary DB check: operational memory rows still exist for session ---
  local om_count
  om_count=$(query_db "SELECT COUNT(*) as count FROM session_operational_memory WHERE session_id = '$SID' AND key LIKE 'memory:%'" | jq '.[0].count')
  if [[ "$om_count" -ge 1 ]]; then
    pass "Scenario 2: Operational memory rows persist in DB ($om_count entries)"
  else
    fail "Scenario 2: No operational memory rows found in DB for session $SID"
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_operational_memory_populated
test_operational_memory_recall

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
