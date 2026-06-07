#!/usr/bin/env bash
# Test: Architect plan decomposes into operator execution.
# Verifies end-to-end flow: architect produces a plan, operator decomposes it
# into child tasks, child sessions execute, and final files exist and pass
# gofmt syntax check.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
SKIP=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }
skip() { echo "SKIP: $1"; ((SKIP += 1)); }

# Main session ID captured after scenario.
SID=""

# Target directory for generated Go package (outside workspace to avoid
# polluting the repo).
TARGET_DIR="/tmp/qa-archop-$$"

# ---------------------------------------------------------------------------
# Scenario: Architect plans, operator decomposes, children execute, files
# ---------------------------------------------------------------------------
test_architect_operator_pipeline() {
  echo "=== Scenario: Architect plan → operator decomposition → file creation ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    rm -rf "$TARGET_DIR"
    restore_crush
  }
  trap restore_on_exit EXIT

  mkdir -p "$TARGET_DIR"

  start_crush 4
  send_prompt "Create a simple Go package in $TARGET_DIR with 2 files: a types.go file with a struct, and a methods.go file with a method on that struct. Use the architect to plan, then operator to execute."
  if ! wait_for_idle 300; then
    fail "Scenario: Crush did not become idle (300s timeout)"
    capture_evidence 42 "architect-operator"
    return
  fi

  # --- Parent session exists ---
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    pass "Parent session exists (ID = $SID)"
  else
    fail "No parent session ID found in DB"
    capture_evidence 42 "architect-operator"
    return
  fi

  # --- Architect plan evidence in assistant messages ---
  local parent_assistant_text
  parent_assistant_text=$(query_db "SELECT mp.content_json FROM message_parts mp JOIN messages m ON m.id = mp.message_id WHERE m.session_id = '$SID' AND m.role = 'assistant' AND mp.part_type = 'text' ORDER BY m.created_at DESC LIMIT 5")
  local plan_keywords
  plan_keywords=$(echo "$parent_assistant_text" | jq -r '.[].content // ""' | grep -ciE "plan|step|task|file.*types\.go|file.*methods\.go" || echo 0)
  if [[ "$plan_keywords" -ge 1 ]]; then
    pass "Architect plan evidence found in assistant messages ($plan_keywords keyword match(es))"
  else
    fail "No architect plan evidence found in assistant messages"
  fi

  # --- Child sessions via child_sessions_by_parent query ---
  local children
  children=$(run_query "child_sessions_by_parent" "$SID")
  if [[ -z "$children" ]] || [[ "$children" == "[]" ]] || [[ "$children" == "null" ]]; then
    fail "No child sessions found for parent $SID"
    capture_evidence 42 "architect-operator"
    return
  fi

  local child_count
  child_count=$(echo "$children" | jq 'length')
  if [[ "$child_count" -ge 1 ]]; then
    pass "Found $child_count child session(s) via operator (>= 1)"
  else
    fail "Expected >= 1 child sessions, got $child_count"
    capture_evidence 42 "architect-operator"
    return
  fi

  # --- Each child session has scoped task and result ---
  local child_ids
  child_ids=$(echo "$children" | jq -r '.[].id')
  local scoped_failures=0
  while IFS= read -r cid; do
    [[ -z "$cid" ]] && continue

    # Check that child has at least one user message (scoped task).
    local child_user_count
    child_user_count=$(query_db "SELECT COUNT(*) as count FROM messages WHERE session_id = '$cid' AND role = 'user'" | jq '.[0].count')
    if [[ "$child_user_count" -lt 1 ]]; then
      fail "Child $cid has no user message (scoped task)"
      scoped_failures=$((scoped_failures + 1))
    fi

    # Check that child has at least one assistant message (result).
    local child_assistant_count
    child_assistant_count=$(query_db "SELECT COUNT(*) as count FROM messages WHERE session_id = '$cid' AND role = 'assistant'" | jq '.[0].count')
    if [[ "$child_assistant_count" -lt 1 ]]; then
      fail "Child $cid has no assistant message (result)"
      scoped_failures=$((scoped_failures + 1))
    fi
  done <<< "$child_ids"
  if [[ "$scoped_failures" -eq 0 ]]; then
    pass "Every child session has scoped task (user) and result (assistant)"
  fi

  # --- Final files exist ---
  local types_file="$TARGET_DIR/types.go"
  local methods_file="$TARGET_DIR/methods.go"

  if [[ -f "$types_file" ]]; then
    pass "types.go exists at $types_file"
  else
    fail "types.go not found at $types_file"
  fi

  if [[ -f "$methods_file" ]]; then
    pass "methods.go exists at $methods_file"
  else
    fail "methods.go not found at $methods_file"
  fi

  # --- Final files are syntactically correct Go (gofmt check) ---
  if [[ -f "$types_file" ]]; then
    local gofmt_types
    gofmt_types=$(gofmt -e "$types_file" 2>&1 || true)
    if echo "$gofmt_types" | grep -qiE "error|expected|unexpected"; then
      fail "types.go has Go syntax errors: $gofmt_types"
    else
      pass "types.go passes gofmt syntax check"
    fi
  fi

  if [[ -f "$methods_file" ]]; then
    local gofmt_methods
    gofmt_methods=$(gofmt -e "$methods_file" 2>&1 || true)
    if echo "$gofmt_methods" | grep -qiE "error|expected|unexpected"; then
      fail "methods.go has Go syntax errors: $gofmt_methods"
    else
      pass "methods.go passes gofmt syntax check"
    fi
  fi

  capture_evidence 42 "architect-operator"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_architect_operator_pipeline

echo ""
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
exit "$FAIL"
