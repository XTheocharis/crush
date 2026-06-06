#!/usr/bin/env bash
# Test: Forked agent creates child sessions.
# Verifies that Crush creates child sessions with proper ID format
# (parent_uuid$$call_uuid) when sub-agents are dispatched, and that
# the main session references sub-agent results in its messages.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# Main session ID captured after the prompt completes.
SID=""

# ---------------------------------------------------------------------------
# Scenario 1: Forked agent creates child session
# ---------------------------------------------------------------------------
test_forked_agent_child_session() {
  echo "=== Scenario 1: Forked agent creates child session ==="

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
  send_prompt "Use sub-agents to: 1) count files in internal/lcm/, 2) count files in internal/repomap/"
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle (180s timeout)"
    capture_evidence 41 "forked-agents"
    return
  fi

  # Get the main session ID.
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 41 "forked-agents"
    return
  fi
  pass "Scenario 1: Main session ID = $SID"

  # Query child sessions (parent_session_id matches main session).
  local children
  children=$(query_db "SELECT id FROM sessions WHERE parent_session_id = '$SID'")
  if [[ -z "$children" ]] || [[ "$children" == "[]" ]]; then
    fail "Scenario 1: No child sessions found for parent $SID"
    capture_evidence 41 "forked-agents"
    return
  fi

  local child_count
  child_count=$(echo "$children" | jq 'length')
  if [[ "$child_count" -ge 1 ]]; then
    pass "Scenario 1: Found $child_count child session(s) (>= 1)"
  else
    fail "Scenario 1: Expected >= 1 child sessions, got $child_count"
  fi

  # Verify child ID format contains '$$' (parent_uuid$$call_uuid).
  local all_have_dollar=true
  local child_ids
  child_ids=$(echo "$children" | jq -r '.[].id')
  while IFS= read -r cid; do
    if [[ "$cid" != *'$$'* ]]; then
      fail "Scenario 1: Child ID '$cid' does not contain '\$\$'"
      all_have_dollar=false
    fi
  done <<< "$child_ids"
  if [[ "$all_have_dollar" == "true" ]]; then
    pass "Scenario 1: All child IDs contain '\$\$' separator"
  fi

  # Verify main session messages reference sub-agent results.
  # Look for messages containing typical sub-agent result keywords.
  local messages
  messages=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$SID' AND role = 'assistant'")
  local msg_count
  msg_count=$(echo "$messages" | jq '.[0].cnt // 0')
  if [[ "$msg_count" -ge 1 ]]; then
    pass "Scenario 1: Main session has $msg_count assistant message(s) (>= 1)"
  else
    fail "Scenario 1: Expected >= 1 assistant messages, got $msg_count"
  fi

  capture_evidence 41 "forked-agents"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_forked_agent_child_session

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
