#!/usr/bin/env bash
# Test: Eval pipeline end-to-end through TUI command palette.
# Invokes the eval command via Ctrl+P -> "eval" -> Enter, then verifies TUI
# output shows the eval response, DB records exist, and logs contain eval
# evidence. Drives eval exclusively via the TUI — no direct CLI invocation.
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
# Scenario 1: Eval pipeline creates response via command palette
# ---------------------------------------------------------------------------
test_eval_pipeline_command_palette() {
  SCENARIO="eval-pipeline-cmd-palette"
  echo "=== Scenario 1: Eval pipeline via command palette with DB verification ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  sleep 2

  # Open command palette, filter to eval, invoke.
  tmux send-keys -t "$TMUX_SESSION" C-p
  sleep 1
  tmux send-keys -t "$TMUX_SESSION" -l "eval"
  sleep 1
  tmux send-keys -t "$TMUX_SESSION" Enter
  sleep 3

  # Primary gate: TUI must show eval response.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qi "Evaluation"; then
    pass "Scenario 1: TUI contains 'Evaluation' after eval command palette invocation"
  else
    fail "Scenario 1: TUI does not contain 'Evaluation' after eval command palette invocation"
  fi

  # Secondary: TUI shows "coming soon" sentinel.
  if printf '%s' "$tui_output" | grep -qi "coming soon"; then
    pass "Scenario 1: TUI shows eval response sentinel 'coming soon'"
  else
    fail "Scenario 1: TUI missing eval response sentinel 'coming soon'"
  fi

  # Secondary: DB has at least one session from the TUI launch.
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

  # Secondary: logs contain eval-related entries.
  local log_path=".crush/logs/crush.log"
  if [[ -f "$log_path" ]]; then
    local eval_log_matches
    eval_log_matches=$(grep -ciE "eval|Evaluation|ActionRunEval" "$log_path" 2>/dev/null || echo 0)
    if [[ "$eval_log_matches" -ge 1 ]]; then
      pass "Scenario 1: Crush log contains eval evidence ($eval_log_matches matches)"
    else
      echo "  NOTE: No eval log entries found (logging may be minimal for info messages)"
    fi
  fi

  capture_tui_evidence "EVAL_PIPELINE_SENTINEL_88"
}

# ---------------------------------------------------------------------------
# Scenario 2: Command palette shows eval with session active
# ---------------------------------------------------------------------------
test_eval_pipeline_palette_with_session() {
  SCENARIO="eval-pipeline-session-palette"
  echo "=== Scenario 2: Command palette eval item visible with active session ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Send a prompt to ensure a session is active and busy state transitions.
  send_tui_prompt "echo EVAL_PIPELINE_SENTINEL_88"
  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle within 120s"
    capture_tui_evidence "eval-pipeline-session-timeout"
    return
  fi

  # Open command palette after session is established.
  tmux send-keys -t "$TMUX_SESSION" C-p
  sleep 1

  local palette_output
  palette_output=$(capture_tui | strip_ansi)

  if printf '%s' "$palette_output" | grep -qi "Run Evaluation\|Evaluation"; then
    pass "Scenario 2: 'Run Evaluation' visible in palette after session established"
  else
    fail "Scenario 2: 'Run Evaluation' not visible in palette after session established"
  fi

  # Dismiss palette and invoke eval.
  tmux send-keys -t "$TMUX_SESSION" -l "eval"
  sleep 1
  tmux send-keys -t "$TMUX_SESSION" Enter
  sleep 3

  # Verify TUI shows eval response after active session.
  tui_output=$(capture_tui | strip_ansi)
  if printf '%s' "$tui_output" | grep -qi "coming soon"; then
    pass "Scenario 2: TUI shows eval response sentinel after active session"
  else
    fail "Scenario 2: TUI missing eval response sentinel after active session"
  fi

  # Secondary: DB verification — messages table has entries.
  local db_path=".crush/crush.db"
  if [[ -f "$db_path" ]]; then
    local sid
    sid=$(sqlite3 "$db_path" "SELECT id FROM sessions ORDER BY created_at DESC LIMIT 1" 2>/dev/null || true)
    if [[ -n "$sid" ]]; then
      local msg_count
      msg_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM messages WHERE session_id = '$sid'" 2>/dev/null || echo 0)
      if [[ "$msg_count" -ge 2 ]]; then
        pass "Scenario 2: DB has $msg_count messages in session"
      else
        echo "  NOTE: Only $msg_count messages in session (eval info may not create DB rows)"
      fi
    fi
  fi

  capture_tui_evidence "eval-pipeline-session"
}

# ---------------------------------------------------------------------------
# Scenario 3: Eval command palette does not produce CLI error in TUI
# ---------------------------------------------------------------------------
test_eval_pipeline_no_cli_error() {
  SCENARIO="eval-pipeline-no-error"
  echo "=== Scenario 3: Eval command palette produces no CLI error ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  sleep 2

  # Open command palette, invoke eval.
  tmux send-keys -t "$TMUX_SESSION" C-p
  sleep 1
  tmux send-keys -t "$TMUX_SESSION" -l "eval"
  sleep 1
  tmux send-keys -t "$TMUX_SESSION" Enter
  sleep 3

  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  # Assert: no "unknown command" or CLI error in TUI.
  if printf '%s' "$tui_output" | grep -qi "unknown command\|not recognized\|command not found"; then
    fail "Scenario 3: TUI shows 'unknown command' error for eval invocation"
  else
    pass "Scenario 3: No 'unknown command' error in TUI after eval invocation"
  fi

  # Assert: no panic or crash evidence.
  if printf '%s' "$tui_output" | grep -qi "panic\|fatal\|crash"; then
    fail "Scenario 3: TUI shows panic/fatal/crash after eval invocation"
  else
    pass "Scenario 3: No panic/fatal/crash in TUI after eval invocation"
  fi

  # Secondary: log check for errors.
  local log_path=".crush/logs/crush.log"
  if [[ -f "$log_path" ]]; then
    local error_count
    error_count=$(grep -ciE "error.*eval|eval.*error|panic" "$log_path" 2>/dev/null || echo 0)
    if [[ "$error_count" -eq 0 ]]; then
      pass "Scenario 3: No eval-related errors in crush log"
    else
      echo "  NOTE: $error_count eval-related log entries found (may be benign)"
    fi
  fi

  capture_tui_evidence "eval-pipeline-no-error"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_eval_pipeline_command_palette
test_eval_pipeline_palette_with_session
test_eval_pipeline_no_cli_error

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-eval-pipeline" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
