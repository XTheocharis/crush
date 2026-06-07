#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

restore_on_exit() {
  restore_crush
}

# ---------------------------------------------------------------------------
# Scenario 1: --session resumes a specific session
# ---------------------------------------------------------------------------
test_session_flag_resume() {
  echo "--- Scenario 1: --session resumes specific session ---"
  WAVE=1
  SCENARIO="session-resume"

  setup_clean_crush
  trap restore_on_exit EXIT

  # First conversation with a deterministic sentinel.
  start_crush_tui 1
  focus_editor
  send_tui_prompt "What is 2+2? Reply with exactly SENTINEL_FIRST_42 and nothing else."

  if ! wait_for_tui_idle 120; then
    fail "session-resume: First run did not become idle"
    capture_tui_evidence "first-run-timeout"
    return
  fi

  # Primary gate: first sentinel visible in TUI.
  if assert_tui_contains "SENTINEL_FIRST_42"; then
    pass "session-resume: First run shows SENTINEL_FIRST_42"
  else
    fail "session-resume: First run missing SENTINEL_FIRST_42"
    capture_tui_evidence "first-run-no-sentinel"
    return
  fi

  SID=$(get_session_id)
  MSG_COUNT_BEFORE=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id='$SID'")

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true

  # Resume that session with --session flag.
  start_crush_tui 1 --session "$SID"
  focus_editor
  send_tui_prompt "What about 3+3? Reply with exactly SENTINEL_SECOND_88 and nothing else."

  if ! wait_for_tui_idle 120; then
    fail "session-resume: Resumed run did not become idle"
    capture_tui_evidence "resume-timeout"
    return
  fi

  # Primary gate: second sentinel visible after resume.
  if assert_tui_contains "SENTINEL_SECOND_88"; then
    pass "session-resume: Resumed TUI shows SENTINEL_SECOND_88"
  else
    fail "session-resume: Resumed TUI missing SENTINEL_SECOND_88"
    capture_tui_evidence "resume-no-sentinel"
    return
  fi

  # The resumed TUI should also show the first sentinel (prior context).
  if assert_tui_contains "SENTINEL_FIRST_42"; then
    pass "session-resume: Resumed TUI shows prior sentinel SENTINEL_FIRST_42"
  else
    fail "session-resume: Resumed TUI missing prior sentinel SENTINEL_FIRST_42"
  fi

  capture_tui_evidence "resume-context"

  # --- Secondary DB checks ---
  MSG_COUNT_AFTER=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id='$SID'")

  if (( MSG_COUNT_AFTER >= MSG_COUNT_BEFORE + 2 )); then
    pass "session-resume: message count increased by >= 2"
  else
    fail "session-resume: expected >= $((MSG_COUNT_BEFORE + 2)) messages, got $MSG_COUNT_AFTER"
  fi

  LATEST_SID=$(get_session_id)
  if [[ "$LATEST_SID" == "$SID" ]]; then
    pass "session-resume: latest session ID matches resumed SID"
  else
    fail "session-resume: latest session ID ($LATEST_SID) != resumed SID ($SID)"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: --continue resumes most recent session
# ---------------------------------------------------------------------------
test_continue_flag_resume() {
  echo "--- Scenario 2: --continue resumes most recent session ---"
  WAVE=1
  SCENARIO="continue-resume"

  setup_clean_crush
  trap restore_on_exit EXIT

  # First conversation with a deterministic sentinel.
  start_crush_tui 1
  focus_editor
  send_tui_prompt "Hello. Reply with exactly SENTINEL_CONTINUE_HELLO_55 and nothing else."

  if ! wait_for_tui_idle 120; then
    fail "continue-resume: First run did not become idle"
    capture_tui_evidence "first-run-timeout"
    return
  fi

  # Primary gate: first sentinel visible.
  if assert_tui_contains "SENTINEL_CONTINUE_HELLO_55"; then
    pass "continue-resume: First run shows SENTINEL_CONTINUE_HELLO_55"
  else
    fail "continue-resume: First run missing SENTINEL_CONTINUE_HELLO_55"
    capture_tui_evidence "first-run-no-sentinel"
    return
  fi

  SID_ORIGINAL=$(get_session_id)

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true

  # Continue in most recent session with --continue flag.
  start_crush_tui 1 --continue
  focus_editor
  send_tui_prompt "Goodbye. Reply with exactly SENTINEL_CONTINUE_BYE_66 and nothing else."

  if ! wait_for_tui_idle 120; then
    fail "continue-resume: Continued run did not become idle"
    capture_tui_evidence "continue-timeout"
    return
  fi

  # Primary gate: second sentinel visible after continue.
  if assert_tui_contains "SENTINEL_CONTINUE_BYE_66"; then
    pass "continue-resume: Continued TUI shows SENTINEL_CONTINUE_BYE_66"
  else
    fail "continue-resume: Continued TUI missing SENTINEL_CONTINUE_BYE_66"
    capture_tui_evidence "continue-no-sentinel"
    return
  fi

  # The continued TUI should also show the first sentinel (prior context).
  if assert_tui_contains "SENTINEL_CONTINUE_HELLO_55"; then
    pass "continue-resume: Continued TUI shows prior sentinel SENTINEL_CONTINUE_HELLO_55"
  else
    fail "continue-resume: Continued TUI missing prior sentinel SENTINEL_CONTINUE_HELLO_55"
  fi

  capture_tui_evidence "continue-context"

  # --- Secondary DB check: same session ---
  SID_LATEST=$(get_session_id)

  if [[ "$SID_LATEST" == "$SID_ORIGINAL" ]]; then
    pass "continue-resume: session ID unchanged after --continue"
  else
    fail "continue-resume: session ID changed ($SID_ORIGINAL -> $SID_LATEST)"
  fi
}

# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------
test_session_flag_resume
test_continue_flag_resume

echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
