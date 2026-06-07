#!/usr/bin/env bash
# Test: Explorer semantic quality verification.
# Verifies that the explorer subsystem correctly parses a Go fixture and
# produces an answer containing the expected symbols while excluding decoys.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

FIXTURE_REL="test/xrush-qa/fixtures/explorer_fixture.go"

# ---------------------------------------------------------------------------
# Scenario 1: Explorer produces semantically correct summary
# ---------------------------------------------------------------------------
test_explorer_semantic_quality() {
  echo "=== Scenario 1: Explorer semantic quality ==="

  setup_clean_crush
  cleanup_test() {
    restore_crush
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
  }
  trap cleanup_test EXIT

  start_crush 2
  send_prompt "Use the explorer to summarize the file $FIXTURE_REL. Include package name, exported type, functions, imports, and purpose."
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 8 "explorer-semantic"
    stop_crush
    return
  fi

  # Capture the full pane output for assertion.
  local pane_output
  pane_output=$(tmux capture-pane -t "$TMUX_SESSION" -p -S -1000 2>/dev/null || true)

  # Positive assertions: answer must contain expected symbols.
  if echo "$pane_output" | grep -q "qaexplorer"; then
    pass "Scenario 1: Answer contains 'qaexplorer' (package name)"
  else
    fail "Scenario 1: Answer missing 'qaexplorer' (package name)"
  fi

  if echo "$pane_output" | grep -q "FixtureStruct"; then
    pass "Scenario 1: Answer contains 'FixtureStruct'"
  else
    fail "Scenario 1: Answer missing 'FixtureStruct'"
  fi

  if echo "$pane_output" | grep -q "FixtureFunc"; then
    pass "Scenario 1: Answer contains 'FixtureFunc'"
  else
    fail "Scenario 1: Answer missing 'FixtureFunc'"
  fi

  # Negative assertion: decoy symbol must NOT appear.
  if echo "$pane_output" | grep -q "DecoyNotReal"; then
    fail "Scenario 1: Answer incorrectly contains decoy 'DecoyNotReal'"
  else
    pass "Scenario 1: Answer correctly excludes decoy 'DecoyNotReal'"
  fi

  # DB assertion: messages reference the fixture path.
  local session_id
  session_id=$(get_session_id)
  if [[ -n "$session_id" ]]; then
    local fixture_refs
    fixture_refs=$(query_db "SELECT COUNT(*) FROM messages WHERE session_id = '${session_id}' AND content LIKE '%explorer_fixture.go%';" 2>/dev/null || echo "0")
    local ref_count
    ref_count=$(echo "$fixture_refs" | jq '.[0]["COUNT(*)"]' 2>/dev/null || echo "$fixture_refs")
    if [[ "$ref_count" -gt 0 ]]; then
      pass "Scenario 1: DB references fixture path ($ref_count rows)"
    else
      fail "Scenario 1: DB has no references to fixture path"
    fi
  else
    fail "Scenario 1: Could not get session ID for DB check"
  fi

  capture_evidence 8 "explorer-semantic"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_explorer_semantic_quality

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
