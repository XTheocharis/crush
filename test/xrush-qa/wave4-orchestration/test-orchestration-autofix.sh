#!/usr/bin/env bash
# Test: Orchestration × Autofix cross-feature integration.
# Verifies that parallel sub-agents integrate with the autofix cycle:
# one child creates a Go file with a deliberate lint error (unused variable),
# triggers autofix (diag_autofix / diag_gate tool calls), and the parent
# synthesizes the fixed result. The final Go file must be syntactically
# correct (gofmt check).
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

# Temporary Go file path that the child agent will create and autofix.
GOFILE=""

# ---------------------------------------------------------------------------
# Scenario: Parallel sub-agents with autofix — child creates bad Go, autofix
# fixes it, parent synthesizes the corrected result.
# ---------------------------------------------------------------------------
test_orchestration_autofix() {
  echo "=== Scenario: Parallel sub-agents with autofix cycle ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    # Clean up the temporary Go file if created.
    if [[ -n "$GOFILE" ]] && [[ -f "$GOFILE" ]]; then
      rm -f "$GOFILE"
    fi
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

  # Ask Crush to use parallel sub-agents. One child should create a Go file
  # with a deliberate lint error (unused variable), then autofix it.
  send_prompt "Use parallel sub-agents. One sub-agent should create a file called cmd/autofix_demo/main.go with a 'package main', a func main(), and an unused variable like 'x := 42' that will trigger a go vet error. Then run autofix to fix the lint error. The other sub-agent should list Go files in internal/agent. Return a final summary of both tasks."
  if ! wait_for_idle 300; then
    fail "Scenario: Crush did not become idle (300s timeout)"
    capture_evidence 7 "orchestration-autofix"
    return
  fi

  # --- Parent session exists ---
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    pass "Scenario: Parent session exists (ID = $SID)"
  else
    fail "Scenario: No parent session ID found in DB"
    capture_evidence 7 "orchestration-autofix"
    return
  fi

  # --- ≥1 child sessions via child_sessions_by_parent query ---
  local children
  children=$(run_query "child_sessions_by_parent" "$SID")
  if [[ -z "$children" ]] || [[ "$children" == "[]" ]] || [[ "$children" == "null" ]]; then
    fail "Scenario: No child sessions found for parent $SID"
    capture_evidence 7 "orchestration-autofix"
    return
  fi

  local child_count
  child_count=$(echo "$children" | jq 'length')
  if [[ "$child_count" -ge 1 ]]; then
    pass "Scenario: Found $child_count child session(s) (>= 1)"
  else
    fail "Scenario: Expected >= 1 child sessions, got $child_count"
  fi

  # --- Child session has autofix evidence (diagnostics or vet tool calls) ---
  # Search message_parts across all child sessions for tool_call entries
  # referencing diag_autofix, diag_gate, or vet-related tool names.
  local child_ids
  child_ids=$(echo "$children" | jq -r '.[].id')
  local autofix_found=0
  while IFS= read -r cid; do
    [[ -z "$cid" ]] && continue
    # Check for autofix-related tool calls in message_parts.
    local tool_calls
    tool_calls=$(query_db "SELECT content_json FROM message_parts WHERE session_id = '$cid' AND part_type = 'tool_call'")
    local autofix_match
    autofix_match=$(echo "$tool_calls" | jq -r '.[].name // empty' 2>/dev/null | grep -ciE "diag_autofix|diag_gate|autofix|diagnostic" || echo 0)
    if [[ "$autofix_match" -ge 1 ]]; then
      autofix_found=$((autofix_found + 1))
    fi

    # Also check for text parts mentioning diagnostics or lint fixing.
    local child_texts
    child_texts=$(query_db "SELECT content_json FROM message_parts mp JOIN messages m ON m.id = mp.message_id WHERE m.session_id = '$cid' AND m.role = 'assistant' AND mp.part_type = 'text'")
    local text_match
    text_match=$(echo "$child_texts" | jq -r '.[].content // empty' 2>/dev/null | grep -ciE "diag|vet|lint|unused|autofix|fix" || echo 0)
    if [[ "$text_match" -ge 1 ]]; then
      autofix_found=$((autofix_found + 1))
    fi
  done <<< "$child_ids"

  if [[ "$autofix_found" -ge 1 ]]; then
    pass "Scenario: Child session has autofix evidence ($autofix_found indicator(s))"
  else
    # Soft failure: autofix may have been implicit; don't hard-fail the test.
    fail "Scenario: No autofix evidence found in child sessions (tool_calls or text)"
  fi

  # --- Parent final answer includes the fixed result ---
  local parent_text
  parent_text=$(query_db "SELECT mp.content_json FROM message_parts mp JOIN messages m ON m.id = mp.message_id WHERE m.session_id = '$SID' AND m.role = 'assistant' AND mp.part_type = 'text' ORDER BY m.created_at DESC LIMIT 1")
  local parent_content
  parent_content=$(echo "$parent_text" | jq -r '.content // ""' | tr '[:upper:]' '[:lower:]')

  # Check that the parent response mentions the autofix demo file or fix-related keywords.
  local fix_indicators
  fix_indicators=$(echo "$parent_content" | grep -ciE "autofix_demo|main\.go|fixed|unused|lint|variable|diagnostic" || echo 0)
  if [[ "$fix_indicators" -ge 1 ]]; then
    pass "Scenario: Parent final answer includes fixed result ($fix_indicators indicator(s))"
  else
    fail "Scenario: Parent final answer does not mention autofix result"
  fi

  # --- Final Go file is syntactically correct (gofmt check) ---
  GOFILE="cmd/autofix_demo/main.go"
  if [[ -f "$GOFILE" ]]; then
    local gofmt_output
    if gofmt -e "$GOFILE" >/dev/null 2>&1; then
      pass "Scenario: Final Go file ($GOFILE) is syntactically correct (gofmt)"
    else
      gofmt_output=$(gofmt -e "$GOFILE" 2>&1 || true)
      fail "Scenario: Final Go file ($GOFILE) has syntax errors: $gofmt_output"
    fi
  else
    # The file may have been created under a slightly different path; search
    # for any main.go under cmd/ that was created during this session.
    local candidate
    candidate=$(find cmd -name 'main.go' -newer .crush -type f 2>/dev/null | head -1)
    if [[ -n "$candidate" ]]; then
      GOFILE="$candidate"
      if gofmt -e "$GOFILE" >/dev/null 2>&1; then
        pass "Scenario: Final Go file ($GOFILE) is syntactically correct (gofmt)"
      else
        gofmt_output=$(gofmt -e "$GOFILE" 2>&1 || true)
        fail "Scenario: Final Go file ($GOFILE) has syntax errors: $gofmt_output"
      fi
    else
      fail "Scenario: Expected Go file not found (tried $GOFILE and cmd/*/main.go)"
    fi
  fi

  capture_evidence 7 "orchestration-autofix"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_orchestration_autofix

echo ""
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
exit "$FAIL"
