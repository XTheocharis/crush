#!/usr/bin/env bash
# Test: Map tool execution and DB recording via TUI.
# Verifies that agentic_map spawns sub-agents, records the run in lcm_map_runs,
# and tracks individual items in lcm_map_items. Uses deterministic sentinels
# for TUI-based assertion.
set -euo pipefail

WAVE=3
SCENARIO="map-tools-init"
source "$(dirname "$0")/../lib/common.sh"
source "$(dirname "$0")/../lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup_test() { cleanup_tui; restore_crush; }
trap cleanup_test EXIT

# Shared session ID across scenarios (set in Scenario 1, reused in Scenario 2).
SID=""

# ---------------------------------------------------------------------------
# Scenario 1: Map tool execution recorded in DB
# ---------------------------------------------------------------------------
test_map_tool_recording() {
  SCENARIO="map-tools-s1"
  echo "=== Scenario 1: Map tool execution recorded in DB ==="

  setup_clean_crush

  # Copy fixture to /tmp for the tool to operate on.
  QA_DIR="${QA_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
  cp "$QA_DIR/fixtures/sample.jsonl" /tmp/test-map-sentinel.jsonl

  start_crush_tui 3

  focus_editor
  send_tui_prompt "Use the agentic_map tool on /tmp/test-map-sentinel.jsonl to add a 'processed' field set to true to each item. Write output to /tmp/test-map-sentinel-output.jsonl. When done, reply with the sentinel MAP_TOOLS_SENTINEL_42"
  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_tui_evidence "map-tools-timeout"
    return
  fi

  # PRIMARY: assert TUI contains the sentinel confirming completion.
  if assert_tui_contains "MAP_TOOLS_SENTINEL_42"; then
    pass "Scenario 1: TUI contains sentinel MAP_TOOLS_SENTINEL_42"
  else
    fail "Scenario 1: TUI missing sentinel MAP_TOOLS_SENTINEL_42"
    capture_tui_evidence "map-tools-missing-sentinel"
    return
  fi

  # Get the session ID for DB secondary checks.
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_tui_evidence "map-tools-no-sid"
    return
  fi

  # SECONDARY: at least 1 row in lcm_map_runs with correct tool_type.
  local map_runs run_count
  map_runs=$(query_db "SELECT COUNT(*) as cnt FROM lcm_map_runs WHERE session_id = '$SID' AND tool_type IN ('agentic_map', 'llm_map')")
  run_count=$(echo "$map_runs" | jq '.[0].cnt // 0')
  if [[ "$run_count" -ge 1 ]]; then
    pass "Scenario 1: Found $run_count map run(s) in lcm_map_runs"
  else
    fail "Scenario 1: Expected >= 1 map run(s) in lcm_map_runs, got $run_count"
  fi

  # SECONDARY: get run_id for item-level checks.
  local run_id
  run_id=$(query_db "SELECT run_id FROM lcm_map_runs WHERE session_id = '$SID' AND tool_type IN ('agentic_map', 'llm_map') LIMIT 1" | jq -r '.[0].run_id // empty')
  if [[ -z "$run_id" ]]; then
    fail "Scenario 1: Could not retrieve run_id from lcm_map_runs"
    capture_tui_evidence "map-tools-no-run-id"
    return
  fi

  # SECONDARY: map run metadata records DONE status and paths.
  local run_metadata run_metadata_count
  run_metadata=$(query_db "SELECT COUNT(*) as cnt FROM lcm_map_runs WHERE run_id = '$run_id' AND status = 'DONE' AND input_path = '/tmp/test-map-sentinel.jsonl' AND output_path = '/tmp/test-map-sentinel-output.jsonl'")
  run_metadata_count=$(echo "$run_metadata" | jq '.[0].cnt // 0')
  if [[ "$run_metadata_count" -eq 1 ]]; then
    pass "Scenario 1: Map run metadata records DONE status and exact input/output paths"
  else
    fail "Scenario 1: Map run metadata missing DONE status or exact input/output paths"
  fi

  # SECONDARY: completed items in lcm_map_items.
  local completed_items completed_count
  completed_items=$(query_db "SELECT COUNT(*) as cnt FROM lcm_map_items WHERE run_id = '$run_id' AND status = 'DONE'")
  completed_count=$(echo "$completed_items" | jq '.[0].cnt // 0')
  if [[ "$completed_count" -ge 1 ]]; then
    pass "Scenario 1: Found $completed_count done item(s) in lcm_map_items"
  else
    fail "Scenario 1: Expected >= 1 done item(s) in lcm_map_items, got $completed_count"
  fi

  # SECONDARY: total items match fixture line count.
  local total_items item_count fixture_lines
  total_items=$(query_db "SELECT COUNT(*) as cnt FROM lcm_map_items WHERE run_id = '$run_id'")
  item_count=$(echo "$total_items" | jq '.[0].cnt // 0')
  fixture_lines=$(wc -l < "$QA_DIR/fixtures/sample.jsonl")
  if [[ "$item_count" -eq "$fixture_lines" ]]; then
    pass "Scenario 1: lcm_map_items has $item_count items (matches fixture)"
  else
    fail "Scenario 1: lcm_map_items has $item_count items, expected $fixture_lines"
  fi

  capture_tui_evidence "map-tools-final"
  rm -f /tmp/test-map-sentinel.jsonl /tmp/test-map-sentinel-output.jsonl
}

# ---------------------------------------------------------------------------
# Scenario 2: LLM map transform with sentinel output
# ---------------------------------------------------------------------------
test_llm_map_transform() {
  SCENARIO="map-tools-s2"
  echo "=== Scenario 2: LLM map transform with sentinel output ==="

  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID from Scenario 1 — skipping"
    return
  fi

  # Create a second fixture for llm_map.
  echo '{"text":"hello world"}' > /tmp/test-llm-map-sentinel.jsonl
  echo '{"text":"goodbye world"}' >> /tmp/test-llm-map-sentinel.jsonl

  focus_editor
  send_tui_prompt "Use the llm_map tool on /tmp/test-llm-map-sentinel.jsonl to translate each item's text field to uppercase. Write output to /tmp/test-llm-map-sentinel-output.jsonl. When done, reply with the sentinel MAP_AGENT_OUTPUT_55"
  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle within 180s"
    capture_tui_evidence "llm-map-timeout"
    return
  fi

  # PRIMARY: assert TUI contains the sentinel.
  if assert_tui_contains "MAP_AGENT_OUTPUT_55"; then
    pass "Scenario 2: TUI contains sentinel MAP_AGENT_OUTPUT_55"
  else
    fail "Scenario 2: TUI missing sentinel MAP_AGENT_OUTPUT_55"
    capture_tui_evidence "llm-map-missing-sentinel"
    return
  fi

  # SECONDARY: verify additional map runs were recorded.
  local total_runs
  total_runs=$(query_db "SELECT COUNT(*) as cnt FROM lcm_map_runs WHERE session_id = '$SID' AND tool_type IN ('agentic_map', 'llm_map')" | jq '.[0].cnt // 0')
  if [[ "$total_runs" -ge 1 ]]; then
    pass "Scenario 2: Total map runs in session: $total_runs"
  else
    fail "Scenario 2: No map runs recorded"
  fi

  capture_tui_evidence "llm-map-final"
  rm -f /tmp/test-llm-map-sentinel.jsonl /tmp/test-llm-map-sentinel-output.jsonl
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_map_tool_recording
test_llm_map_transform

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
