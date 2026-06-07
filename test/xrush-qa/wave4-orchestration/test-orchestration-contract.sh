#!/usr/bin/env bash
# Test: Orchestration parent-child contract verification.
# Verifies the contract between parent orchestration sessions and child
# sub-agent sessions: proper parent_session_id linkage, scoped prompts,
# numeric results in children, and synthesized final answers in the parent.
# Includes a failure scenario where one subtask uses an impossible path,
# asserting partial success reporting and inclusion of valid child results.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# Main session ID captured after each scenario.
SID=""

# ---------------------------------------------------------------------------
# Scenario 1: Success — parallel sub-agents with scoped child sessions
# ---------------------------------------------------------------------------
test_orchestration_contract_success() {
  echo "=== Scenario 1: Parallel sub-agents — parent-child contract ==="

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

  # Wave 4 config enables orchestration features.
  start_crush 4
  send_prompt "Use parallel sub-agents. Count Go files under internal/lcm, internal/repomap, and internal/treesitter. Return a final table."
  if ! wait_for_idle 240; then
    fail "Scenario 1: Crush did not become idle (240s timeout)"
    capture_evidence 7 "orchestration-contract-s1"
    return
  fi

  # --- Parent session exists ---
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    pass "Scenario 1: Parent session exists (ID = $SID)"
  else
    fail "Scenario 1: No parent session ID found in DB"
    capture_evidence 7 "orchestration-contract-s1"
    return
  fi

  # --- Child sessions via child_sessions_by_parent query ---
  local children
  children=$(run_query "child_sessions_by_parent" "$SID")
  if [[ -z "$children" ]] || [[ "$children" == "[]" ]] || [[ "$children" == "null" ]]; then
    fail "Scenario 1: No child sessions found for parent $SID"
    capture_evidence 7 "orchestration-contract-s1"
    return
  fi

  local child_count
  child_count=$(echo "$children" | jq 'length')
  if [[ "$child_count" -ge 3 ]]; then
    pass "Scenario 1: Found $child_count child sessions (>= 3)"
  else
    fail "Scenario 1: Expected >= 3 child sessions, got $child_count"
  fi

  # --- Each child has user and assistant messages ---
  local child_ids
  child_ids=$(echo "$children" | jq -r '.[].id')
  local msg_exchange_failures=0
  while IFS= read -r cid; do
    [[ -z "$cid" ]] && continue
    local child_role_counts child_user_count child_assistant_count
    child_role_counts=$(query_db "SELECT role, COUNT(*) as count FROM messages WHERE session_id = '$cid' GROUP BY role")
    child_user_count=$(echo "$child_role_counts" | jq '[.[] | select(.role == "user")][0].count // 0')
    child_assistant_count=$(echo "$child_role_counts" | jq '[.[] | select(.role == "assistant")][0].count // 0')
    if [[ "$child_user_count" -lt 1 ]] || [[ "$child_assistant_count" -lt 1 ]]; then
      fail "Scenario 1: Child $cid missing user/assistant exchange (user=$child_user_count, assistant=$child_assistant_count)"
      msg_exchange_failures=$((msg_exchange_failures + 1))
    fi
  done <<< "$child_ids"
  if [[ "$msg_exchange_failures" -eq 0 ]]; then
    pass "Scenario 1: Every child session contains a user/assistant exchange"
  fi

  # --- Each child result includes a numeric count ---
  local numeric_count_failures=0
  while IFS= read -r cid; do
    [[ -z "$cid" ]] && continue
    local child_assistant_text
    child_assistant_text=$(query_db "SELECT mp.content_json FROM message_parts mp JOIN messages m ON m.id = mp.message_id WHERE m.session_id = '$cid' AND m.role = 'assistant' AND mp.part_type = 'text' LIMIT 1")
    # Look for at least one digit in the assistant response (a numeric count).
    local has_number
    has_number=$(echo "$child_assistant_text" | jq -r '.content // ""' | grep -cE '[0-9]+' || echo 0)
    if [[ "$has_number" -lt 1 ]]; then
      fail "Scenario 1: Child $cid assistant response does not contain a numeric count"
      numeric_count_failures=$((numeric_count_failures + 1))
    fi
  done <<< "$child_ids"
  if [[ "$numeric_count_failures" -eq 0 ]]; then
    pass "Scenario 1: Every child result includes a numeric count"
  fi

  # --- Parent final answer includes all package names ---
  local parent_text
  parent_text=$(query_db "SELECT mp.content_json FROM message_parts mp JOIN messages m ON m.id = mp.message_id WHERE m.session_id = '$SID' AND m.role = 'assistant' AND mp.part_type = 'text' ORDER BY m.created_at DESC LIMIT 1")
  local parent_content
  parent_content=$(echo "$parent_text" | jq -r '.content // ""' | tr '[:upper:]' '[:lower:]')
  local pkg_names=("lcm" "repomap" "treesitter")
  local missing_pkgs=()
  for pkg in "${pkg_names[@]}"; do
    if [[ "$parent_content" != *"$pkg"* ]]; then
      missing_pkgs+=("$pkg")
    fi
  done
  if [[ ${#missing_pkgs[@]} -eq 0 ]]; then
    pass "Scenario 1: Parent final answer includes all package names (lcm, repomap, treesitter)"
  else
    fail "Scenario 1: Parent final answer missing packages: ${missing_pkgs[*]}"
  fi

  capture_evidence 7 "orchestration-contract-s1"
  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: Partial failure — one subtask uses an impossible path
# ---------------------------------------------------------------------------
test_orchestration_contract_partial_failure() {
  echo "=== Scenario 2: Partial failure — impossible path in one subtask ==="

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

  start_crush 4
  send_prompt "Use parallel sub-agents. Count Go files under internal/lcm, internal/repomap, and internal/THIS_PATH_DOES_NOT_EXIST_AT_ALL. Return a final table."
  if ! wait_for_idle 240; then
    fail "Scenario 2: Crush did not become idle (240s timeout)"
    capture_evidence 7 "orchestration-contract-s2"
    return
  fi

  # --- Parent session exists ---
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    pass "Scenario 2: Parent session exists (ID = $SID)"
  else
    fail "Scenario 2: No parent session ID found in DB"
    capture_evidence 7 "orchestration-contract-s2"
    return
  fi

  # --- Child sessions exist ---
  local children
  children=$(run_query "child_sessions_by_parent" "$SID")
  if [[ -z "$children" ]] || [[ "$children" == "[]" ]] || [[ "$children" == "null" ]]; then
    fail "Scenario 2: No child sessions found for parent $SID"
    capture_evidence 7 "orchestration-contract-s2"
    return
  fi

  local child_count
  child_count=$(echo "$children" | jq 'length')
  if [[ "$child_count" -ge 1 ]]; then
    pass "Scenario 2: Found $child_count child session(s) (>= 1)"
  else
    fail "Scenario 2: Expected >= 1 child sessions, got $child_count"
  fi

  # --- Parent reports partial success ---
  # The parent response should acknowledge the task was partially completed.
  local parent_text
  parent_text=$(query_db "SELECT mp.content_json FROM message_parts mp JOIN messages m ON m.id = mp.message_id WHERE m.session_id = '$SID' AND m.role = 'assistant' AND mp.part_type = 'text' ORDER BY m.created_at DESC LIMIT 1")
  local parent_content
  parent_content=$(echo "$parent_text" | jq -r '.content // ""' | tr '[:upper:]' '[:lower:]')
  # Look for partial-failure indicators: words like "partial", "could not",
  # "not found", "unable", "error", or "missing".
  local partial_indicators
  partial_indicators=$(echo "$parent_content" | grep -ciE "partial|could not|not found|unable|error|missing|does not exist|failed" || echo 0)
  if [[ "$partial_indicators" -ge 1 ]]; then
    pass "Scenario 2: Parent reports partial success (found $partial_indicators indicator(s))"
  else
    fail "Scenario 2: Parent response does not indicate partial success"
  fi

  # --- Successful child results still included ---
  # At least one valid package (lcm or repomap) should appear in the parent
  # response, demonstrating that successful subtasks were incorporated.
  local valid_pkgs=("lcm" "repomap")
  local found_valid=0
  for pkg in "${valid_pkgs[@]}"; do
    if [[ "$parent_content" == *"$pkg"* ]]; then
      found_valid=$((found_valid + 1))
    fi
  done
  if [[ "$found_valid" -ge 1 ]]; then
    pass "Scenario 2: Parent includes results from successful children ($found_valid valid package(s) found)"
  else
    fail "Scenario 2: Parent does not include results from any successful children"
  fi

  capture_evidence 7 "orchestration-contract-s2"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_orchestration_contract_success
test_orchestration_contract_partial_failure

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
