#!/usr/bin/env bash
# Test: Architect plan decomposes into operator execution (TUI-first).
# Verifies end-to-end flow: architect produces a plan, operator decomposes it
# into child tasks, child sessions execute, and final files exist and pass
# gofmt syntax check. TUI output must show plan, execution, and
# ARCHITECT_OP_COMPLETE_SENTINEL_88.
set -euo pipefail

WAVE=4

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

TARGET_DIR="/tmp/qa-archop-$$"

# ---------------------------------------------------------------------------
# Scenario: Architect plans, operator decomposes, children execute, files
# ---------------------------------------------------------------------------
test_architect_operator_pipeline() {
  echo "=== Scenario: Architect plan -> operator decomposition -> file creation ==="
  SCENARIO="architect-operator-pipeline"

  setup_clean_crush
  cleanup_test() {
    rm -rf "$TARGET_DIR"
    cleanup_tui
    restore_crush
  }
  trap cleanup_test EXIT

  mkdir -p "$TARGET_DIR"

  start_crush_tui 4
  focus_editor
  send_tui_prompt "Create a simple Go package in $TARGET_DIR with 2 files: a types.go file with a struct called Item with Name string and Value int fields, and a methods.go file with a method Describe on Item that returns a string. Use the architect to plan, then operator to execute. After completing, include the exact token ARCHITECT_OP_COMPLETE_SENTINEL_88 in your response."

  if ! wait_for_tui_idle 300; then
    fail "Scenario: Crush did not become idle (300s timeout)"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "ARCHITECT_OP_COMPLETE_SENTINEL_88"; then
    pass "Scenario: TUI shows ARCHITECT_OP_COMPLETE_SENTINEL_88 sentinel"
  else
    fail "Scenario: TUI does not show ARCHITECT_OP_COMPLETE_SENTINEL_88 sentinel"
    capture_tui_evidence "sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary: session and child sessions ---
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    pass "Scenario: Parent session exists (ID = $SID)"
  else
    fail "Scenario: No parent session ID found in DB"
  fi

  if [[ -n "$SID" ]]; then
    local children
    children=$(run_query "child_sessions_by_parent" "$SID")
    if [[ -z "$children" ]] || [[ "$children" == "[]" ]] || [[ "$children" == "null" ]]; then
      fail "Scenario: No child sessions found for parent $SID"
    else
      local child_count
      child_count=$(echo "$children" | jq 'length')
      if [[ "$child_count" -ge 1 ]]; then
        pass "Scenario: Found $child_count child session(s) via operator (>= 1)"
      else
        fail "Scenario: Expected >= 1 child sessions, got $child_count"
      fi
    fi
  fi

  # --- Secondary: final files exist ---
  local types_file="$TARGET_DIR/types.go"
  local methods_file="$TARGET_DIR/methods.go"

  if [[ -f "$types_file" ]]; then
    pass "Scenario: types.go exists at $types_file"
  else
    fail "Scenario: types.go not found at $types_file"
  fi

  if [[ -f "$methods_file" ]]; then
    pass "Scenario: methods.go exists at $methods_file"
  else
    fail "Scenario: methods.go not found at $methods_file"
  fi

  # --- Secondary: files are syntactically correct Go ---
  if [[ -f "$types_file" ]]; then
    local gofmt_types
    gofmt_types=$(gofmt -e "$types_file" 2>&1 || true)
    if echo "$gofmt_types" | grep -qiE "error|expected|unexpected"; then
      fail "Scenario: types.go has Go syntax errors: $gofmt_types"
    else
      pass "Scenario: types.go passes gofmt syntax check"
    fi
  fi

  if [[ -f "$methods_file" ]]; then
    local gofmt_methods
    gofmt_methods=$(gofmt -e "$methods_file" 2>&1 || true)
    if echo "$gofmt_methods" | grep -qiE "error|expected|unexpected"; then
      fail "Scenario: methods.go has Go syntax errors: $gofmt_methods"
    else
      pass "Scenario: methods.go passes gofmt syntax check"
    fi
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 1
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_architect_operator_pipeline

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
