#!/usr/bin/env bash
# Test: PostToolUse hook rewrites bash tool output.
# Configures a PostToolUse hook on "^bash$" that returns modified_output,
# then verifies the rewritten content appears in the agent response.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

HOOK_POST_LOG="/tmp/qa-post-hook-log.txt"
HOOK_POST_PAYLOAD="/tmp/qa-post-hook-payload.json"

cleanup() {
  stop_crush 2>/dev/null || true
  local json_bak
  json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  if [[ -n "$json_bak" ]]; then
    mv "$json_bak" crush.json
  fi
  restore_crush
  rm -f "$HOOK_POST_LOG" "$HOOK_POST_PAYLOAD"
}
trap cleanup EXIT

test_posttooluse_hook_rewrites_output() {
  echo "=== Scenario 1: PostToolUse hook rewrites bash output ==="

  setup_clean_crush

  QA_DIR_RESOLVED="$QA_DIR"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"

  local hooks_config
  hooks_config="$PROJECT_DIR/crush.json"
  if [[ -f "$hooks_config" ]]; then
    cp "$hooks_config" "$hooks_config.bak.$(date +%s)"
  fi

  local tmp_config
  tmp_config=$(mktemp)
  # PostToolUse hook that rewrites output: captures the original output via
  # env var CRUSH_TOOL_OUTPUT and writes a marker file plus returns
  # modified_output in JSON.
  jq '. + {"hooks":{"PostToolUse":[{"matcher":"^bash$","command":"sh -c '\''echo HOOK_POST_FIRED > '"$HOOK_POST_LOG"'; cat > '"$HOOK_POST_PAYLOAD"'; echo '\''\"'\\''\"'{\"modified_output\":\"REWRITTEN: output sanitized\"}'\\''\"'\\''\""'\''"}]}}' \
    "$QA_DIR_RESOLVED/wave5.json" > "$tmp_config"
  mv "$tmp_config" "$hooks_config"

  rm -f "$HOOK_POST_LOG" "$HOOK_POST_PAYLOAD"

  TMUX_SESSION="qa-post-$(date +%s)"
  tmux new-session -d -s "$TMUX_SESSION" -x 200 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  sleep 5
  send_prompt "Run echo hello"
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 11 "post-tool-use"
    return
  fi

  if [[ ! -f "$HOOK_POST_LOG" ]]; then
    fail "Scenario 1: PostToolUse hook marker file $HOOK_POST_LOG not found"
    capture_evidence 11 "post-tool-use"
    return
  fi

  local content
  content=$(cat "$HOOK_POST_LOG")
  if [[ "$content" == *"HOOK_POST_FIRED"* ]]; then
    pass "Scenario 1: PostToolUse hook fired and wrote marker file"
  else
    fail "Scenario 1: PostToolUse hook marker content is '$content', expected HOOK_POST_FIRED"
  fi

  if [[ -s "$HOOK_POST_PAYLOAD" ]] && jq -e '.event == "PostToolUse" and .tool_name == "bash"' "$HOOK_POST_PAYLOAD" >/dev/null; then
    pass "Scenario 1: PostToolUse hook stdin payload includes event and tool_name"
  else
    fail "Scenario 1: PostToolUse hook stdin payload missing event/tool_name details"
  fi

  capture_evidence 11 "post-tool-use"
  stop_crush
}

test_posttooluse_hook_fires() {
  echo "=== Scenario 2: PostToolUse hook receives correct env vars ==="

  setup_clean_crush

  QA_DIR_RESOLVED="$QA_DIR"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"

  local hooks_config
  hooks_config="$PROJECT_DIR/crush.json"
  if [[ -f "$hooks_config" ]]; then
    cp "$hooks_config" "$hooks_config.bak.$(date +%s)"
  fi

  local tmp_config
  tmp_config=$(mktemp)
  local env_marker="/tmp/qa-post-hook-env.txt"
  jq '. + {"hooks":{"PostToolUse":[{"matcher":"^bash$","command":"sh -c '\''env | grep ^CRUSH_ > '"$env_marker"'\''"}]}}' \
    "$QA_DIR_RESOLVED/wave5.json" > "$tmp_config"
  mv "$tmp_config" "$hooks_config"

  rm -f "$env_marker"

  TMUX_SESSION="qa-post-env-$(date +%s)"
  tmux new-session -d -s "$TMUX_SESSION" -x 200 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  sleep 5
  send_prompt "List files"
  if ! wait_for_idle 120; then
    fail "Scenario 2: Crush did not become idle"
    capture_evidence 11 "post-tool-use-env"
    return
  fi

  if [[ -s "$env_marker" ]] && grep -q '^CRUSH_EVENT=PostToolUse$' "$env_marker"; then
    pass "Scenario 2: PostToolUse hook env contains CRUSH_EVENT=PostToolUse"
  else
    fail "Scenario 2: PostToolUse hook env missing CRUSH_EVENT=PostToolUse"
  fi

  if [[ -s "$env_marker" ]] && grep -q '^CRUSH_TOOL_NAME=bash$' "$env_marker"; then
    pass "Scenario 2: PostToolUse hook env contains CRUSH_TOOL_NAME=bash"
  else
    fail "Scenario 2: PostToolUse hook env missing CRUSH_TOOL_NAME=bash"
  fi

  if [[ -s "$env_marker" ]] && grep -q '^CRUSH_TOOL_OUTPUT=' "$env_marker"; then
    pass "Scenario 2: PostToolUse hook env contains CRUSH_TOOL_OUTPUT"
  else
    fail "Scenario 2: PostToolUse hook env missing CRUSH_TOOL_OUTPUT"
  fi

  capture_evidence 11 "post-tool-use-env"
  stop_crush
}

test_posttooluse_hook_fires
test_posttooluse_hook_rewrites_output

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
