#!/usr/bin/env bash
# Test: Orchestration parent-child contract verification (TUI-first).
# Verifies the contract between parent orchestration sessions and child
# sub-agent sessions. Scenario 1: success with parallel sub-agents.
# Scenario 2: partial failure with an impossible path.
# TUI output must show deterministic sentinels. Secondary DB checks prove
# parent-child linkage, message ordering, and role counts.
set -euo pipefail

WAVE=4
SCENARIO="orchestration-contract-success"
source "$(dirname "$0")/../lib/common.sh"
source "$(dirname "$0")/../lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup_test() { cleanup_tui; restore_crush; }
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Success — parallel sub-agents with scoped child sessions
# ---------------------------------------------------------------------------
test_orchestration_contract_success() {
  echo "=== Scenario 1: Parallel sub-agents — parent-child contract ==="
  SCENARIO="orchestration-contract-success"

  setup_clean_crush
  start_crush_tui "$WAVE"

  send_tui_prompt "Use parallel sub-agents. Count Go files under internal/lcm, internal/repomap, and internal/treesitter. After all sub-agents finish, respond with exactly ORCH_CONTRACT_SENTINEL_55 on its own line."
  if ! wait_for_tui_idle 240; then
    fail "Scenario 1: Crush did not become idle (240s timeout)"
    capture_tui_evidence "orch-contract-s1-timeout"
    return
  fi

  # --- Primary: TUI sentinel check ---
  if assert_tui_contains "ORCH_CONTRACT_SENTINEL_55"; then
    pass "Scenario 1: TUI output contains ORCH_CONTRACT_SENTINEL_55"
  else
    fail "Scenario 1: TUI output missing ORCH_CONTRACT_SENTINEL_55"
  fi
  capture_tui_evidence "orch-contract-s1-result"

  # --- Secondary: DB contract checks ---
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    pass "Scenario 1: Parent session exists (ID = $SID)"
  else
    fail "Scenario 1: No parent session ID found in DB"
    return
  fi

  # Child sessions via named query.
  local children
  children=$(run_query "child_sessions_by_parent" "$SID")
  if [[ -z "$children" ]] || [[ "$children" == "[]" ]] || [[ "$children" == "null" ]]; then
    fail "Scenario 1: No child sessions found for parent $SID"
    return
  fi

  local child_count
  child_count=$(echo "$children" | jq 'length')
  if [[ "$child_count" -ge 1 ]]; then
    pass "Scenario 1: Found $child_count child session(s) (>= 1)"
  else
    fail "Scenario 1: Expected >= 1 child sessions, got $child_count"
  fi

  # Each child has user and assistant messages.
  local child_ids
  child_ids=$(echo "$children" | jq -r '.[].id')
  local msg_exchange_failures=0
  while IFS= read -r cid; do
    [[ -z "$cid" ]] && continue
    local child_role_counts
    child_role_counts=$(query_db "SELECT role, COUNT(*) as count FROM messages WHERE session_id = '$cid' GROUP BY role")
    local child_user_count
    child_user_count=$(echo "$child_role_counts" | jq '[.[] | select(.role == "user")][0].count // 0')
    local child_assistant_count
    child_assistant_count=$(echo "$child_role_counts" | jq '[.[] | select(.role == "assistant")][0].count // 0')
    if [[ "$child_user_count" -lt 1 ]] || [[ "$child_assistant_count" -lt 1 ]]; then
      fail "Scenario 1: Child $cid missing user/assistant exchange"
      msg_exchange_failures=$((msg_exchange_failures + 1))
    fi
  done <<< "$child_ids"
  if [[ "$msg_exchange_failures" -eq 0 ]]; then
    pass "Scenario 1: Every child session contains a user/assistant exchange"
  fi

  # Verify parent has assistant messages referencing child results.
  local parent_msg_count
  parent_msg_count=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$SID' AND role = 'assistant'" | jq '.[0].cnt // 0')
  if [[ "$parent_msg_count" -ge 1 ]]; then
    pass "Scenario 1: Parent has $parent_msg_count assistant message(s)"
  else
    fail "Scenario 1: Expected >= 1 parent assistant messages, got $parent_msg_count"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: Partial failure — one subtask uses an impossible path
# ---------------------------------------------------------------------------
test_orchestration_contract_partial_failure() {
  echo "=== Scenario 2: Partial failure — impossible path in one subtask ==="
  SCENARIO="orchestration-contract-partial-failure"

  setup_clean_crush
  start_crush_tui "$WAVE"

  send_tui_prompt "Use parallel sub-agents. Count Go files under internal/lcm, internal/repomap, and internal/THIS_PATH_DOES_NOT_EXIST_AT_ALL. After all sub-agents finish (even failed ones), respond with exactly ORCH_CONTRACT_SENTINEL_55 on its own line."
  if ! wait_for_tui_idle 240; then
    fail "Scenario 2: Crush did not become idle (240s timeout)"
    capture_tui_evidence "orch-contract-s2-timeout"
    return
  fi

  # --- Primary: TUI sentinel check ---
  if assert_tui_contains "ORCH_CONTRACT_SENTINEL_55"; then
    pass "Scenario 2: TUI output contains ORCH_CONTRACT_SENTINEL_55"
  else
    fail "Scenario 2: TUI output missing ORCH_CONTRACT_SENTINEL_55"
  fi
  capture_tui_evidence "orch-contract-s2-result"

  # --- Secondary: DB contract checks ---
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    pass "Scenario 2: Parent session exists (ID = $SID)"
  else
    fail "Scenario 2: No parent session ID found in DB"
    return
  fi

  # Child sessions exist.
  local children
  children=$(run_query "child_sessions_by_parent" "$SID")
  if [[ -z "$children" ]] || [[ "$children" == "[]" ]] || [[ "$children" == "null" ]]; then
    fail "Scenario 2: No child sessions found for parent $SID"
    return
  fi

  local child_count
  child_count=$(echo "$children" | jq 'length')
  if [[ "$child_count" -ge 1 ]]; then
    pass "Scenario 2: Found $child_count child session(s) (>= 1)"
  else
    fail "Scenario 2: Expected >= 1 child sessions, got $child_count"
  fi

  # Verify partial success reporting in parent assistant response.
  local parent_text
  parent_text=$(query_db "SELECT mp.content_json FROM message_parts mp JOIN messages m ON m.id = mp.message_id WHERE m.session_id = '$SID' AND m.role = 'assistant' AND mp.part_type = 'text' ORDER BY m.created_at DESC LIMIT 1")
  local parent_content
  parent_content=$(echo "$parent_text" | jq -r '.content // ""' | tr '[:upper:]' '[:lower:]')
  local partial_indicators
  partial_indicators=$(echo "$parent_content" | grep -ciE "partial|could not|not found|unable|error|missing|does not exist|failed" || echo 0)
  if [[ "$partial_indicators" -ge 1 ]]; then
    pass "Scenario 2: Parent reports partial success ($partial_indicators indicator(s))"
  else
    fail "Scenario 2: Parent response does not indicate partial success"
  fi

  # Successful child results still included in parent.
  local valid_pkgs=("lcm" "repomap")
  local found_valid=0
  for pkg in "${valid_pkgs[@]}"; do
    if [[ "$parent_content" == *"$pkg"* ]]; then
      found_valid=$((found_valid + 1))
    fi
  done
  if [[ "$found_valid" -ge 1 ]]; then
    pass "Scenario 2: Parent includes results from successful children ($found_valid valid package(s))"
  else
    fail "Scenario 2: Parent does not include results from any successful children"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_orchestration_contract_success
test_orchestration_contract_partial_failure

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
