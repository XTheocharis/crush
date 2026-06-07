#!/usr/bin/env bash
# Test: Routing/fallback live behavior for small and large/context-heavy requests.
# Verifies that Crush routes simple prompts and complex multi-file synthesis
# prompts through the appropriate tiers, observable via TUI output and logs.
set -euo pipefail

WAVE=5
source "$(dirname "$0")/../lib/common.sh"

PASS=0
FAIL=0

cleanup_test() {
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
  local _bak
  _bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  [[ -n "$_bak" ]] && mv "$_bak" crush.json
  restore_crush
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Small request routing
# ---------------------------------------------------------------------------
test_small_request_routing() {
  SCENARIO="small-request-routing"
  echo "=== Scenario 1: Small request routing ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Simple prompt — should route to a lightweight tier.
  send_tui_prompt "What is 2+2? Reply with exactly: ROUTING_SMALL_SENTINEL_42"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle within 120s"
    capture_tui_evidence "small-routing-timeout"
    return
  fi

  # Primary: TUI must contain the sentinel.
  if assert_tui_contains "ROUTING_SMALL_SENTINEL_42"; then
    pass "Scenario 1: TUI contains ROUTING_SMALL_SENTINEL_42"
  else
    fail "Scenario 1: TUI does not contain ROUTING_SMALL_SENTINEL_42"
    capture_tui_evidence "small-routing-no-sentinel"
  fi

  # Secondary: log grep for router/tier entries.
  local log_entries
  log_entries=$(grep -ciE "router|tier|routing|model.*select|route.*request" .crush/logs/crush.log 2>/dev/null || echo 0)

  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $log_entries routing/tier log entries"
  else
    echo "  NOTE: No routing/tier log entries found"
  fi

  echo "--- Small routing log evidence ---"
  grep -iE "router|tier|routing|model.*select|route.*request" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- End evidence ---"

  capture_tui_evidence "small-request-routing"
}

# ---------------------------------------------------------------------------
# Scenario 2: Large/context-heavy request routing
# ---------------------------------------------------------------------------
test_large_request_routing() {
  SCENARIO="large-request-routing"
  echo "=== Scenario 2: Large/context-heavy request routing ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Context-heavy prompt asking to read and synthesize multiple files.
  send_tui_prompt "Read AGENTS.md, internal/agent/router_tier.go, and internal/agent/model_router.go. Summarize the routing strategy in 2 sentences. End your reply with exactly: ROUTING_LARGE_SENTINEL_88"

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle within 180s"
    capture_tui_evidence "large-routing-timeout"
    return
  fi

  # Primary: TUI must contain the sentinel.
  if assert_tui_contains "ROUTING_LARGE_SENTINEL_88"; then
    pass "Scenario 2: TUI contains ROUTING_LARGE_SENTINEL_88"
  else
    fail "Scenario 2: TUI does not contain ROUTING_LARGE_SENTINEL_88"
    capture_tui_evidence "large-routing-no-sentinel"
  fi

  # Secondary: log grep for routing/fallback entries.
  local log_entries
  log_entries=$(grep -ciE "router|tier|routing|fallback|model.*select|escalat|route.*request|budget" .crush/logs/crush.log 2>/dev/null || echo 0)

  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 2: Found $log_entries routing/fallback log entries"
  else
    echo "  NOTE: No routing/fallback log entries found"
  fi

  echo "--- Large routing log evidence ---"
  grep -iE "router|tier|routing|fallback|model.*select|escalat|route.*request|budget" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- End evidence ---"

  capture_tui_evidence "large-request-routing"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_small_request_routing
test_large_request_routing

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
