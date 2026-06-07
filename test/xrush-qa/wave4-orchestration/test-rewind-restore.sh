#!/usr/bin/env bash
# Test: Rewind restore verification — code and conversation rewind via TUI.
# Scenario A: Create file V1 → modify to V2 → rewind (code only) → verify V1 restored.
# Scenario B: Send 3 sentinel turns → rewind (convo only) to turn 2 → verify turn 3 removed.
# Drives rewind through the TUI message-options dialog (press 'o' on a message).
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
# Scenario A: Code restore — create V1 → modify to V2 → rewind code → V1 restored
# ---------------------------------------------------------------------------
test_code_restore() {
  echo "=== Scenario A: Code restore (V1 → V2 → rewind code → V1) ==="
  export SCENARIO="rewind-restore-code"

  local sentinel_file="/tmp/qa-rewind-restore-code-$$.txt"

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
    rm -f "$sentinel_file"
  }
  trap cleanup_test EXIT

  WAVE=4 start_crush_tui 4
  focus_editor

  # Turn 1: Create file with V1 content.
  send_tui_prompt "Create a new file $sentinel_file with exactly this content: REWIND_CODE_V1_SENTINEL"
  if ! wait_for_tui_idle 120; then
    fail "Scenario A: Crush did not become idle after turn 1"
    capture_tui_evidence "after-turn1-timeout"
    return
  fi
  capture_tui_evidence "after-turn1"

  # Verify V1 content in TUI.
  if assert_tui_contains "REWIND_CODE_V1_SENTINEL"; then
    pass "Scenario A: TUI shows REWIND_CODE_V1_SENTINEL after turn 1"
  else
    fail "Scenario A: TUI does not show REWIND_CODE_V1_SENTINEL after turn 1"
    capture_tui_evidence "turn1-missing-v1"
    return
  fi

  # Verify file on disk.
  if [[ -f "$sentinel_file" ]] && grep -q "REWIND_CODE_V1_SENTINEL" "$sentinel_file"; then
    pass "Scenario A: File $sentinel_file exists with V1 content"
  else
    fail "Scenario A: File $sentinel_file missing or wrong content after turn 1"
    capture_tui_evidence "turn1-file-check-fail"
    return
  fi

  # Turn 2: Modify file to V2 content.
  send_tui_prompt "Replace the entire content of $sentinel_file with exactly: REWIND_CODE_V2_SENTINEL"
  if ! wait_for_tui_idle 120; then
    fail "Scenario A: Crush did not become idle after turn 2"
    capture_tui_evidence "after-turn2-timeout"
    return
  fi
  capture_tui_evidence "after-turn2"

  # Verify V2 content in TUI.
  if assert_tui_contains "REWIND_CODE_V2_SENTINEL"; then
    pass "Scenario A: TUI shows REWIND_CODE_V2_SENTINEL after turn 2"
  else
    fail "Scenario A: TUI does not show REWIND_CODE_V2_SENTINEL after turn 2"
    capture_tui_evidence "turn2-missing-v2"
    return
  fi

  # Verify file on disk has V2.
  if [[ -f "$sentinel_file" ]] && grep -q "REWIND_CODE_V2_SENTINEL" "$sentinel_file"; then
    pass "Scenario A: File modified to REWIND_CODE_V2_SENTINEL"
  else
    fail "Scenario A: File not modified as expected after turn 2"
    capture_tui_evidence "turn2-file-check-fail"
    return
  fi

  # --- Trigger rewind via TUI ---
  # Focus the chat list pane.
  focus_chat
  sleep 0.5

  # Navigate to the assistant message from turn 2 (offset 0 = last message,
  # which is the assistant's V2 response at the bottom).
  select_message_by_offset 0
  capture_tui_evidence "before-o-key"

  # Press 'o' to open message options dialog.
  tmux send-keys -t "$TMUX_SESSION" o
  sleep 0.8
  capture_tui_evidence "after-o-key"

  # Select "Rewind (code only)" — index 0, just press Enter.
  tmux send-keys -t "$TMUX_SESSION" Enter
  capture_tui_evidence "after-rewind-enter"

  # Wait for rewind to complete.
  if ! wait_for_tui_idle 60; then
    fail "Scenario A: Crush did not become idle after rewind"
    capture_tui_evidence "rewind-timeout"
    return
  fi
  capture_tui_evidence "after-rewind-idle"

  # --- Assertions after rewind ---
  # Primary: file should now have V1 content restored.
  if [[ -f "$sentinel_file" ]] && grep -q "REWIND_CODE_V1_SENTINEL" "$sentinel_file"; then
    pass "Scenario A: File restored to REWIND_CODE_V1_SENTINEL after code rewind"
  else
    fail "Scenario A: File not restored to V1 after code rewind"
    capture_tui_evidence "rewind-file-not-restored"
  fi

  # Secondary: V2 content should no longer be in the file.
  if ! grep -q "REWIND_CODE_V2_SENTINEL" "$sentinel_file" 2>/dev/null; then
    pass "Scenario A: V2 content no longer in file after rewind"
  else
    fail "Scenario A: V2 content still present in file after rewind"
  fi

  # Secondary DB check: snapshots should exist.
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    local snapshot_count
    snapshot_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM turn_snapshots WHERE session_id = '$SID'" 2>/dev/null || echo 0)
    if [[ "$snapshot_count" -ge 1 ]]; then
      pass "Scenario A: $snapshot_count snapshot(s) exist in DB"
    else
      fail "Scenario A: No snapshots in DB"
    fi
  fi

  # TUI should show rewind-related indicator.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)
  if echo "$tui_output" | grep -qi "rewind\|Rewind\|restored"; then
    pass "Scenario A: TUI shows rewind indicator"
  fi
}

# ---------------------------------------------------------------------------
# Scenario B: Conversation truncation — 3 turns → rewind convo → turn 3 removed
# ---------------------------------------------------------------------------
test_conversation_truncation() {
  echo "=== Scenario B: Conversation truncation (3 turns → rewind convo → turn 3 removed) ==="
  export SCENARIO="rewind-restore-convo"

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

  WAVE=4 start_crush_tui 4
  focus_editor

  # Turn 1: Sent with sentinel A.
  send_tui_prompt "Reply with exactly: REWIND_CONVO_A_CONFIRMED and nothing else"
  if ! wait_for_tui_idle 120; then
    fail "Scenario B: Crush did not become idle after turn 1"
    capture_tui_evidence "convo-turn1-timeout"
    return
  fi
  capture_tui_evidence "convo-after-turn1"

  if assert_tui_contains "REWIND_CONVO_A_CONFIRMED"; then
    pass "Scenario B: TUI shows REWIND_CONVO_A_CONFIRMED after turn 1"
  else
    fail "Scenario B: TUI does not show REWIND_CONVO_A_CONFIRMED after turn 1"
    capture_tui_evidence "convo-turn1-missing"
    return
  fi

  # Turn 2: Sent with sentinel B.
  send_tui_prompt "Reply with exactly: REWIND_CONVO_B_CONFIRMED and nothing else"
  if ! wait_for_tui_idle 120; then
    fail "Scenario B: Crush did not become idle after turn 2"
    capture_tui_evidence "convo-turn2-timeout"
    return
  fi
  capture_tui_evidence "convo-after-turn2"

  if assert_tui_contains "REWIND_CONVO_B_CONFIRMED"; then
    pass "Scenario B: TUI shows REWIND_CONVO_B_CONFIRMED after turn 2"
  else
    fail "Scenario B: TUI does not show REWIND_CONVO_B_CONFIRMED after turn 2"
    capture_tui_evidence "convo-turn2-missing"
    return
  fi

  # Turn 3: Sent with sentinel C.
  send_tui_prompt "Reply with exactly: REWIND_CONVO_C_CONFIRMED and nothing else"
  if ! wait_for_tui_idle 120; then
    fail "Scenario B: Crush did not become idle after turn 3"
    capture_tui_evidence "convo-turn3-timeout"
    return
  fi
  capture_tui_evidence "convo-after-turn3"

  if assert_tui_contains "REWIND_CONVO_C_CONFIRMED"; then
    pass "Scenario B: TUI shows REWIND_CONVO_C_CONFIRMED after turn 3"
  else
    fail "Scenario B: TUI does not show REWIND_CONVO_C_CONFIRMED after turn 3"
    capture_tui_evidence "convo-turn3-missing"
    return
  fi

  # Verify all 3 turns in DB before rewind.
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    local pre_msg_count
    pre_msg_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID'" 2>/dev/null || echo 0)
    echo "  Message count before rewind: $pre_msg_count"
    if [[ "$pre_msg_count" -ge 6 ]]; then
      pass "Scenario B: $pre_msg_count messages before rewind (expected >= 6)"
    else
      fail "Scenario B: Only $pre_msg_count messages before rewind (expected >= 6)"
    fi
  fi

  # --- Trigger rewind via TUI ---
  # Focus the chat list pane.
  focus_chat
  sleep 0.5

  # Navigate to the assistant message from turn 3 (offset 0 = last message).
  select_message_by_offset 0
  capture_tui_evidence "convo-before-o-key"

  # Press 'o' to open message options dialog.
  tmux send-keys -t "$TMUX_SESSION" o
  sleep 0.8
  capture_tui_evidence "convo-after-o-key"

  # Select "Rewind (convo only)" — index 1: Down then Enter.
  tmux send-keys -t "$TMUX_SESSION" Down
  sleep 0.2
  tmux send-keys -t "$TMUX_SESSION" Enter
  capture_tui_evidence "convo-after-rewind-enter"

  # Wait for rewind to complete.
  if ! wait_for_tui_idle 60; then
    fail "Scenario B: Crush did not become idle after convo rewind"
    capture_tui_evidence "convo-rewind-timeout"
    return
  fi
  capture_tui_evidence "convo-after-rewind-idle"

  # --- Assertions after rewind ---
  # Primary TUI: sentinel C should no longer be visible.
  if assert_tui_not_contains "REWIND_CONVO_C_CONFIRMED"; then
    pass "Scenario B: TUI no longer shows REWIND_CONVO_C_CONFIRMED after convo rewind"
  else
    fail "Scenario B: TUI still shows REWIND_CONVO_C_CONFIRMED after convo rewind"
    capture_tui_evidence "convo-c-still-visible"
  fi

  # Secondary: sentinels A and B should still be present.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)
  if echo "$tui_output" | grep -q "REWIND_CONVO_A_CONFIRMED"; then
    pass "Scenario B: TUI still shows REWIND_CONVO_A_CONFIRMED"
  else
    echo "  (REWIND_CONVO_A_CONFIRMED scrolled off TUI, DB check below is authoritative)"
  fi

  if echo "$tui_output" | grep -q "REWIND_CONVO_B_CONFIRMED"; then
    pass "Scenario B: TUI still shows REWIND_CONVO_B_CONFIRMED"
  else
    echo "  (REWIND_CONVO_B_CONFIRMED scrolled off TUI, DB check below is authoritative)"
  fi

  # Secondary DB check: message count should have decreased.
  if [[ -n "$SID" ]]; then
    local post_msg_count
    post_msg_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID'" 2>/dev/null || echo 0)
    echo "  Message count after rewind: $post_msg_count"

    # Convo rewind should remove user msg 3 + assistant msg 3 (2 messages).
    if [[ "$post_msg_count" -lt "$pre_msg_count" ]]; then
      pass "Scenario B: Message count decreased from $pre_msg_count to $post_msg_count"
    else
      fail "Scenario B: Message count did not decrease after convo rewind ($pre_msg_count → $post_msg_count)"
    fi

    # Verify sentinel C is no longer in the DB.
    local c_count
    c_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID' AND content LIKE '%REWIND_CONVO_C%'" 2>/dev/null || echo 0)
    if [[ "$c_count" -eq 0 ]]; then
      pass "Scenario B: REWIND_CONVO_C messages removed from DB"
    else
      fail "Scenario B: REWIND_CONVO_C messages still in DB (count: $c_count)"
    fi
  fi

  # TUI should show rewind-related indicator.
  if echo "$tui_output" | grep -qi "rewind\|Rewind"; then
    pass "Scenario B: TUI shows rewind indicator"
  else
    echo "  NOTE: No rewind indicator visible in TUI"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_code_restore
test_conversation_truncation

echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
