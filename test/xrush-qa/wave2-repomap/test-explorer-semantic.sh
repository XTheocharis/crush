#!/usr/bin/env bash
# Test: Explorer semantic quality verification (TUI-first approach).
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
WAVE=2

# ---------------------------------------------------------------------------
# Scenario 1: Explorer produces semantically correct summary
# ---------------------------------------------------------------------------
test_explorer_semantic_quality() {
  echo "=== Scenario 1: Explorer semantic quality ==="
  SCENARIO="explorer-semantic"

  setup_clean_crush
  cleanup_test() { restore_crush; }
  trap cleanup_test EXIT

  start_crush_tui "$WAVE"
  focus_editor
  send_tui_prompt "Explore $FIXTURE_REL and report all symbols found. Include the package name, exported types, and functions. Reply with EXPLORER_SEMANTIC_SENTINEL_qaexplorer in your answer."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "EXPLORER_SEMANTIC_SENTINEL_qaexplorer"; then
    pass "Scenario 1: TUI shows EXPLORER_SEMANTIC_SENTINEL_qaexplorer sentinel"
  else
    fail "Scenario 1: TUI does not show EXPLORER_SEMANTIC_SENTINEL_qaexplorer sentinel"
    capture_tui_evidence "sentinel-missing"
    tmux send-keys -t "$TMUX_SESSION" C-c
    tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
    return
  fi

  # Positive assertions: answer must contain expected symbols.
  if assert_tui_contains "FixtureStruct"; then
    pass "Scenario 1: TUI shows FixtureStruct"
  else
    fail "Scenario 1: TUI missing FixtureStruct"
  fi

  if assert_tui_contains "FixtureFunc"; then
    pass "Scenario 1: TUI shows FixtureFunc"
  else
    fail "Scenario 1: TUI missing FixtureFunc"
  fi

  # Negative assertion: decoy symbol must NOT appear.
  if assert_tui_not_contains "DecoyNotReal"; then
    pass "Scenario 1: TUI correctly excludes decoy DecoyNotReal"
  else
    fail "Scenario 1: TUI incorrectly contains decoy DecoyNotReal"
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary DB check: messages reference the fixture path ---
  local SID
  SID=$(get_session_id)
  if [[ -n "$SID" ]]; then
    local fixture_refs
    fixture_refs=$(query_db "SELECT COUNT(*) FROM messages WHERE session_id = '${SID}' AND content LIKE '%explorer_fixture.go%';" 2>/dev/null || echo "0")
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

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_explorer_semantic_quality

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
