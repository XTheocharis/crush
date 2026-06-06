#!/usr/bin/env bash
# Test: AutoFix loop with validation flags enabled.
# Verifies that the diagnostic pipeline (diag/autofix/vet/lint) triggers when
# Crush creates a file with a syntax error, using wave4 config which has
# validation.enabled, validation.auto_fix, and validation.autofix_loop_enabled
# all set to true.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: AutoFix detects and corrects syntax error
# ---------------------------------------------------------------------------
test_autofix_syntax_error() {
  echo "=== Scenario 1: AutoFix detects and corrects syntax error ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_on_exit is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    command restore_crush
    rm -f /tmp/qa-broken.go
  }
  trap restore_on_exit EXIT

  start_crush 4
  send_prompt 'Create a file /tmp/qa-broken.go with a deliberate syntax error: package main; func main() { fmt.Println("hello"'
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle (autofix cycle timeout)"
    capture_evidence 14 "autofix-timeout"
    return
  fi

  # Check that the diagnostic pipeline ran by grepping logs.
  local diag_count
  diag_count=$(grep -ciE "diag_autofix|autofix.*loop|auto.?fix.*diagnostic|running.*go test|running.*go vet|validation.*auto.?fix" .crush/logs/crush.log 2>/dev/null || echo 0)
  if [[ "$diag_count" -gt 0 ]]; then
    pass "Scenario 1: Diagnostic pipeline ran ($diag_count log matches)"
  else
    fail "Scenario 1: validation pipeline not triggered (0 specific autofix log matches)"
  fi

  if [[ ! -f /tmp/qa-broken.go ]]; then
    fail "Scenario 1: /tmp/qa-broken.go was not created"
  elif gofmt -e /tmp/qa-broken.go >/tmp/qa-broken-go-test.log 2>&1; then
    pass "Scenario 1: /tmp/qa-broken.go is syntactically valid Go after autofix"
  else
    fail "Scenario 1: /tmp/qa-broken.go is still syntactically invalid after autofix"
    cat /tmp/qa-broken-go-test.log
  fi

  capture_evidence 14 "autofix-syntax-error"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_autofix_syntax_error

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
