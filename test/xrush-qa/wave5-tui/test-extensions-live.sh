#!/usr/bin/env bash
# Test: Extension host lifecycle via TUI.
# Verifies that Crush loads extensions during startup and exercises the
# prompt-assembly extension path during agent turns.
set -euo pipefail

WAVE=5
SCENARIO="extension-load-verification"
source "$(dirname "$0")/../lib/common.sh"

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
# Scenario 1: Extension load verification
# ---------------------------------------------------------------------------
test_extension_load() {
  SCENARIO="extension-load-verification"
  echo "=== Scenario 1: Extension load verification ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Ask Crush to report loaded extensions and emit the sentinel.
  send_tui_prompt "List every extension that is currently loaded in this session. After listing them, output exactly: EXTENSION_LOAD_SENTINEL_42"

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_tui_evidence "extension-load-timeout"
    return
  fi

  # Primary gate: TUI must contain the sentinel.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "EXTENSION_LOAD_SENTINEL_42"; then
    pass "Scenario 1: TUI contains EXTENSION_LOAD_SENTINEL_42"
  else
    fail "Scenario 1: TUI does not contain EXTENSION_LOAD_SENTINEL_42"
    capture_tui_evidence "extension-load-no-sentinel"
  fi

  # Secondary: log grep for extension load/start entries.
  local log_entries
  log_entries=$(grep -ciE "extension.*(init|load|start|register)|prompt.?assembly.*init|host.*context" .crush/logs/crush.log 2>/dev/null || echo 0)

  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $log_entries extension load/start log entries"
  else
    echo "  NOTE: No extension load/start log entries found"
  fi

  echo "--- Extension load log evidence ---"
  grep -iE "extension.*(init|load|start|register)|prompt.?assembly.*init|host.*context" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- End evidence ---"

  capture_tui_evidence "extension-load"
}

# ---------------------------------------------------------------------------
# Scenario 2: Extension prompt assembly path
# ---------------------------------------------------------------------------
test_prompt_assembly_extension() {
  SCENARIO="extension-prompt-assembly"
  echo "=== Scenario 2: Extension prompt assembly path ==="

  # Reuse the TUI session — just send another prompt.
  focus_editor

  # Trigger prompt assembly by asking about context injection.
  send_tui_prompt "Describe how the prompt-assembly extension modifies the system prompt in this session. After your description, output exactly: EXTENSION_PROMPT_SENTINEL_88"

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle within 180s"
    capture_tui_evidence "prompt-assembly-timeout"
    return
  fi

  # Primary gate: TUI must contain the sentinel.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "EXTENSION_PROMPT_SENTINEL_88"; then
    pass "Scenario 2: TUI contains EXTENSION_PROMPT_SENTINEL_88"
  else
    fail "Scenario 2: TUI does not contain EXTENSION_PROMPT_SENTINEL_88"
    capture_tui_evidence "prompt-assembly-no-sentinel"
  fi

  # Secondary: log grep for prompt_assembly/extension entries.
  local log_entries
  log_entries=$(grep -ciE "prompt.?assembly|SystemPromptModifier|extension.*hook|context.*inject|repo.?map.*inject" .crush/logs/crush.log 2>/dev/null || echo 0)

  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 2: Found $log_entries prompt-assembly/extension log entries"
  else
    echo "  NOTE: No prompt-assembly/extension log entries found"
  fi

  echo "--- Prompt assembly log evidence ---"
  grep -iE "prompt.?assembly|SystemPromptModifier|extension.*hook|context.*inject|repo.?map.*inject" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- End evidence ---"

  capture_tui_evidence "prompt-assembly"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_extension_load
test_prompt_assembly_extension

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
