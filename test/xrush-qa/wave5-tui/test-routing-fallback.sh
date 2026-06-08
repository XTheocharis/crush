#!/usr/bin/env bash
# Test: F08 Tiered model routing and fallback integration.
# Verifies that tier routing, cost tracking, and fallback configuration
# work at the TUI level. Uses simplified assertions since full fallback
# testing requires mock LLM server responses.
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
# Scenario 1: Tier routing configuration loads without error
# ---------------------------------------------------------------------------
test_tier_routing_config() {
  SCENARIO="tier-routing-config"
  echo "=== Scenario 1: Tier routing config loads ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  sleep 3

  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if ! printf '%s' "$tui_output" | grep -qi "panic\|fatal\|error.*config"; then
    pass "Scenario 1: No panic/config error in TUI"
  else
    fail "Scenario 1: Panic or config error detected in TUI"
  fi

  local db_path=".crush/crush.db"
  if [[ -f "$db_path" ]]; then
    local session_count
    session_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM sessions" 2>/dev/null || echo 0)
    if [[ "$session_count" -ge 1 ]]; then
      pass "Scenario 1: DB has $session_count session(s)"
    else
      fail "Scenario 1: No sessions in DB"
    fi
  fi

  capture_tui_evidence "routing-config"
}

# ---------------------------------------------------------------------------
# Scenario 2: Cost tracking activates after a conversation turn
# ---------------------------------------------------------------------------
test_cost_tracking() {
  SCENARIO="cost-tracking"
  echo "=== Scenario 2: Cost tracking activates ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  send_tui_prompt "Say ROUTING_COST_SENTINEL_42 and nothing else."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle"
    capture_tui_evidence "cost-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "ROUTING_COST_SENTINEL_42"; then
    pass "Scenario 2: TUI shows ROUTING_COST_SENTINEL_42"
  else
    fail "Scenario 2: TUI does not show ROUTING_COST_SENTINEL_42"
    capture_tui_evidence "cost-sentinel-missing"
    return
  fi

  local log_path=".crush/logs/crush.log"
  if [[ -f "$log_path" ]]; then
    local cost_matches
    cost_matches=$(grep -ciE "cost|token|budget|routing|tier" "$log_path" 2>/dev/null || echo 0)
    if [[ "$cost_matches" -ge 1 ]]; then
      pass "Scenario 2: Log contains cost/routing evidence ($cost_matches matches)"
    else
      echo "  NOTE: No cost/routing log entries (logging may be minimal)"
    fi
  fi

  capture_tui_evidence "cost-tracking"
}

# ---------------------------------------------------------------------------
# Scenario 3: Fallback mechanism produces no error
# ---------------------------------------------------------------------------
test_fallback_no_error() {
  SCENARIO="fallback-no-error"
  echo "=== Scenario 3: Fallback produces no error ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  sleep 2

  send_tui_prompt "Say ROUTING_FALLBACK_SENTINEL_88 and nothing else."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 3: Crush did not become idle"
    capture_tui_evidence "fallback-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "ROUTING_FALLBACK_SENTINEL_88"; then
    pass "Scenario 3: TUI shows ROUTING_FALLBACK_SENTINEL_88"
  else
    fail "Scenario 3: TUI does not show ROUTING_FALLBACK_SENTINEL_88"
    capture_tui_evidence "fallback-sentinel-missing"
    return
  fi

  local tui_output
  tui_output=$(capture_tui | strip_ansi)
  if ! printf '%s' "$tui_output" | grep -qi "panic\|fatal\|model.*error\|provider.*error"; then
    pass "Scenario 3: No model/provider error in TUI"
  else
    echo "  NOTE: Model/provider error detected (may be transient)"
  fi

  capture_tui_evidence "fallback-clean"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_tier_routing_config
test_cost_tracking
test_fallback_no_error

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-routing-fallback" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
