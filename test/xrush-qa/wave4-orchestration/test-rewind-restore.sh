#!/usr/bin/env bash
# Test: Rewind restore verification — code and conversation rewind.
# Verifies that snapshots exist after file-editing turns and documents the
# expected behaviour for code restore and conversation truncation via rewind.
#
# SKIP: Rewind is not exposed via CLI or shell-accessible command. Rewind is
# triggered through the TUI message-options dialog (press 'o' on a message,
# then select a rewind mode). Since tmux send-keys cannot reliably navigate
# the interactive dialog menu, the restore scenarios below are documented as
# what-should-happen but marked SKIP. The snapshot preconditions are verified.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
SKIP=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }
skip() { echo "SKIP: $1"; ((SKIP += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: Code restore — snapshot created, rewind restores original file
# ---------------------------------------------------------------------------
# What this test would verify:
#   1. Create a file with known content via Crush.
#   2. Verify a turn snapshot exists via turn_snapshot_counts query.
#   3. Modify the file via a second Crush turn.
#   4. Trigger rewind-to-turn-1 (RewindCodeOnly) via TUI dialog.
#   5. Assert file content matches the original snapshot.
#
# SKIP reason: Rewind is only available through the TUI message-options
# dialog (press 'o' on a message, then select "Rewind (code only)"). There is
# no `crush rewind` CLI subcommand and no /rewind slash command that accepts
# arguments from the prompt line. The dialog requires interactive navigation
# (arrow keys, Enter) that cannot be reliably driven from tmux send-keys.
#
# Precondition checks (snapshot creation) are verified in test-rewind.sh.
# ---------------------------------------------------------------------------
test_code_restore() {
  echo "=== Scenario 1: Code restore (snapshot → modify → rewind → assert original) ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_on_exit is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    command restore_crush
    rm -f /tmp/qa-rewind-restore-s1.txt
  }
  trap restore_on_exit EXIT

  start_crush 4

  # Turn 1: Create a file with known content.
  send_prompt "Create a new file /tmp/qa-rewind-restore-s1.txt with exactly this content: ORIGINAL_CONTENT_V1"
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle after turn 1"
    capture_evidence 44 "rewind-restore-code"
    return
  fi

  # Verify file was created with expected content.
  if [[ -f /tmp/qa-rewind-restore-s1.txt ]] && grep -q "ORIGINAL_CONTENT_V1" /tmp/qa-rewind-restore-s1.txt; then
    pass "Scenario 1: File created with ORIGINAL_CONTENT_V1 after turn 1"
  else
    fail "Scenario 1: File missing or wrong content after turn 1"
    capture_evidence 44 "rewind-restore-code"
    stop_crush
    return
  fi

  # Turn 2: Modify the file.
  send_prompt "Replace the content of /tmp/qa-rewind-restore-s1.txt with exactly: MODIFIED_CONTENT_V2"
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle after turn 2"
    capture_evidence 44 "rewind-restore-code"
    return
  fi

  # Verify file was modified.
  if [[ -f /tmp/qa-rewind-restore-s1.txt ]] && grep -q "MODIFIED_CONTENT_V2" /tmp/qa-rewind-restore-s1.txt; then
    pass "Scenario 1: File modified to MODIFIED_CONTENT_V2 after turn 2"
  else
    fail "Scenario 1: File not modified as expected after turn 2"
    capture_evidence 44 "rewind-restore-code"
    stop_crush
    return
  fi

  # Verify snapshots exist in DB using turn_snapshot_counts query.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 44 "rewind-restore-code"
    stop_crush
    return
  fi

  local snapshot_data
  snapshot_data=$(run_query "turn_snapshot_counts" "$SID")
  local snapshot_count file_count
  snapshot_count=$(echo "$snapshot_data" | jq '.[0].snapshot_count // 0')
  file_count=$(echo "$snapshot_data" | jq '.[0].file_count // 0')

  echo "  Snapshot count: $snapshot_count, File count: $file_count"

  if [[ "$snapshot_count" -ge 1 ]]; then
    pass "Scenario 1: $snapshot_count snapshot(s) exist in DB — rewind data available"
  else
    fail "Scenario 1: No snapshots in DB — rewind cannot be triggered"
  fi

  # --- SKIP: Rewind trigger via TUI dialog ---
  # To complete this scenario, we would need to:
  #   1. Navigate to the first user message in the TUI (Up arrow keys)
  #   2. Press 'o' to open message options dialog
  #   3. Select "Rewind (code only)" from the dialog
  #   4. Wait for rewind to complete
  #   5. Assert: grep -q "ORIGINAL_CONTENT_V1" /tmp/qa-rewind-restore-s1.txt
  #
  # This requires interactive TUI navigation that tmux send-keys cannot
  # reliably perform because the number of keypresses to reach the target
  # message depends on the dynamic number of assistant response lines.
  skip "Scenario 1: Code restore rewind — no CLI command; requires TUI dialog navigation"

  capture_evidence 44 "rewind-restore-code"
  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: Conversation truncation — rewind removes later messages
# ---------------------------------------------------------------------------
# What this test would verify:
#   1. Send 3 turns with unique sentinel text (SENTINEL_A, SENTINEL_B, SENTINEL_C).
#   2. Verify messages exist via query_db.
#   3. Trigger rewind-to-turn-2 (RewindConvoOnly) via TUI dialog.
#   4. Assert: SENTINEL_A and SENTINEL_B messages preserved, SENTINEL_C removed.
#
# SKIP reason: Same as Scenario 1 — rewind requires TUI dialog interaction.
# ---------------------------------------------------------------------------
test_conversation_truncation() {
  echo "=== Scenario 2: Conversation truncation (3 turns → rewind to middle → assert later removed) ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_on_exit is called via trap
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

  start_crush 4

  # Turn 1: Sent with unique sentinel A.
  send_prompt "Reply with exactly: SENTINEL_A_CONFIRMED and nothing else"
  if ! wait_for_idle 120; then
    fail "Scenario 2: Crush did not become idle after turn 1"
    capture_evidence 44 "rewind-restore-convo"
    return
  fi

  # Turn 2: Sent with unique sentinel B.
  send_prompt "Reply with exactly: SENTINEL_B_CONFIRMED and nothing else"
  if ! wait_for_idle 120; then
    fail "Scenario 2: Crush did not become idle after turn 2"
    capture_evidence 44 "rewind-restore-convo"
    return
  fi

  # Turn 3: Sent with unique sentinel C.
  send_prompt "Reply with exactly: SENTINEL_C_CONFIRMED and nothing else"
  if ! wait_for_idle 120; then
    fail "Scenario 2: Crush did not become idle after turn 3"
    capture_evidence 44 "rewind-restore-convo"
    return
  fi

  # Verify all 3 turns exist in the DB.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID found in DB"
    capture_evidence 44 "rewind-restore-convo"
    stop_crush
    return
  fi

  local message_count
  message_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID'")
  echo "  Message count after 3 turns: $message_count"

  if [[ "$message_count" -ge 6 ]]; then
    # Expect at least 6 messages: 3 user + 3 assistant.
    pass "Scenario 2: $message_count messages recorded after 3 turns (expected >= 6)"
  else
    fail "Scenario 2: Only $message_count messages after 3 turns (expected >= 6)"
  fi

  # Verify snapshots exist.
  local snapshot_data
  snapshot_data=$(run_query "turn_snapshot_counts" "$SID")
  local snapshot_count
  snapshot_count=$(echo "$snapshot_data" | jq '.[0].snapshot_count // 0')
  echo "  Snapshot count after 3 turns: $snapshot_count"

  if [[ "$snapshot_count" -ge 1 ]]; then
    pass "Scenario 2: $snapshot_count snapshot(s) exist — rewind data available"
  else
    fail "Scenario 2: No snapshots in DB after 3 turns"
  fi

  # --- SKIP: Rewind trigger via TUI dialog ---
  # To complete this scenario, we would need to:
  #   1. Navigate to the second user message in the TUI
  #   2. Press 'o' to open message options dialog
  #   3. Select "Rewind (conversation only)" from the dialog
  #   4. Wait for rewind to complete
  #   5. Assert: query_db for SENTINEL_A, SENTINEL_B → found
  #   6. Assert: query_db for SENTINEL_C → NOT found (message deleted)
  #   7. Assert: message count reduced by 2 (user msg 3 + assistant msg 3 removed)
  #
  # Expected SQL for post-rewind verification:
  #   SELECT COUNT(*) FROM messages WHERE session_id = '$SID'
  #     AND content LIKE '%SENTINEL_A%'  → >= 1
  #   SELECT COUNT(*) FROM messages WHERE session_id = '$SID'
  #     AND content LIKE '%SENTINEL_B%'  → >= 1
  #   SELECT COUNT(*) FROM messages WHERE session_id = '$SID'
  #     AND content LIKE '%SENTINEL_C%'  → 0
  skip "Scenario 2: Conversation truncation rewind — no CLI command; requires TUI dialog navigation"

  capture_evidence 44 "rewind-restore-convo"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_code_restore
test_conversation_truncation

echo ""
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
echo ""
echo "NOTE: $SKIP scenario(s) skipped because rewind is not exposed via CLI."
echo "      Rewind is triggered through the TUI message-options dialog (press 'o'"
echo "      on a message, then select rewind mode). To make these tests fully"
echo "      automatable, one of the following would be needed:"
echo "      1. A 'crush rewind --session <id> --seq <n> --mode <code|convo|both>' CLI command"
echo "      2. A /rewind <seq> <mode> slash command that accepts arguments"
echo "      3. A programmatic API endpoint for rewind operations"
exit "$FAIL"
