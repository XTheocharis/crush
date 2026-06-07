#!/usr/bin/env bash
# Test: Orchestration × Autofix cross-feature integration (TUI-first approach).
# Verifies that parallel sub-agents integrate with the autofix cycle:
# one child creates a Go file with a deliberate lint error (unused variable),
# triggers autofix (diag_autofix / diag_gate tool calls), and the parent
# synthesizes the fixed result. The final Go file must be syntactically
# correct (gofmt check).
set -euo pipefail

WAVE=4
SCENARIO="orchestration-autofix"

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

cleanup_test() { cleanup_tui; restore_crush; }
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Parallel sub-agents with autofix — child creates bad Go,
# autofix fixes it, parent synthesizes the corrected result.
# ---------------------------------------------------------------------------
test_orchestration_autofix() {
  echo "=== Scenario 1: Parallel sub-agents with autofix cycle ==="
  SCENARIO="orchestration-autofix-parallel"
  local GOFILE="cmd/autofix_demo/main.go"

  setup_clean_crush
  start_crush_tui "$WAVE"
  focus_editor

  # Ask Crush to use parallel sub-agents. One child should create a Go file
  # with a deliberate lint error (unused variable), then autofix it.
  send_tui_prompt "Use parallel sub-agents. One sub-agent should create a file called cmd/autofix_demo/main.go with a 'package main', a func main(), and an unused variable like 'x := 42' that will trigger a go vet error. Then run autofix to fix the lint error. The other sub-agent should list Go files in internal/agent. After both sub-agents complete, reply with exactly ORCH_AUTOFIX_COMPLETE_SENTINEL_88 and provide a summary of both tasks."

  if ! wait_for_tui_idle 300; then
    fail "Scenario 1: Crush did not become idle (300s timeout)"
    capture_tui_evidence "orchestration-autofix-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "ORCH_AUTOFIX_COMPLETE_SENTINEL_88"; then
    pass "Scenario 1: TUI shows ORCH_AUTOFIX_COMPLETE_SENTINEL_88 sentinel"
  else
    fail "Scenario 1: TUI does not show ORCH_AUTOFIX_COMPLETE_SENTINEL_88 sentinel"
    capture_tui_evidence "sentinel-missing"
    return
  fi

  capture_tui_evidence "orchestration-autofix-response"

  # --- Secondary DB check: parent session exists ---
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    pass "Scenario 1: Parent session exists (ID = $SID)"
  else
    fail "Scenario 1: No parent session ID found in DB"
    return
  fi

  # --- Secondary DB check: >= 1 child sessions ---
  local children
  children=$(run_query "child_sessions_by_parent" "$SID")
  if [[ -z "$children" ]] || [[ "$children" == "[]" ]] || [[ "$children" == "null" ]]; then
    fail "Scenario 1: No child sessions found for parent $SID"
  else
    local child_count
    child_count=$(echo "$children" | jq 'length')
    if [[ "$child_count" -ge 1 ]]; then
      pass "Scenario 1: Found $child_count child session(s) (>= 1)"
    else
      fail "Scenario 1: Expected >= 1 child sessions, got $child_count"
    fi
  fi

  # --- Secondary DB check: child session has autofix evidence ---
  local child_ids
  child_ids=$(echo "$children" | jq -r '.[].id')
  local autofix_found=0
  while IFS= read -r cid; do
    [[ -z "$cid" ]] && continue
    local tool_calls
    tool_calls=$(query_db "SELECT content_json FROM message_parts WHERE session_id = '$cid' AND part_type = 'tool_call'")
    local autofix_match
    autofix_match=$(echo "$tool_calls" | jq -r '.[].name // empty' 2>/dev/null | grep -ciE "diag_autofix|diag_gate|autofix|diagnostic" ) || autofix_match=0
    if [[ "$autofix_match" -ge 1 ]]; then
      autofix_found=$((autofix_found + 1))
    fi

    local child_texts
    child_texts=$(query_db "SELECT content_json FROM message_parts mp JOIN messages m ON m.id = mp.message_id WHERE m.session_id = '$cid' AND m.role = 'assistant' AND mp.part_type = 'text'")
    local text_match
    text_match=$(echo "$child_texts" | jq -r '.[].content // empty' 2>/dev/null | grep -ciE "diag|vet|lint|unused|autofix|fix" ) || text_match=0
    if [[ "$text_match" -ge 1 ]]; then
      autofix_found=$((autofix_found + 1))
    fi
  done <<< "$child_ids"

  if [[ "$autofix_found" -ge 1 ]]; then
    pass "Scenario 1: Child session has autofix evidence ($autofix_found indicator(s))"
  else
    fail "Scenario 1: No autofix evidence found in child sessions"
  fi

  # --- Secondary filesystem check: final Go file is syntactically correct ---
  if [[ -f "$GOFILE" ]]; then
    if gofmt -e "$GOFILE" >/dev/null 2>&1; then
      pass "Scenario 1: Final Go file ($GOFILE) is syntactically correct (gofmt)"
    else
      local gofmt_output
      gofmt_output=$(gofmt -e "$GOFILE" 2>&1 || true)
      fail "Scenario 1: Final Go file ($GOFILE) has syntax errors: $gofmt_output"
    fi
  else
    # The file may have been created under a slightly different path.
    local candidate
    candidate=$(find cmd -name 'main.go' -newer .crush -type f 2>/dev/null | head -1)
    if [[ -n "$candidate" ]]; then
      if gofmt -e "$candidate" >/dev/null 2>&1; then
        pass "Scenario 1: Final Go file ($candidate) is syntactically correct (gofmt)"
      else
        local gofmt_output
        gofmt_output=$(gofmt -e "$candidate" 2>&1 || true)
        fail "Scenario 1: Final Go file ($candidate) has syntax errors: $gofmt_output"
      fi
    else
      fail "Scenario 1: Expected Go file not found (tried $GOFILE and cmd/*/main.go)"
    fi
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: Operator-based autofix — operator decomposes task and autofix
# runs within the orchestration flow.
# ---------------------------------------------------------------------------
test_operator_autofix() {
  echo "=== Scenario 2: Operator-based autofix flow ==="
  SCENARIO="operator-autofix"
  local GOFILE="/tmp/qa-operator-autofix.go"

  setup_clean_crush
  start_crush_tui "$WAVE"
  focus_editor

  # Ask Crush to use operator decomposition to create and fix a Go file.
  send_tui_prompt "Use the operator to decompose this task into subtasks: First, create a file /tmp/qa-operator-autofix.go with a deliberate syntax error: package main; func main() { y := 10 (missing closing brace). Then fix the file. Reply with exactly ORCH_AUTOFIX_OP_SENTINEL_55 when complete."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle (operator autofix timeout)"
    capture_tui_evidence "operator-autofix-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "ORCH_AUTOFIX_OP_SENTINEL_55"; then
    pass "Scenario 2: TUI shows ORCH_AUTOFIX_OP_SENTINEL_55 sentinel"
  else
    fail "Scenario 2: TUI does not show ORCH_AUTOFIX_OP_SENTINEL_55 sentinel"
    capture_tui_evidence "operator-sentinel-missing"
    return
  fi

  capture_tui_evidence "operator-autofix-response"

  # --- Secondary filesystem check: file exists and is valid Go ---
  if [[ ! -f "$GOFILE" ]]; then
    fail "Scenario 2: $GOFILE was not created"
  elif gofmt -e "$GOFILE" >/dev/null 2>&1; then
    pass "Scenario 2: $GOFILE is syntactically valid Go after autofix"
  else
    fail "Scenario 2: $GOFILE is still syntactically invalid after autofix"
  fi

  # --- Secondary log check: orchestration + diagnostic tooling ---
  local orch_diag_count
  orch_diag_count=$(grep -ciE "operator|parallel|sub.?agent|diag_autofix|autofix|vet" .crush/logs/crush.log 2>/dev/null ) || orch_diag_count=0
  if [[ "$orch_diag_count" -gt 0 ]]; then
    pass "Scenario 2: Orchestration + diagnostic pipeline ran ($orch_diag_count log matches)"
  else
    fail "Scenario 2: No orchestration/diagnostic log matches found"
  fi

  # Clean up temp file.
  rm -f "$GOFILE"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_orchestration_autofix
test_operator_autofix

echo ""
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
exit "$FAIL"
