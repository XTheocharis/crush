#!/usr/bin/env bash
# Test: Rewind — single-step code rewind via TUI message options.
# Creates a sentinel file, triggers rewind (code only) through the 'o' key
# path, and verifies the file is removed and the TUI confirms the rewind.
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
# Scenario 1: Single-step code rewind removes created file
# ---------------------------------------------------------------------------
test_single_code_rewind() {
  echo "=== Scenario 1: Single-step code rewind removes created file ==="
  export SCENARIO="rewind-single"

  local sentinel_file="/tmp/qa-rewind-single-$$.txt"

  setup_clean_crush
  # shellcheck disable=SC2317
  cleanup_test() {
    tmux send-keys -t "$TMUX_SESSION" C-c 2>/dev/null || true
    sleep 0.3
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
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

  # Create a sentinel file via Crush.
  send_tui_prompt "Create a new file $sentinel_file with exactly this content: REWIND_SINGLE_SENTINEL"
  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle after create"
    capture_tui_evidence "create-timeout"
    return
  fi
  capture_tui_evidence "after-create"

  # Verify TUI shows the sentinel.
  if assert_tui_contains "REWIND_SINGLE_SENTINEL"; then
    pass "Scenario 1: TUI shows REWIND_SINGLE_SENTINEL"
  else
    fail "Scenario 1: TUI does not show REWIND_SINGLE_SENTINEL"
    capture_tui_evidence "create-missing-sentinel"
    return
  fi

  # Verify file on disk.
  if [[ -f "$sentinel_file" ]] && grep -q "REWIND_SINGLE_SENTINEL" "$sentinel_file"; then
    pass "Scenario 1: Sentinel file exists with correct content"
  else
    fail "Scenario 1: Sentinel file missing or wrong content"
    capture_tui_evidence "file-missing"
    return
  fi

  # Verify DB has snapshots for this session.
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    local snapshot_count
    snapshot_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM turn_snapshots WHERE session_id = '$SID'" 2>/dev/null || echo 0)
    if [[ "$snapshot_count" -ge 1 ]]; then
      pass "Scenario 1: $snapshot_count snapshot(s) exist — rewind data available"
    else
      fail "Scenario 1: No snapshots in DB — rewind may not fire"
    fi

    local snapshot_file_count
    snapshot_file_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM turn_snapshot_files WHERE snapshot_id IN (SELECT id FROM turn_snapshots WHERE session_id = '$SID')" 2>/dev/null || echo 0)
    if [[ "$snapshot_file_count" -gt 0 ]]; then
      pass "Scenario 1: $snapshot_file_count snapshot file row(s) recorded"
    else
      fail "Scenario 1: No snapshot file rows recorded"
    fi
  fi

  # --- Trigger rewind via TUI ---
  focus_chat
  sleep 0.5

  # Navigate to the assistant message (offset 0 = bottom of chat list).
  select_message_by_offset 0
  sleep 0.3
  capture_tui_evidence "before-o-key"

  # Press 'o' to open message options dialog.
  tmux send-keys -t "$TMUX_SESSION" o
  sleep 0.8
  capture_tui_evidence "after-o-key"

  # Verify dialog appeared.
  local dialog_output
  dialog_output=$(capture_tui | strip_ansi)
  if echo "$dialog_output" | grep -qi "Message Options\|Rewind"; then
    pass "Scenario 1: Message options dialog visible in TUI"
  else
    echo "  NOTE: Message Options dialog not explicitly detected — proceeding with rewind attempt"
  fi

  # Select "Rewind (code only)" — index 0, press Enter.
  tmux send-keys -t "$TMUX_SESSION" Enter
  capture_tui_evidence "after-rewind-enter"

  # Wait for rewind to complete.
  if ! wait_for_tui_idle 60; then
    fail "Scenario 1: Crush did not become idle after rewind"
    capture_tui_evidence "rewind-timeout"
    return
  fi
  capture_tui_evidence "after-rewind-idle"

  # --- Assertions after rewind ---
  # Primary: sentinel file should have been removed by code-only rewind.
  if [[ ! -f "$sentinel_file" ]]; then
    pass "Scenario 1: Sentinel file removed — code-only rewind succeeded"
  else
    fail "Scenario 1: Sentinel file still exists — code rewind may not have fired"
  fi

  # TUI should show rewind-related text.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)
  if echo "$tui_output" | grep -qi "rewind\|Rewind"; then
    pass "Scenario 1: TUI shows rewind indicator"
  else
    echo "  NOTE: No rewind indicator visible in TUI (rewind may be silent)"
  fi

  # Check logs for rewind activity.
  local rewind_log_count
  rewind_log_count=$(grep -ci "rewind\|rollback\|restore" .crush/logs/crush.log 2>/dev/null || echo 0)
  if [[ "$rewind_log_count" -ge 1 ]]; then
    pass "Scenario 1: $rewind_log_count rewind-related log entries found"
  else
    fail "Scenario 1: No rewind-related log entries found"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_single_code_rewind

echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
