#!/usr/bin/env bash
# Test: Shell enhancements — deterministic commands, env expansion, JSON
# processing, and background-job cancellation through TUI.
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
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
  local _bak
  _bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  [[ -n "$_bak" ]] && mv "$_bak" crush.json
  restore_crush
}
trap cleanup_test EXIT

# ---------------------------------------------------------------------------
# Scenario 1: Deterministic shell commands with environment expansion
# ---------------------------------------------------------------------------
# Sends a prompt asking Crush to run echo commands with QA-only env vars and
# jq filtering. TUI must show the deterministic sentinels. Secondary checks
# verify log evidence and that no real secret patterns leaked.
# ---------------------------------------------------------------------------
test_shell_env_expansion() {
  SCENARIO="shell-env-expansion"
  echo "=== Scenario 1: Deterministic shell commands with env expansion ==="

  # Export QA-only fake values for expansion testing.
  export QA_SHELL_TEST_VAR="expanded-qa-value-42"
  export QA_SHELL_FALLBACK_VAR=""

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Ask Crush to run deterministic shell commands that produce sentinels.
  send_tui_prompt "Run these exact shell commands in order and show all output: (1) echo SHELL_CMD_SENTINEL_42 (2) echo \$QA_SHELL_TEST_VAR (3) echo \${QA_SHELL_FALLBACK_VAR:-SHELL_ENV_SENTINEL_88} (4) echo '{\"active\":true,\"id\":1}' | jq '.id'"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle within 120s"
    capture_tui_evidence "env-expansion-timeout"
    return
  fi

  # Primary gate: TUI must contain both sentinels.
  if assert_tui_contains "SHELL_CMD_SENTINEL_42"; then
    pass "Scenario 1: TUI contains SHELL_CMD_SENTINEL_42"
  else
    fail "Scenario 1: TUI does not contain SHELL_CMD_SENTINEL_42"
  fi

  if assert_tui_contains "SHELL_ENV_SENTINEL_88"; then
    pass "Scenario 1: TUI contains SHELL_ENV_SENTINEL_88 (fallback expansion)"
  else
    fail "Scenario 1: TUI does not contain SHELL_ENV_SENTINEL_88"
  fi

  # Verify $VAR expansion resolved.
  if assert_tui_contains "expanded-qa-value-42"; then
    pass "Scenario 1: TUI contains expanded QA_SHELL_TEST_VAR"
  else
    fail "Scenario 1: TUI does not contain expanded QA_SHELL_TEST_VAR"
  fi

  # Verify jq output (id 1 from the JSON filter).
  local tui_output
  tui_output=$(capture_tui | strip_ansi)
  if printf '%s' "$tui_output" | grep -qE '(^|\s)1(\s|$)'; then
    pass "Scenario 1: jq filter produced correct output"
  else
    fail "Scenario 1: jq filter output not found in TUI"
  fi

  # Secondary: no real secret patterns leaked.
  local real_secret_count
  real_secret_count=$(printf '%s' "$tui_output" | grep -ciE "sk-[a-zA-Z0-9]{20,}|AKIA[A-Z0-9]{16}|-----BEGIN.*PRIVATE KEY" || echo 0)
  if [[ "$real_secret_count" -eq 0 ]]; then
    pass "Scenario 1: No real secrets leaked in output"
  else
    fail "Scenario 1: Real secret patterns found ($real_secret_count matches)"
  fi

  # Secondary: check crush log for shell tool usage evidence.
  local log_file=".crush/logs/crush.log"
  if [[ -f "$log_file" ]]; then
    local shell_log_matches
    shell_log_matches=$(grep -ciE "bash.*command|shell.*exec|tool.*bash|running.*command" "$log_file" 2>/dev/null || echo 0)
    if [[ "$shell_log_matches" -ge 1 ]]; then
      pass "Scenario 1: Crush log contains shell execution evidence ($shell_log_matches matches)"
    else
      echo "  NOTE: No shell execution log entries found"
    fi
  fi

  capture_tui_evidence "env-expansion"
}

# ---------------------------------------------------------------------------
# Scenario 2: Background job start and cancellation
# ---------------------------------------------------------------------------
# Asks Crush to start a long-running background process, then cancel it.
# TUI must show cancellation sentinel. Secondary process checks prove no
# background jobs remain after cancellation.
# ---------------------------------------------------------------------------
test_background_job_cancel() {
  SCENARIO="background-job-cancel"
  echo "=== Scenario 2: Background job start and cancellation ==="

  # Record baseline sleep process count before the test.
  local sleep_before
  sleep_before=$(pgrep -c "sleep" 2>/dev/null || echo 0)

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Ask Crush to start a long-running background process.
  send_tui_prompt "Run this exact command in the background: sleep 300 && echo done. Then confirm you started it by printing exactly: SHELL_BG_STARTED_55"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle after starting background job"
    capture_tui_evidence "bg-start-timeout"
    return
  fi

  # Verify background job was acknowledged.
  if assert_tui_contains "SHELL_BG_STARTED_55"; then
    pass "Scenario 2: TUI contains SHELL_BG_STARTED_55 (job started)"
  else
    fail "Scenario 2: TUI does not contain SHELL_BG_STARTED_55"
  fi

  capture_tui_evidence "bg-start"

  # Now ask Crush to cancel the background job.
  send_tui_prompt "Stop the background sleep process you just started. After confirming it is stopped, print exactly: SHELL_BG_CANCEL_55"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle after cancelling background job"
    capture_tui_evidence "bg-cancel-timeout"
    return
  fi

  # Primary gate: TUI must contain cancellation sentinel.
  if assert_tui_contains "SHELL_BG_CANCEL_55"; then
    pass "Scenario 2: TUI contains SHELL_BG_CANCEL_55 (job cancelled)"
  else
    fail "Scenario 2: TUI does not contain SHELL_BG_CANCEL_55"
  fi

  capture_tui_evidence "bg-cancel"

  # Secondary: verify no orphaned background sleep processes.
  sleep 2
  local sleep_after
  sleep_after=$(pgrep -c "sleep" 2>/dev/null || echo 0)
  local delta=$((sleep_after - sleep_before))
  if [[ "$delta" -le 0 ]]; then
    pass "Scenario 2: No orphaned background sleep processes (delta=$delta)"
  else
    fail "Scenario 2: Orphaned background process detected (delta=$delta)"
  fi

  # Secondary: check crush log for background job evidence.
  local log_file=".crush/logs/crush.log"
  if [[ -f "$log_file" ]]; then
    local bg_log_matches
    bg_log_matches=$(grep -ciE "background|signal|kill.*sleep|SIGTERM|SIGKILL|process.*stop" "$log_file" 2>/dev/null || echo 0)
    if [[ "$bg_log_matches" -ge 1 ]]; then
      pass "Scenario 2: Crush log contains background job evidence ($bg_log_matches matches)"
    else
      echo "  NOTE: No background job log entries found"
    fi
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_shell_env_expansion
test_background_job_cancel

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-shell-enhancements" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
