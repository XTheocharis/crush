#!/usr/bin/env bash
# Test: File-type explorer dispatch during file viewing.
# Verifies that the explorer subsystem is invoked when Crush views
# both code files (Go) and non-code files (Markdown), using log evidence.
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
# Scenario 1: Explorer dispatched during file view
# ---------------------------------------------------------------------------
test_explorer_dispatch_code_file() {
  echo "=== Scenario 1: Explorer dispatched during file view ==="

  setup_clean_crush
  restore_crush() {
    command restore_crush
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
  }
  trap restore_crush EXIT

  start_crush 2
  send_prompt "Show me internal/config/config.go"
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 11 "explorer-dispatch"
    stop_crush
    return
  fi

  # Check logs for explorer activity.
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

  capture_evidence 11 "explorer-dispatch"
  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: Explorer handles non-code file
# ---------------------------------------------------------------------------
test_explorer_noncode_file() {
  echo "=== Scenario 2: Explorer handles non-code file ==="

  setup_clean_crush
  restore_crush() {
    command restore_crush
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
  }
  trap restore_crush EXIT

  start_crush 2
  send_prompt "Show me the README.md file"
  if ! wait_for_idle 120; then
    fail "Scenario 2: Crush did not become idle"
    capture_evidence 11 "explorer-noncode"
    stop_crush
    return
  fi

  # Check logs for explorer activity on non-code file.
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

  capture_evidence 11 "explorer-noncode"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_explorer_dispatch_code_file
test_explorer_noncode_file

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
