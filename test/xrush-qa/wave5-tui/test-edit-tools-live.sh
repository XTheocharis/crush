#!/usr/bin/env bash
# Test: Edit tool end-to-end smoke test.
# Scenario 1: Ask Crush to edit a file with specific content, then verify
#   the file on disk matches the expected output and the DB records an edit
#   tool invocation in message_parts.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
SKIP=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }
skip() { echo "SKIP: $1"; ((SKIP += 1)); }

FIXTURE_DIR=""
TARGET_FILE=""

cleanup() {
  stop_crush 2>/dev/null || true
  local json_bak
  json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  if [[ -n "$json_bak" ]]; then
    mv "$json_bak" crush.json
  fi
  restore_crush
  rm -rf "${FIXTURE_DIR:-/tmp/qa-edit-nonexist}"
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Edit file — write specific content, verify disk and DB
# ---------------------------------------------------------------------------
test_edit_file_content() {
  echo "=== Scenario 1: Edit file with specific content ==="

  setup_clean_crush

  QA_DIR_RESOLVED="$QA_DIR"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"

  # Create a temporary fixture file with initial content.
  FIXTURE_DIR=$(mktemp -d "${TMPDIR:-/tmp}/qa-edit-XXXXXX")
  TARGET_FILE="$FIXTURE_DIR/greeting.txt"
  cat > "$TARGET_FILE" <<'INITIAL'
Hello World
INITIAL

  # Copy the wave5 base config.
  local hooks_config
  hooks_config="$PROJECT_DIR/crush.json"
  if [[ -f "$hooks_config" ]]; then
    cp "$hooks_config" "$hooks_config.bak.$(date +%s)"
  fi
  cp "$QA_DIR_RESOLVED/wave5.json" "$hooks_config"

  TMUX_SESSION="qa-edit-$(date +%s)"
  tmux new-session -d -s "$TMUX_SESSION" -x 200 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  sleep 5

  # Ask Crush to edit the fixture file with specific replacement content.
  send_prompt "Edit the file $TARGET_FILE. Replace the entire content with exactly this line: Greetings from Crush QA"
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 151 "edit-file-content"
    stop_crush
    return
  fi

  capture_evidence 151 "edit-file-content"

  # Assert: target file exists and contains the expected content.
  if [[ ! -f "$TARGET_FILE" ]]; then
    fail "Scenario 1: Target file $TARGET_FILE not found"
    stop_crush
    return
  fi

  local actual
  actual=$(cat "$TARGET_FILE")
  if echo "$actual" | grep -qF "Greetings from Crush QA"; then
    pass "Scenario 1: File content includes expected text 'Greetings from Crush QA'"
  else
    fail "Scenario 1: File content does not match expected. Got: $actual"
  fi

  # Assert: no partial writes — file should not contain the original content
  # if the edit replaced it entirely.
  if echo "$actual" | grep -qF "Hello World"; then
    fail "Scenario 1: File still contains original content (partial write suspected)"
  else
    pass "Scenario 1: No leftover original content (no partial write)"
  fi

  # Assert: DB message_parts shows an edit tool was invoked.
  if [[ -f "$PROJECT_DIR/.crush/crush.db" ]]; then
    local session_id
    session_id=$(cd "$PROJECT_DIR" && get_session_id 2>/dev/null || echo "")
    if [[ -n "$session_id" ]]; then
      local edit_count
      edit_count=$(cd "$PROJECT_DIR" && sqlite3 .crush/crush.db \
        "SELECT COUNT(*) FROM message_parts WHERE session_id = '$session_id' AND (tool_name = 'edit' OR tool_name = 'edit_batch')" \
        2>/dev/null || echo 0)
      if [[ "$edit_count" -ge 1 ]]; then
        pass "Scenario 1: DB message_parts shows edit tool invoked ($edit_count time(s))"
      else
        fail "Scenario 1: No edit tool invocation found in message_parts (got $edit_count)"
      fi
    else
      fail "Scenario 1: Could not retrieve session ID from DB"
    fi
  else
    skip "Scenario 1: crush.db not found — skipping DB assertion"
  fi

  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_edit_file_content

echo ""
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
finish_test "test-edit-tools-live" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
