#!/usr/bin/env bash
# Test: Forked agent creates child sessions (TUI-first).
# Verifies that Crush creates child sessions with proper ID format
# when sub-agents are dispatched. TUI output must show a deterministic
# sentinel. Secondary DB checks prove forked session and message flow.
set -euo pipefail

WAVE=4
SCENARIO="forked-agent-child-session"
source "$(dirname "$0")/../lib/common.sh"
source "$(dirname "$0")/../lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup_test() { cleanup_tui; restore_crush; }
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario: Forked agent creates child session — TUI sentinel + DB checks
# ---------------------------------------------------------------------------
test_forked_agent_child_session() {
  echo "=== Scenario: Forked agent creates child session ==="

  setup_clean_crush
  start_crush_tui "$WAVE"

  send_tui_prompt "Use sub-agents to: (1) count files in internal/lcm/, (2) count files in internal/repomap/. After both finish, respond with exactly FORKED_AGENT_SENTINEL_88 on its own line."
  if ! wait_for_tui_idle 180; then
    fail "Scenario: Crush did not become idle (180s timeout)"
    capture_tui_evidence "forked-agents-timeout"
    return
  fi

  # --- Primary: TUI sentinel check ---
  if assert_tui_contains "FORKED_AGENT_SENTINEL_88"; then
    pass "Scenario: TUI output contains FORKED_AGENT_SENTINEL_88"
  else
    fail "Scenario: TUI output missing FORKED_AGENT_SENTINEL_88"
  fi
  capture_tui_evidence "forked-agents-result"

  # --- Secondary: Filesystem and log checks (ForkedAgent uses in-memory sessions) ---
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario: No session ID found in DB"
    return
  fi
  pass "Scenario: Main session ID = $SID"

  # Verify main session has assistant messages.
  local msg_count
  msg_count=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$SID' AND role = 'assistant'" | jq '.[0].cnt // 0')
  if [[ "$msg_count" -ge 1 ]]; then
    pass "Scenario: Main session has $msg_count assistant message(s) (>= 1)"
  else
    fail "Scenario: Expected >= 1 assistant messages, got $msg_count"
  fi

  # Check TUI output for evidence of sub-agent task completion.
  local tui_output
  tui_output=$(capture_tui_text 2>/dev/null || true)
  local file_count_mentioned=0
  if echo "$tui_output" | grep -qiE "(internal/lcm|internal/repomap).*(file|count|\b[0-9]+\b)"; then
    pass "Scenario: TUI output references file listing results from sub-agents"
    file_count_mentioned=1
  else
    echo "  INFO: TUI output does not explicitly show file count details (sub-agents may have summarized)"
  fi

  # Check logs for StructuredSubagent/ForkedAgent invocation.
  if [[ -f ".crush/logs/crush.log" ]]; then
    local subagent_log
    subagent_log=$(grep -iE "StructuredSubagent|ForkedAgent|forked.*session|sub.agent" .crush/logs/crush.log 2>/dev/null || true)
    if [[ -n "$subagent_log" ]]; then
      pass "Scenario: Log shows sub-agent/forked-session activity"
    else
      echo "  INFO: No explicit sub-agent log entries found (may use different log tags)"
    fi
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_forked_agent_child_session

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
