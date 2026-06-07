#!/usr/bin/env bash
# Test: AutoFix loop with validation flags enabled (TUI-first approach).
# Verifies that the diagnostic pipeline (diag/autofix/vet/lint) triggers when
# Crush creates a file with a syntax error, using wave4 config which has
# validation.enabled, validation.auto_fix, and validation.autofix_loop_enabled
# all set to true.
set -euo pipefail

WAVE=4
SCENARIO="autofix-syntax-error"

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup_test() { cleanup_tui; restore_crush; }
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: AutoFix detects and corrects syntax error
# ---------------------------------------------------------------------------
test_autofix_syntax_error() {
  echo "=== Scenario 1: AutoFix detects and corrects syntax error ==="
  SCENARIO="autofix-syntax-error"

  setup_clean_crush
  start_crush_tui "$WAVE"
  focus_editor

  # Ask Crush to create a Go file with a deliberate syntax error.
  # The prompt requests an unclosed brace so go vet / diagnostics will fire,
  # then autofix should correct it. The sentinel must appear in TUI output.
  send_tui_prompt 'Create a file /tmp/qa-broken.go with exactly this content (do NOT fix it yourself, write it verbatim): package main; import "fmt"; func main() { fmt.Println("hello". Then wait for autofix to run and fix the syntax error. After autofix completes, reply with exactly AUTOFIX_FIXED_SENTINEL_42 and explain what was fixed.'

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle (autofix cycle timeout)"
    capture_tui_evidence "autofix-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "AUTOFIX_FIXED_SENTINEL_42"; then
    pass "Scenario 1: TUI shows AUTOFIX_FIXED_SENTINEL_42 sentinel"
  else
    fail "Scenario 1: TUI does not show AUTOFIX_FIXED_SENTINEL_42 sentinel"
    capture_tui_evidence "sentinel-missing"
    return
  fi

  capture_tui_evidence "autofix-response"

  # --- Secondary filesystem check: file exists and is valid Go ---
  if [[ ! -f /tmp/qa-broken.go ]]; then
    fail "Scenario 1: /tmp/qa-broken.go was not created"
  elif gofmt -e /tmp/qa-broken.go >/dev/null 2>&1; then
    pass "Scenario 1: /tmp/qa-broken.go is syntactically valid Go after autofix"
  else
    fail "Scenario 1: /tmp/qa-broken.go is still syntactically invalid after autofix"
  fi

  # --- Secondary log check: diagnostic tooling was used ---
  local diag_count
  diag_count=$(grep -ciE "diag_autofix|autofix.*loop|auto.?fix.*diagnostic|running.*go vet|validation.*auto.?fix" .crush/logs/crush.log 2>/dev/null ) || diag_count=0
  if [[ "$diag_count" -gt 0 ]]; then
    pass "Scenario 1: Diagnostic pipeline ran ($diag_count log matches)"
  else
    fail "Scenario 1: No diagnostic pipeline log matches found"
  fi

  # Clean up temp file.
  rm -f /tmp/qa-broken.go
}

# ---------------------------------------------------------------------------
# Scenario 2: AutoFix detects and corrects unused variable lint error
# ---------------------------------------------------------------------------
test_autofix_lint_error() {
  echo "=== Scenario 2: AutoFix detects and corrects lint error ==="
  SCENARIO="autofix-lint-error"

  setup_clean_crush
  start_crush_tui "$WAVE"
  focus_editor

  # Ask Crush to create a Go file with an unused variable (go vet error).
  # Autofix should detect the unused variable and remove it.
  send_tui_prompt 'Create a file /tmp/qa-lint.go with exactly this content (write it verbatim, do NOT fix it): package main; func main() { x := 42; println("done") }. Then wait for autofix to detect and fix the unused variable x. After autofix completes, reply with exactly AUTOFIX_LINT_SENTINEL_77 and explain the lint fix.'

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle (lint autofix timeout)"
    capture_tui_evidence "lint-autofix-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "AUTOFIX_LINT_SENTINEL_77"; then
    pass "Scenario 2: TUI shows AUTOFIX_LINT_SENTINEL_77 sentinel"
  else
    fail "Scenario 2: TUI does not show AUTOFIX_LINT_SENTINEL_77 sentinel"
    capture_tui_evidence "lint-sentinel-missing"
    return
  fi

  capture_tui_evidence "lint-autofix-response"

  # --- Secondary filesystem check: file exists and is valid Go ---
  if [[ ! -f /tmp/qa-lint.go ]]; then
    fail "Scenario 2: /tmp/qa-lint.go was not created"
  elif gofmt -e /tmp/qa-lint.go >/dev/null 2>&1; then
    pass "Scenario 2: /tmp/qa-lint.go is syntactically valid Go after autofix"
  else
    fail "Scenario 2: /tmp/qa-lint.go is still syntactically invalid after autofix"
  fi

  # --- Secondary log check ---
  local diag_count
  diag_count=$(grep -ciE "diag_autofix|autofix.*loop|auto.?fix.*diagnostic|unused|vet" .crush/logs/crush.log 2>/dev/null ) || diag_count=0
  if [[ "$diag_count" -gt 0 ]]; then
    pass "Scenario 2: Diagnostic pipeline ran ($diag_count log matches)"
  else
    fail "Scenario 2: No diagnostic pipeline log matches found"
  fi

  # Clean up temp file.
  rm -f /tmp/qa-lint.go
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_autofix_syntax_error
test_autofix_lint_error

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
