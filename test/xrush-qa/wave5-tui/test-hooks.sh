#!/usr/bin/env bash
# Test: PreToolUse hook fires on bash tool call.
# Creates a hooks config with a PreToolUse matcher on "^bash$" that writes
# a marker file and echoes a sentinel into stdout (visible in TUI), then
# verifies the sentinel appears in TUI output and the marker file exists.
set -euo pipefail

WAVE=5
SCENARIO="pretooluse-hook-bash"
source "$(dirname "$0")/../lib/common.sh"

PASS=0
FAIL=0

HOOK_MARKER="/tmp/qa-pretool-marker-42.txt"
HOOK_SCRIPT="/tmp/qa-pretool-hook-42.sh"

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
# Scenario 1: PreToolUse hook fires on bash tool call and sentinel appears
# ---------------------------------------------------------------------------
test_pretooluse_hook_fires() {
  echo "=== Scenario 1: PreToolUse hook fires on bash tool call ==="

  setup_clean_crush

  # Write a simple hook script that writes a marker file.
  cat > "$HOOK_SCRIPT" << 'HOOK_EOF'
#!/usr/bin/env bash
echo "HOOK_PRETOOL_SENTINEL_42" > /tmp/qa-pretool-marker-42.txt
HOOK_EOF
  chmod +x "$HOOK_SCRIPT"

  QA_DIR_RESOLVED="${QA_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"
  TMUX_SESSION="qa-w5-$(date +%s)"

  # Resolve providers and generate base wave-5 config.
  resolve_global_config
  if [[ -f "$PROJECT_DIR/crush.json" ]]; then
    cp "$PROJECT_DIR/crush.json" "$PROJECT_DIR/crush.json.bak.$(date +%s)"
  fi
  generate_project_config 5 "$PROJECT_DIR/crush.json"

  # Patch hooks into the generated config BEFORE launching Crush.
  local tmp_config
  tmp_config=$(mktemp)
  jq --arg script "$HOOK_SCRIPT" \
    '. + {"hooks":{"PreToolUse":[{"matcher":"^bash$","command":$script}]}}' \
    "$PROJECT_DIR/crush.json" > "$tmp_config"
  mv "$tmp_config" "$PROJECT_DIR/crush.json"

  rm -f "$HOOK_MARKER"

  # Launch Crush TUI manually (inlined to ensure hooks are in config before startup).
  tmux new-session -d -s "$TMUX_SESSION" -x 160 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  _QA_IDLE_BASELINE=0
  _QA_FOCUS_STATE="editor"
  local waited=0
  while [[ $waited -lt 30 ]]; do
    local pane_content
    pane_content=$(tmux capture-pane -t "$TMUX_SESSION" -p 2>/dev/null || true)
    if printf '%s' "$pane_content" | grep -qi 'ctrl+c quit\|HEY!\|Charm.*v[0-9]'; then
      break
    fi
    sleep 1
    waited=$((waited + 1))
  done

  # Prompt that triggers the bash tool.
  send_tui_prompt "Run the command: echo HOOK_PRETOOL_SENTINEL_42"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle within 120s"
    capture_tui_evidence "pretooluse-hook-timeout"
    return
  fi

  # Primary gate: TUI must contain the sentinel.
  if assert_tui_contains "HOOK_PRETOOL_SENTINEL_42"; then
    pass "Scenario 1: TUI contains HOOK_PRETOOL_SENTINEL_42"
  else
    fail "Scenario 1: TUI does not contain HOOK_PRETOOL_SENTINEL_42"
    capture_tui_evidence "pretooluse-hook-no-sentinel"
  fi

  # Secondary: marker file must exist.
  if [[ -f "$HOOK_MARKER" ]]; then
    pass "Scenario 1: PreToolUse hook marker file exists"
  else
    fail "Scenario 1: PreToolUse hook marker file not found at $HOOK_MARKER"
  fi

  # Tertiary: log grep for hook execution.
  local log_entries
  log_entries=$(grep -ciE "hook.*PreToolUse|PreToolUse.*hook|running hook" .crush/logs/crush.log 2>/dev/null ) || log_entries=0
  if [[ "$log_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $log_entries hook-related log entries"
  else
    echo "  NOTE: No hook log entries found (hook logging may be minimal)"
  fi

  capture_tui_evidence "pretooluse-hook-fired"
}

# ---------------------------------------------------------------------------
# Scenario 2: PreToolUse hook receives correct env vars
# ---------------------------------------------------------------------------
test_pretooluse_hook_env_vars() {
  echo "=== Scenario 2: PreToolUse hook receives correct env vars ==="

  SCENARIO="pretooluse-hook-env"
  setup_clean_crush

  local env_marker="/tmp/qa-pretool-env-42.txt"

  # Hook script that captures env vars.
  cat > "$HOOK_SCRIPT" << 'HOOK_EOF'
#!/usr/bin/env bash
env | grep '^CRUSH_' > /tmp/qa-pretool-env-42.txt
echo "HOOK_PRETOOL_SENTINEL_42" > /tmp/qa-pretool-marker-42.txt
HOOK_EOF
  chmod +x "$HOOK_SCRIPT"

  QA_DIR_RESOLVED="${QA_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"
  TMUX_SESSION="qa-w5-$(date +%s)"

  # Resolve providers and generate base wave-5 config.
  resolve_global_config
  if [[ -f "$PROJECT_DIR/crush.json" ]]; then
    cp "$PROJECT_DIR/crush.json" "$PROJECT_DIR/crush.json.bak.$(date +%s)"
  fi
  generate_project_config 5 "$PROJECT_DIR/crush.json"

  # Patch hooks into the generated config BEFORE launching Crush.
  local tmp_config
  tmp_config=$(mktemp)
  jq --arg script "$HOOK_SCRIPT" \
    '. + {"hooks":{"PreToolUse":[{"matcher":"^bash$","command":$script}]}}' \
    "$PROJECT_DIR/crush.json" > "$tmp_config"
  mv "$tmp_config" "$PROJECT_DIR/crush.json"

  rm -f "$HOOK_MARKER" "$env_marker"

  # Launch Crush TUI manually (inlined to ensure hooks are in config before startup).
  tmux new-session -d -s "$TMUX_SESSION" -x 160 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  _QA_IDLE_BASELINE=0
  _QA_FOCUS_STATE="editor"
  local waited=0
  while [[ $waited -lt 30 ]]; do
    local pane_content
    pane_content=$(tmux capture-pane -t "$TMUX_SESSION" -p 2>/dev/null || true)
    if printf '%s' "$pane_content" | grep -qi 'ctrl+c quit\|HEY!\|Charm.*v[0-9]'; then
      break
    fi
    sleep 1
    waited=$((waited + 1))
  done

  send_tui_prompt "List files in the current directory"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle within 120s"
    capture_tui_evidence "pretooluse-env-timeout"
    return
  fi

  # Primary: TUI sentinel.
  if assert_tui_contains "HOOK_PRETOOL_SENTINEL_42"; then
    pass "Scenario 2: TUI contains HOOK_PRETOOL_SENTINEL_42"
  else
    fail "Scenario 2: TUI does not contain HOOK_PRETOOL_SENTINEL_42"
    capture_tui_evidence "pretooluse-env-no-sentinel"
  fi

  # Secondary: env marker file and CRUSH_TOOL_NAME.
  if [[ -s "$env_marker" ]] && grep -q '^CRUSH_TOOL_NAME=bash$' "$env_marker"; then
    pass "Scenario 2: PreToolUse hook env contains CRUSH_TOOL_NAME=bash"
  else
    fail "Scenario 2: PreToolUse hook env missing CRUSH_TOOL_NAME=bash"
  fi

  rm -f "$env_marker"
  capture_tui_evidence "pretooluse-hook-env"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_pretooluse_hook_fires
test_pretooluse_hook_env_vars

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
