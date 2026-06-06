#!/usr/bin/env bash
# Test: Hooks engine fires PreToolUse hook on bash tool call.
# Creates a hooks config with a PreToolUse matcher on "^bash$" that writes
# a marker file, then verifies the marker exists after a bash-triggering prompt.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

HOOK_LOG="/tmp/qa-hook-log.txt"
HOOK_PAYLOAD="/tmp/qa-hook-payload.json"
HOOK_ENV="/tmp/qa-hook-env.txt"

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
cleanup() {
  stop_crush 2>/dev/null || true
  # Restore crush.json if backed up by start_crush.
  local json_bak
  json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  if [[ -n "$json_bak" ]]; then
    mv "$json_bak" crush.json
  fi
  restore_crush
  rm -f "$HOOK_LOG" "$HOOK_PAYLOAD" "$HOOK_ENV"
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Scenario 1: PreToolUse hook fires on bash tool call
# ---------------------------------------------------------------------------
test_pretooluse_hook_fires() {
  echo "=== Scenario 1: PreToolUse hook fires on bash tool call ==="

  setup_clean_crush

  # Build a hooks config by extending the wave5 base with a PreToolUse hook.
  # The hook writes "HOOK_FIRED" to the marker file whenever the bash tool is
  # invoked.
  QA_DIR_RESOLVED="$QA_DIR"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"

  local hooks_config
  hooks_config="$PROJECT_DIR/crush.json"
  if [[ -f "$hooks_config" ]]; then
    cp "$hooks_config" "$hooks_config.bak.$(date +%s)"
  fi

  # Copy the wave5 base config and inject the hooks section.
  local tmp_config
  tmp_config=$(mktemp)
  jq '. + {"hooks":{"PreToolUse":[{"matcher":"^bash$","command":"sh -c '\''echo HOOK_FIRED > '"$HOOK_LOG"'; cat > '"$HOOK_PAYLOAD"'; env | grep '^CRUSH_TOOL_' > '"$HOOK_ENV"''\''"}]}}' \
    "$QA_DIR_RESOLVED/wave5.json" > "$tmp_config"
  mv "$tmp_config" "$hooks_config"

  # Clear any stale marker from a previous run.
  rm -f "$HOOK_LOG" "$HOOK_PAYLOAD" "$HOOK_ENV"

  TMUX_SESSION="qa-hooks-$(date +%s)"
  tmux new-session -d -s "$TMUX_SESSION" -x 200 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  sleep 5
  send_prompt "List files in the current directory"
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 51 "hooks-fired"
    return
  fi

  # Assert: the hook marker file was created and contains HOOK_FIRED.
  if [[ ! -f "$HOOK_LOG" ]]; then
    fail "Scenario 1: Hook marker file $HOOK_LOG not found"
    capture_evidence 51 "hooks-fired"
    return
  fi

  local content
  content=$(cat "$HOOK_LOG")
  if [[ "$content" == *"HOOK_FIRED"* ]]; then
    pass "Scenario 1: PreToolUse hook fired and wrote HOOK_FIRED to marker file"
  else
    fail "Scenario 1: Hook marker file exists but content is '$content', expected HOOK_FIRED"
  fi

  if [[ -s "$HOOK_PAYLOAD" ]] && jq -e '.event == "PreToolUse" and .tool_name == "bash" and (.tool_input.command | type == "string")' "$HOOK_PAYLOAD" >/dev/null; then
    pass "Scenario 1: Hook stdin payload includes event, bash tool name, and command input"
  else
    fail "Scenario 1: Hook stdin payload missing event/tool/input details"
  fi

  if [[ -s "$HOOK_ENV" ]] && grep -q '^CRUSH_TOOL_NAME=bash$' "$HOOK_ENV"; then
    pass "Scenario 1: Hook environment includes CRUSH_TOOL_NAME=bash"
  else
    fail "Scenario 1: Hook environment missing CRUSH_TOOL_NAME=bash"
  fi

  capture_evidence 51 "hooks-fired"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_pretooluse_hook_fires

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
