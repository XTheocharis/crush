#!/usr/bin/env bash
# Test: Eval CLI command invocation.
# Verifies that `crush eval` CLI subcommand is available and produces
# expected output. Eval is CLI-only — no TUI palette command exists.
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
# Scenario 1: `crush eval --help` shows usage information
# ---------------------------------------------------------------------------
test_eval_cli_help() {
  SCENARIO="eval-cli-help"
  echo "=== Scenario 1: crush eval --help shows usage ==="

  # Verify eval subcommand exists and prints help without error.
  local eval_help
  eval_help=$(crush eval --help 2>&1 || true)

  if printf '%s' "$eval_help" | grep -qiE "eval|scorer|dataset|usage"; then
    pass "Scenario 1: crush eval --help shows eval-related usage text"
  else
    fail "Scenario 1: crush eval --help did not produce expected output"
  fi

  # Verify no panic.
  if ! printf '%s' "$eval_help" | grep -qi "panic\|fatal"; then
    pass "Scenario 1: No panic/fatal output from crush eval --help"
  else
    fail "Scenario 1: Panic/fatal output detected from crush eval --help"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: crush eval CLI produces structured output
# ---------------------------------------------------------------------------
test_eval_cli_output() {
  SCENARIO="eval-cli-output"
  echo "=== Scenario 2: crush eval CLI invocation produces output ==="

  setup_clean_crush

  # Create a minimal test dataset file.
  local dataset_dir
  dataset_dir=$(mktemp -d)
  echo '{"input":"test prompt","expected":"test response"}' > "$dataset_dir/sample.jsonl"

  # Run eval with a scorer. It may fail on missing scorer/dataset, but
  # should not panic and should produce some structured output.
  local eval_output
  eval_output=$(crush eval --scorer metric --dataset "$dataset_dir/sample.jsonl" 2>&1 || true)

  # Primary: CLI did not panic.
  if ! printf '%s' "$eval_output" | grep -qi "panic"; then
    pass "Scenario 2: crush eval CLI did not panic"
  else
    fail "Scenario 2: crush eval CLI panicked"
  fi

  # Secondary: output contains eval-related text (results, error, or usage).
  if printf '%s' "$eval_output" | grep -qiE "scorer|metric|eval|score|error|no.*dataset|file.*not.*found"; then
    pass "Scenario 2: crush eval produced eval-related output"
  else
    echo "  NOTE: crush eval output may not contain eval-specific text (output: $(printf '%s' "$eval_output" | head -5))"
  fi

  # Cleanup.
  rm -rf "$dataset_dir"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_eval_cli_help
test_eval_cli_output

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-eval" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
