#!/usr/bin/env bash
# Test: File-type explorer dispatch during file viewing (TUI-first approach).
# Verifies that the explorer subsystem is invoked when Crush views
# both code files (Go) and non-code files (Markdown).
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

WAVE=2

# ---------------------------------------------------------------------------
# Scenario 1: Explorer dispatched during code file view
# ---------------------------------------------------------------------------
test_explorer_dispatch_code_file() {
  echo "=== Scenario 1: Explorer dispatched during code file view ==="
  SCENARIO="explorer-dispatch"

  setup_clean_crush
  cleanup_test() { cleanup_tui; restore_crush; }
  trap cleanup_test EXIT

  start_crush_tui "$WAVE"
  focus_editor
  send_tui_prompt "Show me internal/config/config.go and describe its contents. Reply with EXPLORER_DISPATCH_SENTINEL_42 in your answer."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains_retry "EXPLORER_DISPATCH_SENTINEL_42" 3 10; then
    pass "Scenario 1: TUI shows EXPLORER_DISPATCH_SENTINEL_42 sentinel"
  else
    fail "Scenario 1: TUI does not show EXPLORER_DISPATCH_SENTINEL_42 sentinel"
    capture_tui_evidence "sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Primary: DB assertions for tree-sitter analysis ---
  # repo_map_tags/file_cache are keyed by repo_key+rel_path (no session_id).
  local db_path="${CRUSH_DB:-.crush/crush.db}"
  local tags_count=0 poll_elapsed=0
  while [[ $poll_elapsed -lt 30 ]]; do
    tags_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM repo_map_tags WHERE rel_path LIKE '%config/config.go'" 2>/dev/null || echo "0")
    [[ "$tags_count" -ge 1 ]] && break
    sleep 2; poll_elapsed=$((poll_elapsed + 2))
  done
  if [[ "$tags_count" -ge 1 ]]; then
    pass "Scenario 1: repo_map_tags has $tags_count rows for config.go (tree-sitter parsed)"
  else
    fail "Scenario 1: repo_map_tags empty for config.go (tree-sitter parsing not recorded)"
  fi

  local cache_count=0; poll_elapsed=0
  while [[ $poll_elapsed -lt 30 ]]; do
    cache_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM repo_map_file_cache WHERE rel_path LIKE '%config/config.go'" 2>/dev/null || echo "0")
    [[ "$cache_count" -ge 1 ]] && break
    sleep 2; poll_elapsed=$((poll_elapsed + 2))
  done
  if [[ "$cache_count" -ge 1 ]]; then
    pass "Scenario 1: repo_map_file_cache has $cache_count rows for config.go"
  else
    fail "Scenario 1: repo_map_file_cache empty for config.go (no tree-sitter analysis cached)"
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Scenario 2: Explorer handles non-code file
# ---------------------------------------------------------------------------
test_explorer_noncode_file() {
  echo "=== Scenario 2: Explorer handles non-code file ==="
  SCENARIO="explorer-noncode"

  setup_clean_crush
  cleanup_test() { cleanup_tui; restore_crush; }
  trap cleanup_test EXIT

  start_crush_tui "$WAVE"
  focus_editor
  send_tui_prompt "Show me the README.md file and summarize it. Reply with EXPLORER_NONCODE_SENTINEL_77 in your answer."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains_retry "EXPLORER_NONCODE_SENTINEL_77" 3 10; then
    pass "Scenario 2: TUI shows EXPLORER_NONCODE_SENTINEL_77 sentinel"
  else
    fail "Scenario 2: TUI does not show EXPLORER_NONCODE_SENTINEL_77 sentinel"
    capture_tui_evidence "sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Primary: DB assertions for tree-sitter analysis ---
  # repo_map_tags/file_cache are keyed by repo_key+rel_path (no session_id).
  local db_path="${CRUSH_DB:-.crush/crush.db}"
  local tags_count=0 poll_elapsed=0
  while [[ $poll_elapsed -lt 30 ]]; do
    tags_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM repo_map_tags WHERE rel_path LIKE '%README.md'" 2>/dev/null || echo "0")
    [[ "$tags_count" -ge 1 ]] && break
    sleep 2; poll_elapsed=$((poll_elapsed + 2))
  done
  if [[ "$tags_count" -ge 1 ]]; then
    pass "Scenario 2: repo_map_tags has $tags_count rows for README.md"
  else
    fail "Scenario 2: repo_map_tags empty for README.md"
  fi

  local cache_count=0; poll_elapsed=0
  while [[ $poll_elapsed -lt 30 ]]; do
    cache_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM repo_map_file_cache WHERE rel_path LIKE '%README.md'" 2>/dev/null || echo "0")
    [[ "$cache_count" -ge 1 ]] && break
    sleep 2; poll_elapsed=$((poll_elapsed + 2))
  done
  if [[ "$cache_count" -ge 1 ]]; then
    pass "Scenario 2: repo_map_file_cache has $cache_count rows for README.md"
  else
    fail "Scenario 2: repo_map_file_cache empty for README.md"
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_explorer_dispatch_code_file
test_explorer_noncode_file

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
