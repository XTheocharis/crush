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
# Scenario 1: --session resumes a specific session
# ---------------------------------------------------------------------------
test_session_flag_resume() {
  echo "--- Scenario 1: --session resumes specific session ---"

  setup_clean_crush

  # First conversation.
  start_crush 1
  send_prompt "What is 2+2? Reply with just the number."
  wait_for_idle 120

  SID=$(get_session_id)
  MSG_COUNT_BEFORE=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id='$SID'")

  stop_crush

  # Resume that session.
  start_crush 1 --session "$SID"
  send_prompt "What about 3+3? Reply with just the number."
  wait_for_idle 120

  MSG_COUNT_AFTER=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id='$SID'")

  # New messages: at least 1 user + 1 assistant = +2.
  if (( MSG_COUNT_AFTER >= MSG_COUNT_BEFORE + 2 )); then
    pass "session-resume: message count increased by >= 2"
  else
    fail "session-resume: expected >= $((MSG_COUNT_BEFORE + 2)) messages, got $MSG_COUNT_AFTER"
  fi

  # Verify no new session was created.
  LATEST_SID=$(get_session_id)
  if [[ "$LATEST_SID" == "$SID" ]]; then
    pass "session-resume: latest session ID matches resumed SID"
  else
    fail "session-resume: latest session ID ($LATEST_SID) != resumed SID ($SID)"
  fi

  capture_evidence 8 "session-resume"
  stop_crush
  restore_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: --continue resumes most recent session
# ---------------------------------------------------------------------------
test_continue_flag_resume() {
  echo "--- Scenario 2: --continue resumes most recent session ---"

  setup_clean_crush

  # First conversation.
  start_crush 1
  send_prompt "Hello world"
  wait_for_idle 120

  SID_ORIGINAL=$(get_session_id)
  stop_crush

  # Continue in most recent session.
  start_crush 1 --continue
  send_prompt "Goodbye world"
  wait_for_idle 120

  SID_LATEST=$(get_session_id)

  if [[ "$SID_LATEST" == "$SID_ORIGINAL" ]]; then
    pass "continue-resume: session ID unchanged after --continue"
  else
    fail "continue-resume: session ID changed ($SID_ORIGINAL -> $SID_LATEST)"
  fi

  capture_evidence 8 "continue-resume"
  stop_crush
  restore_crush
}

# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------
test_session_flag_resume
test_continue_flag_resume

echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
