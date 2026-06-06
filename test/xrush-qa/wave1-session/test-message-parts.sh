#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1"; ((FAIL += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: Message parts have correct types
# ---------------------------------------------------------------------------
test_message_parts_types() {
  echo "=== Scenario 1: Message parts have correct types ==="

  setup_clean_crush
  start_crush 1
  send_prompt "Read the file go.mod and tell me the module name"
  wait_for_idle 120

  SID=$(get_session_id)

  # Query distinct part types.
  local part_types
  part_types=$(sqlite3 .crush/crush.db \
    "SELECT DISTINCT part_type FROM message_parts WHERE session_id='$SID' ORDER BY part_type")

  echo "Part types found: $part_types"

  # Must contain 'text' at minimum (both user and assistant messages have text).
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
  gomod_tool_parts=$(sqlite3 .crush/crush.db \
    "SELECT COUNT(*) FROM message_parts WHERE session_id='$SID' AND part_type IN ('tool_call', 'tool_result') AND content_json LIKE '%go.mod%'")
  if [[ "$gomod_tool_parts" -ge 1 ]]; then
    pass "message_parts records go.mod in tool call/result content"
  else
    fail "message_parts did not record go.mod in tool call/result content"
  fi

  capture_evidence 6 "message-parts-types"
  stop_crush
  restore_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: Messages have valid timestamps and sequential part_index
# ---------------------------------------------------------------------------
test_message_parts_timestamps() {
  echo "=== Scenario 2: Messages have valid timestamps ==="

  setup_clean_crush
  start_crush 1
  send_prompt "Read the file go.mod and tell me the module name"
  wait_for_idle 120

  SID=$(get_session_id)

  # Check messages have valid timestamps (created_at > 0).
  local ts_count
  ts_count=$(sqlite3 .crush/crush.db \
    "SELECT COUNT(*) FROM messages WHERE session_id='$SID' AND created_at > 0")

  if [[ "$ts_count" -ge 2 ]]; then
    pass "messages with valid timestamps >= 2 (got $ts_count)"
  else
    fail "messages with valid timestamps < 2 (got $ts_count)"
  fi

  # Check part_index values are sequential per message.
  # Get all part_index values ordered by message_id, part_index.
  local indices
  indices=$(sqlite3 .crush/crush.db \
    "SELECT part_index FROM message_parts WHERE session_id='$SID' ORDER BY message_id, part_index")

  echo "Part indices (ordered): $indices"

  # For each message_id, verify part_index starts at 0 and increments.
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
      ((expected++))
    done <<< "$parts"
  done <<< "$msg_ids"

  if [[ "$seq_ok" == "true" ]]; then
    pass "part_index values are sequential per message"
  fi

  capture_evidence 6 "message-parts-timestamps"
  stop_crush
  restore_crush
}

# ---------------------------------------------------------------------------
# Run tests
# ---------------------------------------------------------------------------
test_message_parts_types
test_message_parts_timestamps

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
