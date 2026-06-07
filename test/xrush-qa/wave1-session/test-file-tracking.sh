#!/usr/bin/env bash
# Test: File tracking in read_files and written_files tables.
# Verifies that Crush records files viewed by the agent in read_files,
# and files written by the agent in written_files.
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
# Scenario 1: Viewed file appears in read_files
# ---------------------------------------------------------------------------
test_read_files_tracking() {
  echo "=== Scenario 1: Viewed file appears in read_files ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # cleanup_test is called below
  cleanup_test() {
    restore_crush
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
  }
  trap cleanup_test EXIT

  start_crush 1
  send_prompt "Show me the contents of go.mod"
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 7 "read-files"
    stop_crush
    return
  fi

  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 7 "read-files"
    stop_crush
    return
  fi

  # Query read_files table for this session.
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

  capture_evidence 7 "read-files"
  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: Written file appears in written_files
# ---------------------------------------------------------------------------
test_written_files_tracking() {
  echo "=== Scenario 2: Written file appears in written_files ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # cleanup_test is called below
  cleanup_test() {
    restore_crush
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
  }
  trap cleanup_test EXIT

  start_crush 1
  send_prompt "Create a file /tmp/qa-test-file.txt with the text 'hello qa test'"
  if ! wait_for_idle 120; then
    fail "Scenario 2: Crush did not become idle"
    capture_evidence 7 "written-files"
    stop_crush
    return
  fi

  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID found in DB"
    capture_evidence 7 "written-files"
    stop_crush
    return
  fi

  # Query written_files table for this session.
  local written_paths
  written_paths=$(query_db "SELECT path FROM written_files WHERE session_id = '$SID'")
  if [[ -z "$written_paths" ]] || [[ "$written_paths" == "[]" ]]; then
    fail "Scenario 2: No entries in written_files for session $SID"
  else
    local has_target
    has_target=$(echo "$written_paths" | jq -r '.[].path' | grep -Ec '^/tmp/qa-test-file\.txt$' || true)
    if [[ "$has_target" -ge 1 ]]; then
      pass "Scenario 2: /tmp/qa-test-file.txt recorded in written_files"
    else
      fail "Scenario 2: /tmp/qa-test-file.txt not recorded in written_files"
    fi
  fi

  # Also verify the actual file was created with expected content.
  if [[ -f /tmp/qa-test-file.txt ]]; then
    local content
    content=$(cat /tmp/qa-test-file.txt)
    if [[ "$content" == *"hello qa test"* ]]; then
      pass "Scenario 2: /tmp/qa-test-file.txt exists with expected content"
    else
      fail "Scenario 2: /tmp/qa-test-file.txt exists but content is unexpected: $content"
    fi
  else
    fail "Scenario 2: /tmp/qa-test-file.txt was not created"
  fi

  capture_evidence 7 "written-files"
  stop_crush
  rm -f /tmp/qa-test-file.txt
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_read_files_tracking
test_written_files_tracking

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
