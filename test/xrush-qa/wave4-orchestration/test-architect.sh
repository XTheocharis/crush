#!/usr/bin/env bash
# Test: Architect planning triggers on complex tasks and plan parse succeeds.
# Scenario 1: Verify architect is invoked when given a complex implementation task.
# Scenario 2: Check logs for "Failed to parse architect plan" — documents the known
#   parse bug without failing the test.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
SKIP=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }
skip() { echo "SKIP: $1"; ((SKIP += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: Architect planning triggers on complex task
# ---------------------------------------------------------------------------
test_architect_triggers() {
  echo "=== Scenario 1: Architect planning triggers on complex task ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    restore_crush
  }
  trap restore_on_exit EXIT

  start_crush 4
  send_prompt "Plan and implement a Fibonacci utility in a new file /tmp/qa-fib.go"
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_evidence 41 "architect-trigger"
    return
  fi

  # Check logs for architect invocation.
  local architect_count
  architect_count=$(grep -ci "architect" .crush/logs/crush.log 2>/dev/null || echo 0)

  if [[ "$architect_count" -gt 0 ]]; then
    pass "Scenario 1: Architect invoked (log mentions: $architect_count occurrences)"
  else
    fail "Scenario 1: Architect not triggered (0 architect log entries)"
  fi

  if [[ -f /tmp/qa-fib.go ]]; then
    pass "Scenario 1: /tmp/qa-fib.go was created"
    if grep -qi "fib" /tmp/qa-fib.go; then
      pass "Scenario 1: /tmp/qa-fib.go contains Fibonacci implementation text"
    else
      fail "Scenario 1: /tmp/qa-fib.go does not contain Fibonacci implementation text"
    fi
  else
    fail "Scenario 1: /tmp/qa-fib.go was not created"
  fi

  capture_evidence 41 "architect-trigger"
  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: Architect plan parse bug check
# ---------------------------------------------------------------------------
test_architect_parse_bug() {
  echo "=== Scenario 2: Architect plan parse bug check ==="

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

  capture_evidence 41 "architect-parse-bug"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_architect_triggers
test_architect_parse_bug

echo ""
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
exit "$FAIL"
