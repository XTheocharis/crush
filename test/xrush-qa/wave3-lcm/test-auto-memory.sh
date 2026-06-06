#!/usr/bin/env bash
# Test: Auto-memory extraction after multi-turn conversation.
# Verifies that Crush extracts structured memories from conversation turns,
# writes CRUSH.memory.md, and assigns varied priority levels (not all 'low').
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# Shared session ID across scenarios (set in Scenario 1, reused in 2 and 3).
SID=""

# ---------------------------------------------------------------------------
# Scenario 1: Auto-memory extracted after turns
# ---------------------------------------------------------------------------
test_auto_memory_extracted() {
  echo "=== Scenario 1: Auto-memory extracted after turns ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    command restore_crush
  }
  trap restore_on_exit EXIT

  # Wave 3 config has auto_memory_interval: 1 (extraction every turn).
  start_crush 3

  # First turn: state preferences to give the memory extractor signal.
  send_prompt "I prefer using table-driven tests in Go. My project uses testify for assertions."
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_evidence 14 "auto-memory-extract"
    return
  fi

  # Second turn: add more preference signal.
  send_prompt "I also like using t.Parallel() for parallel tests."
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle after second prompt"
    capture_evidence 14 "auto-memory-extract"
    return
  fi

  # Give the async memory extractor time to run.
  sleep 5

  # Get the session ID.
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 14 "auto-memory-extract"
    return
  fi

  # Query lcm_auto_memory for this session.
  local mem_count
  mem_count=$(sqlite3 .crush/crush.db "SELECT COUNT(*) FROM lcm_auto_memory WHERE session_id = '$SID'")
  if [[ "$mem_count" -lt 1 ]]; then
    fail "Scenario 1: Expected >= 1 auto-memory entries, got $mem_count"
    capture_evidence 14 "auto-memory-extract"
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

  capture_evidence 14 "auto-memory-extract"
  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: CRUSH.memory.md file created
# ---------------------------------------------------------------------------
test_memory_file_created() {
  echo "=== Scenario 2: CRUSH.memory.md file created ==="

  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID from Scenario 1 — skipping"
    return
  fi

  # The memory file should exist in the project root (working directory).
  if [[ -f "CRUSH.memory.md" ]]; then
    pass "Scenario 2: CRUSH.memory.md file exists"
  else
    fail "Scenario 2: CRUSH.memory.md file not found"
    return
  fi

  # Assert: file is non-empty.
  if [[ -s "CRUSH.memory.md" ]]; then
    pass "Scenario 2: CRUSH.memory.md is non-empty"
  else
    fail "Scenario 2: CRUSH.memory.md is empty"
  fi

  # Assert: file contains the auto-generated marker.
  if grep -q "<!-- auto-generated by Crush -->" "CRUSH.memory.md"; then
    pass "Scenario 2: CRUSH.memory.md has auto-generated marker"
  else
    fail "Scenario 2: CRUSH.memory.md missing auto-generated marker"
  fi

  if grep -Eiq "table-driven|parallel|testify|test" "CRUSH.memory.md"; then
    pass "Scenario 2: CRUSH.memory.md contains stated testing preference content"
  else
    fail "Scenario 2: CRUSH.memory.md does not contain the stated testing preferences"
  fi

  capture_evidence 14 "memory-file"
}

# ---------------------------------------------------------------------------
# Scenario 3: Priority scoring is not all 'low' (bug check)
# ---------------------------------------------------------------------------
test_priority_not_all_low() {
  echo "=== Scenario 3: Priority scoring is not all 'low' (bug check) ==="

  if [[ -z "$SID" ]]; then
    fail "Scenario 3: No session ID from Scenario 1 — skipping"
    return
  fi

  # Query distinct priority levels for this session.
  local distinct_priorities
  distinct_priorities=$(sqlite3 .crush/crush.db \
    "SELECT DISTINCT priority FROM lcm_auto_memory WHERE session_id = '$SID'")

  local priority_count
  priority_count=$(echo "$distinct_priorities" | wc -l)

  # Check for at least one non-'low' priority.
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

  # Also check: at least 2 distinct priority levels is desirable.
  if [[ "$priority_count" -ge 2 ]]; then
    pass "Scenario 3: At least 2 distinct priority levels ($priority_count)"
  else
    fail "Scenario 3: Only $priority_count distinct priority level(s) — expected >= 2"
  fi

  # Report the distribution for debugging.
  echo "  Priority distribution:"
  sqlite3 .crush/crush.db \
    "SELECT priority, COUNT(*) FROM lcm_auto_memory WHERE session_id = '$SID' GROUP BY priority" | while IFS='|' read -r pri cnt; do
    echo "    $pri: $cnt"
  done

  capture_evidence 14 "priority-check"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_auto_memory_extracted
test_memory_file_created
test_priority_not_all_low

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
