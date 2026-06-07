#!/usr/bin/env bash
# Test: Repo map generation and caching (TUI-first approach).
# Verifies that Crush populates the repo map file cache, creates session
# rankings, and injects the repo map into the system prompt during a session.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

SID=""

# ---------------------------------------------------------------------------
# Scenario 1: Repo map file cache populated
# ---------------------------------------------------------------------------
test_repomap_cache() {
  echo "=== Scenario 1: Repo map file cache populated ==="
  WAVE=2
  SCENARIO="repomap-cache"

  setup_clean_crush
  # shellcheck disable=SC2317
  cleanup_test() {
    restore_crush
  }
  trap cleanup_test EXIT

  start_crush_tui 2
  focus_editor
  send_tui_prompt "What files are in this project? Somewhere in your reply include the exact token REPO_MAP_CACHE_SENTINEL_55"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "REPO_MAP_CACHE_SENTINEL_55"; then
    pass "Scenario 1: TUI shows REPO_MAP_CACHE_SENTINEL_55 sentinel"
  else
    fail "Scenario 1: TUI does not show REPO_MAP_CACHE_SENTINEL_55 sentinel"
    capture_tui_evidence "sentinel-missing"
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary DB checks ---
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    return
  fi

  # Verify file cache has entries.
  local cache_count
  cache_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_file_cache" | jq '.[0].count')
  if [[ "$cache_count" -gt 50 ]]; then
    pass "Scenario 1: repo_map_file_cache has $cache_count entries (>50)"
  else
    fail "Scenario 1: repo_map_file_cache has only $cache_count entries, expected >50"
  fi

  # Log sample file paths as evidence.
  local sample_paths
  sample_paths=$(query_db "SELECT rel_path FROM repo_map_file_cache LIMIT 5")
  echo "Sample cached paths: $sample_paths"

  local required_paths
  required_paths=$(query_db "SELECT COUNT(*) as count FROM repo_map_file_cache WHERE rel_path IN ('main.go','internal/agent/agent.go','internal/lcm/manager.go')" | jq '.[0].count')
  if [[ "$required_paths" -eq 3 ]]; then
    pass "Scenario 1: Required project files are present in repo map cache"
  else
    fail "Scenario 1: Expected main.go, internal/agent/agent.go, and internal/lcm/manager.go in repo map cache; found $required_paths/3"
  fi

  # Keep session alive for scenarios 2 and 3 — do NOT kill tmux here.
}

# ---------------------------------------------------------------------------
# Scenario 2: Session rankings created (reuses SID from Scenario 1)
# ---------------------------------------------------------------------------
test_session_rankings() {
  echo "=== Scenario 2: Session rankings created ==="
  WAVE=2
  SCENARIO="repomap-session-rankings"

  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID from Scenario 1"
    return
  fi

  # Send a follow-up prompt in the same session to trigger rankings.
  focus_editor
  send_tui_prompt "Tell me about the repo map session rankings for this project. Somewhere in your reply include the exact token REPO_MAP_SESSION_SENTINEL_33"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "REPO_MAP_SESSION_SENTINEL_33"; then
    pass "Scenario 2: TUI shows REPO_MAP_SESSION_SENTINEL_33 sentinel"
  else
    fail "Scenario 2: TUI does not show REPO_MAP_SESSION_SENTINEL_33 sentinel"
    capture_tui_evidence "sentinel-missing"
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary DB checks ---
  # Verify session rankings exist for this session.
  local ranking_count
  ranking_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_session_rankings WHERE session_id = '$SID'" | jq '.[0].count')
  if [[ "$ranking_count" -gt 0 ]]; then
    pass "Scenario 2: repo_map_session_rankings has $ranking_count rows for session"
  else
    fail "Scenario 2: repo_map_session_rankings has 0 rows for session $SID"
  fi

  # Verify at least some ranks are positive.
  local top_ranked
  top_ranked=$(query_db "SELECT rel_path, rank FROM repo_map_session_rankings WHERE session_id = '$SID' ORDER BY rank DESC LIMIT 10")
  echo "Top ranked files: $top_ranked"

  local positive_count
  positive_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_session_rankings WHERE session_id = '$SID' AND rank > 0" | jq '.[0].count')
  if [[ "$positive_count" -gt 0 ]]; then
    pass "Scenario 2: $positive_count files have rank > 0"
  else
    fail "Scenario 2: No files with rank > 0"
  fi

  local has_agent_or_main
  has_agent_or_main=$(query_db "SELECT COUNT(*) as count FROM repo_map_session_rankings WHERE session_id = '$SID' AND rank > 0 AND rel_path IN ('main.go','internal/agent/agent.go')" | jq '.[0].count')
  if [[ "$has_agent_or_main" -ge 1 ]]; then
    pass "Scenario 2: Ranking includes at least one high-value entrypoint/agent file"
  else
    fail "Scenario 2: Expected ranked entry for main.go or internal/agent/agent.go"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 3: Repo map injected into system prompt (log check, same session)
# ---------------------------------------------------------------------------
test_repomap_logs() {
  echo "=== Scenario 3: Repo map injected into system prompt (log check) ==="
  WAVE=2
  SCENARIO="repomap-logs"

  # Send a follow-up prompt in the same session.
  focus_editor
  send_tui_prompt "Tell me about repo map injection. Somewhere in your reply include the exact token REPO_MAP_LOG_SENTINEL_22"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 3: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "REPO_MAP_LOG_SENTINEL_22"; then
    pass "Scenario 3: TUI shows REPO_MAP_LOG_SENTINEL_22 sentinel"
  else
    fail "Scenario 3: TUI does not show REPO_MAP_LOG_SENTINEL_22 sentinel"
    capture_tui_evidence "sentinel-missing"
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary log checks ---
  local log_file=".crush/logs/crush.log"
  if [[ ! -f "$log_file" ]]; then
    fail "Scenario 3: crush.log not found at $log_file"
    return
  fi

  local match_count
  match_count=$(grep -ci "repo.map\|repomap" "$log_file" 2>/dev/null || echo 0)
  if [[ "$match_count" -ge 1 ]]; then
    pass "Scenario 3: Found $match_count repo map log entries"
  else
    fail "Scenario 3: No repo map log entries found"
  fi

  # Kill tmux — this is the last scenario.
  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 1
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_repomap_cache
test_session_rankings
test_repomap_logs

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
