#!/usr/bin/env bash
# Test: File-type explorer dispatch during file viewing (TUI-first approach).
# Verifies that the explorer subsystem is invoked when Crush views
# both code files (Go) and non-code files (Markdown).
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

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "EXPLORER_DISPATCH_SENTINEL_42"; then
    pass "Scenario 1: TUI shows EXPLORER_DISPATCH_SENTINEL_42 sentinel"
  else
    fail "Scenario 1: TUI does not show EXPLORER_DISPATCH_SENTINEL_42 sentinel"
    capture_tui_evidence "sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary: log grep for explorer activity ---
  local explorer_lines
  explorer_lines=$(grep -i "explorer" .crush/logs/crush.log 2>/dev/null | tail -20 || true)
  if [[ -n "$explorer_lines" ]]; then
    local match_count
    match_count=$(echo "$explorer_lines" | grep -ciE "dispatch|processing|explore" || echo 0)
    if [[ "$match_count" -ge 1 ]]; then
      pass "Scenario 1: Explorer dispatch/processing found in logs ($match_count matches)"
    else
      fail "Scenario 1: Explorer mentioned in logs but no dispatch/processing keywords"
    fi
  else
    fail "Scenario 1: No explorer-related log lines found"
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
  if assert_tui_contains "EXPLORER_NONCODE_SENTINEL_77"; then
    pass "Scenario 2: TUI shows EXPLORER_NONCODE_SENTINEL_77 sentinel"
  else
    fail "Scenario 2: TUI does not show EXPLORER_NONCODE_SENTINEL_77 sentinel"
    capture_tui_evidence "sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary: log grep for explorer activity ---
  local explorer_lines
  explorer_lines=$(grep -i "explorer" .crush/logs/crush.log 2>/dev/null || true)
  if [[ -n "$explorer_lines" ]]; then
    local match_count
    match_count=$(echo "$explorer_lines" | grep -ciE "dispatch|processing|explore" || echo 0)
    if [[ "$match_count" -ge 1 ]]; then
      pass "Scenario 2: Explorer processing found for non-code file ($match_count matches)"
    else
      fail "Scenario 2: Explorer mentioned but no processing keywords for non-code file"
    fi
  else
    fail "Scenario 2: No explorer-related log lines found for non-code file"
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
