#!/usr/bin/env bash
# Test: File tracking in read_files and written_files tables.
# Verifies that Crush records files viewed by the agent in read_files,
# and files written by the agent in written_files.
# TUI-first: primary gate is sentinel in TUI output; DB checks are secondary.
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
# Scenario 1: Read file appears in TUI output and read_files table
# ---------------------------------------------------------------------------
test_read_files_tracking() {
  echo "=== Scenario 1: Read file appears in TUI output and read_files ==="
  export WAVE=1
  export SCENARIO="read-files"

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

  start_crush_tui 1
  focus_editor
  send_tui_prompt "Read the file go.mod and reply with exactly: FILE_READ_SENTINEL_charmbracelet_crush"
  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "read-files"
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "FILE_READ_SENTINEL_charmbracelet_crush"; then
    pass "Scenario 1: TUI output contains FILE_READ_SENTINEL_charmbracelet_crush"
  else
    fail "Scenario 1: TUI output does not contain FILE_READ_SENTINEL_charmbracelet_crush"
    capture_tui_evidence "read-files"
    return
  fi

  capture_tui_evidence "read-files"

  # Secondary DB checks.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    return
  fi

  local read_paths
  read_paths=$(query_db "SELECT path FROM read_files WHERE session_id = '$SID'")
  if [[ -z "$read_paths" ]] || [[ "$read_paths" == "[]" ]]; then
    fail "Scenario 1: No entries in read_files for session $SID"
  else
    local has_gomod
    has_gomod=$(echo "$read_paths" | jq -r '.[].path' | grep -Ec '(^|/)go\.mod$' || true)
    if [[ "$has_gomod" -ge 1 ]]; then
      pass "Scenario 1: go.mod found in read_files"
    else
      fail "Scenario 1: go.mod not found in read_files for targeted read request"
    fi
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: Written file appears in TUI output and written_files table
# ---------------------------------------------------------------------------
test_written_files_tracking() {
  echo "=== Scenario 2: Written file appears in TUI output and written_files ==="
  export WAVE=1
  export SCENARIO="written-files"
  local tmpfile="/tmp/qa-file-tracking-$$.txt"

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
    rm -f "$tmpfile"
  }
  trap cleanup_test EXIT

  start_crush_tui 1
  focus_editor
  send_tui_prompt "Create the file $tmpfile with exactly this content: FILE_WRITE_SENTINEL_hello_qa"
  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle"
    capture_tui_evidence "written-files"
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "FILE_WRITE_SENTINEL_hello_qa"; then
    pass "Scenario 2: TUI output contains FILE_WRITE_SENTINEL_hello_qa"
  else
    fail "Scenario 2: TUI output does not contain FILE_WRITE_SENTINEL_hello_qa"
    capture_tui_evidence "written-files"
    return
  fi

  capture_tui_evidence "written-files"

  # Verify the actual file was created with expected content.
  if [[ -f "$tmpfile" ]]; then
    local content
    content=$(cat "$tmpfile")
    if [[ "$content" == *"FILE_WRITE_SENTINEL_hello_qa"* ]]; then
      pass "Scenario 2: $tmpfile exists with expected content"
    else
      fail "Scenario 2: $tmpfile exists but content is unexpected: $content"
    fi
  else
    fail "Scenario 2: $tmpfile was not created"
  fi

  # Secondary DB checks.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID found in DB"
    return
  fi

  local written_paths
  written_paths=$(query_db "SELECT path FROM written_files WHERE session_id = '$SID'")
  if [[ -z "$written_paths" ]] || [[ "$written_paths" == "[]" ]]; then
    fail "Scenario 2: No entries in written_files for session $SID"
  else
    local has_target
    has_target=$(echo "$written_paths" | jq -r '.[].path' | grep -cF "$tmpfile" || true)
    if [[ "$has_target" -ge 1 ]]; then
      pass "Scenario 2: $tmpfile recorded in written_files"
    else
      fail "Scenario 2: $tmpfile not recorded in written_files"
    fi
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_read_files_tracking
test_written_files_tracking

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
