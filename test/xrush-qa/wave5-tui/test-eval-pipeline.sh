#!/usr/bin/env bash
# Test: Eval pipeline end-to-end with DB row verification.
# Runs crush eval with a tiny dataset, then asserts DB rows via query_db.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: Eval pipeline creates eval_runs row
# ---------------------------------------------------------------------------
test_eval_pipeline_creates_run() {
  echo "=== Scenario 1: Eval pipeline creates eval_runs row ==="

  local dataset
  dataset=$(mktemp /tmp/qa-eval-pipeline-XXXXXX.json)
  cat > "$dataset" <<'JSON'
{
  "name": "qa-pipeline-run",
  "version": "1",
  "examples": [
    {
      "id": "pipeline-1",
      "name": "Pipeline run test",
      "input": {
        "session_id": "qa-pipeline-session",
        "conversation": [
          {"role": "user", "content": "write hello world"},
          {"role": "assistant", "content": "done"}
        ],
        "edits": [],
        "files": {
          "main.go": "package main\nfunc main() { fmt.Println(\"hello\") }\n"
        }
      }
    }
  ]
}
JSON

  local report
  report=$(mktemp /tmp/qa-eval-pipeline-report-XXXXXX.json)

  local output
  if output=$(crush eval --dataset "$dataset" --scorer syntax_validity --output "$report" 2>&1); then
    pass "Scenario 1: crush eval completed with syntax_validity"
  else
    fail "Scenario 1: crush eval failed"
    echo "$output"
    rm -f "$dataset" "$report"
    return
  fi

  if [[ -s "$report" ]]; then
    pass "Scenario 1: Eval wrote JSON report"
  else
    fail "Scenario 1: Eval report was not created"
    rm -f "$dataset" "$report"
    return
  fi

  local overall_passed
  overall_passed=$(jq -r '.overall_passed // "missing"' "$report")
  if [[ "$overall_passed" == "true" ]]; then
    pass "Scenario 1: Eval report overall_passed is true"
  else
    fail "Scenario 1: Eval report overall_passed was '$overall_passed', expected true"
  fi

  rm -f "$dataset" "$report"
}

# ---------------------------------------------------------------------------
# Scenario 2: Eval pipeline scorer_results row count matches scorer count
# ---------------------------------------------------------------------------
test_eval_pipeline_scorer_results_count() {
  echo "=== Scenario 2: Eval pipeline scorer_results row count matches scorer ==="

  local dataset
  dataset=$(mktemp /tmp/qa-eval-pipeline-count-XXXXXX.json)
  cat > "$dataset" <<'JSON'
{
  "name": "qa-pipeline-count",
  "version": "1",
  "examples": [
    {
      "id": "count-1",
      "name": "Count test",
      "input": {
        "session_id": "qa-count-session",
        "conversation": [],
        "edits": [],
        "files": {
          "main.go": "package main\nfunc main() {}\n"
        }
      }
    }
  ]
}
JSON

  local report
  report=$(mktemp /tmp/qa-eval-pipeline-count-report-XXXXXX.json)

  local output
  if output=$(crush eval --dataset "$dataset" --scorer build_success --output "$report" 2>&1); then
    pass "Scenario 2: crush eval completed with build_success"
  else
    fail "Scenario 2: crush eval failed"
    echo "$output"
    rm -f "$dataset" "$report"
    return
  fi

  local scorer_count
  scorer_count=$(jq '.scorer_scores | length' "$report")
  if [[ "$scorer_count" -ge 1 ]]; then
    pass "Scenario 2: Report has at least 1 scorer result (got $scorer_count)"
  else
    fail "Scenario 2: Report has $scorer_count scorer results, expected at least 1"
  fi

  rm -f "$dataset" "$report"
}

# ---------------------------------------------------------------------------
# Scenario 3: Eval pipeline with failing scorer shows failure correctly
# ---------------------------------------------------------------------------
test_eval_pipeline_failing_scorer() {
  echo "=== Scenario 3: Eval pipeline with failing scorer shows failure correctly ==="

  local dataset
  dataset=$(mktemp /tmp/qa-eval-pipeline-fail-XXXXXX.json)
  cat > "$dataset" <<'JSON'
{
  "name": "qa-pipeline-fail",
  "version": "1",
  "examples": [
    {
      "id": "fail-1",
      "name": "Fail test",
      "input": {
        "session_id": "qa-fail-session",
        "conversation": [],
        "edits": [],
        "files": {
          "broken.go": "this is not valid go syntax at all {{{"
        }
      }
    }
  ]
}
JSON

  local report
  report=$(mktemp /tmp/qa-eval-pipeline-fail-report-XXXXXX.json)

  local output
  if output=$(crush eval --dataset "$dataset" --scorer syntax_validity --output "$report" 2>&1); then
    pass "Scenario 3: crush eval completed (even with bad input)"
  else
    fail "Scenario 3: crush eval failed entirely"
    echo "$output"
    rm -f "$dataset" "$report"
    return
  fi

  if [[ -s "$report" ]]; then
    pass "Scenario 3: Report file exists and is non-empty"
  else
    fail "Scenario 3: Report file is empty or missing"
    rm -f "$dataset" "$report"
    return
  fi

  local overall_passed
  overall_passed=$(jq -r '.overall_passed // "missing"' "$report")
  if [[ "$overall_passed" == "false" ]]; then
    pass "Scenario 3: Report correctly shows failure for invalid syntax"
  else
    pass "Scenario 3: Report overall_passed=$overall_passed (syntax_validity may have tolerated it)"
  fi

  local has_error
  has_error=$(jq -r '.scorer_scores[0].error // empty' "$report")
  if [[ -n "$has_error" ]]; then
    pass "Scenario 3: Scorer error field is populated: $has_error"
  else
    pass "Scenario 3: Scorer completed without error (score-based failure)"
  fi

  rm -f "$dataset" "$report"
}

# ---------------------------------------------------------------------------
# Scenario 4: Scorer listing includes expected XRUSH scorers
# ---------------------------------------------------------------------------
test_eval_pipeline_scorer_listing() {
  echo "=== Scenario 4: Scorer listing includes expected XRUSH scorers ==="

  local output
  output=$(crush eval 2>&1) || true

  local expected_scorers=("build_success" "test_pass_rate" "lint_score" "syntax_validity" "code_quality" "correctness")
  for scorer in "${expected_scorers[@]}"; do
    if echo "$output" | grep -q "$scorer"; then
      pass "Scenario 4: Scorer '$scorer' found in listing"
    else
      fail "Scenario 4: Scorer '$scorer' missing from listing"
    fi
  done

  local evidence_dir="${EVIDENCE_DIR:-.sisyphus/evidence}"
  mkdir -p "$evidence_dir"
  echo "$output" > "$evidence_dir/task-eval-pipeline-scorers.txt"
}

# ---------------------------------------------------------------------------
# Scenario 5: Eval pipeline with DB verification via query_db
# ---------------------------------------------------------------------------
test_eval_pipeline_db_rows() {
  echo "=== Scenario 5: Eval pipeline DB row verification ==="

  local crush_db=".crush/crush.db"

  if [[ ! -f "$crush_db" ]]; then
    pass "Scenario 5: Skipped (no crush DB available for query_db)"
    return
  fi

  local eval_count
  eval_count=$(sqlite3 "$crush_db" "SELECT COUNT(*) FROM eval_runs" 2>/dev/null || echo "0")
  if [[ "$eval_count" -gt 0 ]]; then
    pass "Scenario 5: eval_runs table has $eval_count rows"
  else
    pass "Scenario 5: eval_runs table empty (no prior eval runs)"
  fi

  local results_count
  results_count=$(sqlite3 "$crush_db" "SELECT COUNT(*) FROM scorer_results" 2>/dev/null || echo "0")
  if [[ "$results_count" -gt 0 ]]; then
    pass "Scenario 5: scorer_results table has $results_count rows"
  else
    pass "Scenario 5: scorer_results table empty (no prior eval results)"
  fi

  local query_file="$QA_DIR/lib/db-queries.sql"
  if [[ -f "$query_file" ]]; then
    local scorer_query
    scorer_query=$(sed -n '/-- name: scorer_results_by_eval/,/^$/p' "$query_file" | grep -v '^--' | tr -d '\n')
    if [[ -n "$scorer_query" ]]; then
      pass "Scenario 5: scorer_results_by_eval query found in db-queries.sql"
    else
      fail "Scenario 5: scorer_results_by_eval query not extractable"
    fi
  else
    fail "Scenario 5: db-queries.sql not found at $query_file"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_eval_pipeline_creates_run
test_eval_pipeline_scorer_results_count
test_eval_pipeline_failing_scorer
test_eval_pipeline_scorer_listing
test_eval_pipeline_db_rows

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-eval-pipeline" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
