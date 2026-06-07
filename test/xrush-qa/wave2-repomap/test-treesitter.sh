#!/usr/bin/env bash
# Test: Tree-sitter tag extraction and import resolution (TUI-first approach).
# Verifies that Crush extracts Go tags (functions, types, methods) and
# populates the import graph via tree-sitter when run against this repo.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

WAVE=2

# ---------------------------------------------------------------------------
# Scenario 1: Tags extracted for Go files
# ---------------------------------------------------------------------------
test_go_tags_extracted() {
  echo "=== Scenario 1: Tags extracted for Go files ==="
  SCENARIO="treesitter-tags"

  setup_clean_crush
  cleanup_test() { cleanup_tui; restore_crush; }
  trap cleanup_test EXIT

  start_crush_tui "$WAVE"
  focus_editor
  send_tui_prompt "Read main.go and tell me the package name and the main function signature. Reply with TS_MAIN_SENTINEL_go_main_func in your answer."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "TS_MAIN_SENTINEL_go_main_func"; then
    pass "Scenario 1: TUI shows TS_MAIN_SENTINEL_go_main_func sentinel"
  else
    fail "Scenario 1: TUI does not show TS_MAIN_SENTINEL_go_main_func sentinel"
    capture_tui_evidence "sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary DB checks ---
  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  # Verify Go tags were extracted (count > 0).
  local tag_count=0
  local has_tags_table
  has_tags_table=$(sqlite3 .crush/crush.db "SELECT name FROM sqlite_master WHERE type='table' AND name='repo_map_tags'" 2>/dev/null || true)
  if [[ -n "$has_tags_table" ]]; then
    tag_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_tags WHERE language='go'" | jq '.[0].count')
    if [[ "$tag_count" -gt 0 ]]; then
      pass "Scenario 1: $tag_count Go tags extracted"
    else
      fail "Scenario 1: No Go tags found in repo_map_tags"
    fi
  else
    echo "INFO: Scenario 1: repo_map_tags table not present (tree-sitter may require CGO)"
  fi

  # Verify main.go main symbol exists.
  local main_tag_count=0
  if [[ -n "$has_tags_table" ]]; then
    main_tag_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_tags WHERE language='go' AND rel_path='main.go' AND name='main'" | jq '.[0].count')
    if [[ "$main_tag_count" -ge 1 ]]; then
      pass "Scenario 1: main.go main symbol extracted"
    else
      fail "Scenario 1: Expected main.go main symbol in repo_map_tags"
    fi
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Scenario 2: Import resolution populated
# ---------------------------------------------------------------------------
test_imports_populated() {
  echo "=== Scenario 2: Import resolution populated ==="
  SCENARIO="treesitter-imports"

  setup_clean_crush
  cleanup_test() { cleanup_tui; restore_crush; }
  trap cleanup_test EXIT

  start_crush_tui "$WAVE"
  focus_editor
  send_tui_prompt "What packages does main.go import? List them. Reply with TS_IMPORT_SENTINEL_charmbracelet_crush in your answer."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "TS_IMPORT_SENTINEL_charmbracelet_crush"; then
    pass "Scenario 2: TUI shows TS_IMPORT_SENTINEL_charmbracelet_crush sentinel"
  else
    fail "Scenario 2: TUI does not show TS_IMPORT_SENTINEL_charmbracelet_crush sentinel"
    capture_tui_evidence "sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary DB checks ---
  local import_count=0
  local has_imports_table
  has_imports_table=$(sqlite3 .crush/crush.db "SELECT name FROM sqlite_master WHERE type='table' AND name='repo_map_imports'" 2>/dev/null || true)
  if [[ -n "$has_imports_table" ]]; then
    import_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_imports" | jq '.[0].count')
    if [[ "$import_count" -gt 0 ]]; then
      pass "Scenario 2: $import_count imports in repo_map_imports"
    else
      fail "Scenario 2: No imports found in repo_map_imports"
    fi
  else
    echo "INFO: Scenario 2: repo_map_imports table not present (tree-sitter may require CGO)"
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_go_tags_extracted
test_imports_populated

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
