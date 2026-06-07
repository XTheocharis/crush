#!/usr/bin/env bash
# Test: PostToolUse hook fires after file_edit tool.
# Creates a hooks config with a PostToolUse matcher on "^file_edit$" that
# writes a marker file, then verifies the sentinel appears in TUI output
# and the marker file exists after an edit-triggering prompt.
set -euo pipefail

WAVE=5
SCENARIO="posttooluse-hook-edit"
source "$(dirname "$0")/../lib/common.sh"

PASS=0
FAIL=0

HOOK_MARKER="/tmp/qa-posttool-marker-88.txt"
HOOK_SCRIPT="/tmp/qa-posttool-hook-88.sh"

cleanup_test() {
    cleanup_tui
  local _bak
  _bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  [[ -n "$_bak" ]] && mv "$_bak" crush.json
  restore_crush
  rm -f "$HOOK_MARKER" "$HOOK_SCRIPT"
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: PostToolUse hook fires after file_edit tool
# ---------------------------------------------------------------------------
test_posttooluse_hook_fires() {
  echo "=== Scenario 1: PostToolUse hook fires after file_edit tool ==="

  setup_clean_crush

  # Create a throwaway target file for the edit.
  local target_file="/tmp/qa-posttool-target-88.txt"
  echo "original content" > "$target_file"

  cat > "$HOOK_SCRIPT" << 'HOOK_EOF'
#!/usr/bin/env bash
echo "HOOK_POSTTOOL_SENTINEL_88" > /tmp/qa-posttool-marker-88.txt
HOOK_EOF
  chmod +x "$HOOK_SCRIPT"

  QA_DIR_RESOLVED="${QA_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"

  local hooks_config="$PROJECT_DIR/crush.json"
  if [[ -f "$hooks_config" ]]; then
    cp "$hooks_config" "$hooks_config.bak.$(date +%s)"
  fi

  local tmp_config
  tmp_config=$(mktemp)
  jq --arg script "$HOOK_SCRIPT" \
    '. + {"hooks":{"PostToolUse":[{"matcher":"^file_edit$","command":$script}]}}' \
    "$QA_DIR_RESOLVED/wave5.json" > "$tmp_config"
  mv "$tmp_config" "$hooks_config"

  rm -f "$HOOK_MARKER"

  start_crush_tui 5
  focus_editor

  send_tui_prompt "Edit the file /tmp/qa-posttool-target-88.txt and replace 'original content' with 'updated content'. When done, output exactly: HOOK_POSTTOOL_SENTINEL_88"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle within 120s"
    capture_tui_evidence "posttooluse-hook-timeout"
    rm -f "$target_file"
    return
  fi

  # Primary gate: TUI must contain the sentinel.
  if assert_tui_contains "HOOK_POSTTOOL_SENTINEL_88"; then
    pass "Scenario 1: TUI contains HOOK_POSTTOOL_SENTINEL_88"
  else
    fail "Scenario 1: TUI does not contain HOOK_POSTTOOL_SENTINEL_88"
    capture_tui_evidence "posttooluse-hook-no-sentinel"
  fi

  # Secondary: marker file must exist.
  if [[ -f "$HOOK_MARKER" ]]; then
    pass "Scenario 1: PostToolUse hook marker file exists"
  else
    fail "Scenario 1: PostToolUse hook marker file not found at $HOOK_MARKER"
  fi

  # Tertiary: log grep for hook execution.
  local log_entries
  log_entries=$(grep -ciE "hook.*PostToolUse|PostToolUse.*hook|running hook" .crush/logs/crush.log 2>/dev/null || echo 0)
  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $log_entries hook-related log entries"
  else
    echo "  NOTE: No hook log entries found (hook logging may be minimal)"
  fi

  rm -f "$target_file"
  capture_tui_evidence "posttooluse-hook-fired"
}

# ---------------------------------------------------------------------------
# Scenario 2: PostToolUse hook receives correct env vars
# ---------------------------------------------------------------------------
test_posttooluse_hook_env_vars() {
  echo "=== Scenario 2: PostToolUse hook receives correct env vars ==="

  SCENARIO="posttooluse-hook-env"
  setup_clean_crush

  local env_marker="/tmp/qa-posttool-env-88.txt"
  local target_file="/tmp/qa-posttool-target-88b.txt"
  echo "env test content" > "$target_file"

  cat > "$HOOK_SCRIPT" << 'HOOK_EOF'
#!/usr/bin/env bash
env | grep '^CRUSH_' > /tmp/qa-posttool-env-88.txt
echo "HOOK_POSTTOOL_SENTINEL_88" > /tmp/qa-posttool-marker-88.txt
HOOK_EOF
  chmod +x "$HOOK_SCRIPT"

  QA_DIR_RESOLVED="${QA_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"

  local hooks_config="$PROJECT_DIR/crush.json"
  if [[ -f "$hooks_config" ]]; then
    cp "$hooks_config" "$hooks_config.bak.$(date +%s)"
  fi

  local tmp_config
  tmp_config=$(mktemp)
  jq --arg script "$HOOK_SCRIPT" \
    '. + {"hooks":{"PostToolUse":[{"matcher":"^file_edit$","command":$script}]}}' \
    "$QA_DIR_RESOLVED/wave5.json" > "$tmp_config"
  mv "$tmp_config" "$hooks_config"

  rm -f "$HOOK_MARKER" "$env_marker"

  start_crush_tui 5
  focus_editor

  send_tui_prompt "Edit /tmp/qa-posttool-target-88b.txt and change 'env test content' to 'env test updated'. When done, output exactly: HOOK_POSTTOOL_SENTINEL_88"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle within 120s"
    capture_tui_evidence "posttooluse-env-timeout"
    rm -f "$target_file"
    return
  fi

  # Primary: TUI sentinel.
  if assert_tui_contains "HOOK_POSTTOOL_SENTINEL_88"; then
    pass "Scenario 2: TUI contains HOOK_POSTTOOL_SENTINEL_88"
  else
    fail "Scenario 2: TUI does not contain HOOK_POSTTOOL_SENTINEL_88"
    capture_tui_evidence "posttooluse-env-no-sentinel"
  fi

  # Secondary: env vars.
  if [[ -s "$env_marker" ]] && grep -q '^CRUSH_EVENT=PostToolUse$' "$env_marker"; then
    pass "Scenario 2: PostToolUse hook env contains CRUSH_EVENT=PostToolUse"
  else
    fail "Scenario 2: PostToolUse hook env missing CRUSH_EVENT=PostToolUse"
  fi

  if [[ -s "$env_marker" ]] && grep -q '^CRUSH_TOOL_NAME=file_edit$' "$env_marker"; then
    pass "Scenario 2: PostToolUse hook env contains CRUSH_TOOL_NAME=file_edit"
  else
    fail "Scenario 2: PostToolUse hook env missing CRUSH_TOOL_NAME=file_edit"
  fi

  rm -f "$env_marker" "$target_file"
  capture_tui_evidence "posttooluse-hook-env"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_posttooluse_hook_fires
test_posttooluse_hook_env_vars

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
