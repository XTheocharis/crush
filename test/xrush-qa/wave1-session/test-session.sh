#!/usr/bin/env bash
# Test: Basic session creation and persistence.
# Verifies that Crush creates sessions with correct DB state when run
# interactively in tmux, that sessions are listed via CLI, and that
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

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called below
  restore_crush() {
    command restore_crush
    # Also restore crush.json in case start_crush left a backup.
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
  }
  trap restore_crush EXIT

  start_crush 1
  send_prompt "What is 2+2? Reply with just the number."
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 5 "session-created"
    stop_crush
    return
  fi

  # Get the session ID.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 5 "session-created"
    stop_crush
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

    # Verify non-zero timestamps.
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
  answer_count=$(query_db "SELECT COUNT(*) as count FROM message_parts mp JOIN messages m ON m.id = mp.message_id WHERE m.session_id = '$SID' AND m.role = 'assistant' AND mp.part_type = 'text' AND mp.content_json GLOB '*4*'" | jq '.[0].count')
  if [[ "$answer_count" -ge 1 ]]; then
    pass "Scenario 1: Assistant response contains the expected arithmetic answer"
  else
    fail "Scenario 1: Assistant response did not contain the expected answer 4"
  fi

  capture_evidence 5 "session-created"
  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: Session list returns created sessions
# ---------------------------------------------------------------------------
test_session_list() {
  echo "=== Scenario 2: Session list returns created sessions ==="

  # After Scenario 1, at least one session should exist.
  local session_list
  session_list=$(crush session list --json 2>/dev/null || echo "[]")

  local count
  count=$(echo "$session_list" | jq 'length')
  if [[ "$count" -ge 1 ]]; then
    pass "Scenario 2: session list returned $count session(s)"
  else
    fail "Scenario 2: session list returned 0 sessions"
  fi

  if [[ -n "$CREATED_SID" ]] && echo "$session_list" | jq -e --arg sid "$CREATED_SID" 'any(.[]; .id == $sid)' >/dev/null; then
    pass "Scenario 2: session list includes created session $CREATED_SID"
  else
    fail "Scenario 2: session list does not include created session $CREATED_SID"
  fi

  # Save evidence.
  mkdir -p "${EVIDENCE_DIR:-.sisyphus/evidence}"
  echo "$session_list" > "${EVIDENCE_DIR:-.sisyphus/evidence}/task-5-session-list.txt"
}

# ---------------------------------------------------------------------------
# Scenario 3: Multiple Crush runs create distinct sessions
# ---------------------------------------------------------------------------
test_multiple_distinct_sessions() {
  echo "=== Scenario 3: Multiple Crush runs create distinct sessions ==="

  setup_clean_crush

  # First run.
  start_crush 1
  send_prompt "Hello"
  if ! wait_for_idle 120; then
    fail "Scenario 3: First run did not become idle"
    stop_crush
    restore_crush
    return
  fi
  stop_crush

  # Second run.
  start_crush 1
  send_prompt "World"
  if ! wait_for_idle 120; then
    fail "Scenario 3: Second run did not become idle"
    stop_crush
    restore_crush
    return
  fi
  stop_crush

  # Verify 2 distinct session IDs.
  local distinct_count
  distinct_count=$(query_db "SELECT COUNT(DISTINCT id) as count FROM sessions" | jq '.[0].count')
  if [[ "$distinct_count" -eq 2 ]]; then
    pass "Scenario 3: $distinct_count distinct sessions found"
  else
    fail "Scenario 3: Expected 2 distinct sessions, got $distinct_count"
  fi

  capture_evidence 5 "multiple-sessions"
  restore_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_session_created
test_session_list
test_multiple_distinct_sessions

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
