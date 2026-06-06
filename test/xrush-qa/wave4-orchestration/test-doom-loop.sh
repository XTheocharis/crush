#!/usr/bin/env bash
# Test: Doom loop detection active with full intervention.
# Verifies that Crush detects repetitive tool-call patterns and logs doom-loop
# related entries (doom, repetition, escalation) when operating against a
# nonexistent target that forces retries.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: Doom loop detection active
# ---------------------------------------------------------------------------
test_doom_loop_detection() {
  echo "=== Scenario 1: Doom loop detection active ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    command restore_crush
  }
  trap restore_on_exit EXIT

  # Wave 4 config has doom_loop_intervention: "full".
  start_crush 4

  # Send a prompt targeting a nonexistent file — forces repeated edit failures.
  send_prompt "Edit /nonexistent/file.go to add a function called test"

  # Allow up to 180s; repeated failures may trigger doom loop detection.
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_evidence 41 "doom-loop"
    return
  fi

  # Check the crush log for doom-loop-related entries.
  local log_entries
  log_entries=$(grep -ciE "doom.?loop|loop.?detection|repetition.*score|escalation.*level|intervention.*force" .crush/logs/crush.log 2>/dev/null || echo 0)

  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $log_entries doom-loop-related log entries"
  else
    fail "Scenario 1: No doom-loop-related log entries found (expected >= 1)"
  fi

  # Capture log evidence for debugging.
  echo "--- Doom loop log evidence ---"
  grep -iE "doom.?loop|loop.?detection|repetition.*score|escalation.*level|intervention.*force" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- End evidence ---"

  capture_evidence 41 "doom-loop"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_doom_loop_detection

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
