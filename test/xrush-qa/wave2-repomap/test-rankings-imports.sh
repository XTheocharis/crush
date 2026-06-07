#!/usr/bin/env bash
# Test: Repo map rankings and import resolution (TUI-first approach).
# Verifies that PageRank-based session rankings produce positive scores
# and that the import graph contains edges for the project.
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
# Scenario 1: Rankings have positive scores
# ---------------------------------------------------------------------------
test_rankings_positive_scores() {
  echo "=== Scenario 1: Rankings have positive scores ==="
  WAVE=2
  SCENARIO="rankings-positive-scores"

  setup_clean_crush
  # shellcheck disable=SC2317
  cleanup_test() {
    cleanup_tui
    restore_crush
  }
  trap cleanup_test EXIT

  start_crush_tui 2
  focus_editor
  send_tui_prompt "Tell me about the agent package in this project. Somewhere in your reply include the exact token RANKINGS_AGENT_SENTINEL_42"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "RANKINGS_AGENT_SENTINEL_42"; then
    pass "Scenario 1: TUI shows RANKINGS_AGENT_SENTINEL_42 sentinel"
  else
    fail "Scenario 1: TUI does not show RANKINGS_AGENT_SENTINEL_42 sentinel"
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

  # Verify files ranked with positive scores exist.
  local ranked_count
  ranked_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_session_rankings WHERE session_id = '$SID' AND rank > 0" | jq '.[0].count')
  if [[ "$ranked_count" -gt 0 ]]; then
    pass "Scenario 1: $ranked_count files ranked with positive scores"
  else
    fail "Scenario 1: No files ranked with positive scores"
  fi

  # Log top-ranked files for evidence.
  local top_files
  top_files=$(query_db "SELECT rel_path, rank FROM repo_map_session_rankings WHERE session_id = '$SID' AND rank > 0 ORDER BY rank DESC LIMIT 10")
  echo "Top-ranked files:"
  echo "$top_files" | jq -r '.[] | "  \(.rel_path) score=\(.rank)"' 2>/dev/null || echo "$top_files"

  local agent_ranked
  agent_ranked=$(query_db "SELECT COUNT(*) as count FROM repo_map_session_rankings WHERE session_id = '$SID' AND rank > 0 AND rel_path LIKE 'internal/agent/%'" | jq '.[0].count')
  if [[ "$agent_ranked" -ge 1 ]]; then
    pass "Scenario 1: Agent-package prompt ranked internal/agent files"
  else
    fail "Scenario 1: Agent-package prompt did not rank any internal/agent files"
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 1
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Scenario 2: Import graph has edges
# ---------------------------------------------------------------------------
test_import_graph_edges() {
  echo "=== Scenario 2: Import graph has edges ==="
  WAVE=2
  SCENARIO="import-graph-edges"

  setup_clean_crush
  # shellcheck disable=SC2317
  cleanup_test() {
    cleanup_tui
    restore_crush
  }
  trap cleanup_test EXIT

  start_crush_tui 2
  focus_editor
  send_tui_prompt "Tell me about the project imports. Somewhere in your reply include the exact token IMPORTS_GRAPH_SENTINEL_88"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "IMPORTS_GRAPH_SENTINEL_88"; then
    pass "Scenario 2: TUI shows IMPORTS_GRAPH_SENTINEL_88 sentinel"
  else
    fail "Scenario 2: TUI does not show IMPORTS_GRAPH_SENTINEL_88 sentinel"
    capture_tui_evidence "sentinel-missing"
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary DB checks ---
  local import_count
  import_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_imports" | jq '.[0].count')
  if [[ "$import_count" -gt 100 ]]; then
    pass "Scenario 2: $import_count import edges found (>100)"
  else
    fail "Scenario 2: Only $import_count import edges found, expected >100"
  fi

  # Log agent-package imports for evidence.
  local agent_imports
  agent_imports=$(query_db "SELECT path, import_path FROM repo_map_imports WHERE path LIKE '%/agent/%' LIMIT 10")
  echo "Agent package imports (sample):"
  echo "$agent_imports" | jq -r '.[] | "  \(.path) -> \(.import_path)"' 2>/dev/null || echo "$agent_imports"

  # Verify known agent->message import edge.
  local known_import
  known_import=$(query_db "SELECT COUNT(*) as count FROM repo_map_imports WHERE path='internal/agent/agent.go' AND import_path='github.com/charmbracelet/crush/internal/message'" | jq '.[0].count')
  if [[ "$known_import" -ge 1 ]]; then
    pass "Scenario 2: Known internal/agent -> internal/message import edge found"
  else
    fail "Scenario 2: Missing known internal/agent/agent.go -> internal/message import edge"
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 1
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_rankings_positive_scores
test_import_graph_edges

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
