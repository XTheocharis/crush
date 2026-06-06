#!/usr/bin/env bash
# Test: PreCompact and PostCompact compaction hooks write marker files.
# Configures PreCompact and PostCompact hooks that write to marker files,
# then triggers compaction via a long conversation and verifies both markers.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

PRE_COMPACT_MARKER="/tmp/qa-pre-compact-marker.txt"
POST_COMPACT_MARKER="/tmp/qa-post-compact-marker.txt"
COMPACT_ENV="/tmp/qa-compact-env.txt"

cleanup() {
  stop_crush 2>/dev/null || true
  local json_bak
  json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  if [[ -n "$json_bak" ]]; then
    mv "$json_bak" crush.json
  fi
  restore_crush
  rm -f "$PRE_COMPACT_MARKER" "$POST_COMPACT_MARKER" "$COMPACT_ENV"
}
trap cleanup EXIT

test_compaction_hooks_fire() {
  echo "=== Scenario 1: PreCompact and PostCompact hooks both fire ==="

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
  jq '. + {"hooks":{"PreCompact":[{"matcher":"","command":"sh -c '\''echo PRE_COMPACT_FIRED > '"$PRE_COMPACT_MARKER"'; env | grep ^CRUSH_ > '"$COMPACT_ENV"'\''"}],"PostCompact":[{"matcher":"","command":"sh -c '\''echo POST_COMPACT_FIRED > '"$POST_COMPACT_MARKER"'\''"}]}}' \
    "$QA_DIR_RESOLVED/wave5.json" > "$tmp_config"
  mv "$tmp_config" "$hooks_config"

  rm -f "$PRE_COMPACT_MARKER" "$POST_COMPACT_MARKER" "$COMPACT_ENV"

  TMUX_SESSION="qa-compact-$(date +%s)"
  tmux new-session -d -s "$TMUX_SESSION" -x 200 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  sleep 5

  # Send a long prompt to build up context and trigger compaction.
  local long_prompt
  long_prompt="Please list all the Go source files in the internal/hooks directory, then list all files in internal/lcm, then list all files in internal/config. After that, summarize what each file does. Be verbose and thorough."
  send_prompt "$long_prompt"
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_evidence 11 "compaction-hooks"
    return
  fi

  # Send additional prompts to build more context and potentially trigger
  # compaction if the first round didn't push past the threshold.
  local i
  for i in 1 2 3; do
    send_prompt "Now list all files in internal/repomap, internal/treesitter, and internal/processor. Describe the purpose of each file in detail. Include the key types and functions exported."
    if ! wait_for_idle 120; then
      fail "Scenario 1: Crush did not become idle on iteration $i"
      capture_evidence 11 "compaction-hooks-iter-$i"
      break
    fi
  done

  # Check if PreCompact hook fired.
  if [[ -f "$PRE_COMPACT_MARKER" ]]; then
    local pre_content
    pre_content=$(cat "$PRE_COMPACT_MARKER")
    if [[ "$pre_content" == *"PRE_COMPACT_FIRED"* ]]; then
      pass "Scenario 1: PreCompact hook fired and wrote marker file"
    else
      fail "Scenario 1: PreCompact marker exists but content is '$pre_content'"
    fi
  else
    fail "Scenario 1: PreCompact hook marker file $PRE_COMPACT_MARKER not found (compaction may not have triggered)"
  fi

  # Check if PostCompact hook fired (should fire after PreCompact succeeds).
  if [[ -f "$POST_COMPACT_MARKER" ]]; then
    local post_content
    post_content=$(cat "$POST_COMPACT_MARKER")
    if [[ "$post_content" == *"POST_COMPACT_FIRED"* ]]; then
      pass "Scenario 1: PostCompact hook fired and wrote marker file"
    else
      fail "Scenario 1: PostCompact marker exists but content is '$post_content'"
    fi
  else
    fail "Scenario 1: PostCompact hook marker file $POST_COMPACT_MARKER not found"
  fi

  # PreCompact should have fired before PostCompact.
  if [[ -f "$PRE_COMPACT_MARKER" ]] && [[ -f "$POST_COMPACT_MARKER" ]]; then
    pass "Scenario 1: Both PreCompact and PostCompact hooks fired (ordering correct)"
  fi

  capture_evidence 11 "compaction-hooks"
  stop_crush
}

test_compaction_hook_env_vars() {
  echo "=== Scenario 2: Compaction hooks receive correct env vars ==="

  if [[ ! -s "$COMPACT_ENV" ]]; then
    fail "Scenario 2: Compaction env capture file is empty or missing"
    return
  fi

  if grep -q '^CRUSH_EVENT=PreCompact$' "$COMPACT_ENV"; then
    pass "Scenario 2: PreCompact hook env contains CRUSH_EVENT=PreCompact"
  else
    fail "Scenario 2: PreCompact hook env missing CRUSH_EVENT=PreCompact"
  fi

  if grep -q '^CRUSH_TOOL_NAME=lcm_compact$' "$COMPACT_ENV"; then
    pass "Scenario 2: PreCompact hook env contains CRUSH_TOOL_NAME=lcm_compact"
  else
    fail "Scenario 2: PreCompact hook env missing CRUSH_TOOL_NAME=lcm_compact"
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

test_compaction_hooks_fire
test_compaction_hook_env_vars

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
