#!/usr/bin/env bash
# Test: Eval command lists scorers and provides usage info.
# Verifies that `crush eval` (no flags) lists available scorers and
# that the CLI does not return "unknown command" errors.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: Eval command lists available scorers
# ---------------------------------------------------------------------------
test_eval_lists_scorers() {
  echo "=== Scenario 1: Eval command lists available scorers ==="

  local output
  output=$(crush eval 2>&1) || true

  echo "--- Output ---"
  echo "$output"
  echo "--- End output ---"

  # Assert: output contains "Available scorers" header.
  if echo "$output" | grep -q "Available scorers"; then
    pass "Scenario 1: Output contains 'Available scorers' header"
  else
    fail "Scenario 1: Output missing 'Available scorers' header"
  fi

  # Assert: output is NOT "unknown command".
  if echo "$output" | grep -qi "unknown command"; then
    fail "Scenario 1: 'eval' reported as unknown command"
  else
    pass "Scenario 1: 'eval' is a recognized command"
  fi

  # Assert: output contains expected scorer names (spot-check a few).
  local expected_scorers=("build_success" "code_quality" "correctness" "lint_score")
  for scorer in "${expected_scorers[@]}"; do
    if echo "$output" | grep -q "$scorer"; then
      pass "Scenario 1: Scorer '$scorer' listed"
    else
      fail "Scenario 1: Scorer '$scorer' not found in output"
    fi
  done

  # Save evidence.
  local evidence_dir="${EVIDENCE_DIR:-.sisyphus/evidence}"
  mkdir -p "$evidence_dir"
  echo "$output" > "$evidence_dir/task-eval-scorers.txt"
}

test_eval_runs_tiny_dataset() {
  echo "=== Scenario 2: Eval command runs a tiny deterministic dataset ==="

  local dataset=/tmp/qa-eval-dataset.json
  local report=/tmp/qa-eval-report.json
  cat > "$dataset" <<'JSON'
{
  "name": "qa-eval-smoke",
  "version": "1",
  "examples": [
    {
      "id": "balanced-go",
      "name": "Balanced Go snippet",
      "input": {
        "session_id": "qa-eval-session",
        "conversation": [
          {"role": "user", "content": "write a small function"},
          {"role": "assistant", "content": "done"}
        ],
        "edits": [],
        "files": {
          "main.go": "package main\nfunc main() { println(\"ok\") }\n"
        }
      }
    }
  ]
}
JSON

  local output
  if output=$(crush eval --dataset "$dataset" --scorer syntax_validity --output "$report" 2>&1); then
    pass "Scenario 2: crush eval completed syntax_validity dataset run"
  else
    fail "Scenario 2: crush eval failed to run syntax_validity dataset"
    echo "$output"
    rm -f "$dataset" "$report"
    return
  fi

  if [[ -s "$report" ]]; then
    pass "Scenario 2: Eval wrote JSON report"
  else
    fail "Scenario 2: Eval report was not created"
    rm -f "$dataset" "$report"
    return
  fi

  local scorer_name
  scorer_name=$(jq -r '.scorer_scores[0].name // empty' "$report")
  if [[ "$scorer_name" == "syntax_validity" ]]; then
    pass "Scenario 2: Report contains syntax_validity scorer result"
  else
    fail "Scenario 2: Report scorer was '$scorer_name', expected syntax_validity"
  fi

  local passed
  passed=$(jq -r '.overall_passed' "$report")
  if [[ "$passed" == "true" ]]; then
    pass "Scenario 2: Eval report passed for balanced snippet"
  else
    fail "Scenario 2: Eval report did not pass for balanced snippet"
  fi

  rm -f "$dataset" "$report"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_eval_lists_scorers
test_eval_runs_tiny_dataset

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-eval" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
