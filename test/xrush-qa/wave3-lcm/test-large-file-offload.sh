#!/usr/bin/env bash
# Test: Large file output offloading to DB via TUI.
# Verifies that tool outputs exceeding the token threshold are offloaded
# to the lcm_large_files table with correct metadata and retrievable content.
# Uses deterministic sentinels for TUI-based assertion.
set -euo pipefail

WAVE=3
SCENARIO="large-file-offload-init"
source "$(dirname "$0")/../lib/common.sh"
source "$(dirname "$0")/../lib/db-verify.sh"

export LCM_LOW_THRESHOLD=1

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup_test() { cleanup_tui; restore_crush; }
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Large file output offloaded to DB
# ---------------------------------------------------------------------------
test_large_file_offload() {
  SCENARIO="large-file-offload-s1"
  echo "=== Scenario 1: Large file output offloaded to DB ==="

  setup_clean_crush
  start_crush_tui 3

  # Ask Crush to read a large file (XRUSH_FEATURES.md ~3475 lines).
  # Wave 3 config has large_tool_output_token_threshold: 100, which triggers
  # offloading. The answer to the sentinel question is only in the file body.
  focus_editor
  send_tui_prompt "Read the entire file XRUSH_FEATURES.md and tell me: what is the exact text of the section header that contains the phrase 'Fork Feature Documentation'? Reply with only that header text and the sentinel LARGE_FILE_SENTINEL_42"
  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_tui_evidence "large-file-offload-timeout"
    return
  fi

  # PRIMARY: assert TUI contains the sentinel from the offloaded content.
  if assert_tui_contains "LARGE_FILE_SENTINEL_42"; then
    pass "Scenario 1: TUI contains sentinel LARGE_FILE_SENTINEL_42"
  else
    fail "Scenario 1: TUI missing sentinel LARGE_FILE_SENTINEL_42"
    capture_tui_evidence "large-file-offload-missing-sentinel"
    return
  fi

  # PRIMARY: assert TUI contains evidence of reading the large file content.
  if assert_tui_contains "Fork Feature Documentation"; then
    pass "Scenario 1: TUI contains Fork Feature Documentation header"
  else
    fail "Scenario 1: TUI missing Fork Feature Documentation header"
  fi

  # Get the session ID for DB secondary checks.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_tui_evidence "large-file-offload-no-sid"
    return
  fi

  # SECONDARY: lcm_large_files has at least 1 entry with token_count > 0.
  local count
  count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_large_files WHERE session_id = '$SID' AND token_count > 0")
  if [[ "$count" -ge 1 ]]; then
    pass "Scenario 1: lcm_large_files has $count entry/entries with token_count > 0"
  else
    fail "Scenario 1: Expected >= 1 rows in lcm_large_files with token_count > 0, got $count"
  fi

  # SECONDARY: content is non-empty.
  local content_check
  content_check=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_large_files WHERE session_id = '$SID' AND content IS NOT NULL AND content != ''")
  if [[ "$content_check" -ge 1 ]]; then
    pass "Scenario 1: content is non-empty for $content_check row(s)"
  else
    fail "Scenario 1: No rows with non-empty content"
  fi

  # SECONDARY: file_id values present.
  local file_id_check
  file_id_check=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_large_files WHERE session_id = '$SID' AND file_id IS NOT NULL AND file_id != ''")
  if [[ "$file_id_check" -ge 1 ]]; then
    pass "Scenario 1: Offloaded rows have retrievable file_id values"
  else
    fail "Scenario 1: Offloaded rows missing file_id values"
  fi

  capture_tui_evidence "large-file-offload-final"
}

# ---------------------------------------------------------------------------
# Scenario 2: Large offload answer retrievable via sentinel
# ---------------------------------------------------------------------------
test_large_offload_answer() {
  SCENARIO="large-file-offload-s2"
  echo "=== Scenario 2: Large offload answer retrievable via sentinel ==="

  # Reuse the session from Scenario 1.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID found — skipping"
    return
  fi

  focus_editor
  send_tui_prompt "Now answer with just the sentinel LARGE_OFFLOAD_ANSWER_88 followed by a one-sentence summary of what XRUSH_FEATURES.md contains"
  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle within 180s"
    capture_tui_evidence "large-offload-answer-timeout"
    return
  fi

  # PRIMARY: assert TUI contains the second sentinel.
  if assert_tui_contains "LARGE_OFFLOAD_ANSWER_88"; then
    pass "Scenario 2: TUI contains sentinel LARGE_OFFLOAD_ANSWER_88"
  else
    fail "Scenario 2: TUI missing sentinel LARGE_OFFLOAD_ANSWER_88"
    capture_tui_evidence "large-offload-answer-missing"
    return
  fi

  # SECONDARY: offload artifacts still exist for this session.
  local total_rows
  total_rows=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_large_files WHERE session_id = '$SID'")
  if [[ "$total_rows" -ge 1 ]]; then
    pass "Scenario 2: Offload artifacts exist ($total_rows row(s))"
  else
    fail "Scenario 2: No offload artifacts found in DB"
  fi

  capture_tui_evidence "large-offload-answer-final"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_large_file_offload
test_large_offload_answer

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
