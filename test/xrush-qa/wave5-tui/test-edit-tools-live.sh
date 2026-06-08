#!/usr/bin/env bash
# Test: Edit tool end-to-end TUI-first scenarios with deterministic sentinels.
# Scenario 1: Multi-file edit — edit multiple fixture files, verify content.
# Scenario 2: Precision edit — target specific section content, verify precision.
# Scenario 3: Approximate match edit + rollback — fuzzy edit, then rollback verification.
set -euo pipefail

WAVE=5

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

FIXTURE_DIR=""
# Shared fixture root — all scenarios create subdirectories here.
FIXTURE_DIR=$(mktemp -d "${TMPDIR:-/tmp}/qa-edit-tools-XXXXXX")

cleanup_test() {
    cleanup_tui
  local _bak
  _bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  [[ -n "$_bak" ]] && mv "$_bak" crush.json
  restore_crush
  rm -rf "$FIXTURE_DIR"
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Batch edit — create 3 fixture files, edit all atomically
# ---------------------------------------------------------------------------
test_batch_edit() {
  SCENARIO="edit-batch"
  echo "=== Scenario 1: Batch edit — multi-file atomic edit ==="

  local batch_dir="$FIXTURE_DIR/batch"
  mkdir -p "$batch_dir"

  cat > "$batch_dir/alpha.txt" <<'EOF'
Line one of alpha.
Line two of alpha.
EOF
  cat > "$batch_dir/beta.txt" <<'EOF'
Line one of beta.
Line two of beta.
EOF
  cat > "$batch_dir/gamma.txt" <<'EOF'
Line one of gamma.
Line two of gamma.
EOF

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  send_tui_prompt "Use the edit tool to make the following changes atomically: (1) In $batch_dir/alpha.txt replace 'Line one of alpha.' with 'EDIT_BATCH_SENTINEL_42 alpha updated'. (2) In $batch_dir/beta.txt replace 'Line one of beta.' with 'EDIT_BATCH_SENTINEL_42 beta updated'. (3) In $batch_dir/gamma.txt replace 'Line one of gamma.' with 'EDIT_BATCH_SENTINEL_42 gamma updated'. After all three edits are done, output exactly: EDIT_BATCH_SENTINEL_42"

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_tui_evidence "batch-timeout"
    return
  fi

  # Primary: TUI must contain the sentinel.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "EDIT_BATCH_SENTINEL_42"; then
    pass "Scenario 1: TUI contains EDIT_BATCH_SENTINEL_42"
  else
    fail "Scenario 1: TUI does not contain EDIT_BATCH_SENTINEL_42"
    capture_tui_evidence "batch-no-sentinel"
  fi

  # Secondary: filesystem — verify all 3 files modified.
  local files_ok=0
  for f in alpha beta gamma; do
    if [[ -f "$batch_dir/${f}.txt" ]] && grep -qF "EDIT_BATCH_SENTINEL_42 ${f} updated" "$batch_dir/${f}.txt"; then
      ((files_ok += 1))
    else
      fail "Scenario 1: ${f}.txt does not contain expected updated content"
    fi
  done

  if [[ "$files_ok" -eq 3 ]]; then
    pass "Scenario 1: All 3 fixture files updated (batch atomicity confirmed)"
  fi

  # Secondary: verify no leftover original content.
  local leftover=0
  for f in alpha beta gamma; do
    if grep -qF "Line one of ${f}." "$batch_dir/${f}.txt" 2>/dev/null; then
      ((leftover += 1))
    fi
  done

  if [[ "$leftover" -eq 0 ]]; then
    pass "Scenario 1: No leftover original content in any fixture file"
  else
    fail "Scenario 1: $leftover file(s) still contain original content"
  fi

  capture_tui_evidence "batch-edit"
}

# ---------------------------------------------------------------------------
# Scenario 2: Anchor edit — target specific content sections precisely
# ---------------------------------------------------------------------------
test_anchor_edit() {
  SCENARIO="edit-anchor"
  echo "=== Scenario 2: Anchor edit — precise section targeting ==="

  local anchor_dir="$FIXTURE_DIR/anchor"
  mkdir -p "$anchor_dir"

  cat > "$anchor_dir/sections.txt" <<'EOF'
# BEGIN_SECTION_A
Content for section A — original text.
# END_SECTION_A

# BEGIN_SECTION_B
Content for section B — original text.
# END_SECTION_B

# BEGIN_SECTION_C
Content for section C — original text.
# END_SECTION_C
EOF

  # Kill previous session and start fresh.
    cleanup_tui
  start_crush_tui 5
  focus_editor

  send_tui_prompt "In the file $anchor_dir/sections.txt, replace ONLY the line 'Content for section B — original text.' with 'EDIT_ANCHOR_SENTINEL_88 section B updated'. Do NOT modify sections A or C. After completing the edit, output exactly: EDIT_ANCHOR_SENTINEL_88"

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle within 180s"
    capture_tui_evidence "anchor-timeout"
    return
  fi

  # Primary: TUI must contain the sentinel.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "EDIT_ANCHOR_SENTINEL_88"; then
    pass "Scenario 2: TUI contains EDIT_ANCHOR_SENTINEL_88"
  else
    fail "Scenario 2: TUI does not contain EDIT_ANCHOR_SENTINEL_88"
    capture_tui_evidence "anchor-no-sentinel"
  fi

  # Secondary: verify section B was modified.
  if grep -qF "EDIT_ANCHOR_SENTINEL_88 section B updated" "$anchor_dir/sections.txt" 2>/dev/null; then
    pass "Scenario 2: Section B contains updated content"
  else
    fail "Scenario 2: Section B does not contain expected updated content"
  fi

  # Secondary: verify sections A and C were NOT modified (anchor precision).
  if grep -qF "Content for section A — original text." "$anchor_dir/sections.txt" 2>/dev/null; then
    pass "Scenario 2: Section A unchanged (anchor precision confirmed)"
  else
    fail "Scenario 2: Section A was modified (anchor missed target)"
  fi

  if grep -qF "Content for section C — original text." "$anchor_dir/sections.txt" 2>/dev/null; then
    pass "Scenario 2: Section C unchanged (anchor precision confirmed)"
  else
    fail "Scenario 2: Section C was modified (anchor missed target)"
  fi

  # Secondary: original section B content should be gone.
  if ! grep -qF "Content for section B — original text." "$anchor_dir/sections.txt" 2>/dev/null; then
    pass "Scenario 2: Original section B content replaced"
  else
    fail "Scenario 2: Original section B content still present"
  fi

  capture_tui_evidence "anchor-edit"
}

# ---------------------------------------------------------------------------
# Scenario 3: Fuzzy match edit + rollback verification
# ---------------------------------------------------------------------------
test_fuzzy_rollback() {
  SCENARIO="edit-fuzzy-rollback"
  echo "=== Scenario 3: Fuzzy match edit + rollback ==="

  local fr_dir="$FIXTURE_DIR/fuzzy-rollback"
  mkdir -p "$fr_dir"

  # File for fuzzy match testing — contains a near-duplicate line.
  cat > "$fr_dir/fuzzy.txt" <<'EOF'
This is the original content of the file.
It has multiple lines of text.
Some lines are very similar to others.
This is the original content of the file — duplicated with extra.
EOF

  # File for rollback testing — save original content for comparison.
  cat > "$fr_dir/rollback.txt" <<'ROLLBACK_ORIG'
Before rollback line one.
Before rollback line two.
ROLLBACK_ORIG

  local orig_rollback
  orig_rollback=$(cat "$fr_dir/rollback.txt")

  # Kill previous session and start fresh.
    cleanup_tui
  start_crush_tui 5
  focus_editor

  # Phase 1: Fuzzy match edit.
  send_tui_prompt "In the file $fr_dir/fuzzy.txt, use a fuzzy or approximate match edit to find and replace the line containing 'duplicated with extra' with 'EDIT_FUZZY_SENTINEL_55 fuzzy match succeeded'. Do not modify the first line that starts with 'This is the original content' but does NOT contain 'duplicated'. After completing the edit, output exactly: EDIT_FUZZY_SENTINEL_55"

  if ! wait_for_tui_idle 180; then
    fail "Scenario 3: Crush did not become idle within 180s (fuzzy phase)"
    capture_tui_evidence "fuzzy-timeout"
    return
  fi

  # Primary: TUI must contain the fuzzy sentinel.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "EDIT_FUZZY_SENTINEL_55"; then
    pass "Scenario 3: TUI contains EDIT_FUZZY_SENTINEL_55"
  else
    fail "Scenario 3: TUI does not contain EDIT_FUZZY_SENTINEL_55"
    capture_tui_evidence "fuzzy-no-sentinel"
  fi

  # Secondary: filesystem check for fuzzy match result.
  if grep -qF "EDIT_FUZZY_SENTINEL_55 fuzzy match succeeded" "$fr_dir/fuzzy.txt" 2>/dev/null; then
    pass "Scenario 3: Fuzzy match edit applied to target file"
  else
    fail "Scenario 3: Fuzzy match edit not applied to target file"
  fi

  # Secondary: non-target line should be untouched.
  if grep -qF "This is the original content of the file." "$fr_dir/fuzzy.txt" 2>/dev/null; then
    pass "Scenario 3: Non-target lines preserved after fuzzy edit"
  else
    fail "Scenario 3: Non-target lines not preserved after fuzzy edit"
  fi

  capture_tui_evidence "fuzzy-edit"

  # Phase 2: Edit then rollback on same session.
  send_tui_prompt "Now edit the file $fr_dir/rollback.txt — replace 'Before rollback line one.' with 'EDIT_ROLLBACK_SENTINEL_33 changed'. Then use the rollback tool to revert the file to its state before this edit. After rolling back, output exactly: EDIT_ROLLBACK_SENTINEL_33"

  if ! wait_for_tui_idle 180; then
    fail "Scenario 3: Crush did not become idle within 180s (rollback phase)"
    capture_tui_evidence "rollback-timeout"
    return
  fi

  # Primary: TUI must contain the rollback sentinel.
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "EDIT_ROLLBACK_SENTINEL_33"; then
    pass "Scenario 3: TUI contains EDIT_ROLLBACK_SENTINEL_33"
  else
    fail "Scenario 3: TUI does not contain EDIT_ROLLBACK_SENTINEL_33"
    capture_tui_evidence "rollback-no-sentinel"
  fi

  # Secondary: rollback file should be restored to original content.
  local current_rollback
  current_rollback=$(cat "$fr_dir/rollback.txt" 2>/dev/null || echo "")
  if [[ "$current_rollback" == "$orig_rollback" ]]; then
    pass "Scenario 3: Rollback file restored to original content"
  else
    fail "Scenario 3: Rollback file not restored to original (rollback failed)"
  fi

  capture_tui_evidence "rollback-edit"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_batch_edit
test_anchor_edit
test_fuzzy_rollback

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-edit-tools-live" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
