#!/usr/bin/env bash
# Test: Operator/parallel/swarm orchestration tools.
# Sends a prompt that encourages the agent to use orchestration modes
# (operator, parallel, or swarm) and verifies that orchestration-related
# log entries appear in the crush log. This test does NOT hard-fail when
# the agent chooses not to orchestrate — it documents the outcome instead.
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
# Scenario 1: Orchestration tools attempted
# ---------------------------------------------------------------------------
test_orchestration_attempted() {
  echo "=== Scenario 1: Orchestration tools attempted ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    command restore_crush
  }
  trap restore_on_exit EXIT

  # Wave 4 config enables orchestration features.
  start_crush 4
  send_prompt "Break down this complex task into subtasks using operator mode: analyze the architecture of internal/lcm/, internal/repomap/, and internal/treesitter/ simultaneously"
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle (180s timeout)"
    capture_evidence 42 "orchestration"
    return
  fi

  # Check logs for orchestration-related entries.
  local log_entries
  log_entries=$(grep -ciE "operator|parallel|swarm|orchestrat" .crush/logs/crush.log 2>/dev/null || echo 0)
  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $log_entries orchestration log entry/entries (>= 1)"
    # Show sample of matched lines for evidence.
    grep -iE "operator|parallel|swarm|orchestrat" .crush/logs/crush.log | head -20
  else
    fail "Scenario 1: No orchestration evidence found in logs"
  fi

  # Verify that the session was created and has messages (basic liveness).
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    pass "Scenario 1: Session created (ID = $SID)"
  else
    fail "Scenario 1: No session ID found in DB"
  fi

  local messages
  messages=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$SID' AND role = 'assistant'")
  local msg_count
  msg_count=$(echo "$messages" | jq '.[0].cnt // 0')
  if [[ "$msg_count" -ge 1 ]]; then
    pass "Scenario 1: Session has $msg_count assistant message(s) (>= 1)"
  else
    fail "Scenario 1: Expected >= 1 assistant messages, got $msg_count"
  fi

  capture_evidence 42 "orchestration"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_orchestration_attempted

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
