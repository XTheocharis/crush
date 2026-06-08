#!/usr/bin/env bash
# Test: Compaction hooks (PreCompact/PostCompact) or lifecycle Stop hook.
# Tests compaction hooks by triggering compaction through a long conversation.
# If compaction hooks are not available, falls back to testing a Stop lifecycle
# hook. Verifies sentinel in TUI, marker files, and log entries.
set -euo pipefail

WAVE=5
SCENARIO="compaction-hooks-fire"
source "$(dirname "$0")/../lib/common.sh"

PASS=0
FAIL=0

PRE_COMPACT_MARKER="/tmp/qa-pre-compact-marker-55.txt"
POST_COMPACT_MARKER="/tmp/qa-post-compact-marker-55.txt"
COMPACT_ENV="/tmp/qa-compact-env-55.txt"
STOP_MARKER="/tmp/qa-stop-marker-55.txt"
PRE_COMPACT_SCRIPT="/tmp/qa-pre-compact-hook-55.sh"
POST_COMPACT_SCRIPT="/tmp/qa-post-compact-hook-55.sh"
STOP_SCRIPT="/tmp/qa-stop-hook-55.sh"

cleanup_test() {
    cleanup_tui
  local _bak
  _bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  [[ -n "$_bak" ]] && mv "$_bak" crush.json
  restore_crush
  rm -f "$PRE_COMPACT_MARKER" "$POST_COMPACT_MARKER" "$COMPACT_ENV" "$STOP_MARKER" \
    "$PRE_COMPACT_SCRIPT" "$POST_COMPACT_SCRIPT" "$STOP_SCRIPT"
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Compaction hooks fire during long session
# ---------------------------------------------------------------------------
test_compaction_hooks_fire() {
  echo "=== Scenario 1: Compaction hooks fire during long session ==="

  setup_clean_crush

  # Create hook scripts for PreCompact and PostCompact.
  cat > "$PRE_COMPACT_SCRIPT" << 'HOOK_EOF'
#!/usr/bin/env bash
echo "HOOK_COMPACTION_SENTINEL_55" > /tmp/qa-pre-compact-marker-55.txt
env | grep '^CRUSH_' > /tmp/qa-compact-env-55.txt
HOOK_EOF
  chmod +x "$PRE_COMPACT_SCRIPT"

  cat > "$POST_COMPACT_SCRIPT" << 'HOOK_EOF'
#!/usr/bin/env bash
echo "HOOK_COMPACTION_SENTINEL_55" > /tmp/qa-post-compact-marker-55.txt
HOOK_EOF
  chmod +x "$POST_COMPACT_SCRIPT"

  # Also create a Stop hook as fallback verification.
  cat > "$STOP_SCRIPT" << 'HOOK_EOF'
#!/usr/bin/env bash
echo "HOOK_COMPACTION_SENTINEL_55" > /tmp/qa-stop-marker-55.txt
HOOK_EOF
  chmod +x "$STOP_SCRIPT"

  QA_DIR_RESOLVED="${QA_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"

  local hooks_config="$PROJECT_DIR/crush.json"
  if [[ -f "$hooks_config" ]]; then
    cp "$hooks_config" "$hooks_config.bak.$(date +%s)"
  fi

  local tmp_config
  tmp_config=$(mktemp)
  jq --arg pre "$PRE_COMPACT_SCRIPT" --arg post "$POST_COMPACT_SCRIPT" --arg stop "$STOP_SCRIPT" \
    '. + {"hooks":{"PreCompact":[{"matcher":"","command":$pre}],"PostCompact":[{"matcher":"","command":$post}],"Stop":[{"matcher":"","command":$stop}]}}' \
    "$QA_DIR_RESOLVED/wave5.json" > "$tmp_config"
  mv "$tmp_config" "$hooks_config"

  # Verify hooks config is present in generated config.
  if jq -e '.hooks.PreCompact' "$hooks_config" >/dev/null 2>&1; then
    pass "Scenario 1: Hooks config present in crush.json"
  else
    fail "Scenario 1: Hooks config missing from crush.json"
  fi

  rm -f "$PRE_COMPACT_MARKER" "$POST_COMPACT_MARKER" "$COMPACT_ENV" "$STOP_MARKER"

  start_crush_tui 5
  focus_editor

  # Send a long prompt to build up context and trigger compaction.
  send_tui_prompt "Please list all the Go source files in the internal/hooks directory, then list all files in internal/lcm, then list all files in internal/config. After that, summarize what each file does. Be verbose and thorough. Output HOOK_COMPACTION_SENTINEL_55 when done."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_tui_evidence "compaction-hooks-timeout"
    return
  fi

  # Send additional prompts to build more context and potentially trigger
  # compaction if the first round did not push past the threshold.
  local i
  for i in 1 2 3; do
    send_tui_prompt "Now list all files in internal/repomap, internal/treesitter, and internal/processor. Describe the purpose of each file in detail. Include the key types and functions exported. Output HOOK_COMPACTION_SENTINEL_55 when done."
    if ! wait_for_tui_idle 180; then
      fail "Scenario 1: Crush did not become idle on iteration $i"
      capture_tui_evidence "compaction-hooks-iter-$i"
      break
    fi
  done

  # Primary gate: TUI must contain the sentinel (from prompts or hook output).
  if assert_tui_contains "HOOK_COMPACTION_SENTINEL_55"; then
    pass "Scenario 1: TUI contains HOOK_COMPACTION_SENTINEL_55"
  else
    fail "Scenario 1: TUI does not contain HOOK_COMPACTION_SENTINEL_55"
    capture_tui_evidence "compaction-hooks-no-sentinel"
  fi

  # Secondary: PreCompact marker file (polling with timeout).
  if wait_for_file "$PRE_COMPACT_MARKER" 30; then
    pass "Scenario 1: PreCompact hook marker file exists (compaction triggered)"
  else
    fail "Scenario 1: PreCompact marker not found (compaction did not trigger)"
  fi

  # Secondary: PostCompact marker file.
  if wait_for_file "$POST_COMPACT_MARKER" 10; then
    pass "Scenario 1: PostCompact hook marker file exists"
  else
    echo "  NOTE: PostCompact marker not found"
  fi

  # Both markers means correct ordering.
  if [[ -f "$PRE_COMPACT_MARKER" ]] && [[ -f "$POST_COMPACT_MARKER" ]]; then
    pass "Scenario 1: Both PreCompact and PostCompact hooks fired"
  fi

  # Tertiary: log grep for compaction-related entries.
  local log_entries
  log_entries=$(grep -ciE "compact|compaction|PreCompact|PostCompact" .crush/logs/crush.log 2>/dev/null ) || log_entries=0
  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $log_entries compaction-related log entries"
  else
    echo "  NOTE: No compaction log entries found"
  fi

  capture_tui_evidence "compaction-hooks-fired"
}

# ---------------------------------------------------------------------------
# Scenario 2: Compaction hooks receive correct env vars
# ---------------------------------------------------------------------------
test_compaction_hook_env_vars() {
  echo "=== Scenario 2: Compaction hooks receive correct env vars ==="

  SCENARIO="compaction-hooks-env"

  if [[ ! -s "$COMPACT_ENV" ]]; then
    fail "Scenario 2: Compaction env capture file is empty or missing (compaction did not trigger)"
    if [[ -f "$STOP_MARKER" ]]; then
      pass "Scenario 2 (fallback): Stop hook marker file exists"
    fi
    return
  fi

  if grep -q '^CRUSH_EVENT=PreCompact$' "$COMPACT_ENV"; then
    pass "Scenario 2: PreCompact hook env contains CRUSH_EVENT=PreCompact"
  else
    fail "Scenario 2: PreCompact hook env missing CRUSH_EVENT=PreCompact"
  fi

  if grep -q '^CRUSH_SESSION_ID=.' "$COMPACT_ENV"; then
    pass "Scenario 2: PreCompact hook env contains non-empty CRUSH_SESSION_ID"
  else
    fail "Scenario 2: PreCompact hook env missing CRUSH_SESSION_ID"
  fi

  if grep -q '^CRUSH_CWD=.' "$COMPACT_ENV"; then
    pass "Scenario 2: PreCompact hook env contains non-empty CRUSH_CWD"
  else
    fail "Scenario 2: PreCompact hook env missing CRUSH_CWD"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_compaction_hooks_fire
test_compaction_hook_env_vars

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
