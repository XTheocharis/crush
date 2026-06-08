#!/usr/bin/env bash
# Test: Rewind after orchestration — cross-feature interaction.
# Creates parent/child-session work via orchestration prompt, then rewinds
# through TUI (focus_chat → select_message → 'o' → navigate to rewind).
# Verifies parent/child state consistency after rewind via DB checks.
set -euo pipefail

WAVE=4
SCENARIO="rewind-orchestration"
source "$(dirname "$0")/../lib/common.sh"
source "$(dirname "$0")/../lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup_test() { cleanup_tui; restore_crush; }
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario: Orchestration → Rewind (code only) — child work undone, parent intact
# ---------------------------------------------------------------------------
test_orchestration_rewind_code() {
  echo "=== Scenario: Orchestration → Rewind (code only) ==="
  export SCENARIO="rewind-orchestration-code"

  local sentinel_file="/tmp/qa-rewind-orch-code-$$.txt"

  setup_clean_crush
  start_crush_tui "$WAVE"
  focus_editor

  # Turn 1: Orchestration prompt — create file via sub-agents with sentinel.
  send_tui_prompt "Use parallel sub-agents to: (1) Create file $sentinel_file with content REWIND_ORCH_SENTINEL_42, (2) List files in internal/lcm/. After both finish, reply with exactly REWIND_ORCH_SENTINEL_42 on its own line."
  if ! wait_for_tui_idle 180; then
    fail "Scenario: Crush did not become idle after orchestration turn (180s)"
    capture_tui_evidence "orch-turn1-timeout"
    return
  fi
  capture_tui_evidence "orch-after-turn1"

  # Primary gate: TUI sentinel present.
  if assert_tui_contains "REWIND_ORCH_SENTINEL_42"; then
    pass "Scenario: TUI shows REWIND_ORCH_SENTINEL_42 after orchestration"
  else
    fail "Scenario: TUI missing REWIND_ORCH_SENTINEL_42 after orchestration"
    capture_tui_evidence "orch-missing-sentinel"
    return
  fi

  # Verify file on disk.
  if [[ -f "$sentinel_file" ]] && grep -q "REWIND_ORCH_SENTINEL_42" "$sentinel_file"; then
    pass "Scenario: Sentinel file exists with REWIND_ORCH_SENTINEL_42"
  else
    fail "Scenario: Sentinel file missing or wrong content"
    capture_tui_evidence "orch-file-missing"
    return
  fi

  # Record pre-rewind DB state.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario: No session ID found"
    return
  fi
  pass "Scenario: Session ID = $SID"

  local pre_msg_count
  pre_msg_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID'" 2>/dev/null || echo 0)
  echo "  Pre-rewind message count: $pre_msg_count"

  local pre_snapshot_count=0
  if wait_for_snapshots "$SID" 1 30; then
    pre_snapshot_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM turn_snapshots WHERE session_id = '$SID'" 2>/dev/null || echo 0)
  fi
  echo "  Pre-rewind snapshot count: $pre_snapshot_count"

  # Check for child sessions.
  local children
  children=$(run_query "child_sessions_by_parent" "$SID" 2>/dev/null || echo "[]")
  local child_count
  child_count=$(echo "$children" | jq 'length')
  if [[ "$child_count" -ge 1 ]]; then
    pass "Scenario: Found $child_count child session(s) linked to parent"
  else
    echo "  INFO: No child sessions (agent may not have used orchestration)"
  fi

  # --- Trigger rewind (code only) via TUI ---
  focus_chat
  sleep 0.5

  # Select last assistant message (offset 0 = bottom).
  select_message_by_offset 0
  sleep 0.3
  capture_tui_evidence "orch-before-o-key"

  # Press 'o' to open message options dialog.
  tmux send-keys -t "$TMUX_SESSION" o
  sleep 0.8
  capture_tui_evidence "orch-after-o-key"

  # Select "Rewind (code only)" — index 0, press Enter immediately.
  tmux send-keys -t "$TMUX_SESSION" Enter
  capture_tui_evidence "orch-after-rewind-enter"

  # Wait for rewind to complete.
  if ! wait_for_tui_idle 60; then
    fail "Scenario: Crush did not become idle after rewind"
    capture_tui_evidence "orch-rewind-timeout"
    return
  fi
  capture_tui_evidence "orch-after-rewind-idle"

  # --- Assertions after rewind ---
  # Primary: file should be gone or restored to pre-turn state (no sentinel).
  if [[ ! -f "$sentinel_file" ]] || ! grep -q "REWIND_ORCH_SENTINEL_42" "$sentinel_file"; then
    pass "Scenario: Sentinel file reverted after code rewind"
  else
    fail "Scenario: Sentinel file still contains REWIND_ORCH_SENTINEL_42 after rewind"
    capture_tui_evidence "orch-file-not-reverted"
  fi

  # Secondary: conversation should still be intact (code-only rewind).
  local post_msg_count
  post_msg_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID'" 2>/dev/null || echo 0)
  echo "  Post-rewind message count: $post_msg_count"

  if [[ "$post_msg_count" -eq "$pre_msg_count" ]]; then
    pass "Scenario: Message count unchanged after code-only rewind ($post_msg_count)"
  else
    fail "Scenario: Message count changed after code-only rewind ($pre_msg_count → $post_msg_count)"
  fi

  # Secondary: snapshots should exist (async goroutine — poll).
  local post_snapshot_count=0
  if wait_for_snapshots "$SID" 1 30; then
    post_snapshot_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM turn_snapshots WHERE session_id = '$SID'" 2>/dev/null || echo 0)
  fi
  if [[ "$post_snapshot_count" -ge 1 ]]; then
    pass "Scenario: $post_snapshot_count snapshot(s) exist in DB"
  else
    fail "Scenario: No snapshots in DB after rewind"
  fi

  # Secondary: child sessions should still exist in DB (code rewind
  # doesn't delete sessions, only restores file state).
  local children_post
  children_post=$(run_query "child_sessions_by_parent" "$SID" 2>/dev/null || echo "[]")
  local child_count_post
  child_count_post=$(echo "$children_post" | jq 'length')
  if [[ "$child_count_post" -eq "$child_count" ]]; then
    pass "Scenario: Child session count unchanged after code rewind ($child_count_post)"
  else
    fail "Scenario: Child session count changed after code rewind ($child_count → $child_count_post)"
  fi

  rm -f "$sentinel_file"
}

# ---------------------------------------------------------------------------
# Scenario: Orchestration → Rewind (convo only) — messages removed, files intact
# ---------------------------------------------------------------------------
test_orchestration_rewind_convo() {
  echo "=== Scenario: Orchestration → Rewind (convo only) ==="
  export SCENARIO="rewind-orchestration-convo"

  local sentinel_file="/tmp/qa-rewind-orch-convo-$$.txt"

  setup_clean_crush
  start_crush_tui "$WAVE"
  focus_editor

  # Turn 1: Orchestration prompt — create file with child sentinel.
  send_tui_prompt "Use parallel sub-agents to: (1) Create file $sentinel_file with content REWIND_ORCH_CHILD_88, (2) List files in internal/repomap/. After both finish, reply with exactly REWIND_ORCH_CHILD_88 on its own line."
  if ! wait_for_tui_idle 180; then
    fail "Scenario: Crush did not become idle after orchestration turn (180s)"
    capture_tui_evidence "convo-turn1-timeout"
    return
  fi
  capture_tui_evidence "convo-after-turn1"

  # Primary gate: TUI sentinel present.
  if assert_tui_contains "REWIND_ORCH_CHILD_88"; then
    pass "Scenario: TUI shows REWIND_ORCH_CHILD_88 after orchestration"
  else
    fail "Scenario: TUI missing REWIND_ORCH_CHILD_88 after orchestration"
    capture_tui_evidence "convo-missing-sentinel"
    return
  fi

  # Verify file on disk.
  if [[ -f "$sentinel_file" ]] && grep -q "REWIND_ORCH_CHILD_88" "$sentinel_file"; then
    pass "Scenario: Sentinel file exists with REWIND_ORCH_CHILD_88"
  else
    fail "Scenario: Sentinel file missing or wrong content"
    capture_tui_evidence "convo-file-missing"
    return
  fi

  # Record pre-rewind DB state.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario: No session ID found"
    return
  fi
  pass "Scenario: Session ID = $SID"

  local pre_msg_count
  pre_msg_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID'" 2>/dev/null || echo 0)
  echo "  Pre-rewind message count: $pre_msg_count"

  # --- Trigger rewind (convo only) via TUI ---
  focus_chat
  sleep 0.5

  # Select last assistant message (offset 0 = bottom).
  select_message_by_offset 0
  sleep 0.3
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
    fail "Scenario: Crush did not become idle after convo rewind"
    capture_tui_evidence "convo-rewind-timeout"
    return
  fi
  capture_tui_evidence "convo-after-rewind-idle"

  # --- Assertions after rewind ---
  # Primary TUI: sentinel should no longer be visible.
  if assert_tui_not_contains "REWIND_ORCH_CHILD_88"; then
    pass "Scenario: TUI no longer shows REWIND_ORCH_CHILD_88 after convo rewind"
  else
    fail "Scenario: TUI still shows REWIND_ORCH_CHILD_88 after convo rewind"
    capture_tui_evidence "convo-sentinel-still-visible"
  fi

  # Secondary: file should still exist on disk (convo-only rewind).
  if [[ -f "$sentinel_file" ]] && grep -q "REWIND_ORCH_CHILD_88" "$sentinel_file"; then
    pass "Scenario: File still exists with REWIND_ORCH_CHILD_88 after convo rewind"
  else
    fail "Scenario: File missing or altered after convo-only rewind"
    capture_tui_evidence "convo-file-altered"
  fi

  # Secondary DB: message count should have decreased.
  local post_msg_count
  post_msg_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID'" 2>/dev/null || echo 0)
  echo "  Post-rewind message count: $post_msg_count"

  if [[ "$post_msg_count" -lt "$pre_msg_count" ]]; then
    pass "Scenario: Message count decreased ($pre_msg_count → $post_msg_count)"
  else
    fail "Scenario: Message count did not decrease after convo rewind ($pre_msg_count → $post_msg_count)"
  fi

  # Secondary DB: sentinel should be gone from messages.
  local sentinel_in_db
  sentinel_in_db=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID' AND content LIKE '%REWIND_ORCH_CHILD_88%'" 2>/dev/null || echo 0)
  if [[ "$sentinel_in_db" -eq 0 ]]; then
    pass "Scenario: REWIND_ORCH_CHILD_88 messages removed from DB"
  else
    fail "Scenario: REWIND_ORCH_CHILD_88 messages still in DB (count: $sentinel_in_db)"
  fi

  # Secondary: child sessions in DB should still be queryable (rewind
  # only affects conversation messages, not session metadata).
  local children_post
  children_post=$(run_query "child_sessions_by_parent" "$SID" 2>/dev/null || echo "[]")
  local child_count_post
  child_count_post=$(echo "$children_post" | jq 'length')
  if [[ "$child_count_post" -ge 0 ]]; then
    pass "Scenario: Child sessions still queryable after convo rewind ($child_count_post)"
  else
    fail "Scenario: Child session query failed after convo rewind"
  fi

  rm -f "$sentinel_file"
}

# ---------------------------------------------------------------------------
# Scenario: Orchestration → Rewind (both) — files and convo reverted
# ---------------------------------------------------------------------------
test_orchestration_rewind_both() {
  echo "=== Scenario: Orchestration → Rewind (both code+convo) ==="
  export SCENARIO="rewind-orchestration-both"

  local sentinel_file="/tmp/qa-rewind-orch-both-$$.txt"

  setup_clean_crush
  start_crush_tui "$WAVE"
  focus_editor

  # Turn 1: Orchestration prompt — create file with dual sentinel.
  send_tui_prompt "Use parallel sub-agents to: (1) Create file $sentinel_file with content REWIND_ORCH_DUAL_77, (2) List files in internal/treesitter/. After both finish, reply with exactly REWIND_ORCH_DUAL_77 on its own line."
  if ! wait_for_tui_idle 180; then
    fail "Scenario: Crush did not become idle after orchestration turn (180s)"
    capture_tui_evidence "both-turn1-timeout"
    return
  fi
  capture_tui_evidence "both-after-turn1"

  # Primary gate: TUI sentinel present.
  if assert_tui_contains "REWIND_ORCH_DUAL_77"; then
    pass "Scenario: TUI shows REWIND_ORCH_DUAL_77 after orchestration"
  else
    fail "Scenario: TUI missing REWIND_ORCH_DUAL_77 after orchestration"
    capture_tui_evidence "both-missing-sentinel"
    return
  fi

  # Verify file on disk.
  if [[ -f "$sentinel_file" ]] && grep -q "REWIND_ORCH_DUAL_77" "$sentinel_file"; then
    pass "Scenario: Sentinel file exists with REWIND_ORCH_DUAL_77"
  else
    fail "Scenario: Sentinel file missing or wrong content"
    capture_tui_evidence "both-file-missing"
    return
  fi

  # Record pre-rewind DB state.
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario: No session ID found"
    return
  fi
  pass "Scenario: Session ID = $SID"

  local pre_msg_count
  pre_msg_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID'" 2>/dev/null || echo 0)
  echo "  Pre-rewind message count: $pre_msg_count"

  # --- Trigger rewind (both) via TUI ---
  focus_chat
  sleep 0.5

  # Select last assistant message (offset 0 = bottom).
  select_message_by_offset 0
  sleep 0.3
  capture_tui_evidence "both-before-o-key"

  # Press 'o' to open message options dialog.
  tmux send-keys -t "$TMUX_SESSION" o
  sleep 0.8
  capture_tui_evidence "both-after-o-key"

  # Select "Rewind (both)" — index 2: Down Down Enter.
  tmux send-keys -t "$TMUX_SESSION" Down
  sleep 0.2
  tmux send-keys -t "$TMUX_SESSION" Down
  sleep 0.2
  tmux send-keys -t "$TMUX_SESSION" Enter
  capture_tui_evidence "both-after-rewind-enter"

  # Wait for rewind to complete.
  if ! wait_for_tui_idle 60; then
    fail "Scenario: Crush did not become idle after both rewind"
    capture_tui_evidence "both-rewind-timeout"
    return
  fi
  capture_tui_evidence "both-after-rewind-idle"

  # --- Assertions after rewind ---
  # Primary TUI: sentinel should no longer be visible.
  if assert_tui_not_contains "REWIND_ORCH_DUAL_77"; then
    pass "Scenario: TUI no longer shows REWIND_ORCH_DUAL_77 after both rewind"
  else
    fail "Scenario: TUI still shows REWIND_ORCH_DUAL_77 after both rewind"
    capture_tui_evidence "both-sentinel-still-visible"
  fi

  # Secondary: file should be gone or reverted (no sentinel).
  if [[ ! -f "$sentinel_file" ]] || ! grep -q "REWIND_ORCH_DUAL_77" "$sentinel_file"; then
    pass "Scenario: File reverted after both rewind"
  else
    fail "Scenario: File still contains REWIND_ORCH_DUAL_77 after both rewind"
    capture_tui_evidence "both-file-not-reverted"
  fi

  # Secondary DB: message count should have decreased.
  local post_msg_count
  post_msg_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID'" 2>/dev/null || echo 0)
  echo "  Post-rewind message count: $post_msg_count"

  if [[ "$post_msg_count" -lt "$pre_msg_count" ]]; then
    pass "Scenario: Message count decreased ($pre_msg_count → $post_msg_count)"
  else
    fail "Scenario: Message count did not decrease after both rewind ($pre_msg_count → $post_msg_count)"
  fi

  # Secondary DB: sentinel should be gone from messages.
  local sentinel_in_db
  sentinel_in_db=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM messages WHERE session_id = '$SID' AND content LIKE '%REWIND_ORCH_DUAL_77%'" 2>/dev/null || echo 0)
  if [[ "$sentinel_in_db" -eq 0 ]]; then
    pass "Scenario: REWIND_ORCH_DUAL_77 messages removed from DB"
  else
    fail "Scenario: REWIND_ORCH_DUAL_77 messages still in DB (count: $sentinel_in_db)"
  fi

  # Secondary: snapshots should exist (async goroutine — poll).
  local snapshot_count=0
  if wait_for_snapshots "$SID" 1 30; then
    snapshot_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM turn_snapshots WHERE session_id = '$SID'" 2>/dev/null || echo 0)
  fi
  if [[ "$snapshot_count" -ge 1 ]]; then
    pass "Scenario: $snapshot_count snapshot(s) exist in DB"
  else
    fail "Scenario: No snapshots in DB after both rewind"
  fi

  rm -f "$sentinel_file"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_orchestration_rewind_code
test_orchestration_rewind_convo
test_orchestration_rewind_both

echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
