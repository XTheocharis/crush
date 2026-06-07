#!/usr/bin/env bash
# Test: Eval command invocation through TUI command palette.
# Opens the command palette via Ctrl+P, filters to "eval", presses Enter,
# then asserts the TUI shows the eval response message.
# Drives eval exclusively via the TUI — no direct CLI invocation.
set -euo pipefail

WAVE=5

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup_test() {
    cleanup_tui
  local _bak
  _bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  [[ -n "$_bak" ]] && mv "$_bak" crush.json
  restore_crush
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Invoke eval command via command palette and verify TUI output
# ---------------------------------------------------------------------------
test_eval_command_palette() {
  SCENARIO="eval-cmd-palette"
  echo "=== Scenario 1: Eval command via command palette ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Give the TUI a moment to fully initialize and create a session.
  sleep 2

  # Open command palette with Ctrl+P.
  tmux send-keys -t "$TMUX_SESSION" C-p
  sleep 1

  # Type "eval" to filter to the "Run Evaluation" command.
  tmux send-keys -t "$TMUX_SESSION" -l "eval"
  sleep 1

  # Press Enter to select the filtered command.
  tmux send-keys -t "$TMUX_SESSION" Enter

  # Wait for the info message to appear in the TUI.
  sleep 3

  # Primary gate: TUI must contain the eval response message.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qi "Evaluation"; then
    pass "Scenario 1: TUI contains 'Evaluation' after command palette invocation"
  else
    fail "Scenario 1: TUI does not contain 'Evaluation' after command palette invocation"
  fi

  # Secondary gate: assert the sentinel "feature coming soon" appears.
  if printf '%s' "$tui_output" | grep -qi "coming soon"; then
    pass "Scenario 1: TUI contains eval response sentinel 'coming soon'"
  else
    fail "Scenario 1: TUI does not contain eval response sentinel 'coming soon'"
  fi

  # Tertiary: verify command palette was dismissed (no filter text visible).
  if ! printf '%s' "$tui_output" | grep -qi "command palette\|filter\|commands"; then
    pass "Scenario 1: Command palette dismissed after eval invocation"
  else
    echo "  NOTE: Command palette text may still be visible (timing-dependent)"
  fi

  # Secondary: DB check — verify a session was created by the TUI launch.
  local db_path=".crush/crush.db"
  if [[ -f "$db_path" ]]; then
    local session_count
    session_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM sessions" 2>/dev/null || echo 0)
    if [[ "$session_count" -ge 1 ]]; then
      pass "Scenario 1: DB has $session_count session(s) from TUI launch"
    else
      fail "Scenario 1: No sessions in DB after TUI launch"
    fi
  else
    echo "  NOTE: No crush DB found (TUI may not have fully initialized)"
  fi

  capture_tui_evidence "EVAL_CMD_SENTINEL_42"
}

# ---------------------------------------------------------------------------
# Scenario 2: Eval command palette item appears in the command list
# ---------------------------------------------------------------------------
test_eval_in_command_list() {
  SCENARIO="eval-cmd-list-visible"
  echo "=== Scenario 2: Eval command visible in command palette list ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Give the TUI a moment to initialize and create a session (required for
  # eval command to appear — it's only shown when hasSession is true).
  sleep 2

  # Open command palette with Ctrl+P.
  tmux send-keys -t "$TMUX_SESSION" C-p
  sleep 1

  # Capture the palette contents before filtering.
  local palette_output
  palette_output=$(capture_tui | strip_ansi)

  # Assert: "Run Evaluation" appears as a command item.
  if printf '%s' "$palette_output" | grep -qi "Run Evaluation\|run_eval\|Evaluation"; then
    pass "Scenario 2: 'Run Evaluation' visible in command palette list"
  else
    fail "Scenario 2: 'Run Evaluation' not visible in command palette list"
  fi

  # Assert: no error about unknown command.
  if ! printf '%s' "$palette_output" | grep -qi "unknown command\|error\|not found"; then
    pass "Scenario 2: No error messages in command palette"
  else
    fail "Scenario 2: Error message visible in command palette"
  fi

  capture_tui_evidence "eval-command-list"

  # Dismiss the palette.
  tmux send-keys -t "$TMUX_SESSION" Escape
  sleep 1
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_eval_command_palette
test_eval_in_command_list

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-eval" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
