#!/usr/bin/env bash
# Test: LCM compaction triggers when context exceeds lowered threshold (TUI-first).
# Verifies that summaries are created and context items reference them
# after multi-turn conversations drive past the cutoff.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

export LCM_LOW_THRESHOLD=1

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

SID=""

# ---------------------------------------------------------------------------
# Scenario 1: Compaction triggered by multi-turn conversation
# ---------------------------------------------------------------------------
test_compaction_triggered() {
  echo "=== Scenario 1: Compaction triggered by multi-turn conversation ==="
  WAVE=3
  SCENARIO="compaction-triggered"

  setup_clean_crush
  cleanup_test() {
    cleanup_tui
    restore_crush
  }
  trap cleanup_test EXIT

  start_crush_tui 3
  focus_editor
  send_tui_prompt "Explain every Go file in internal/lcm/ in detail, one by one. For each file, describe its purpose, key functions, and how it interacts with other parts of the LCM system. Somewhere in your reply include the exact token LCM_COMPACTION_SENTINEL_01."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_tui_evidence "idle-timeout-p1"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "LCM_COMPACTION_SENTINEL_01"; then
    pass "Scenario 1: TUI shows LCM_COMPACTION_SENTINEL_01 sentinel"
  else
    fail "Scenario 1: TUI does not show LCM_COMPACTION_SENTINEL_01 sentinel"
    capture_tui_evidence "sentinel-missing-p1"
    return
  fi

  focus_editor
  send_tui_prompt "Now describe the compaction pipeline in detail. Explain each of the 9 compaction layers, their priorities, and when each one triggers. Include code examples where relevant. Somewhere in your reply include the exact token LCM_COMPACTION_SENTINEL_02."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after second prompt"
    capture_tui_evidence "idle-timeout-p2"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "LCM_COMPACTION_SENTINEL_02"; then
    pass "Scenario 1: TUI shows LCM_COMPACTION_SENTINEL_02 sentinel"
  else
    fail "Scenario 1: TUI does not show LCM_COMPACTION_SENTINEL_02 sentinel"
    capture_tui_evidence "sentinel-missing-p2"
    return
  fi

  focus_editor
  send_tui_prompt "List all the LCM agent tools and explain how each one works, including their parameters and return types. Also explain how the LCM explorer subsystem works. Somewhere in your reply include the exact token LCM_COMPACTION_SENTINEL_03."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after third prompt"
    capture_tui_evidence "idle-timeout-p3"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "LCM_COMPACTION_SENTINEL_03"; then
    pass "Scenario 1: TUI shows LCM_COMPACTION_SENTINEL_03 sentinel"
  else
    fail "Scenario 1: TUI does not show LCM_COMPACTION_SENTINEL_03 sentinel"
    capture_tui_evidence "sentinel-missing-p3"
    return
  fi

  capture_tui_evidence "tui-response"

  # --- Secondary DB checks ---
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    return
  fi

  # Assert: lcm_summaries has at least 1 row for this session.
  local summary_count
  summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$summary_count" -ge 1 ]]; then
    pass "Scenario 1: lcm_summaries has $summary_count rows (>= 1)"
  else
    fail "Scenario 1: lcm_summaries has $summary_count rows, expected >= 1"
  fi

  # Assert: summary kind values are valid.
  local kinds
  kinds=$(query_db "SELECT DISTINCT kind FROM lcm_summaries WHERE session_id = '$SID'" | jq -r '.[].kind')
  local kinds_ok=true
  if [[ -z "$kinds" ]]; then
    kinds_ok=false
    fail "Scenario 1: No summary kinds found"
  else
    while IFS= read -r kind; do
      case "$kind" in
        leaf|condensed|session|archive_stub) ;;
        *)
          kinds_ok=false
          fail "Scenario 1: Unexpected summary kind: $kind"
          ;;
      esac
    done <<< "$kinds"
  fi
  if [[ "$kinds_ok" == "true" ]]; then
    pass "Scenario 1: All summary kinds are valid"
  fi

  # Assert: summaries have valid token_count > 0.
  local zero_token_summaries
  zero_token_summaries=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID' AND token_count <= 0" | jq '.[0].cnt // 0')
  if [[ "$zero_token_summaries" -eq 0 ]]; then
    pass "Scenario 1: All summaries have token_count > 0"
  else
    fail "Scenario 1: $zero_token_summaries summaries have token_count <= 0"
  fi

  # Assert: minimum summary token count > 0.
  local summary_tokens
  summary_tokens=$(query_db "SELECT MIN(token_count) as min_tokens FROM lcm_summaries WHERE session_id = '$SID'" | jq '.[0].min_tokens // 0')
  if [[ "$summary_tokens" -gt 0 ]]; then
    pass "Scenario 1: Minimum summary token_count = $summary_tokens (> 0)"
  else
    fail "Scenario 1: Minimum summary token_count is $summary_tokens, expected > 0"
  fi

  # Assert: at least one summary contains LCM-related content.
  local relevant_summary_count
  relevant_summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID' AND (lower(content) LIKE '%lcm%' OR lower(content) LIKE '%compaction%' OR lower(content) LIKE '%summary%')" | jq '.[0].cnt // 0')
  if [[ "$relevant_summary_count" -ge 1 ]]; then
    pass "Scenario 1: At least one summary contains LCM/compaction-relevant content"
  else
    fail "Scenario 1: No summary content mentions LCM, compaction, or summary topics"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: Compacted context items reference summaries
# ---------------------------------------------------------------------------
test_context_items_reference_summaries() {
  echo "=== Scenario 2: Compacted context items reference summaries ==="
  WAVE=3
  SCENARIO="compaction-context-references"

  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID from Scenario 1"
    return
  fi

  # Assert: at least one context item has a summary_id (not NULL).
  local summary_items
  summary_items=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$SID' AND summary_id IS NOT NULL" | jq '.[0].cnt // 0')
  if [[ "$summary_items" -ge 1 ]]; then
    pass "Scenario 2: $summary_items context items reference summaries (>= 1)"
  else
    fail "Scenario 2: No context items reference summaries, expected >= 1"
  fi

  # Verify that summary context items have item_type = 'summary'.
  local mismatch
  mismatch=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$SID' AND summary_id IS NOT NULL AND item_type != 'summary'" | jq '.[0].cnt // 0')
  if [[ "$mismatch" -eq 0 ]]; then
    pass "Scenario 2: All summary-referencing items have item_type = 'summary'"
  else
    fail "Scenario 2: $mismatch items with summary_id have wrong item_type"
  fi

  # Verify that the summary_ids in context items exist in lcm_summaries.
  local orphan_count
  orphan_count=$(query_db "
    SELECT COUNT(*) as cnt FROM lcm_context_items ci
    WHERE ci.session_id = '$SID'
      AND ci.summary_id IS NOT NULL
      AND NOT EXISTS (
        SELECT 1 FROM lcm_summaries s WHERE s.summary_id = ci.summary_id
      )" | jq '.[0].cnt // 0')
  if [[ "$orphan_count" -eq 0 ]]; then
    pass "Scenario 2: No orphan summary references in context items"
  else
    fail "Scenario 2: $orphan_count context items reference non-existent summaries"
  fi

  # Verify context items still have both message and summary types.
  local msg_items
  msg_items=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$SID' AND item_type = 'message'" | jq '.[0].cnt // 0')
  if [[ "$msg_items" -ge 1 ]]; then
    pass "Scenario 2: $msg_items message-type context items remain (>= 1)"
  else
    fail "Scenario 2: No message-type context items found"
  fi

  local summary_content_links
  summary_content_links=$(query_db "
    SELECT COUNT(*) as cnt FROM lcm_context_items ci
    JOIN lcm_summaries s ON s.summary_id = ci.summary_id
    WHERE ci.session_id = '$SID'
      AND ci.summary_id IS NOT NULL
      AND length(s.content) > 20" | jq '.[0].cnt // 0')
  if [[ "$summary_content_links" -ge 1 ]]; then
    pass "Scenario 2: Summary context items link to non-empty summary content"
  else
    fail "Scenario 2: Summary context items do not link to substantive summary content"
  fi

  capture_tui_evidence "context-references"

  # Kill tmux — this is the last scenario.
  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 1
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_compaction_triggered
test_context_items_reference_summaries

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
