#!/usr/bin/env bash
# Test: Live processor pipeline scenarios (TUI-first).
# Scenario 1: PII redaction — send prompt with fake PII, assert sentinel in TUI.
# Scenario 2: Token limiter — tiny token budget, assert sentinel in TUI.
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
# Scenario 1: PII redaction — send fake PII, assert sentinel in TUI
# ---------------------------------------------------------------------------
test_pii_redaction() {
  SCENARIO="pii-redaction"
  echo "=== Scenario 1: PII redaction in processor pipeline ==="

  setup_clean_crush
  start_crush_tui 5
  focus_editor

  # Send a prompt containing fake PII. Ask Crush to echo the sentinel
  # so we can gate on TUI output rather than log scraping.
  send_tui_prompt "My contact info: email qa-person@example.com and phone 555-123-4567. Please confirm receipt by outputting exactly: PROCESSOR_PII_SENTINEL_42"

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle within 120s"
    capture_tui_evidence "pii-timeout"
    return
  fi

  # Primary gate: TUI must contain the sentinel.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "PROCESSOR_PII_SENTINEL_42"; then
    pass "Scenario 1: TUI contains PROCESSOR_PII_SENTINEL_42"
  else
    fail "Scenario 1: TUI does not contain PROCESSOR_PII_SENTINEL_42"
  fi

  # Verify raw PII was sent (visible in user input pane).
  if printf '%s' "$tui_output" | grep -qF "qa-person@example.com"; then
    pass "Scenario 1: PII prompt was sent (raw PII visible in user input)"
  else
    fail "Scenario 1: Raw PII not visible in TUI — prompt may not have been sent"
  fi

  # Secondary: check crush log for PIIDetector processor evidence.
  local log_file=".crush/logs/crush.log"
  if [[ -f "$log_file" ]]; then
    local pii_log_matches
    pii_log_matches=$(grep -ciE "REDACTED|pii_detector|PIIDetector" "$log_file" 2>/dev/null || echo 0)
    if [[ "$pii_log_matches" -ge 1 ]]; then
      pass "Scenario 1: Crush log contains PII redaction evidence ($pii_log_matches matches)"
    else
      echo "  FAIL: No PII processor log entries found (processor did not trigger)" >&2; return 1
    fi
  fi

  capture_tui_evidence "pii-redaction"
}

# ---------------------------------------------------------------------------
# Scenario 2: Token limiter — tiny budget, assert sentinel in TUI
# ---------------------------------------------------------------------------
test_token_limiter() {
  SCENARIO="token-limiter"
  echo "=== Scenario 2: Token limiter with tiny budget ==="

  setup_clean_crush

  # We need to override the config to set a tiny token budget before
  # starting Crush. start_crush_tui calls generate_project_config, but
  # we need to patch the token_budget afterwards.
  QA_DIR_RESOLVED="$QA_DIR"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"

  # Generate the base wave5 config first, then patch the token budget.
  # start_crush_tui will handle backup/restore via generate_project_config.
  start_crush_tui 5

  # Now patch the running config with a tiny token budget to force trimming.
  local tmp_config
  tmp_config=$(mktemp)
  jq '.options.processors.token_budget = 100' "$PROJECT_DIR/crush.json" > "$tmp_config"
  mv "$tmp_config" "$PROJECT_DIR/crush.json"

  # Restart Crush with the patched config.
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
  TMUX_SESSION="qa-w5-tl-$(date +%s)"
  tmux new-session -d -s "$TMUX_SESSION" -x 160 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  sleep 5

  focus_editor

  # Build up context with sentinel-heavy prompts, then ask for the sentinel.
  send_tui_prompt "Repeat this exactly: SENTINEL_ALPHA_XXXXX"
  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle after first prompt"
    capture_tui_evidence "token-limiter-timeout-1"
    return
  fi

  send_tui_prompt "Repeat this exactly: SENTINEL_BETA_YYYYY"
  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle after second prompt"
    capture_tui_evidence "token-limiter-timeout-2"
    return
  fi

  send_tui_prompt "Repeat this exactly: SENTINEL_GAMMA_ZZZZZ"
  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle after third prompt"
    capture_tui_evidence "token-limiter-timeout-3"
    return
  fi

  # Final prompt asks Crush to output the sentinel so we can gate on TUI.
  send_tui_prompt "What sentinels do you remember from our conversation? Output exactly: PROCESSOR_TOKEN_SENTINEL_88"
  if ! wait_for_tui_idle 120; then
    fail "Scenario 2: Crush did not become idle after recall prompt"
    capture_tui_evidence "token-limiter-timeout-4"
    return
  fi

  # Primary gate: TUI must contain the sentinel.
  local tui_output
  tui_output=$(capture_tui | strip_ansi)

  if printf '%s' "$tui_output" | grep -qF "PROCESSOR_TOKEN_SENTINEL_88"; then
    pass "Scenario 2: TUI contains PROCESSOR_TOKEN_SENTINEL_88"
  else
    fail "Scenario 2: TUI does not contain PROCESSOR_TOKEN_SENTINEL_88"
  fi

  # Secondary: check crush log for TokenLimiter processor evidence.
  local log_file=".crush/logs/crush.log"
  if [[ -f "$log_file" ]]; then
    local tl_log_matches
    tl_log_matches=$(grep -ciE "token_limiter|TokenLimiter|token.*budget|messages_removed|tokens_before|tokens_after|trimming|context.*truncat" "$log_file" 2>/dev/null || echo 0)
    if [[ "$tl_log_matches" -ge 1 ]]; then
      pass "Scenario 2: Crush log contains token limiter evidence ($tl_log_matches matches)"
    else
      echo "  FAIL: No token limiter log entries found (limiter did not trigger)" >&2; return 1
    fi
  fi

  capture_tui_evidence "token-limiter"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_pii_redaction
test_token_limiter

echo ""
echo "Results: $PASS passed, $FAIL failed"
finish_test "test-processors-live" "$((FAIL == 0 ? 0 : 1))"
exit "$FAIL"
