#!/usr/bin/env bash
# Test: Explorer semantic quality verification (TUI-first approach).
# Verifies that the explorer subsystem correctly parses a Go fixture and
# produces an answer containing the expected symbols while excluding decoys.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"
source "$QA_DIR/lib/llm-assertions.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

FIXTURE_REL="test/xrush-qa/fixtures/explorer_fixture.go"
WAVE=2

# ---------------------------------------------------------------------------
# Scenario 1: Explorer produces semantically correct summary
# ---------------------------------------------------------------------------
test_explorer_semantic_quality() {
  echo "=== Scenario 1: Explorer semantic quality ==="
  SCENARIO="explorer-semantic"

  setup_clean_crush
  cleanup_test() { cleanup_tui; restore_crush; }
  trap cleanup_test EXIT

  start_crush_tui "$WAVE"
  focus_editor
  send_tui_prompt "Explore $FIXTURE_REL and report all symbols found. Include the package name, exported types, and functions. Reply with EXPLORER_SEMANTIC_SENTINEL_qaexplorer in your answer."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains_retry "EXPLORER_SEMANTIC_SENTINEL_qaexplorer" 3 10; then
    pass "Scenario 1: TUI shows EXPLORER_SEMANTIC_SENTINEL_qaexplorer sentinel"
  else
    fail "Scenario 1: TUI does not show EXPLORER_SEMANTIC_SENTINEL_qaexplorer sentinel"
    capture_tui_evidence "sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  # Positive assertions: answer must contain expected symbols.
  if assert_tui_contains "FixtureStruct"; then
    pass "Scenario 1: TUI shows FixtureStruct"
  else
    fail "Scenario 1: TUI missing FixtureStruct"
  fi

  if assert_tui_contains "FixtureFunc"; then
    pass "Scenario 1: TUI shows FixtureFunc"
  else
    fail "Scenario 1: TUI missing FixtureFunc"
  fi

  # Negative assertion: decoy symbol must NOT appear.
  if assert_tui_not_contains "DecoyNotReal"; then
    pass "Scenario 1: TUI correctly excludes decoy DecoyNotReal"
  else
    fail "Scenario 1: TUI incorrectly contains decoy DecoyNotReal"
  fi

  capture_tui_evidence "tui-response"

  # --- Primary: DB assertions for explorer/tree-sitter analysis ---
  # repo_map_tags/file_cache are keyed by repo_key+rel_path (no session_id).
  local db_path="${CRUSH_DB:-.crush/crush.db}"
  local tags_count=0 poll_elapsed=0
  while [[ $poll_elapsed -lt 30 ]]; do
    tags_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM repo_map_tags WHERE rel_path LIKE '%explorer_fixture.go'" 2>/dev/null || echo "0")
    [[ "$tags_count" -ge 1 ]] && break
    sleep 2; poll_elapsed=$((poll_elapsed + 2))
  done
  if [[ "$tags_count" -ge 1 ]]; then
    pass "Scenario 1: repo_map_tags has $tags_count rows for explorer_fixture.go"
  else
    fail "Scenario 1: repo_map_tags empty for explorer_fixture.go"
  fi

  local fixture_symbol_count=0; poll_elapsed=0
  while [[ $poll_elapsed -lt 30 ]]; do
    fixture_symbol_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM repo_map_tags WHERE rel_path LIKE '%explorer_fixture.go' AND name IN ('FixtureStruct','FixtureFunc')" 2>/dev/null || echo "0")
    [[ "$fixture_symbol_count" -ge 2 ]] && break
    sleep 2; poll_elapsed=$((poll_elapsed + 2))
  done
  if [[ "$fixture_symbol_count" -ge 2 ]]; then
    pass "Scenario 1: repo_map_tags contains FixtureStruct and FixtureFunc symbols"
  else
    fail "Scenario 1: repo_map_tags missing expected symbols (found $fixture_symbol_count, need 2)"
  fi

  # Optional: lcm_large_files check (non-blocking — only populated if LCM large-file path triggers).
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    local explorer_used
    explorer_used=$(sqlite3 "$db_path" "SELECT explorer_used FROM lcm_large_files WHERE session_id='$SID' AND explorer_used IS NOT NULL LIMIT 1" 2>/dev/null || true)
    if [[ -n "$explorer_used" ]]; then
      pass "Scenario 1: DB shows explorer_used='$explorer_used' for session"
    fi

    local exploration_summary
    exploration_summary=$(sqlite3 "$db_path" "SELECT exploration_summary FROM lcm_large_files WHERE session_id='$SID' AND exploration_summary IS NOT NULL LIMIT 1" 2>/dev/null || true)
    if [[ -n "$exploration_summary" ]]; then
      pass "Scenario 1: DB has exploration_summary for explored file"
    fi
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_explorer_semantic_quality

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
