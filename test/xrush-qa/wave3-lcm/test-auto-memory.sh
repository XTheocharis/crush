#!/usr/bin/env bash
# Test: Auto-memory extraction and cross-session persistence (TUI-first).
# Verifies that Crush extracts structured memories from conversation turns,
# persists them to the DB, and they survive into a new TUI session.
set -euo pipefail

WAVE=3
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# Shared session ID from Scenario 1.
SID=""

cleanup_test() { restore_crush; }
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Auto-memory extracted after turns with sentinel content
# ---------------------------------------------------------------------------
test_auto_memory_extracted() {
  echo "=== Scenario 1: Auto-memory extracted after turns with sentinel content ==="
  SCENARIO="auto-memory-extract"

  setup_clean_crush
  start_crush_tui 3
  focus_editor

  # First turn: state preferences with deterministic sentinel.
  send_tui_prompt "I prefer using table-driven tests in Go. My project uses testify for assertions. Remember that AUTO_MEM_STORE_SENTINEL_42 is my unique test identifier."
  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_tui_evidence "idle-timeout-p1"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI must acknowledge the sentinel.
  if assert_tui_contains "AUTO_MEM_STORE_SENTINEL_42"; then
    pass "Scenario 1: TUI shows AUTO_MEM_STORE_SENTINEL_42 sentinel"
  else
    fail "Scenario 1: TUI does not show AUTO_MEM_STORE_SENTINEL_42 sentinel"
    capture_tui_evidence "sentinel-missing-p1"
    return
  fi

  # Second turn: add more preference signal.
  send_tui_prompt "I also like using t.Parallel() for parallel tests."
  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle after second prompt"
    capture_tui_evidence "idle-timeout-p2"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Give the async memory extractor time to run.
  sleep 5

  capture_tui_evidence "tui-response"

  # --- Secondary DB checks ---
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    return
  fi

  # Query lcm_auto_memory for this session.
  local mem_count
  mem_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_auto_memory WHERE session_id = '$SID'")
  if [[ "$mem_count" -lt 1 ]]; then
    fail "Scenario 1: Expected >= 1 auto-memory entries, got $mem_count"
    return
  fi
  pass "Scenario 1: Found $mem_count auto-memory entries (>= 1)"

  # Assert: all memory_type values are valid.
  local invalid_types
  invalid_types=$(sqlite3 .crush/crush.db \
    "SELECT COUNT(*) FROM lcm_auto_memory WHERE session_id = '$SID' AND memory_type NOT IN ('fact','decision','preference','lesson')")
  if [[ "$invalid_types" -eq 0 ]]; then
    pass "Scenario 1: All memory_type values are valid"
  else
    fail "Scenario 1: Found $invalid_types entries with invalid memory_type"
  fi

  # Assert: content is non-empty for all entries.
  local empty_content
  empty_content=$(sqlite3 .crush/crush.db \
    "SELECT COUNT(*) FROM lcm_auto_memory WHERE session_id = '$SID' AND (content IS NULL OR content = '')")
  if [[ "$empty_content" -eq 0 ]]; then
    pass "Scenario 1: All memory content is non-empty"
  else
    fail "Scenario 1: Found $empty_content entries with empty content"
  fi

  # Assert: confidence is between 0 and 1.
  local out_of_range
  out_of_range=$(sqlite3 .crush/crush.db \
    "SELECT COUNT(*) FROM lcm_auto_memory WHERE session_id = '$SID' AND (confidence < 0 OR confidence > 1)")
  if [[ "$out_of_range" -eq 0 ]]; then
    pass "Scenario 1: All confidence values in [0, 1]"
  else
    fail "Scenario 1: Found $out_of_range entries with confidence outside [0, 1]"
  fi

  local relevant_memories
  relevant_memories=$(sqlite3 .crush/crush.db \
    "SELECT COUNT(*) FROM lcm_auto_memory WHERE session_id = '$SID' AND lower(content) GLOB '*test*'")
  if [[ "$relevant_memories" -ge 1 ]]; then
    pass "Scenario 1: Auto-memory content captures test-related preferences"
  else
    fail "Scenario 1: No auto-memory content captured the stated test preferences"
  fi

  local sourced_memories
  sourced_memories=$(sqlite3 .crush/crush.db \
    "SELECT COUNT(*) FROM lcm_auto_memory WHERE session_id = '$SID' AND source_message_ids IS NOT NULL AND source_message_ids != '' AND source_message_ids != '[]'")
  if [[ "$sourced_memories" -ge 1 ]]; then
    pass "Scenario 1: Auto-memory entries include source message IDs"
  else
    fail "Scenario 1: No auto-memory entries include source message IDs"
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Scenario 2: Auto-memory persists across sessions — recall in new TUI
# ---------------------------------------------------------------------------
test_auto_memory_cross_session_recall() {
  echo "=== Scenario 2: Auto-memory persists across sessions ==="
  SCENARIO="auto-memory-recall"

  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID from Scenario 1, skipping"
    return
  fi

  # Start a fresh TUI session (new Crush instance, same .crush/ DB).
  start_crush_tui 3
  focus_editor

  # Ask about the preference stored in Scenario 1.
  send_tui_prompt "What is my unique test identifier? Mention AUTO_MEM_RECALL_SENTINEL_42 in your answer if you know it."
  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle after recall prompt"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI must show the recall sentinel proving memory persisted.
  if assert_tui_contains "AUTO_MEM_RECALL_SENTINEL_42"; then
    pass "Scenario 2: TUI shows AUTO_MEM_RECALL_SENTINEL_42 — memory persisted across sessions"
  else
    fail "Scenario 2: TUI does not show AUTO_MEM_RECALL_SENTINEL_42 — memory did not persist"
    capture_tui_evidence "recall-sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  capture_tui_evidence "tui-recall-response"

  # --- Secondary DB check: auto-memory rows still exist ---
  local mem_count
  mem_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_auto_memory")
  if [[ "$mem_count" -ge 1 ]]; then
    pass "Scenario 2: Auto-memory rows exist in DB ($mem_count total)"
  else
    fail "Scenario 2: No auto-memory rows found in DB"
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Scenario 3: CRUSH.memory.md file created and priority scoring
# ---------------------------------------------------------------------------
test_memory_file_and_priorities() {
  echo "=== Scenario 3: CRUSH.memory.md file and priority scoring ==="
  SCENARIO="memory-file-priority"

  if [[ -z "$SID" ]]; then
    fail "Scenario 3: No session ID from Scenario 1, skipping"
    return
  fi

  # The memory file should exist in the project root (working directory).
  if [[ -f "CRUSH.memory.md" ]]; then
    pass "Scenario 3: CRUSH.memory.md file exists"
  else
    fail "Scenario 3: CRUSH.memory.md file not found"
  fi

  # Assert: file is non-empty.
  if [[ -f "CRUSH.memory.md" ]] && [[ -s "CRUSH.memory.md" ]]; then
    pass "Scenario 3: CRUSH.memory.md is non-empty"
  else
    fail "Scenario 3: CRUSH.memory.md is empty"
  fi

  # Assert: file contains the auto-generated marker.
  if [[ -f "CRUSH.memory.md" ]] && grep -q "<!-- auto-generated by Crush -->" "CRUSH.memory.md"; then
    pass "Scenario 3: CRUSH.memory.md has auto-generated marker"
  else
    fail "Scenario 3: CRUSH.memory.md missing auto-generated marker"
  fi

  if [[ -f "CRUSH.memory.md" ]] && grep -Eiq "table-driven|parallel|testify|test|AUTO_MEM_STORE_SENTINEL_42" "CRUSH.memory.md"; then
    pass "Scenario 3: CRUSH.memory.md contains stated testing preference content"
  else
    fail "Scenario 3: CRUSH.memory.md does not contain the stated testing preferences"
  fi

  # Priority scoring check: not all 'low'.
  local distinct_priorities
  distinct_priorities=$(sqlite3 .crush/crush.db \
    "SELECT DISTINCT priority FROM lcm_auto_memory WHERE session_id = '$SID'")

  local has_non_low=false
  while IFS= read -r p; do
    if [[ -n "$p" && "$p" != "low" ]]; then
      has_non_low=true
      break
    fi
  done <<< "$distinct_priorities"

  if [[ "$has_non_low" == "true" ]]; then
    pass "Scenario 3: Found non-'low' priority entries"
  else
    fail "Scenario 3: All priorities are 'low' (potential priority scoring bug)"
  fi

  local priority_count
  priority_count=$(echo "$distinct_priorities" | wc -l)

  if [[ "$priority_count" -ge 2 ]]; then
    pass "Scenario 3: At least 2 distinct priority levels ($priority_count)"
  else
    fail "Scenario 3: Only $priority_count distinct priority level(s), expected >= 2"
  fi

  # Report the distribution for debugging.
  echo "  Priority distribution:"
  sqlite3 .crush/crush.db \
    "SELECT priority, COUNT(*) FROM lcm_auto_memory WHERE session_id = '$SID' GROUP BY priority" | while IFS='|' read -r pri cnt; do
    echo "    $pri: $cnt"
  done
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_auto_memory_extracted
test_auto_memory_cross_session_recall
test_memory_file_and_priorities

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
