#!/usr/bin/env bash
# Test: Message parts types, timestamps, and sequential part_index.
# Verifies that message_parts records text, tool_call, and tool_result parts
# when the agent reads go.mod, and that part_index values are sequential.
# TUI-first: primary gate is sentinel in TUI output; DB checks are secondary.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: Message parts have correct types (text, tool_call, tool_result)
# ---------------------------------------------------------------------------
test_message_parts_types() {
  echo "=== Scenario 1: Message parts have correct types ==="
  export WAVE=1
  export SCENARIO="message-parts-types"

  setup_clean_crush
  # shellcheck disable=SC2317
  cleanup_test() {
    cleanup_tui
    restore_crush
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
  }
  trap cleanup_test EXIT

  start_crush_tui 1
  focus_editor
  send_tui_prompt "Read the file go.mod and reply with exactly: MSG_PARTS_SENTINEL_charmbracelet_crush"
  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "message-parts-types"
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "MSG_PARTS_SENTINEL_charmbracelet_crush"; then
    pass "Scenario 1: TUI output contains MSG_PARTS_SENTINEL_charmbracelet_crush"
  else
    fail "Scenario 1: TUI output does not contain MSG_PARTS_SENTINEL_charmbracelet_crush"
    capture_tui_evidence "message-parts-types"
    return
  fi

  capture_tui_evidence "message-parts-types"

  # Secondary DB checks for message part types.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    return
  fi

  local part_types
  part_types=$(query_db "SELECT DISTINCT part_type FROM message_parts WHERE session_id='$SID' ORDER BY part_type")

  echo "Part types found: $part_types"

  if echo "$part_types" | grep -q "text"; then
    pass "message_parts contains 'text' part type"
  else
    fail "message_parts missing 'text' part type"
  fi

  if echo "$part_types" | grep -q "tool_call"; then
    pass "message_parts contains 'tool_call' part type"
  else
    fail "message_parts missing 'tool_call' part type for go.mod read request"
  fi

  if echo "$part_types" | grep -q "tool_result"; then
    pass "message_parts contains 'tool_result' part type"
  else
    fail "message_parts missing 'tool_result' part type for go.mod read request"
  fi

  local gomod_tool_parts
  gomod_tool_parts=$(query_db "SELECT COUNT(*) as cnt FROM message_parts WHERE session_id='$SID' AND part_type IN ('tool_call', 'tool_result') AND content_json LIKE '%go.mod%'" | jq '.[0].cnt')
  if [[ "$gomod_tool_parts" -ge 1 ]]; then
    pass "message_parts records go.mod in tool call/result content"
  else
    fail "message_parts did not record go.mod in tool call/result content"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: Messages have valid timestamps and sequential part_index
# ---------------------------------------------------------------------------
test_message_parts_timestamps() {
  echo "=== Scenario 2: Messages have valid timestamps ==="
  export WAVE=1
  export SCENARIO="message-parts-timestamps"

  setup_clean_crush
  # shellcheck disable=SC2317
  cleanup_test() {
    cleanup_tui
    restore_crush
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
  }
  trap cleanup_test EXIT

  start_crush_tui 1
  focus_editor
  send_tui_prompt "Read the file go.mod and reply with exactly: MSG_PARTS_SENTINEL_charmbracelet_crush"
  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle"
    capture_tui_evidence "message-parts-timestamps"
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "MSG_PARTS_SENTINEL_charmbracelet_crush"; then
    pass "Scenario 2: TUI output contains MSG_PARTS_SENTINEL_charmbracelet_crush"
  else
    fail "Scenario 2: TUI output does not contain MSG_PARTS_SENTINEL_charmbracelet_crush"
    capture_tui_evidence "message-parts-timestamps"
    return
  fi

  capture_tui_evidence "message-parts-timestamps"

  # Secondary DB checks.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID found in DB"
    return
  fi

  local ts_count
  ts_count=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id='$SID' AND created_at > 0" | jq '.[0].cnt')

  if [[ "$ts_count" -ge 2 ]]; then
    pass "messages with valid timestamps >= 2 (got $ts_count)"
  else
    fail "messages with valid timestamps < 2 (got $ts_count)"
  fi

  # Check part_index values are sequential per message.
  local msg_ids
  msg_ids=$(sqlite3 .crush/crush.db \
    "SELECT DISTINCT message_id FROM message_parts WHERE session_id='$SID' ORDER BY message_id")

  local seq_ok=true
  while IFS= read -r mid; do
    [[ -z "$mid" ]] && continue
    local parts
    parts=$(sqlite3 .crush/crush.db \
      "SELECT part_index FROM message_parts WHERE session_id='$SID' AND message_id=$mid ORDER BY part_index")

    local expected=0
    while IFS= read -r idx; do
      [[ -z "$idx" ]] && continue
      if [[ "$idx" -ne "$expected" ]]; then
        fail "message_id=$mid: expected part_index=$expected, got $idx"
        seq_ok=false
        break
      fi
      expected=$((expected + 1))
    done <<< "$parts"
  done <<< "$msg_ids"

  if [[ "$seq_ok" == "true" ]]; then
    pass "part_index values are sequential per message"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_message_parts_types
test_message_parts_timestamps

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
