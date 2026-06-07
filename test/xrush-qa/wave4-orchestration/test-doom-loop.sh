#!/usr/bin/env bash
# Test: Doom loop detection active with full intervention.
# Verifies that Crush detects repetitive tool-call patterns and stops retrying
# when operating against a nonexistent target that forces repeated edit failures.
set -euo pipefail

WAVE=4
SCENARIO="doom-loop-detection"
source "$(dirname "$0")/../lib/common.sh"
source "$(dirname "$0")/../lib/db-verify.sh"

PASS=0
FAIL=0

cleanup_test() {
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
  local _bak
  _bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  [[ -n "$_bak" ]] && mv "$_bak" crush.json
  restore_crush
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Doom loop detection stops repeated impossible edits
# ---------------------------------------------------------------------------
test_doom_loop_detection() {
  echo "=== Scenario 1: Doom loop detection stops repeated impossible edits ==="

  setup_clean_crush
  start_crush_tui 4
  focus_editor

  # Prompt targeting a nonexistent file — forces repeated edit failures.
  # Ask Crush to output the sentinel when it gives up.
  send_tui_prompt "Edit /nonexistent/file.go to add a function called TestDoom. Keep retrying until it succeeds. When you stop trying, output exactly: DOOM_LOOP_STOPPED_SENTINEL_42"

  # Allow up to 180s; repeated failures trigger doom loop detection.
  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_tui_evidence "doom-loop-timeout"
    return
  fi

  # Primary: TUI must contain the sentinel or abort keywords.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "DOOM_LOOP_STOPPED_SENTINEL_42"; then
    pass "Scenario 1: TUI contains DOOM_LOOP_STOPPED_SENTINEL_42"
  elif printf '%s' "$tui_output" | grep -qiE "unable|cannot|failed|abort|impossible|different|won't|can't|not possible"; then
    pass "Scenario 1: TUI shows agent acknowledged failure or stopped retrying"
  else
    fail "Scenario 1: TUI does not contain DOOM_LOOP_STOPPED_SENTINEL_42 or abort keywords"
    capture_tui_evidence "doom-loop-no-sentinel"
  fi

  # Secondary: check the crush log for doom-loop-related entries.
  local log_entries
  log_entries=$(grep -ciE "doom.?loop|loop.?detection|repetition.*score|escalation.*level|intervention.*force" .crush/logs/crush.log 2>/dev/null || echo 0)

  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $log_entries doom-loop-related log entries"
  else
    echo "  FAIL: No doom-loop log entries found (doom detection did not trigger)" >&2; return 1
  fi

  # Capture log evidence for debugging.
  echo "--- Doom loop log evidence ---"
  grep -iE "doom.?loop|loop.?detection|repetition.*score|escalation.*level|intervention.*force" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- End evidence ---"

  capture_tui_evidence "doom-loop-detection"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_doom_loop_detection

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
