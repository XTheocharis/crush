#!/usr/bin/env bash
# Test: Map refresh tool — verifies that Crush can create a new file with a
# unique symbol, refresh the repo map, and report the new symbol.
# Uses TUI-first approach with sentinel strings for deterministic gating.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# Temp dir for test files — unique per PID.
QA_TMPDIR="/tmp/qa-map-refresh-$$"

cleanup_all() {
  # Kill tmux session if running.
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
  # Restore .crush backup.
  restore_crush
  # Restore crush.json if backed up.
  local json_bak
  json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  if [[ -n "$json_bak" ]]; then
    mv "$json_bak" crush.json
  fi
  # Remove temp files.
  rm -rf "$QA_TMPDIR"
  rm -f "/tmp/qa-refresh-test-$$.go"
}

# ---------------------------------------------------------------------------
# Scenario 1: Multi-step map refresh within single TUI session
#   Step 1: Read a known file (go.mod) with read sentinel
#   Step 2: Create a temp Go file with unique symbol MAP_REFRESH_SYMBOL_QA_2024
#   Step 3: Ask Crush to refresh the repo map and report the new symbol
# ---------------------------------------------------------------------------
test_map_refresh_multistep() {
  echo "=== Scenario 1: Multi-step map refresh within single TUI session ==="
  WAVE=2
  SCENARIO="map-refresh-multistep"

  setup_clean_crush
  mkdir -p "$QA_TMPDIR"
  trap cleanup_all EXIT

  start_crush_tui 2

  # --- Step 1: Read a known file ---
  focus_editor
  send_tui_prompt "Read the file go.mod in this project. Then reply with exactly MAP_REFRESH_READ_SENTINEL_42 and nothing else."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1 Step 1: Crush did not become idle"
    capture_tui_evidence "step1-idle-timeout"
    return
  fi

  if assert_tui_contains "MAP_REFRESH_READ_SENTINEL_42"; then
    pass "Scenario 1 Step 1: TUI shows MAP_REFRESH_READ_SENTINEL_42"
  else
    fail "Scenario 1 Step 1: TUI missing MAP_REFRESH_READ_SENTINEL_42"
    capture_tui_evidence "step1-sentinel-missing"
    return
  fi

  capture_tui_evidence "step1-read-file"

  # --- Step 2: Create temp file with unique symbol ---
  # We create the file ourselves so the symbol is deterministic.
  cat > "$QA_TMPDIR/test.go" <<'GOCODE'
package qa

// QARefreshSymbol is a unique test symbol for map-refresh QA.
func QARefreshSymbol() string {
	return "MAP_REFRESH_SYMBOL_QA_2024"
}
GOCODE

  focus_editor
  send_tui_prompt "I created a new Go file at $QA_TMPDIR/test.go with function QARefreshSymbol. Refresh the repo map now. After refreshing, tell me if you can see the symbol QARefreshSymbol or the string MAP_REFRESH_SYMBOL_QA_2024 anywhere. Reply with MAP_REFRESH_FOUND_SENTINEL_88 if you found it, or MAP_REFRESH_NOT_FOUND_33 if you cannot."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1 Step 2: Crush did not become idle after refresh request"
    capture_tui_evidence "step2-idle-timeout"
    return
  fi

  # Primary gate: TUI must show the found sentinel.
  if assert_tui_contains "MAP_REFRESH_FOUND_SENTINEL_88"; then
    pass "Scenario 1 Step 2: TUI shows MAP_REFRESH_FOUND_SENTINEL_88"
  else
    fail "Scenario 1 Step 2: TUI missing MAP_REFRESH_FOUND_SENTINEL_88"
    capture_tui_evidence "step2-sentinel-missing"
    # Continue to secondary checks even if primary gate fails.
  fi

  capture_tui_evidence "step2-map-refresh"

  # --- Secondary DB checks ---
  # Verify repo_map_file_cache or repo_map_tags reflect the new file.
  local cache_count
  cache_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_file_cache" | jq '.[0].count')
  if [[ "$cache_count" -gt 0 ]]; then
    pass "Scenario 1: repo_map_file_cache has $cache_count entries (>0)"
  else
    fail "Scenario 1: repo_map_file_cache is empty"
  fi

  # Check if the temp file appears in the file cache (path may be absolute).
  local tmp_in_cache
  tmp_in_cache=$(query_db "SELECT COUNT(*) as count FROM repo_map_file_cache WHERE rel_path LIKE '%qa-map-refresh%'" | jq '.[0].count')
  if [[ "$tmp_in_cache" -ge 1 ]]; then
    pass "Scenario 1: Temp file appears in repo_map_file_cache"
  else
    # The temp file may not be indexed if tree-sitter only scans the project
    # dir. Log this but don't hard-fail — the primary TUI gate is authoritative.
    echo "INFO: Scenario 1: Temp file not in repo_map_file_cache (may be out-of-project)"
  fi

  # Verify repo_map_tags table is populated (tree-sitter extraction).
  local tags_count
  tags_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_tags" | jq '.[0].count')
  if [[ "$tags_count" -gt 0 ]]; then
    pass "Scenario 1: repo_map_tags has $tags_count entries (>0)"
  else
    fail "Scenario 1: repo_map_tags is empty"
  fi

  # Log evidence for refresh/cache-invalidation.
  local log_file=".crush/logs/crush.log"
  if [[ -f "$log_file" ]]; then
    local refresh_hits
    refresh_hits=$(grep -ci "map.refresh\|map_refresh\|cache.invalidat\|PreIndex" "$log_file" 2>/dev/null ) || refresh_hits=0
    if [[ "$refresh_hits" -ge 1 ]]; then
      pass "Scenario 1: Found $refresh_hits map-refresh/invalidation log entries"
    else
      echo "INFO: Scenario 1: No map-refresh log entries found (model may not have invoked map_refresh tool)"
    fi
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: Simpler approach — create file then ask Crush about it
#   Create /tmp/qa-refresh-test-$$.go with unique function QARefreshFunc42.
#   Ask Crush to refresh the map and report with sentinel REFRESH_NEW_SYMBOL_77.
# ---------------------------------------------------------------------------
test_map_refresh_simple() {
  echo "=== Scenario 2: Simple map refresh — create file and ask about it ==="
  WAVE=2
  SCENARIO="map-refresh-simple"

  setup_clean_crush
  trap cleanup_all EXIT

  # Create temp file with unique symbol.
  cat > "/tmp/qa-refresh-test-$$.go" <<'GOCODE'
package qa

// QARefreshFunc42 is a unique test function for map refresh verification.
func QARefreshFunc42() string {
	return "REFRESH_NEW_SYMBOL_77"
}
GOCODE

  start_crush_tui 2
  focus_editor
  send_tui_prompt "A new Go file was created at /tmp/qa-refresh-test-$$.go containing function QARefreshFunc42. Please refresh the repo map and then tell me what symbols you can see in that file. Include REFRESH_NEW_SYMBOL_77 in your response when done."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle"
    capture_tui_evidence "simple-idle-timeout"
    return
  fi

  # Primary gate: TUI must contain the sentinel.
  if assert_tui_contains "REFRESH_NEW_SYMBOL_77"; then
    pass "Scenario 2: TUI shows REFRESH_NEW_SYMBOL_77"
  else
    fail "Scenario 2: TUI missing REFRESH_NEW_SYMBOL_77"
    capture_tui_evidence "simple-sentinel-missing"
    return
  fi

  capture_tui_evidence "simple-map-refresh"

  # --- Secondary: log grep for refresh/cache-invalidated evidence ---
  local log_file=".crush/logs/crush.log"
  if [[ -f "$log_file" ]]; then
    local refresh_log
    refresh_log=$(grep -ci "map.refresh\|map_refresh\|repo.map\|repomap" "$log_file" 2>/dev/null ) || refresh_log=0
    if [[ "$refresh_log" -ge 1 ]]; then
      pass "Scenario 2: Found $refresh_log repo-map log entries"
    else
      echo "INFO: Scenario 2: No repo-map log entries found"
    fi
  fi

  # Verify DB has session rankings (map was generated during session).
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    local ranking_count
    ranking_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_session_rankings WHERE session_id = '$SID'" | jq '.[0].count')
    if [[ "$ranking_count" -gt 0 ]]; then
      pass "Scenario 2: repo_map_session_rankings has $ranking_count entries"
    else
      fail "Scenario 2: repo_map_session_rankings is empty for session $SID"
    fi
  else
    fail "Scenario 2: No session ID found in DB"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_map_refresh_multistep
test_map_refresh_simple

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
