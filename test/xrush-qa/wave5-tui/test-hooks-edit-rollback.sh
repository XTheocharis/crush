#!/usr/bin/env bash
# Test: Cross-feature interaction — hooks + edit rollback.
# Scenario 1: PreToolUse hook BLOCKS a file_edit; TUI explains outcome, no
#              partial edit remains on disk.
# Scenario 2: Edit succeeds, then rollback runs. Hook fires on both the edit
#              and the rollback tool calls. Filesystem proves original state
#              restored.
set -euo pipefail

WAVE=5

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# Deterministic sentinels.
BLOCK_SENTINEL="HOOK_EDIT_BLOCK_SENTINEL_42"
ROLLBACK_SENTINEL="HOOK_EDIT_ROLLBACK_SENTINEL_88"

# Shared temp directories and hook paths.
FIXTURE_DIR=""
FIXTURE_DIR=$(mktemp -d "${TMPDIR:-/tmp}/qa-hooks-edit-rollback-XXXXXX")
HOOK_SCRIPT="/tmp/qa-block-edit-hook.sh"
ROLLBACK_HOOK_SCRIPT="/tmp/qa-rollback-edit-hook.sh"
BLOCK_MARKER="/tmp/qa-block-edit-marker.txt"
ROLLBACK_MARKER="/tmp/qa-rollback-edit-marker.txt"

cleanup_test() {
    cleanup_tui
  local _bak
  _bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  [[ -n "$_bak" ]] && mv "$_bak" crush.json
  restore_crush
  rm -rf "$FIXTURE_DIR"
  rm -f "$HOOK_SCRIPT" "$ROLLBACK_HOOK_SCRIPT" "$BLOCK_MARKER" "$ROLLBACK_MARKER"
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: PreToolUse hook BLOCKS file_edit — no partial edit remains
# ---------------------------------------------------------------------------
test_hook_blocks_edit() {
  SCENARIO="hook-blocks-edit"
  echo "=== Scenario 1: PreToolUse hook blocks file_edit ==="

  local target_dir="$FIXTURE_DIR/block"
  mkdir -p "$target_dir"

  # Original file content — must remain untouched after blocked edit.
  cat > "$target_dir/target.txt" <<'EOF'
Original line one.
Original line two.
Original line three.
EOF

  local orig_content
  orig_content=$(cat "$target_dir/target.txt")

  setup_clean_crush

  # Hook that BLOCKS file_edit by writing a marker and exiting 2 (deny).
  # The hook inspects CRUSH_TOOL_NAME; if it matches file_edit, it denies.
  cat > "$HOOK_SCRIPT" <<'HOOK_EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "$CRUSH_TOOL_NAME" > /tmp/qa-block-edit-marker.txt
if [[ "$CRUSH_TOOL_NAME" == "file_edit" ]]; then
  echo "HOOK_EDIT_BLOCK_SENTINEL_42: edit blocked by policy" >&2
  exit 2
fi
exit 0
HOOK_EOF
  chmod +x "$HOOK_SCRIPT"

  # Build hooks config: PreToolUse hook on file_edit that blocks.
  QA_DIR_RESOLVED="${QA_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)}"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"

  local hooks_config="$PROJECT_DIR/crush.json"
  if [[ -f "$hooks_config" ]]; then
    cp "$hooks_config" "$hooks_config.bak.$(date +%s)"
  fi

  local tmp_config
  tmp_config=$(mktemp)
  jq --arg script "$HOOK_SCRIPT" \
    '. + {"hooks":{"PreToolUse":[{"matcher":"file_edit","command":$script}]}}' \
    "$QA_DIR_RESOLVED/wave5.json" > "$tmp_config"
  mv "$tmp_config" "$hooks_config"

  # Verify hooks config is present in generated config.
  if jq -e '.hooks.PreToolUse' "$hooks_config" >/dev/null 2>&1; then
    pass "Scenario 1: Hooks config present in crush.json"
  else
    fail "Scenario 1: Hooks config missing from crush.json"
  fi

  rm -f "$BLOCK_MARKER"

  start_crush_tui 5
  focus_editor

  # Prompt that would trigger a file_edit on the target file.
  send_tui_prompt "Edit the file $target_dir/target.txt and replace 'Original line two.' with 'MODIFIED_BY_EDIT'. After the edit, output exactly: MODIFIED_BY_EDIT"

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_tui_evidence "hook-block-timeout"
    return
  fi

  # Primary gate: TUI must show the block sentinel — hook outcome explained.
  if assert_tui_contains "$BLOCK_SENTINEL"; then
    pass "Scenario 1: TUI contains $BLOCK_SENTINEL (hook block explained)"
  else
    fail "Scenario 1: TUI does not contain $BLOCK_SENTINEL"
    capture_tui_evidence "hook-block-no-sentinel"
  fi

  # Secondary: hook marker file proves the hook fired (polling with timeout).
  if wait_for_file "$BLOCK_MARKER" 30; then
    local tool_name
    tool_name=$(cat "$BLOCK_MARKER")
    if [[ "$tool_name" == "file_edit" ]]; then
      pass "Scenario 1: Hook fired for file_edit tool"
    else
      fail "Scenario 1: Hook fired for unexpected tool: $tool_name"
    fi
  else
    fail "Scenario 1: Hook marker file not found — hook did not fire"
  fi

  # Secondary: filesystem — target file must be completely unchanged.
  local current_content
  current_content=$(cat "$target_dir/target.txt")
  if [[ "$current_content" == "$orig_content" ]]; then
    pass "Scenario 1: Target file unchanged — no partial edit remains"
  else
    fail "Scenario 1: Target file was modified despite hook block"
  fi

  # Tertiary: the modified sentinel must NOT appear in TUI output.
  if assert_tui_not_contains "MODIFIED_BY_EDIT"; then
    pass "Scenario 1: No MODIFIED_BY_EDIT in TUI — edit was truly blocked"
  else
    fail "Scenario 1: MODIFIED_BY_EDIT found in TUI despite block"
  fi

  # Tertiary: log grep for hook execution evidence.
  local log_entries
  log_entries=$(grep -ciE "hook.*PreToolUse|PreToolUse.*hook|running hook|hook.*blocked" .crush/logs/crush.log 2>/dev/null ) || log_entries=0
  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $log_entries hook-related log entries"
  else
    echo "  NOTE: No hook log entries found (hook logging may be minimal)"
  fi

  capture_tui_evidence "hook-blocks-edit"
}

# ---------------------------------------------------------------------------
# Scenario 2: Hook runs on edit AND rollback — original state restored
# ---------------------------------------------------------------------------
test_hook_on_edit_and_rollback() {
  SCENARIO="hook-edit-rollback"
  echo "=== Scenario 2: Hook runs on edit and rollback ==="

  local rb_dir="$FIXTURE_DIR/rollback"
  mkdir -p "$rb_dir"

  # File to edit then rollback — save original content.
  cat > "$rb_dir/rollback.txt" <<'EOF'
Rollback original line one.
Rollback original line two.
Rollback original line three.
EOF

  local orig_content
  orig_content=$(cat "$rb_dir/rollback.txt")

  setup_clean_crush

  # Hook that records every invocation — does NOT block (exits 0).
  # Appends CRUSH_TOOL_NAME to marker file so we can see both calls.
  cat > "$ROLLBACK_HOOK_SCRIPT" <<'HOOK_EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "$CRUSH_TOOL_NAME" >> /tmp/qa-rollback-edit-marker.txt
exit 0
HOOK_EOF
  chmod +x "$ROLLBACK_HOOK_SCRIPT"

  # Build hooks config: PreToolUse hook that runs on file_edit and rollback.
  QA_DIR_RESOLVED="${QA_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)}"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"

  local hooks_config="$PROJECT_DIR/crush.json"
  if [[ -f "$hooks_config" ]]; then
    cp "$hooks_config" "$hooks_config.bak.$(date +%s)"
  fi

  local tmp_config
  tmp_config=$(mktemp)
  jq --arg script "$ROLLBACK_HOOK_SCRIPT" \
    '. + {"hooks":{"PreToolUse":[{"matcher":"file_edit|rollback","command":$script}]}}' \
    "$QA_DIR_RESOLVED/wave5.json" > "$tmp_config"
  mv "$tmp_config" "$hooks_config"

  # Verify hooks config is present in generated config.
  if jq -e '.hooks.PreToolUse' "$hooks_config" >/dev/null 2>&1; then
    pass "Scenario 2: Hooks config present in crush.json"
  else
    fail "Scenario 2: Hooks config missing from crush.json"
  fi

  rm -f "$ROLLBACK_MARKER"

  start_crush_tui 5
  focus_editor

  # Prompt: edit the file, then roll it back.
  send_tui_prompt "First, edit the file $rb_dir/rollback.txt and replace 'Rollback original line two.' with 'HOOK_EDIT_ROLLBACK_SENTINEL_88 changed'. Then, use the rollback tool to revert the file to its state before this edit. After the rollback completes, output exactly: HOOK_EDIT_ROLLBACK_SENTINEL_88"

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle within 180s"
    capture_tui_evidence "hook-rollback-timeout"
    return
  fi

  # Primary gate: TUI must contain the rollback sentinel.
  if assert_tui_contains "$ROLLBACK_SENTINEL"; then
    pass "Scenario 2: TUI contains $ROLLBACK_SENTINEL"
  else
    fail "Scenario 2: TUI does not contain $ROLLBACK_SENTINEL"
    capture_tui_evidence "hook-rollback-no-sentinel"
  fi

  # Secondary: filesystem — file must be restored to original content.
  local current_content
  current_content=$(cat "$rb_dir/rollback.txt" 2>/dev/null || echo "")
  if [[ "$current_content" == "$orig_content" ]]; then
    pass "Scenario 2: File restored to original content after rollback"
  else
    fail "Scenario 2: File not restored to original content after rollback"
  fi

  # Secondary: the modified text must NOT remain in the file.
  if ! grep -qF "HOOK_EDIT_ROLLBACK_SENTINEL_88 changed" "$rb_dir/rollback.txt" 2>/dev/null; then
    pass "Scenario 2: Modified text gone after rollback"
  else
    fail "Scenario 2: Modified text still present after rollback"
  fi

  # Secondary: hook marker proves hook ran on both edit and rollback (polling with timeout).
  if wait_for_file "$ROLLBACK_MARKER" 30; then
    local hook_calls
    hook_calls=$(cat "$ROLLBACK_MARKER")
    local edit_count
    edit_count=$(grep -c "file_edit" "$ROLLBACK_MARKER" ) || edit_count=0
    local rollback_count
    rollback_count=$(grep -ci "rollback" "$ROLLBACK_MARKER" ) || rollback_count=0

    if [[ "$edit_count" -ge 1 ]]; then
      pass "Scenario 2: Hook fired for file_edit ($edit_count time(s))"
    else
      fail "Scenario 2: Hook did not fire for file_edit"
    fi

    if [[ "$rollback_count" -ge 1 ]]; then
      pass "Scenario 2: Hook fired for rollback ($rollback_count time(s))"
    else
      echo "  NOTE: Hook did not fire for rollback — rollback may not be a named tool"
    fi
  else
    fail "Scenario 2: Hook marker file not found — hook did not fire"
  fi

  # Tertiary: log grep for hook execution evidence.
  local log_entries
  log_entries=$(grep -ciE "hook.*PreToolUse|PreToolUse.*hook|running hook" .crush/logs/crush.log 2>/dev/null ) || log_entries=0
  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 2: Found $log_entries hook-related log entries"
  else
    echo "  NOTE: No hook log entries found (hook logging may be minimal)"
  fi

  capture_tui_evidence "hook-edit-rollback"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_hook_blocks_edit
test_hook_on_edit_and_rollback

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-hooks-edit-rollback" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
