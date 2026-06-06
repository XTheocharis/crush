#!/usr/bin/env bash
# Test: Map tool execution and DB recording.
# Verifies that agentic_map spawns sub-agents, records the run in lcm_map_runs,
# and tracks individual items in lcm_map_items.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# Shared session ID.
SID=""

# ---------------------------------------------------------------------------
# Scenario 1: Map tool execution recorded in DB
# ---------------------------------------------------------------------------
test_map_tool_recording() {
  echo "=== Scenario 1: Map tool execution recorded in DB ==="

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

  # Copy fixture to /tmp for the tool to operate on.
  cp "$QA_DIR/fixtures/sample.jsonl" /tmp/test.jsonl

  start_crush 3
  send_prompt "Use the agentic_map tool on /tmp/test.jsonl to add a 'processed' field set to true to each item. Write output to /tmp/test-output.jsonl"
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle (agentic_map may have timed out)"
    capture_evidence 14 "map-tools"
    return
  fi

  # Get the session ID.
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 14 "map-tools"
    return
  fi

  # Assert: at least 1 row in lcm_map_runs for this session with tool_type
  # 'agentic_map' or 'llm_map'.
  local map_runs
  map_runs=$(query_db "SELECT COUNT(*) as cnt FROM lcm_map_runs WHERE session_id = '$SID' AND tool_type IN ('agentic_map', 'llm_map')")
  local run_count
  run_count=$(echo "$map_runs" | jq '.[0].cnt // 0')
  if [[ "$run_count" -ge 1 ]]; then
    pass "Scenario 1: Found $run_count map run(s) in lcm_map_runs"
  else
    fail "Scenario 1: Expected >= 1 map run(s) in lcm_map_runs, got $run_count"
  fi

  # Get the run_id for item checks.
  local run_id
  run_id=$(query_db "SELECT run_id FROM lcm_map_runs WHERE session_id = '$SID' AND tool_type IN ('agentic_map', 'llm_map') LIMIT 1" | jq -r '.[0].run_id // empty')
  if [[ -z "$run_id" ]]; then
    fail "Scenario 1: Could not retrieve run_id from lcm_map_runs"
    capture_evidence 14 "map-tools"
    stop_crush
    return
  fi

  local run_metadata
  run_metadata=$(query_db "SELECT COUNT(*) as cnt FROM lcm_map_runs WHERE run_id = '$run_id' AND status = 'DONE' AND input_path = '/tmp/test.jsonl' AND output_path = '/tmp/test-output.jsonl'")
  local run_metadata_count
  run_metadata_count=$(echo "$run_metadata" | jq '.[0].cnt // 0')
  if [[ "$run_metadata_count" -eq 1 ]]; then
    pass "Scenario 1: Map run metadata records DONE status and exact input/output paths"
  else
    fail "Scenario 1: Map run metadata missing DONE status or exact input/output paths"
  fi

  # Assert: at least 1 item in lcm_map_items with status='DONE'.
  local completed_items
  completed_items=$(query_db "SELECT COUNT(*) as cnt FROM lcm_map_items WHERE run_id = '$run_id' AND status = 'DONE'")
  local completed_count
  completed_count=$(echo "$completed_items" | jq '.[0].cnt // 0')
  if [[ "$completed_count" -ge 1 ]]; then
    pass "Scenario 1: Found $completed_count done item(s) in lcm_map_items"
  else
    fail "Scenario 1: Expected >= 1 done item(s) in lcm_map_items, got $completed_count"
  fi

  # Assert: total items in lcm_map_items matches fixture line count.
  local total_items
  total_items=$(query_db "SELECT COUNT(*) as cnt FROM lcm_map_items WHERE run_id = '$run_id'")
  local item_count
  item_count=$(echo "$total_items" | jq '.[0].cnt // 0')
  local fixture_lines
  fixture_lines=$(wc -l < "$QA_DIR/fixtures/sample.jsonl")
  if [[ "$item_count" -eq "$fixture_lines" ]]; then
    pass "Scenario 1: lcm_map_items has $item_count items (matches fixture)"
  else
    fail "Scenario 1: lcm_map_items has $item_count items, expected $fixture_lines"
  fi

  local output_json_count
  output_json_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_map_items WHERE run_id = '$run_id' AND output_json IS NOT NULL AND output_json != ''" | jq '.[0].cnt // 0')
  if [[ "$output_json_count" -eq "$fixture_lines" ]]; then
    pass "Scenario 1: Every map item has output_json"
  else
    fail "Scenario 1: $output_json_count map items have output_json, expected $fixture_lines"
  fi

  if [[ ! -f /tmp/test-output.jsonl ]]; then
    fail "Scenario 1: /tmp/test-output.jsonl was not created"
  else
    local output_lines
    output_lines=$(wc -l < /tmp/test-output.jsonl)
    if [[ "$output_lines" -eq "$fixture_lines" ]]; then
      pass "Scenario 1: output JSONL line count matches fixture"
    else
      fail "Scenario 1: output JSONL has $output_lines lines, expected $fixture_lines"
    fi

    local processed_count
    processed_count=$(jq -s '[.[] | select(.processed == true)] | length' /tmp/test-output.jsonl)
    if [[ "$processed_count" -eq "$fixture_lines" ]]; then
      pass "Scenario 1: every output item has processed=true"
    else
      fail "Scenario 1: $processed_count output item(s) have processed=true, expected $fixture_lines"
    fi
  fi

  capture_evidence 14 "map-tools"
  stop_crush
  rm -f /tmp/test.jsonl /tmp/test-output.jsonl
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_map_tool_recording

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
