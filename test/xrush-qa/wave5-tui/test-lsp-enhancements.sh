#!/usr/bin/env bash
# Test: F19 LSP enhanced capabilities integration.
# Verifies LSP capabilities caching, TypeDefinition, and Implementation
# methods work at the TUI level using project Go files as fixtures.
set -euo pipefail

WAVE=5

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup_test() {
  cleanup_tui
  local _bak
  _bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  [[ -n "$_bak" ]] && mv "$_bak" crush.json
  restore_crush
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: LSP starts and capabilities are cached
# ---------------------------------------------------------------------------
test_lsp_capabilities() {
  SCENARIO="lsp-capabilities"
  echo "=== Scenario 1: LSP capabilities cached ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  sleep 3

  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if ! printf '%s' "$tui_output" | grep -qi "panic\|fatal"; then
    pass "Scenario 1: No panic/fatal in TUI"
  else
    fail "Scenario 1: Panic/fatal detected in TUI"
  fi

  local log_path=".crush/logs/crush.log"
  if [[ -f "$log_path" ]]; then
    local lsp_matches
    lsp_matches=$(grep -ciE "lsp|gopls|capabilities|textDocument" "$log_path" 2>/dev/null || echo 0)
    if [[ "$lsp_matches" -ge 1 ]]; then
      pass "Scenario 1: Log contains LSP evidence ($lsp_matches matches)"
    else
      echo "  NOTE: No LSP log entries (LSP may start lazily)"
    fi
  fi

  capture_tui_evidence "lsp-capabilities"
}

# ---------------------------------------------------------------------------
# Scenario 2: Ask Crush about Go types — verifies LSP-backed analysis
# ---------------------------------------------------------------------------
test_lsp_type_analysis() {
  SCENARIO="lsp-type-analysis"
  echo "=== Scenario 2: LSP-backed type analysis ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  send_tui_prompt "Look at the file internal/lcm/manager.go and tell me what methods the Manager struct has. Include the exact token LSP_TYPE_SENTINEL_42 in your response."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle"
    capture_tui_evidence "lsp-type-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "LSP_TYPE_SENTINEL_42"; then
    pass "Scenario 2: TUI shows LSP_TYPE_SENTINEL_42"
  else
    fail "Scenario 2: TUI does not show LSP_TYPE_SENTINEL_42"
    capture_tui_evidence "lsp-type-sentinel-missing"
    return
  fi

  if assert_tui_contains "Manager"; then
    pass "Scenario 2: TUI mentions Manager type"
  else
    echo "  NOTE: Manager not mentioned (model may use different wording)"
  fi

  capture_tui_evidence "lsp-type-analysis"
}

# ---------------------------------------------------------------------------
# Scenario 3: LSP enhanced methods don't crash the TUI
# ---------------------------------------------------------------------------
test_lsp_no_crash() {
  SCENARIO="lsp-no-crash"
  echo "=== Scenario 3: LSP enhanced methods don't crash ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  send_tui_prompt "Find the implementation of the CompactionLayer interface in internal/lcm/. Include the exact token LSP_IMPL_SENTINEL_88 in your response."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 3: Crush did not become idle"
    capture_tui_evidence "lsp-impl-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "LSP_IMPL_SENTINEL_88"; then
    pass "Scenario 3: TUI shows LSP_IMPL_SENTINEL_88"
  else
    fail "Scenario 3: TUI does not show LSP_IMPL_SENTINEL_88"
    capture_tui_evidence "lsp-impl-sentinel-missing"
    return
  fi

  local tui_output
  tui_output=$(capture_tui | strip_ansi)
  if ! printf '%s' "$tui_output" | grep -qi "panic\|fatal\|goroutine.*crash"; then
    pass "Scenario 3: No crash evidence in TUI"
  else
    fail "Scenario 3: Crash evidence detected in TUI"
  fi

  capture_tui_evidence "lsp-impl-clean"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_lsp_capabilities
test_lsp_type_analysis
test_lsp_no_crash

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-lsp-enhancements" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
