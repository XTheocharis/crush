#!/usr/bin/env bash
# Test: Repo map rankings and import resolution.
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

  setup_clean_crush
  # shellcheck disable=SC2317
  cleanup_test() {
    restore_crush
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
  }
  trap cleanup_test EXIT

  start_crush 2
  send_prompt "Tell me about the agent package in this project"
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 12 "rankings"
    stop_crush
    return
  fi

  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 12 "rankings"
    stop_crush
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

  capture_evidence 12 "rankings"
  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: Import graph has edges
# ---------------------------------------------------------------------------
test_import_graph_edges() {
  echo "=== Scenario 2: Import graph has edges ==="

  # Import graph is populated during repo indexing, independent of session.
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

  # Verify path contains agent paths.
  local agent_import_count
  agent_import_count=$(echo "$agent_imports" | jq 'length')
  if [[ "$agent_import_count" -gt 0 ]]; then
    pass "Scenario 2: Agent package has import relationships"
  else
    fail "Scenario 2: No agent package imports found"
  fi

  local known_import
  known_import=$(query_db "SELECT COUNT(*) as count FROM repo_map_imports WHERE path='internal/agent/agent.go' AND import_path='github.com/charmbracelet/crush/internal/message'" | jq '.[0].count')
  if [[ "$known_import" -ge 1 ]]; then
    pass "Scenario 2: Known internal/agent -> internal/message import edge found"
  else
    fail "Scenario 2: Missing known internal/agent/agent.go -> internal/message import edge"
  fi

  capture_evidence 12 "imports"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_rankings_positive_scores
test_import_graph_edges

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
