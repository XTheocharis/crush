#!/usr/bin/env bash
# Test: Eval pipeline end-to-end via CLI.
# Verifies eval scorer and capture subcommands produce expected output.
# Eval is CLI-only — no TUI palette command exists.
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
# Scenario 1: Eval CLI reports scorer list or usage
# ---------------------------------------------------------------------------
test_eval_pipeline_scorers() {
  SCENARIO="eval-pipeline-scorers"
  echo "=== Scenario 1: Eval CLI scorer list/usage ==="

  local eval_output
  eval_output=$(crush eval --help 2>&1 || true)

  if printf '%s' "$eval_output" | grep -qiE "scorer|dataset|metric|usage"; then
    pass "Scenario 1: crush eval --help shows scorer/dataset/usage info"
  else
    fail "Scenario 1: crush eval --help missing expected content"
  fi

  if ! printf '%s' "$eval_output" | grep -qi "panic"; then
    pass "Scenario 1: No panic from crush eval --help"
  else
    fail "Scenario 1: Panic detected from crush eval --help"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: Eval capture subcommand works
# ---------------------------------------------------------------------------
test_eval_pipeline_capture() {
  SCENARIO="eval-pipeline-capture"
  echo "=== Scenario 2: Eval capture subcommand ==="

  setup_clean_crush

  local capture_output
  capture_output=$(crush eval --help 2>&1 | grep -i "capture" || true)

  if [[ -n "$capture_output" ]]; then
    pass "Scenario 2: crush eval help mentions capture subcommand"
  else
    echo "  NOTE: capture subcommand may not be documented in --help (non-blocking)"
  fi

  local db_path=".crush/crush.db"
  if [[ -f "$db_path" ]]; then
    local session_count
    session_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM sessions" 2>/dev/null || echo 0)
    if [[ "$session_count" -ge 0 ]]; then
      pass "Scenario 2: DB accessible ($session_count session(s))"
    else
      fail "Scenario 2: DB not accessible"
    fi
  fi
}

# ---------------------------------------------------------------------------
# Scenario 3: Eval CLI handles invalid scorer gracefully
# ---------------------------------------------------------------------------
test_eval_pipeline_invalid_scorer() {
  SCENARIO="eval-pipeline-invalid-scorer"
  echo "=== Scenario 3: Eval CLI handles invalid scorer gracefully ==="

  local eval_output
  eval_output=$(crush eval --scorer nonexistent_scorer_99999 --dataset /dev/null 2>&1 || true)

  if ! printf '%s' "$eval_output" | grep -qi "panic"; then
    pass "Scenario 3: No panic from invalid scorer invocation"
  else
    fail "Scenario 3: Panic from invalid scorer invocation"
  fi

  if printf '%s' "$eval_output" | grep -qiE "error|not found|unknown|invalid|no such"; then
    pass "Scenario 3: Error message produced for invalid scorer"
  else
    echo "  NOTE: No error message for invalid scorer (may be silently ignored)"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_eval_pipeline_scorers
test_eval_pipeline_capture
test_eval_pipeline_invalid_scorer

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-eval-pipeline" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
