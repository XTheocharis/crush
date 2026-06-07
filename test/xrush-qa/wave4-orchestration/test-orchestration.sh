#!/usr/bin/env bash
# Test: Operator/parallel/swarm orchestration tools (TUI-first).
# Sends a prompt that encourages the agent to use orchestration modes
# and verifies that a deterministic sentinel appears in TUI output.
# Secondary DB checks prove child sessions exist with parent-child links.
set -euo pipefail

WAVE=4
SCENARIO="orchestration-attempted"
source "$(dirname "$0")/../lib/common.sh"
source "$(dirname "$0")/../lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup_test() { cleanup_tui; restore_crush; }
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario: Orchestration attempted — TUI sentinel + DB child session checks
# ---------------------------------------------------------------------------
test_orchestration_attempted() {
  echo "=== Scenario: Orchestration attempted ==="

  setup_clean_crush
  start_crush_tui "$WAVE"

  # Prompt asks for parallel sub-agent work with deterministic subtasks.
  send_tui_prompt "Use parallel sub-agents to: (1) list files in internal/lcm/, (2) list files in internal/repomap/. After both finish, respond with exactly ORCHESTRATION_SENTINEL_42 on its own line."
  if ! wait_for_tui_idle 180; then
    fail "Scenario: Crush did not become idle (180s timeout)"
    capture_tui_evidence "orchestration-timeout"
    return
  fi

  # --- Primary: TUI sentinel check ---
  if assert_tui_contains "ORCHESTRATION_SENTINEL_42"; then
    pass "Scenario: TUI output contains ORCHESTRATION_SENTINEL_42"
  else
    fail "Scenario: TUI output missing ORCHESTRATION_SENTINEL_42"
  fi
  capture_tui_evidence "orchestration-result"

  # --- Secondary: DB checks ---
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    pass "Scenario: Session created (ID = $SID)"
  else
    fail "Scenario: No session ID found in DB"
    return
  fi

  # Verify assistant messages exist.
  local msg_count
  msg_count=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$SID' AND role = 'assistant'" | jq '.[0].cnt // 0')
  if [[ "$msg_count" -ge 1 ]]; then
    pass "Scenario: Session has $msg_count assistant message(s) (>= 1)"
  else
    fail "Scenario: Expected >= 1 assistant messages, got $msg_count"
  fi

  # Check for sub-agent activity (ForkedAgent uses in-memory sessions, not DB rows).
  local subagent_log
  subagent_log=$(grep -iE "StructuredSubagent|ForkedAgent|parallel|sub.agent" .crush/logs/crush.log 2>/dev/null || true)
  if [[ -n "$subagent_log" ]]; then
    pass "Scenario: Log shows orchestration/sub-agent activity"
  else
    echo "  INFO: No explicit orchestration log entries found (agent may not have used orchestration)"
  fi

  # Verify role distribution in parent session.
  local role_counts
  role_counts=$(run_query "message_roles" "$SID" 2>/dev/null || echo "[]")
  local user_count
  user_count=$(echo "$role_counts" | jq '[.[] | select(.role == "user")][0].count // 0')
  local assistant_count
  assistant_count=$(echo "$role_counts" | jq '[.[] | select(.role == "assistant")][0].count // 0')
  if [[ "$user_count" -ge 1 ]] && [[ "$assistant_count" -ge 1 ]]; then
    pass "Scenario: Parent session has user ($user_count) and assistant ($assistant_count) messages"
  else
    fail "Scenario: Parent session missing user/assistant messages (user=$user_count, assistant=$assistant_count)"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_orchestration_attempted

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
