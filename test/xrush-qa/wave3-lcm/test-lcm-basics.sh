#!/usr/bin/env bash
# Test: LCM session configuration and context tracking basics (TUI-first).
# Verifies that Crush creates LCM session config rows, tracks context items,
# and that token counts grow across multi-turn conversations.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# Shared session ID across scenarios (set in Scenario 1, reused in 2 and 3).
SID=""
# Token count captured after Scenario 1, compared in Scenario 3.
TOKENS_AFTER_S1=0

# ---------------------------------------------------------------------------
# Scenario 1: LCM session config created on session start
# ---------------------------------------------------------------------------
test_lcm_session_config() {
  echo "=== Scenario 1: LCM session config created on session start ==="
  WAVE=3
  SCENARIO="lcm-session-config"

  setup_clean_crush
  cleanup_test() {
    cleanup_tui
    restore_crush
  }
  trap cleanup_test EXIT

  start_crush_tui 3
  focus_editor
  send_tui_prompt "Hello. Please reply with exactly the token LCM_BASIC_SENTINEL_42 and nothing else."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "LCM_BASIC_SENTINEL_42"; then
    pass "Scenario 1: TUI shows LCM_BASIC_SENTINEL_42 sentinel"
  else
    fail "Scenario 1: TUI does not show LCM_BASIC_SENTINEL_42 sentinel"
    capture_tui_evidence "sentinel-missing"
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary DB checks ---
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    return
  fi

  # Query lcm_session_config for this session.
  local config
  config=$(query_db "SELECT model_ctx_max_tokens, ctx_cutoff_threshold, soft_threshold_tokens, hard_threshold_tokens FROM lcm_session_config WHERE session_id = '$SID'")
  if [[ -z "$config" ]] || [[ "$config" == "[]" ]]; then
    fail "Scenario 1: No lcm_session_config row for session $SID"
    return
  fi

  # Assert: model_ctx_max_tokens > 0.
  local model_ctx_max_tokens
  model_ctx_max_tokens=$(echo "$config" | jq '.[0].model_ctx_max_tokens // 0')
  if [[ "$model_ctx_max_tokens" -gt 0 ]]; then
    pass "Scenario 1: model_ctx_max_tokens = $model_ctx_max_tokens (> 0)"
  else
    fail "Scenario 1: model_ctx_max_tokens is $model_ctx_max_tokens, expected > 0"
  fi

  # Assert: ctx_cutoff_threshold > 0 AND <= 1.
  local cutoff
  cutoff=$(echo "$config" | jq '.[0].ctx_cutoff_threshold // 0')
  local cutoff_ok=true
  if [[ "$cutoff" == "0" ]] || [[ "$cutoff" == "null" ]]; then
    cutoff_ok=false
  fi
  local above_one
  above_one=$(awk -v c="$cutoff" 'BEGIN { print (c > 1) ? 1 : 0 }')
  if [[ "$above_one" == "1" ]]; then
    cutoff_ok=false
  fi
  if [[ "$cutoff_ok" == "true" ]]; then
    pass "Scenario 1: ctx_cutoff_threshold = $cutoff (in (0, 1])"
  else
    fail "Scenario 1: ctx_cutoff_threshold = $cutoff, expected in (0, 1]"
  fi

  # Assert: soft_threshold_tokens > 0 AND hard_threshold_tokens > 0.
  local soft_tokens hard_tokens
  soft_tokens=$(echo "$config" | jq '.[0].soft_threshold_tokens // 0')
  hard_tokens=$(echo "$config" | jq '.[0].hard_threshold_tokens // 0')
  if [[ "$soft_tokens" -gt 0 ]]; then
    pass "Scenario 1: soft_threshold_tokens = $soft_tokens (> 0)"
  else
    fail "Scenario 1: soft_threshold_tokens is $soft_tokens, expected > 0"
  fi
  if [[ "$hard_tokens" -gt 0 ]]; then
    pass "Scenario 1: hard_threshold_tokens = $hard_tokens (> 0)"
  else
    fail "Scenario 1: hard_threshold_tokens is $hard_tokens, expected > 0"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: Context items track conversation tokens
# ---------------------------------------------------------------------------
test_context_items() {
  echo "=== Scenario 2: Context items track conversation tokens ==="
  WAVE=3
  SCENARIO="lcm-context-items"

  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID from Scenario 1"
    return
  fi

  # Query aggregate stats from lcm_context_items for message-type items.
  local stats
  stats=$(query_db "SELECT COUNT(*) as item_count, SUM(token_count) as total_tokens FROM lcm_context_items WHERE session_id = '$SID' AND item_type = 'message'")
  if [[ -z "$stats" ]] || [[ "$stats" == "[]" ]]; then
    fail "Scenario 2: No lcm_context_items rows for session $SID"
    return
  fi

  local item_count total_tokens
  item_count=$(echo "$stats" | jq '.[0].item_count // 0')
  total_tokens=$(echo "$stats" | jq '.[0].total_tokens // 0')

  # Assert: at least 2 items (user + assistant minimum).
  if [[ "$item_count" -ge 2 ]]; then
    pass "Scenario 2: item_count = $item_count (>= 2)"
  else
    fail "Scenario 2: item_count = $item_count, expected >= 2"
  fi

  # Assert: total tokens > 0.
  if [[ "$total_tokens" -gt 0 ]]; then
    pass "Scenario 2: total_tokens = $total_tokens (> 0)"
  else
    fail "Scenario 2: total_tokens = $total_tokens, expected > 0"
  fi

  # Save for comparison in Scenario 3.
  TOKENS_AFTER_S1=$total_tokens

  # Verify position values are valid integers (not null).
  local positions
  positions=$(query_db "SELECT position FROM lcm_context_items WHERE session_id = '$SID' AND item_type = 'message' ORDER BY position" | jq '.[].position')
  local pos_count=0
  local pos_ok=true
  while IFS= read -r pos; do
    if [[ "$pos" == "null" ]] || [[ -z "$pos" ]]; then
      pos_ok=false
      fail "Scenario 2: Position is null at index $pos_count"
    fi
    pos_count=$((pos_count + 1))
  done <<< "$positions"
  if [[ "$pos_ok" == "true" ]] && [[ "$pos_count" -gt 0 ]]; then
    pass "Scenario 2: Positions are valid integers ($pos_count items)"
  elif [[ "$pos_count" -eq 0 ]]; then
    fail "Scenario 2: No position values found"
  fi

  capture_tui_evidence "context-items"
}

# ---------------------------------------------------------------------------
# Scenario 3: Token counts increase with multi-turn conversation
# ---------------------------------------------------------------------------
test_token_growth() {
  echo "=== Scenario 3: Token counts increase with multi-turn conversation ==="
  WAVE=3
  SCENARIO="lcm-token-growth"

  if [[ -z "$SID" ]]; then
    fail "Scenario 3: No session ID from Scenario 1"
    return
  fi

  # Send a follow-up prompt in the same session.
  focus_editor
  send_tui_prompt "Tell me about Go programming. Somewhere in your reply include the exact token LCM_GROWTH_SENTINEL_88."

  if ! wait_for_tui_idle 120; then
    fail "Scenario 3: Crush did not become idle on second turn"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI output must contain the sentinel.
  if assert_tui_contains "LCM_GROWTH_SENTINEL_88"; then
    pass "Scenario 3: TUI shows LCM_GROWTH_SENTINEL_88 sentinel"
  else
    fail "Scenario 3: TUI does not show LCM_GROWTH_SENTINEL_88 sentinel"
    capture_tui_evidence "sentinel-missing"
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary DB checks ---
  local new_stats
  new_stats=$(query_db "SELECT COUNT(*) as item_count, SUM(token_count) as total_tokens FROM lcm_context_items WHERE session_id = '$SID' AND item_type = 'message'")
  local new_total
  new_total=$(echo "$new_stats" | jq '.[0].total_tokens // 0')

  # Assert: tokens now higher than after Scenario 1.
  if [[ "$new_total" -gt "$TOKENS_AFTER_S1" ]]; then
    pass "Scenario 3: Tokens grew from $TOKENS_AFTER_S1 to $new_total"
  else
    fail "Scenario 3: Tokens did not grow ($TOKENS_AFTER_S1 -> $new_total)"
  fi

  # Kill tmux — this is the last scenario.
  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 1
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_lcm_session_config
test_context_items
test_token_growth

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
