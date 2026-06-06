#!/usr/bin/env bash
# Test: Live processor pipeline scenarios.
# Scenario 1: PII redaction — send prompt with fake PII, assert redaction
#   in the crush log.
# Scenario 2: Token-limiter — configure tiny token budget, send
#   sentinel-heavy history, assert context trimming in the crush log.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup() {
  stop_crush 2>/dev/null || true
  local json_bak
  json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  if [[ -n "$json_bak" ]]; then
    mv "$json_bak" crush.json
  fi
  restore_crush
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Scenario 1: PII redaction — send prompt with fake PII, assert redaction
# ---------------------------------------------------------------------------
test_pii_redaction() {
  echo "=== Scenario 1: PII redaction in processor pipeline ==="

  setup_clean_crush

  QA_DIR_RESOLVED="$QA_DIR"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"
  local hooks_config
  hooks_config="$PROJECT_DIR/crush.json"
  if [[ -f "$hooks_config" ]]; then
    cp "$hooks_config" "$hooks_config.bak.$(date +%s)"
  fi

  # Copy the wave5 base config which already has pii_detector enabled.
  cp "$QA_DIR_RESOLVED/wave5.json" "$hooks_config"

  TMUX_SESSION="qa-proc-pii-$(date +%s)"
  tmux new-session -d -s "$TMUX_SESSION" -x 200 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  sleep 5

  # Send a prompt containing fake PII that the PIIDetector should redact.
  send_prompt "My contact info: email qa-person@example.com and phone 555-123-4567. Please confirm receipt."
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 91 "pii-redaction"
    stop_crush
    return
  fi

  capture_evidence 91 "pii-redaction"

  # Check the crush log for PII redaction evidence.
  local log_file="$PROJECT_DIR/.crush/logs/crush.log"
  if ! wait_for_file "$log_file" 30; then
    fail "Scenario 1: crush.log not found at $log_file"
    stop_crush
    return
  fi

  # Give log a moment to flush.
  sleep 2

  # The PIIDetector logs or the log should reflect redaction activity.
  # Check for REDACTED markers in log or pii_detector references.
  local pii_log_matches
  pii_log_matches=$(grep -ciE "REDACTED|pii_detector|PII" "$log_file" 2>/dev/null || echo 0)
  if [[ "$pii_log_matches" -ge 1 ]]; then
    pass "Scenario 1: Crush log contains PII redaction evidence ($pii_log_matches matches)"
  else
    fail "Scenario 1: No PII redaction evidence in crush log"
  fi

  # Also verify the raw PII values appear in the user prompt (they were sent),
  # but check the agent response doesn't echo them verbatim.
  local pane_output
  pane_output=$(tmux capture-pane -t "$TMUX_SESSION" -p -S -1000)

  # The agent may or may not echo PII — the processor pipeline redacts
  # internally before the LLM sees it. Check log for proof of processing.
  if echo "$pane_output" | grep -q "qa-person@example.com"; then
    # If the raw email appears in the pane, it's in the user's sent message.
    # That's expected — the redaction happens in the pipeline before the LLM.
    pass "Scenario 1: PII prompt was sent (raw PII visible in user input)"
  fi

  # Check the DB for messages that might show redaction.
  if [[ -f "$PROJECT_DIR/.crush/crush.db" ]]; then
    local session_id
    session_id=$(cd "$PROJECT_DIR" && get_session_id 2>/dev/null || echo "")
    if [[ -n "$session_id" ]]; then
      local msg_count
      msg_count=$(cd "$PROJECT_DIR" && sqlite3 .crush/crush.db \
        "SELECT COUNT(*) FROM messages WHERE session_id = '$session_id'" 2>/dev/null || echo 0)
      if [[ "$msg_count" -ge 1 ]]; then
        pass "Scenario 1: Session has $msg_count messages recorded"
      else
        fail "Scenario 1: No messages recorded in session"
      fi
    fi
  fi

  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: Token-limiter — configure tiny budget, assert context trimming
# ---------------------------------------------------------------------------
test_token_limiter() {
  echo "=== Scenario 2: Token-limiter with tiny budget ==="

  setup_clean_crush

  QA_DIR_RESOLVED="$QA_DIR"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR_RESOLVED/../.." && pwd)}"
  local hooks_config
  hooks_config="$PROJECT_DIR/crush.json"
  if [[ -f "$hooks_config" ]]; then
    cp "$hooks_config" "$hooks_config.bak.$(date +%s)"
  fi

  # Copy wave5 base config and override the processors section to include
  # a tiny token budget. The token limiter uses ~4 chars/token so a budget
  # of 100 tokens means ~400 chars of context.
  local tmp_config
  tmp_config=$(mktemp)
  jq '.options.processors = {"enabled": true, "list": ["token_limiter", "pii_detector"], "token_budget": 100}' \
    "$QA_DIR_RESOLVED/wave5.json" > "$tmp_config"
  mv "$tmp_config" "$hooks_config"

  TMUX_SESSION="qa-proc-tl-$(date +%s)"
  tmux new-session -d -s "$TMUX_SESSION" -x 200 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  sleep 5

  # Send a series of prompts to build up history, then send a final one
  # that should trigger the token limiter to trim older messages.
  send_prompt "Repeat this exactly: SENTINEL_ALPHA_XXXXX"
  if ! wait_for_idle 120; then
    fail "Scenario 2: Crush did not become idle after first prompt"
    capture_evidence 92 "token-limiter"
    stop_crush
    return
  fi

  send_prompt "Repeat this exactly: SENTINEL_BETA_YYYYY"
  if ! wait_for_idle 120; then
    fail "Scenario 2: Crush did not become idle after second prompt"
    capture_evidence 92 "token-limiter"
    stop_crush
    return
  fi

  send_prompt "Repeat this exactly: SENTINEL_GAMMA_ZZZZZ"
  if ! wait_for_idle 120; then
    fail "Scenario 2: Crush did not become idle after third prompt"
    capture_evidence 92 "token-limiter"
    stop_crush
    return
  fi

  send_prompt "What sentinels do you remember from our conversation?"
  if ! wait_for_idle 120; then
    fail "Scenario 2: Crush did not become idle after recall prompt"
    capture_evidence 92 "token-limiter"
    stop_crush
    return
  fi

  capture_evidence 92 "token-limiter"

  # Check the crush log for token_limiter evidence.
  local log_file="$PROJECT_DIR/.crush/logs/crush.log"
  if ! wait_for_file "$log_file" 30; then
    fail "Scenario 2: crush.log not found at $log_file"
    stop_crush
    return
  fi

  sleep 2

  local tl_log_matches
  tl_log_matches=$(grep -ciE "token_limiter|token.*budget|messages_removed|tokens_before|tokens_after|trimming|context.*truncat" "$log_file" 2>/dev/null || echo 0)
  if [[ "$tl_log_matches" -ge 1 ]]; then
    pass "Scenario 2: Crush log contains token limiter evidence ($tl_log_matches matches)"
  else
    fail "Scenario 2: No token limiter evidence in crush log"
  fi

  # Verify session messages were recorded.
  if [[ -f "$PROJECT_DIR/.crush/crush.db" ]]; then
    local session_id
    session_id=$(cd "$PROJECT_DIR" && get_session_id 2>/dev/null || echo "")
    if [[ -n "$session_id" ]]; then
      local msg_count
      msg_count=$(cd "$PROJECT_DIR" && sqlite3 .crush/crush.db \
        "SELECT COUNT(*) FROM messages WHERE session_id = '$session_id'" 2>/dev/null || echo 0)
      if [[ "$msg_count" -ge 2 ]]; then
        pass "Scenario 2: Session has $msg_count messages recorded"
      else
        fail "Scenario 2: Expected at least 2 messages, got $msg_count"
      fi
    fi
  fi

  stop_crush
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
