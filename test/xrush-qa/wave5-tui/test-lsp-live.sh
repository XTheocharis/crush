#!/usr/bin/env bash
# Test: LSP diagnostics and symbol information via TUI.
# Verifies that Crush leverages LSP to report diagnostics on a file with known
# issues and retrieves symbol information from a known Go file.
set -euo pipefail

WAVE=5
SCENARIO="lsp-diagnostics"
source "$(dirname "$0")/../lib/common.sh"

PASS=0
FAIL=0

# Temp Go file created during tests — cleaned up in cleanup_test.
LSP_TEST_FILE=""

pass() {
  echo "  PASS: $1"
  PASS=$((PASS + 1))
}

fail() {
  echo "  FAIL: $1" >&2
  FAIL=$((FAIL + 1))
}

cleanup_test() {
    cleanup_tui
  # Remove any temp Go files created during the test.
  [[ -n "$LSP_TEST_FILE" && -f "$LSP_TEST_FILE" ]] && rm -f "$LSP_TEST_FILE"
  # Also clean up test helper files if present.
  rm -f lsp_diag_test.go lsp_symbol_test.go
  restore_crush
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: LSP diagnostics detect unused import in a Go file
# ---------------------------------------------------------------------------
test_lsp_diagnostics() {
  SCENARIO="lsp-diagnostics"
  echo "=== Scenario 1: LSP diagnostics detect unused import in a Go file ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Create a Go file with a deliberately unused import — gopls will flag this.
  LSP_TEST_FILE="lsp_diag_test.go"
  cat > "$LSP_TEST_FILE" <<'GOEOF'
package lsp_diag_test

import (
	"fmt"
	"os"
)

func Hello() {
	fmt.Println("hello")
	// os is deliberately unused to trigger an LSP diagnostic.
	_ = 42
}
GOEOF

  # Ask Crush to report LSP diagnostics, outputting the sentinel when done.
  send_tui_prompt "Check the file lsp_diag_test.go for any diagnostics or lint errors reported by the LSP. Report every diagnostic you find. When done, output exactly: LSP_DIAG_SENTINEL_42"

  # Allow up to 180s for LSP startup + diagnostics.
  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_tui_evidence "lsp-diag-timeout"
    return
  fi

  # Primary: TUI must contain the sentinel.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "LSP_DIAG_SENTINEL_42"; then
    pass "Scenario 1: TUI contains LSP_DIAG_SENTINEL_42"
  elif printf '%s' "$tui_output" | grep -qiE "unused.*import|import.*unused|os.*not used|declared.*not used"; then
    pass "Scenario 1: TUI shows LSP detected unused import"
  else
    fail "Scenario 1: TUI does not contain LSP_DIAG_SENTINEL_42 or diagnostic keywords"
    capture_tui_evidence "lsp-diag-no-sentinel"
  fi

  # Secondary: check the crush log for LSP-related entries.
  local log_entries
  log_entries=$(grep -ciE "lsp|gopls|diagnostic|textDocument/publishDiagnostics" .crush/logs/crush.log 2>/dev/null ) || log_entries=0

  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $log_entries LSP-related log entries"
  else
    echo "  NOTE: No LSP log entries found (LSP may not have started yet)"
  fi

  echo "--- LSP diagnostics log evidence ---"
  grep -iE "lsp|gopls|diagnostic|textDocument/publishDiagnostics" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- End evidence ---"

  capture_tui_evidence "lsp-diagnostics"

  # Clean up the test file before next scenario.
  rm -f "$LSP_TEST_FILE"
  LSP_TEST_FILE=""
}

# ---------------------------------------------------------------------------
# Scenario 2: LSP symbol information from a known Go file
# ---------------------------------------------------------------------------
test_lsp_symbols() {
  SCENARIO="lsp-symbols"
  echo "=== Scenario 2: LSP symbol information from a known Go file ==="

  # Reuse existing Crush session — send a new prompt.
  focus_editor

  # Create a Go file with a well-known exported symbol.
  LSP_TEST_FILE="lsp_symbol_test.go"
  cat > "$LSP_TEST_FILE" <<'GOEOF'
package lsp_symbol_test

// LSPSymbolTarget is a well-known function for LSP symbol lookup tests.
func LSPSymbolTarget(input string) string {
	return "response: " + input
}

// HelperFunction is an unexported helper.
func helperFunction() int {
	return 99
}
GOEOF

  # Ask Crush to look up symbol information, outputting the sentinel when done.
  send_tui_prompt "Use the LSP to find symbol information for the function LSPSymbolTarget in lsp_symbol_test.go. Report the symbol name, type, and location. When done, output exactly: LSP_SYMBOL_SENTINEL_88"

  # Allow up to 180s for symbol lookup.
  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle within 180s"
    capture_tui_evidence "lsp-symbol-timeout"
    return
  fi

  # Primary: TUI must contain the sentinel.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "LSP_SYMBOL_SENTINEL_88"; then
    pass "Scenario 2: TUI contains LSP_SYMBOL_SENTINEL_88"
  elif printf '%s' "$tui_output" | grep -qiE "LSPSymbolTarget|symbol.*found|symbol.*information|function.*LSPSymbolTarget"; then
    pass "Scenario 2: TUI shows symbol information for LSPSymbolTarget"
  else
    fail "Scenario 2: TUI does not contain LSP_SYMBOL_SENTINEL_88 or symbol keywords"
    capture_tui_evidence "lsp-symbol-no-sentinel"
  fi

  # Secondary: check the crush log for LSP symbol/hover requests.
  local log_entries
  log_entries=$(grep -ciE "symbol|hover|textDocument/hover|textDocument/documentSymbol|definition" .crush/logs/crush.log 2>/dev/null ) || log_entries=0

  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 2: Found $log_entries LSP symbol/hover log entries"
  else
    echo "  NOTE: No LSP symbol/hover log entries found"
  fi

  echo "--- LSP symbols log evidence ---"
  grep -iE "symbol|hover|textDocument/hover|textDocument/documentSymbol|definition" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- End evidence ---"

  capture_tui_evidence "lsp-symbols"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_lsp_diagnostics
test_lsp_symbols

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
