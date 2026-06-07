#!/usr/bin/env bash
# Test: Large file output offloading to DB.
# Verifies that tool outputs exceeding the token threshold are offloaded
# to the lcm_large_files table with correct metadata and retrievable content.
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
# Scenario 1: Large file output offloaded to DB
# ---------------------------------------------------------------------------
test_large_file_offload() {
  echo "=== Scenario 1: Large file output offloaded to DB ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    restore_crush
  }
  trap restore_on_exit EXIT

  start_crush 3
  # Request the entire contents of a large file (XRUSH_FEATURES.md ~3475 lines).
  # Wave 3 config has large_tool_output_token_threshold: 100, which should trigger offloading.
  send_prompt "Show me the entire contents of XRUSH_FEATURES.md"
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_evidence 14 "large-file-offload"
    return
  fi

  # Get the session ID.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 14 "large-file-offload"
    return
  fi

  # Assert: lcm_large_files has at least 1 entry for this session with token_count > 0.
  local count
  count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_large_files WHERE session_id = '$SID' AND token_count > 0")
  if [[ "$count" -ge 1 ]]; then
    pass "Scenario 1: lcm_large_files has $count entry/entries with token_count > 0"
  else
    fail "Scenario 1: Expected >= 1 rows in lcm_large_files with token_count > 0, got $count"
    capture_evidence 14 "large-file-offload"
    return
  fi

  # Verify: content is non-empty for at least one row.
  local content_check
  content_check=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_large_files WHERE session_id = '$SID' AND content IS NOT NULL AND content != ''")
  if [[ "$content_check" -ge 1 ]]; then
    pass "Scenario 1: content is non-empty for $content_check row(s)"
  else
    fail "Scenario 1: No rows with non-empty content"
  fi

  local path_check
  path_check=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_large_files WHERE session_id = '$SID' AND original_path LIKE '%XRUSH_FEATURES.md'")
  if [[ "$path_check" -ge 1 ]]; then
    pass "Scenario 1: Offloaded row records XRUSH_FEATURES.md as original path"
  else
    fail "Scenario 1: No offloaded row records XRUSH_FEATURES.md original path"
  fi

  local content_marker_check
  content_marker_check=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_large_files WHERE session_id = '$SID' AND content LIKE '%XRush Fork Feature Documentation%'")
  if [[ "$content_marker_check" -ge 1 ]]; then
    pass "Scenario 1: Offloaded content contains XRUSH_FEATURES.md marker text"
  else
    fail "Scenario 1: Offloaded content does not contain XRUSH_FEATURES.md marker text"
  fi

  local file_id_check
  file_id_check=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_large_files WHERE session_id = '$SID' AND file_id IS NOT NULL AND file_id != ''")
  if [[ "$file_id_check" -ge 1 ]]; then
    pass "Scenario 1: Offloaded rows have retrievable file_id values"
  else
    fail "Scenario 1: Offloaded rows missing file_id values"
  fi

  capture_evidence 14 "large-file-offload"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_large_file_offload

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
