#!/usr/bin/env bash
# Test: Tools surface listing and tool usage verification.
# Scenario 1: Ask Crush to list available tools; verify the TUI shows key tool
#   names and a deterministic sentinel.
# Scenario 2: Ask Crush to use specific tools (view, bash) deterministically;
#   verify the sentinel appears in TUI output and tool_call parts exist in DB.
set -euo pipefail

WAVE=5
SCENARIO="tools-surface-list"
source "$(dirname "$0")/../lib/common.sh"
source "$(dirname "$0")/../lib/db-verify.sh"

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
# Scenario 1: Tools surface — list available tools and verify key names
# ---------------------------------------------------------------------------
test_tools_surface_listing() {
  SCENARIO="tools-surface-list"
  echo "=== Scenario 1: Tools surface listing and verification ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Ask Crush to enumerate its tools and output a deterministic sentinel.
  send_tui_prompt "List all the tools you have available. Include at minimum: bash, view, edit, grep. After listing them, output exactly: TOOLS_SURFACE_SENTINEL_42"

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle within 180s"
    capture_tui_evidence "tools-surface-timeout"
    return
  fi

  # Primary: TUI must contain the sentinel.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "TOOLS_SURFACE_SENTINEL_42"; then
    pass "Scenario 1: TUI contains TOOLS_SURFACE_SENTINEL_42"
  else
    fail "Scenario 1: TUI does not contain TOOLS_SURFACE_SENTINEL_42"
    capture_tui_evidence "tools-surface-no-sentinel"
  fi

  # Primary (continued): TUI must contain at least two well-known tool names.
  local tools_found=0
  for tool_name in bash view edit grep; do
    if printf '%s' "$tui_output" | grep -qiE "(^|[^a-z_])${tool_name}([^a-z_]|$)"; then
      ((tools_found += 1))
    fi
  done

  if [[ "$tools_found" -ge 2 ]]; then
    pass "Scenario 1: TUI lists at least $tools_found well-known tool names"
  else
    fail "Scenario 1: TUI lists only $tools_found well-known tool names (expected >= 2)"
  fi

  # Secondary: log grep for tool surface / registry entries.
  local log_entries
  log_entries=$(grep -ciE "tool.?surface|tool.?registry|register.*tool|visible.*tool" .crush/logs/crush.log 2>/dev/null || echo 0)

  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $log_entries tool registry/surface log entries"
  else
    echo "  NOTE: No tool registry/surface log entries found"
  fi

  echo "--- Tools surface log evidence ---"
  grep -iE "tool.?surface|tool.?registry|register.*tool|visible.*tool" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- End evidence ---"

  capture_tui_evidence "tools-surface-listing"
}

# ---------------------------------------------------------------------------
# Scenario 2: Tool usage — exercise view and bash, verify via TUI and DB
# ---------------------------------------------------------------------------
test_tool_usage() {
  SCENARIO="tools-usage-exercise"
  echo "=== Scenario 2: Tool usage verification (view + bash) ==="

  # Reuse the Crush TUI session from scenario 1 (still running).
  focus_editor

  # Ask Crush to use specific tools deterministically with a sentinel output.
  send_tui_prompt "Do the following two things in order: (1) Use the view tool to read the file go.mod and tell me the module path. (2) Use the bash tool to run 'echo hello-tools-qa'. After both, output exactly: TOOLS_USAGE_SENTINEL_88"

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle within 180s"
    capture_tui_evidence "tools-usage-timeout"
    return
  fi

  # Primary: TUI must contain the sentinel.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "TOOLS_USAGE_SENTINEL_88"; then
    pass "Scenario 2: TUI contains TOOLS_USAGE_SENTINEL_88"
  else
    fail "Scenario 2: TUI does not contain TOOLS_USAGE_SENTINEL_88"
    capture_tui_evidence "tools-usage-no-sentinel"
  fi

  # Secondary: DB check for tool_call message parts.
  local db_path=".crush/crush.db"
  if [[ -f "$db_path" ]]; then
    local session_id
    session_id=$(sqlite3 "$db_path" "SELECT id FROM sessions ORDER BY created_at DESC LIMIT 1" 2>/dev/null || echo "")

    if [[ -n "$session_id" ]]; then
      local tool_call_count
      tool_call_count=$(sqlite3 "$db_path" \
        "SELECT COUNT(*) FROM message_parts WHERE session_id = '$session_id' AND part_type = 'tool_call'" \
        2>/dev/null || echo 0)

      if [[ "$tool_call_count" -ge 1 ]]; then
        pass "Scenario 2: DB has $tool_call_count tool_call message parts"
      else
        fail "Scenario 2: DB has no tool_call message parts (expected >= 1)"
      fi

      # Check for specific tool names within tool_call content_json.
      local bash_count view_count
      bash_count=$(sqlite3 "$db_path" \
        "SELECT COUNT(*) FROM message_parts WHERE session_id = '$session_id' AND part_type = 'tool_call' AND content_json LIKE '%bash%'" \
        2>/dev/null || echo 0)
      view_count=$(sqlite3 "$db_path" \
        "SELECT COUNT(*) FROM message_parts WHERE session_id = '$session_id' AND part_type = 'tool_call' AND content_json LIKE '%view%'" \
        2>/dev/null || echo 0)

      if [[ "$bash_count" -ge 1 ]]; then
        pass "Scenario 2: DB records bash tool invocation ($bash_count)"
      else
        echo "  NOTE: No bash tool_call found in DB (may be named differently)"
      fi

      if [[ "$view_count" -ge 1 ]]; then
        pass "Scenario 2: DB records view tool invocation ($view_count)"
      else
        echo "  NOTE: No view tool_call found in DB (may be named differently)"
      fi
    else
      fail "Scenario 2: Could not retrieve session ID from DB"
    fi
  else
    fail "Scenario 2: crush.db not found — cannot verify tool_call parts"
  fi

  # Secondary: log grep for tool execution entries.
  local exec_entries
  exec_entries=$(grep -ciE "tool.?call|tool.?exec|executing.*tool|ran.*tool" .crush/logs/crush.log 2>/dev/null || echo 0)

  if [[ "$exec_entries" -ge 1 ]]; then
    pass "Scenario 2: Found $exec_entries tool execution log entries"
  else
    echo "  NOTE: No tool execution log entries found"
  fi

  echo "--- Tool usage log evidence ---"
  grep -iE "tool.?call|tool.?exec|executing.*tool|ran.*tool" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- End evidence ---"

  capture_tui_evidence "tools-usage-exercise"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_tools_surface_listing
test_tool_usage

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
