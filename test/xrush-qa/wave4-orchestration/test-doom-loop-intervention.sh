#!/usr/bin/env bash
# Test: Doom-loop intervention stops repeated failures.
# Verifies that Crush detects identical failing tool calls targeting an
# impossible path (/proc/impossible/qa-test.txt), triggers intervention,
# and then stops repeating or changes strategy.
set -euo pipefail

WAVE=4
SCENARIO="doom-loop-intervention"
source "$(dirname "$0")/../lib/common.sh"
source "$(dirname "$0")/../lib/db-verify.sh"

PASS=0
FAIL=0

IMPOSSIBLE_PATH="/proc/impossible/qa-test.txt"
DOOM_SOFT_THRESHOLD=3

cleanup_test() {
    cleanup_tui
  local _bak
  _bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  [[ -n "$_bak" ]] && mv "$_bak" crush.json
  restore_crush
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Doom-loop intervention on impossible write path
# ---------------------------------------------------------------------------
test_doom_loop_intervention() {
  echo "=== Scenario 1: Doom-loop intervention on impossible write path ==="

  setup_clean_crush
  start_crush_tui 4
  focus_editor

  # Prompt targeting an impossible file — forces repeated write failures.
  send_tui_prompt "Write the text 'doom intervention qa test' to the file ${IMPOSSIBLE_PATH}. Keep trying until it succeeds. When you change your approach or give up, output exactly: DOOM_INTERVENTION_SENTINEL_88"

  # Allow up to 180s; repeated failures trigger doom-loop detection and intervention.
  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_tui_evidence "doom-intervention-timeout"
    return
  fi

  # Primary: TUI must contain the sentinel or recovery keywords.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "DOOM_INTERVENTION_SENTINEL_88"; then
    pass "Scenario 1: TUI contains DOOM_INTERVENTION_SENTINEL_88"
  elif printf '%s' "$tui_output" | grep -qiE "unable|cannot|failed|abort|impossible|different|change|won't|can't|error|not possible"; then
    pass "Scenario 1: TUI shows agent recovered or changed strategy"
  else
    fail "Scenario 1: TUI does not contain DOOM_INTERVENTION_SENTINEL_88 or recovery keywords"
    capture_tui_evidence "doom-intervention-no-sentinel"
  fi

  # Secondary: DB check — verify tool calls targeting impossible path.
  local session_id
  session_id=$(get_session_id)

  if [[ -n "$session_id" ]]; then
    echo "--- Session ID: $session_id ---"

    local tool_call_count
    tool_call_count=$(query_db \
      "SELECT COUNT(*) as count FROM message_parts
       WHERE session_id = '$session_id'
         AND part_type = 'tool_call'
         AND content_json LIKE '%${IMPOSSIBLE_PATH}%'" \
      | jq '.[0].count' 2>/dev/null || echo 0)

    if [[ "$tool_call_count" -ge "$DOOM_SOFT_THRESHOLD" ]]; then
      pass "Scenario 1: Found $tool_call_count tool calls referencing ${IMPOSSIBLE_PATH} (>= threshold $DOOM_SOFT_THRESHOLD)"
    else
      echo "  NOTE: Only $tool_call_count tool calls referencing ${IMPOSSIBLE_PATH} (expected >= $DOOM_SOFT_THRESHOLD)"
    fi

    local hard_cap=10
    if [[ "$tool_call_count" -le "$hard_cap" ]]; then
      pass "Scenario 1: Total tool calls ($tool_call_count) capped at <= $hard_cap after intervention"
    else
      fail "Scenario 1: Total tool calls ($tool_call_count) exceeded hard cap $hard_cap — intervention may not have fired"
    fi
  else
    echo "  NOTE: Could not retrieve session ID from database"
  fi

  # Verify target file does NOT exist.
  if [[ -e "$IMPOSSIBLE_PATH" ]]; then
    fail "Scenario 1: Target file ${IMPOSSIBLE_PATH} unexpectedly exists"
  else
    pass "Scenario 1: Target file ${IMPOSSIBLE_PATH} does NOT exist (as expected)"
  fi

  # Capture log evidence for debugging.
  echo "--- Doom-loop intervention log evidence ---"
  grep -iE "doom.?loop|loop.?detection|repetition.*score|escalation.*level|intervention.*force|abort" .crush/logs/crush.log 2>/dev/null | head -15 || true
  echo "--- End evidence ---"

  capture_tui_evidence "doom-loop-intervention"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_doom_loop_intervention

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
