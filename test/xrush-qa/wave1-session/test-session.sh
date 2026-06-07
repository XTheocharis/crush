#!/usr/bin/env bash
# Test: Basic session creation and persistence (TUI-first approach).
# Verifies that Crush creates sessions with correct DB state when run
# interactively in tmux, that a second TUI sees prior context, and that
# multiple runs produce distinct session IDs.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }
CREATED_SID=""

# ---------------------------------------------------------------------------
# Scenario 1: Session created after Crush interaction
# ---------------------------------------------------------------------------
test_session_created() {
  echo "=== Scenario 1: Session created after Crush interaction ==="
  WAVE=1
  SCENARIO="session-created"

  setup_clean_crush
  cleanup_test() {
    restore_crush
  }
  trap cleanup_test EXIT

  start_crush_tui 1
  focus_editor
  send_tui_prompt "What is 2+2? Reply with exactly SESSION_CREATE_OK_42 and nothing else."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "SESSION_CREATE_OK_42"; then
    pass "Scenario 1: TUI shows SESSION_CREATE_OK_42 sentinel"
  else
    fail "Scenario 1: TUI does not show SESSION_CREATE_OK_42 sentinel"
    capture_tui_evidence "sentinel-missing"
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary DB checks ---
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    return
  fi
  CREATED_SID="$SID"

  # Verify sessions table row has non-null ID and title.
  local session_row
  session_row=$(query_db "SELECT id, title, created_at, updated_at FROM sessions WHERE id = '$SID'")
  if [[ -z "$session_row" ]] || [[ "$session_row" == "[]" ]]; then
    fail "Scenario 1: No session row for ID=$SID"
  else
    local title
    title=$(echo "$session_row" | jq -r '.[0].title // empty')
    if [[ -z "$title" ]]; then
      fail "Scenario 1: Session title is null"
    else
      pass "Scenario 1: Session exists with non-null title"
    fi

    local created_at updated_at
    created_at=$(echo "$session_row" | jq -r '.[0].created_at // empty')
    updated_at=$(echo "$session_row" | jq -r '.[0].updated_at // empty')
    if [[ -n "$created_at" ]] && [[ "$created_at" != "null" ]] && [[ -n "$updated_at" ]] && [[ "$updated_at" != "null" ]]; then
      pass "Scenario 1: Timestamps are non-zero"
    else
      fail "Scenario 1: Timestamps are null or missing"
    fi
  fi

  # Verify messages table has >=2 messages for this session.
  local msg_count
  msg_count=$(query_db "SELECT COUNT(*) as count FROM messages WHERE session_id = '$SID'" | jq '.[0].count')
  if [[ "$msg_count" -ge 2 ]]; then
    pass "Scenario 1: $msg_count messages found (>=2)"
  else
    fail "Scenario 1: Only $msg_count messages found, expected >=2"
  fi

  local role_counts
  role_counts=$(query_db "SELECT role, COUNT(*) as count FROM messages WHERE session_id = '$SID' GROUP BY role")
  local user_count assistant_count
  user_count=$(echo "$role_counts" | jq '[.[] | select(.role == "user")][0].count // 0')
  assistant_count=$(echo "$role_counts" | jq '[.[] | select(.role == "assistant")][0].count // 0')
  if [[ "$user_count" -ge 1 ]] && [[ "$assistant_count" -ge 1 ]]; then
    pass "Scenario 1: Session has user and assistant messages"
  else
    fail "Scenario 1: Expected user and assistant messages, got user=$user_count assistant=$assistant_count"
  fi

  local seq_gaps
  seq_gaps=$(query_db "SELECT COUNT(*) as count FROM messages WHERE session_id = '$SID' AND seq < 0" | jq '.[0].count')
  if [[ "$seq_gaps" -eq 0 ]]; then
    pass "Scenario 1: Message seq values are non-negative"
  else
    fail "Scenario 1: Found $seq_gaps negative message seq value(s)"
  fi

  local user_before_assistant
  user_before_assistant=$(query_db "SELECT CASE WHEN (SELECT MIN(seq) FROM messages WHERE session_id = '$SID' AND role = 'user') < (SELECT MIN(seq) FROM messages WHERE session_id = '$SID' AND role = 'assistant') THEN 1 ELSE 0 END as ok" | jq '.[0].ok')
  if [[ "$user_before_assistant" -eq 1 ]]; then
    pass "Scenario 1: User message seq precedes assistant response seq"
  else
    fail "Scenario 1: Assistant response does not follow user message in seq order"
  fi

  local answer_count
  answer_count=$(query_db "SELECT COUNT(*) as count FROM message_parts mp JOIN messages m ON m.id = mp.message_id WHERE m.session_id = '$SID' AND m.role = 'assistant' AND mp.part_type = 'text' AND mp.content_json GLOB '*SESSION_CREATE_OK_42*'" | jq '.[0].count')
  if [[ "$answer_count" -ge 1 ]]; then
    pass "Scenario 1: Assistant response contains the sentinel in DB"
  else
    fail "Scenario 1: Assistant response did not contain sentinel in DB"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: Second TUI sees prior session context
# ---------------------------------------------------------------------------
test_session_visible_in_second_tui() {
  echo "=== Scenario 2: Second TUI sees prior session context ==="
  WAVE=1
  SCENARIO="session-list"

  if [[ -z "$CREATED_SID" ]]; then
    fail "Scenario 2: No session from Scenario 1, skipping"
    return
  fi

  # Launch a second TUI attached to the same project.
  # start_crush_tui generates a unique tmux session name (qa-w1-<timestamp>).
  start_crush_tui 1

  # Wait for TUI to initialize and show session list / recent content.
  if ! wait_for_tui_idle 60; then
    fail "Scenario 2: Second TUI did not become idle"
    capture_tui_evidence "second-tui-timeout"
    return
  fi

  # The second TUI should show the sentinel from Scenario 1 in its output.
  if assert_tui_contains "SESSION_CREATE_OK_42"; then
    pass "Scenario 2: Second TUI shows prior session sentinel"
  else
    fail "Scenario 2: Second TUI does not show prior session sentinel"
    capture_tui_evidence "second-tui-no-sentinel"
    return
  fi

  capture_tui_evidence "second-tui-context"
}

# ---------------------------------------------------------------------------
# Scenario 3: Multiple Crush runs create distinct sessions
# ---------------------------------------------------------------------------
test_multiple_distinct_sessions() {
  echo "=== Scenario 3: Multiple Crush runs create distinct sessions ==="
  WAVE=1
  SCENARIO="multiple-sessions"

  setup_clean_crush
  cleanup_test() {
    restore_crush
  }
  trap cleanup_test EXIT

  # First run — uses unique tmux session name via start_crush_tui.
  start_crush_tui 1
  focus_editor
  send_tui_prompt "Say hello. Reply with exactly SENTINEL_HELLO_99."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 3: First run did not become idle"
    capture_tui_evidence "first-run-timeout"
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    restore_crush
    return
  fi

  # Verify first sentinel.
  if assert_tui_contains "SENTINEL_HELLO_99"; then
    pass "Scenario 3: First run shows SENTINEL_HELLO_99"
  else
    fail "Scenario 3: First run missing SENTINEL_HELLO_99"
    capture_tui_evidence "first-run-no-sentinel"
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    restore_crush
    return
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true

  # Second run — start_crush_tui generates a new unique tmux session name.
  start_crush_tui 1
  focus_editor
  send_tui_prompt "Say world. Reply with exactly SENTINEL_WORLD_77."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 3: Second run did not become idle"
    capture_tui_evidence "second-run-timeout"
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    restore_crush
    return
  fi

  # Verify second sentinel.
  if assert_tui_contains "SENTINEL_WORLD_77"; then
    pass "Scenario 3: Second run shows SENTINEL_WORLD_77"
  else
    fail "Scenario 3: Second run missing SENTINEL_WORLD_77"
    capture_tui_evidence "second-run-no-sentinel"
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    restore_crush
    return
  fi

  capture_tui_evidence "second-run-response"

  # --- Secondary DB check: 2 distinct sessions ---
  local distinct_count
  distinct_count=$(query_db "SELECT COUNT(DISTINCT id) as count FROM sessions" | jq '.[0].count')
  if [[ "$distinct_count" -ge 2 ]]; then
    pass "Scenario 3: $distinct_count distinct sessions found"
  else
    fail "Scenario 3: Expected >=2 distinct sessions, got $distinct_count"
  fi

  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
  restore_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_session_created
test_session_visible_in_second_tui
test_multiple_distinct_sessions

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
