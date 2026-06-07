#!/usr/bin/env bash
# Test: TUI streaming response and compact mode (TUI-first).
# Scenario 1: Streaming output — prompt with sentinel, assert visible in TUI pane.
# Scenario 2: Compact mode — launch with compact_mode=true, assert sentinel in TUI.
set -euo pipefail

WAVE=5

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

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
# Scenario 1: Streaming output visible in TUI
# ---------------------------------------------------------------------------
test_streaming_output() {
  SCENARIO="streaming-output"
  echo "=== Scenario 1: Streaming output visible in TUI ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Prompt asks the LLM to echo a deterministic sentinel so the TUI pane
  # assertion is the primary gate.
  send_tui_prompt "Say exactly this and nothing else: TUI_STREAM_SENTINEL_42"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle within 120s"
    capture_tui_evidence "stream-timeout"
    return
  fi

  # Primary gate: TUI pane must show the sentinel.
  if assert_tui_contains "TUI_STREAM_SENTINEL_42"; then
    pass "Scenario 1: TUI contains TUI_STREAM_SENTINEL_42"
  else
    fail "Scenario 1: TUI does not contain TUI_STREAM_SENTINEL_42"
  fi

  capture_tui_evidence "stream-output"

  # Secondary: DB must have at least 2 messages (user + assistant).
  local db_path=".crush/crush.db"
  if [[ -f "$db_path" ]]; then
    local sid
    sid=$(sqlite3 "$db_path" "SELECT id FROM sessions ORDER BY created_at DESC LIMIT 1" 2>/dev/null || true)
    if [[ -n "$sid" ]]; then
      local msg_count
      msg_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM messages WHERE session_id = '$sid'" 2>/dev/null || echo 0)
      if [[ "$msg_count" -ge 2 ]]; then
        pass "Scenario 1: DB has >= 2 messages (user + assistant)"
      else
        fail "Scenario 1: DB has $msg_count messages, expected >= 2"
      fi
    else
      fail "Scenario 1: No session found in DB"
    fi
  else
    echo "  NOTE: DB not found, skipping secondary DB check"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: Compact mode toggle — launch with compact_mode=true
# ---------------------------------------------------------------------------
test_compact_mode() {
  SCENARIO="compact-mode"
  echo "=== Scenario 2: Compact mode rendering in TUI ==="

  setup_clean_crush

  QA_DIR_RESOLVED="$QA_DIR"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"

  # start_crush_tui generates the base wave5 config. We patch compact_mode
  # onto it before the TUI launches.
  start_crush_tui 5

  # Patch the running config with compact_mode enabled.
  local tmp_config
  tmp_config=$(mktemp)
  jq '.options.tui.compact_mode = true' "$PROJECT_DIR/crush.json" > "$tmp_config"
  mv "$tmp_config" "$PROJECT_DIR/crush.json"

  # Restart with compact config.
    cleanup_tui
  TMUX_SESSION="qa-w5-compact-$(date +%s)"
  tmux new-session -d -s "$TMUX_SESSION" -x 160 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  sleep 5

  focus_editor

  send_tui_prompt "Say exactly this and nothing else: TUI_COMPACT_SENTINEL_88"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle in compact mode within 120s"
    capture_tui_evidence "compact-timeout"
    return
  fi

  # Primary gate: TUI pane must show the sentinel.
  if assert_tui_contains "TUI_COMPACT_SENTINEL_88"; then
    pass "Scenario 2: TUI contains TUI_COMPACT_SENTINEL_88"
  else
    fail "Scenario 2: TUI does not contain TUI_COMPACT_SENTINEL_88"
  fi

  capture_tui_evidence "compact-mode"

  # Secondary: DB check for assistant response.
  local db_path=".crush/crush.db"
  if [[ -f "$db_path" ]]; then
    local sid
    sid=$(sqlite3 "$db_path" "SELECT id FROM sessions ORDER BY created_at DESC LIMIT 1" 2>/dev/null || true)
    if [[ -n "$sid" ]]; then
      local msg_count
      msg_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM messages WHERE session_id = '$sid'" 2>/dev/null || echo 0)
      if [[ "$msg_count" -ge 2 ]]; then
        pass "Scenario 2: DB has >= 2 messages in compact mode session"
      else
        fail "Scenario 2: DB has $msg_count messages, expected >= 2"
      fi
    else
      fail "Scenario 2: No session found in DB"
    fi
  else
    echo "  NOTE: DB not found, skipping secondary DB check"
  fi
}

# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------
test_streaming_output
test_compact_mode

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-tui-streaming" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
