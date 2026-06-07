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

cleanup_test() { restore_crush; }
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

  # --- Secondary: DB checks ---
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario: No session ID found in DB"
    return
  fi
  pass "Scenario: Main session ID = $SID"

  # Query child sessions via parent_session_id.
  local children
  children=$(query_db "SELECT id FROM sessions WHERE parent_session_id = '$SID'")
  if [[ -z "$children" ]] || [[ "$children" == "[]" ]]; then
    fail "Scenario: No child sessions found for parent $SID"
    return
  fi

  local child_count
  child_count=$(echo "$children" | jq 'length')
  if [[ "$child_count" -ge 1 ]]; then
    pass "Scenario: Found $child_count child session(s) (>= 1)"
  else
    fail "Scenario: Expected >= 1 child sessions, got $child_count"
  fi

  # Verify child ID format contains '$$' (parent_uuid$$call_uuid).
  local all_have_dollar=true
  local child_ids
  child_ids=$(echo "$children" | jq -r '.[].id')
  while IFS= read -r cid; do
    [[ -z "$cid" ]] && continue
    if [[ "$cid" != *'$$'* ]]; then
      fail "Scenario: Child ID '$cid' does not contain '\$\$'"
      all_have_dollar=false
    fi
  done <<< "$child_ids"
  if [[ "$all_have_dollar" == "true" ]]; then
    pass "Scenario: All child IDs contain '\$\$' separator"
  fi

  # Verify each child has user and assistant messages.
  local child_message_failures=0
  while IFS= read -r cid; do
    [[ -z "$cid" ]] && continue
    local child_role_counts
    child_role_counts=$(query_db "SELECT role, COUNT(*) as count FROM messages WHERE session_id = '$cid' GROUP BY role")
    local child_user_count
    child_user_count=$(echo "$child_role_counts" | jq '[.[] | select(.role == "user")][0].count // 0')
    local child_assistant_count
    child_assistant_count=$(echo "$child_role_counts" | jq '[.[] | select(.role == "assistant")][0].count // 0')
    if [[ "$child_user_count" -lt 1 ]] || [[ "$child_assistant_count" -lt 1 ]]; then
      fail "Scenario: Child session $cid lacks user/assistant message exchange"
      child_message_failures=$((child_message_failures + 1))
    fi
  done <<< "$child_ids"
  if [[ "$child_message_failures" -eq 0 ]]; then
    pass "Scenario: Every child session contains a user/assistant exchange"
  fi

  # Verify main session has assistant messages.
  local msg_count
  msg_count=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$SID' AND role = 'assistant'" | jq '.[0].cnt // 0')
  if [[ "$msg_count" -ge 1 ]]; then
    pass "Scenario: Main session has $msg_count assistant message(s) (>= 1)"
  else
    fail "Scenario: Expected >= 1 assistant messages, got $msg_count"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_forked_agent_child_session

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
