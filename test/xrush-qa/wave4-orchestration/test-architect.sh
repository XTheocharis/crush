#!/usr/bin/env bash
# Test: Architect planning triggers on complex tasks and plan parse succeeds
# (TUI-first).
# Scenario 1: Verify architect is invoked when given a complex implementation
#   task. TUI output must show the plan and ARCHITECT_PLAN_SENTINEL_42.
#   Secondary filesystem checks prove plan artifacts.
# Scenario 2: Check logs for "Failed to parse architect plan" — documents the
#   known parse bug without failing the test.
set -euo pipefail

WAVE=4

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: Architect planning triggers on complex task
# ---------------------------------------------------------------------------
test_architect_plan() {
  echo "=== Scenario 1: Architect planning triggers on complex task ==="
  SCENARIO="architect-plan"

  setup_clean_crush
  cleanup_test() { cleanup_tui; restore_crush; }
  trap cleanup_test EXIT

  start_crush_tui 4
  focus_editor
  send_tui_prompt "Plan and implement a simple Go package in /tmp/qa-arch-plan-$$/ that computes Fibonacci numbers. Include a types.go with a FibRequest struct and a compute.go with a Compute function. After planning, include the exact token ARCHITECT_PLAN_SENTINEL_42 in your response."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "ARCHITECT_PLAN_SENTINEL_42"; then
    pass "Scenario 1: TUI shows ARCHITECT_PLAN_SENTINEL_42 sentinel"
  else
    fail "Scenario 1: TUI does not show ARCHITECT_PLAN_SENTINEL_42 sentinel"
    capture_tui_evidence "sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary: architect log evidence ---
  local architect_count
  architect_count=$(grep -ci "architect" .crush/logs/crush.log 2>/dev/null || echo 0)

  if [[ "$architect_count" -gt 0 ]]; then
    pass "Scenario 1: Architect invoked (log mentions: $architect_count occurrences)"
  else
    fail "Scenario 1: Architect not triggered (0 architect log entries)"
  fi

  # --- Secondary: filesystem checks ---
  local target_dir="/tmp/qa-arch-plan-$$"
  if [[ -d "$target_dir" ]]; then
    pass "Scenario 1: Target directory $target_dir was created"
    local file_count
    file_count=$(find "$target_dir" -name "*.go" -type f 2>/dev/null | wc -l)
    if [[ "$file_count" -ge 1 ]]; then
      pass "Scenario 1: Found $file_count Go file(s) in $target_dir"
    else
      fail "Scenario 1: No Go files found in $target_dir"
    fi
  else
    fail "Scenario 1: Target directory $target_dir was not created"
  fi

  # Cleanup target dir.
  rm -rf "$target_dir"

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 1
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Scenario 2: Architect plan parse bug check
# ---------------------------------------------------------------------------
test_architect_parse_bug() {
  echo "=== Scenario 2: Architect plan parse bug check ==="
  SCENARIO="architect-parse-bug"

  # The log file must exist from Scenario 1.
  if [[ ! -f .crush/logs/crush.log ]]; then
    fail "Scenario 2: No crush.log found — Scenario 1 did not produce required evidence"
    return
  fi

  local parse_fail_count
  parse_fail_count=$(grep -c "Failed to parse architect plan" .crush/logs/crush.log 2>/dev/null || echo 0)

  if [[ "$parse_fail_count" -eq 0 ]]; then
    pass "Scenario 2: No architect plan parse failures detected"
  else
    echo "NOTE: Scenario 2: BUG STILL PRESENT — $parse_fail_count parse failure(s) detected"
    echo "  Log excerpts:"
    grep "Failed to parse architect plan" .crush/logs/crush.log | head -4 | while IFS= read -r line; do
      echo "    $line"
    done
    fail "Scenario 2: Architect plan parse failures detected ($parse_fail_count)"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_architect_plan
test_architect_parse_bug

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
