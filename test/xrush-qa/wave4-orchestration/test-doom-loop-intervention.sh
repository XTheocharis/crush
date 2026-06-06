#!/usr/bin/env bash
# Test: Doom-loop intervention stops repeated failures.
# Verifies that Crush detects identical failing tool calls targeting an
# impossible path (/proc/impossible/qa-test.txt), triggers intervention,
# and then stops repeating or changes strategy. Also verifies the target
# file does NOT exist on the filesystem.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# The impossible path that the LLM will be asked to write to.
# /proc is a virtual filesystem — writes always fail.
IMPOSSIBLE_PATH="/proc/impossible/qa-test.txt"

# Doom-loop thresholds from DefaultDoomLoopThresholds (doom.go).
# Soft=3, Medium=5, Hard=7. Intervention triggers at soft threshold.
DOOM_SOFT_THRESHOLD=3

# ---------------------------------------------------------------------------
# Scenario 1: Identical failing tool calls reach threshold before intervention
# ---------------------------------------------------------------------------
assert_tool_call_threshold_reached() {
  local session_id="$1"
  local min_count="$2"

  # Count tool_call parts that reference the impossible path.
  local tool_call_count
  tool_call_count=$(query_db \
    "SELECT COUNT(*) as count FROM message_parts
     WHERE session_id = '$session_id'
       AND part_type = 'tool_call'
       AND content_json LIKE '%${IMPOSSIBLE_PATH}%'" \
    | jq '.[0].count')

  if [[ "$tool_call_count" -ge "$min_count" ]]; then
    pass "Scenario 1: Found $tool_call_count tool calls referencing ${IMPOSSIBLE_PATH} (>= threshold $min_count)"
  else
    fail "Scenario 1: Only $tool_call_count tool calls referencing ${IMPOSSIBLE_PATH} (expected >= $min_count)"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: After threshold, repeats stop or strategy changes
# ---------------------------------------------------------------------------
assert_intervention_stops_repeats() {
  local session_id="$1"

  # Count tool_call parts before and after intervention.
  # The total tool calls referencing the impossible path should not vastly
  # exceed the hard threshold (7). If doom-loop intervention works, the
  # agent stops retrying after escalation.
  local total_calls
  total_calls=$(query_db \
    "SELECT COUNT(*) as count FROM message_parts
     WHERE session_id = '$session_id'
       AND part_type = 'tool_call'
       AND content_json LIKE '%${IMPOSSIBLE_PATH}%'" \
    | jq '.[0].count')

  # Hard threshold is 7; allow some slack but not unbounded growth.
  local hard_cap=10
  if [[ "$total_calls" -le "$hard_cap" ]]; then
    pass "Scenario 2: Total tool calls ($total_calls) capped at <= $hard_cap after intervention"
  else
    fail "Scenario 2: Total tool calls ($total_calls) exceeded hard cap $hard_cap — intervention may not have fired"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 3: Final assistant message reports change/abort
# ---------------------------------------------------------------------------
assert_final_message_reports_abort() {
  local session_id="$1"

  # Get the last assistant message content.
  local last_msg
  last_msg=$(query_db \
    "SELECT content FROM messages
     WHERE session_id = '$session_id'
       AND role = 'assistant'
     ORDER BY created_at DESC LIMIT 1" \
    | jq -r '.[0].content // empty' 2>/dev/null || echo "")

  # Check for keywords indicating the agent changed strategy or aborted.
  if echo "$last_msg" | grep -qiE "unable|cannot|failed|abort|impossible|different|change|unable to|won't|can't|error|not possible"; then
    pass "Scenario 3: Final assistant message acknowledges failure or strategy change"
  else
    fail "Scenario 3: Final assistant message does not indicate abort/strategy change"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 4: Target impossible file does NOT exist
# ---------------------------------------------------------------------------
assert_target_file_not_exists() {
  if [[ -e "$IMPOSSIBLE_PATH" ]]; then
    fail "Scenario 4: Target file ${IMPOSSIBLE_PATH} unexpectedly exists"
  else
    pass "Scenario 4: Target file ${IMPOSSIBLE_PATH} does NOT exist (as expected)"
  fi
}

# ---------------------------------------------------------------------------
# Main test: Doom-loop intervention on impossible write path
# ---------------------------------------------------------------------------
test_doom_loop_intervention() {
  echo "=== Doom-loop intervention on impossible write path ==="

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

  # Wave 4 config has doom_loop_intervention: "full" and aggressive thresholds.
  start_crush 4

  # Prompt targeting an impossible file — forces repeated edit/write failures.
  send_prompt "Write the text 'doom loop qa test' to the file ${IMPOSSIBLE_PATH}. Keep trying until it succeeds."

  # Allow up to 180s; repeated failures trigger doom-loop detection and intervention.
  if ! wait_for_idle 180; then
    fail "Crush did not become idle within 180s"
    capture_evidence 6 "doom-loop-intervention"
    return
  fi

  # Get session ID for DB queries.
  local session_id
  session_id=$(get_session_id)

  if [[ -z "$session_id" ]]; then
    fail "Could not retrieve session ID from database"
    capture_evidence 6 "doom-loop-intervention"
    return
  fi

  echo "--- Session ID: $session_id ---"

  # Run all assertion scenarios.
  assert_tool_call_threshold_reached "$session_id" "$DOOM_SOFT_THRESHOLD"
  assert_intervention_stops_repeats "$session_id"
  assert_final_message_reports_abort "$session_id"
  assert_target_file_not_exists

  # Capture log evidence for debugging.
  echo "--- Doom-loop intervention log evidence ---"
  grep -iE "doom.?loop|loop.?detection|repetition.*score|escalation.*level|intervention.*force|abort" .crush/logs/crush.log 2>/dev/null | head -15 || true
  echo "--- End evidence ---"

  # Dump tool call details for debugging.
  echo "--- Tool calls referencing impossible path ---"
  query_db \
    "SELECT id, part_type, substr(content_json, 1, 200) as snippet
     FROM message_parts
     WHERE session_id = '$session_id'
       AND part_type = 'tool_call'
       AND content_json LIKE '%${IMPOSSIBLE_PATH}%'" 2>/dev/null || true
  echo "--- End tool calls ---"

  capture_evidence 6 "doom-loop-intervention"
  stop_crush
}

# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------
test_doom_loop_intervention

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
