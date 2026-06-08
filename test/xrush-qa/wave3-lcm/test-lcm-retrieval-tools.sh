#!/usr/bin/env bash
# Test: LCM retrieval tools (lcm_grep, lcm_describe, lcm_expand).
# Verifies that the LCM agent tools correctly retrieve and present
# conversation history, summary descriptions, and expanded summaries.
# TUI-first approach: primary gates use assert_tui_contains with
# deterministic sentinel strings; DB checks are secondary evidence.
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

# Shared session ID across scenarios (set in Scenario 1, reused in 2 and 3).
SID=""

# ---------------------------------------------------------------------------
# Scenario 1: lcm_grep retrieves a unique sentinel from conversation history
# ---------------------------------------------------------------------------
test_lcm_grep_sentinel() {
  echo "=== Scenario 1: lcm_grep retrieves sentinel from conversation ==="
  WAVE=3
  SCENARIO="lcm-grep-sentinel"

  setup_clean_crush
  cleanup_test() { cleanup_tui; restore_crush; }
  trap cleanup_test EXIT

  start_crush_tui 3
  focus_editor

  # Inject a unique sentinel into the conversation.
  send_tui_prompt "Please remember this exact identifier: LCM_GREP_SENTINEL_42. This is a unique test marker. Repeat it back to me and confirm you have stored it."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_tui_evidence "idle-timeout-prompt1"
    return
  fi

  # Primary gate: the sentinel must appear in the TUI output.
  if assert_tui_contains "LCM_GREP_SENTINEL_42"; then
    pass "Scenario 1: TUI shows LCM_GREP_SENTINEL_42 after injection"
  else
    fail "Scenario 1: TUI does not show LCM_GREP_SENTINEL_42 after injection"
    capture_tui_evidence "sentinel-missing-prompt1"
    return
  fi

  capture_tui_evidence "sentinel-injected"

  # Ask Crush to use lcm_grep to search for the sentinel.
  send_tui_prompt "Use the lcm_grep tool to search for 'LCM_GREP_SENTINEL_42' in our conversation history. Tell me the exact result you found."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after lcm_grep prompt"
    capture_tui_evidence "idle-timeout-prompt2"
    return
  fi

  # Primary gate: the lcm_grep result must include the sentinel.
  if assert_tui_contains "LCM_GREP_SENTINEL_42"; then
    pass "Scenario 1: TUI shows LCM_GREP_SENTINEL_42 after lcm_grep retrieval"
  else
    fail "Scenario 1: TUI does not show LCM_GREP_SENTINEL_42 after lcm_grep retrieval"
    capture_tui_evidence "sentinel-missing-prompt2"
    return
  fi

  capture_tui_evidence "lcm-grep-result"

  # --- Secondary DB checks ---
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    return
  fi

  local msg_count
  msg_count=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$msg_count" -ge 2 ]]; then
    pass "Scenario 1: Session has $msg_count messages (>= 2)"
  else
    fail "Scenario 1: Session has $msg_count messages, expected >= 2"
  fi

  local sentinel_msg_count
  sentinel_msg_count=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$SID' AND lower(parts) LIKE '%lcm_grep_sentinel_42%'" | jq '.[0].cnt // 0')
  if [[ "$sentinel_msg_count" -ge 1 ]]; then
    pass "Scenario 1: $sentinel_msg_count messages contain sentinel in DB"
  else
    fail "Scenario 1: No messages contain sentinel in DB"
  fi

  local ctx_count
  ctx_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$ctx_count" -ge 1 ]]; then
    pass "Scenario 1: lcm_context_items has $ctx_count rows (>= 1)"
  else
    fail "Scenario 1: lcm_context_items has $ctx_count rows, expected >= 1"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: lcm_describe returns facts about a known summary
# ---------------------------------------------------------------------------
test_lcm_describe_facts() {
  echo "=== Scenario 2: lcm_describe returns summary facts ==="
  WAVE=3
  SCENARIO="lcm-describe-facts"

  setup_clean_crush
  cleanup_test() { cleanup_tui; restore_crush; }
  trap cleanup_test EXIT

  start_crush_tui 3
  focus_editor

  # First, inject a sentinel and build enough conversation for compaction.
  send_tui_prompt "Please remember this identifier: LCM_DESCRIBE_SENTINEL_88. Now explain the Go testing conventions used in this project. Describe testify patterns, table-driven tests, and parallel test execution. Be thorough."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle after first prompt"
    capture_tui_evidence "idle-timeout-prompt1"
    return
  fi

  # Build more context to trigger compaction and summaries.
  send_tui_prompt "Now explain how the LCM compaction layers work. Describe each layer's priority, when it triggers, and what it optimizes. Include specific details about token budgets and thresholds. Also repeat the identifier LCM_DESCRIBE_SENTINEL_88 back to me."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle after second prompt"
    capture_tui_evidence "idle-timeout-prompt2"
    return
  fi

  # Get session ID and check for summaries.
  local current_sid
  current_sid=$(get_session_id)
  if [[ -z "$current_sid" ]]; then
    fail "Scenario 2: No session ID found"
    capture_tui_evidence "no-session-id"
    return
  fi

  # Ask Crush to use lcm_describe on available summaries or context items.
  send_tui_prompt "Use the lcm_describe tool to describe the most recent summary or context item in our conversation. Tell me what kind it is and what topics it covers. If it references LCM_DESCRIBE_SENTINEL_88, mention that explicitly."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle after lcm_describe prompt"
    capture_tui_evidence "idle-timeout-prompt3"
    return
  fi

  # Primary gate: the describe result must mention the sentinel.
  if assert_tui_contains "LCM_DESCRIBE_SENTINEL_88"; then
    pass "Scenario 2: TUI shows LCM_DESCRIBE_SENTINEL_88 after describe"
  else
    fail "Scenario 2: TUI does not show LCM_DESCRIBE_SENTINEL_88 after describe"
    capture_tui_evidence "sentinel-missing-describe"
    return
  fi

  capture_tui_evidence "lcm-describe-result"

  # --- Secondary DB checks ---
  local summary_count
  summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$current_sid'" | jq '.[0].cnt // 0')
  if [[ "$summary_count" -ge 1 ]]; then
    pass "Scenario 2: $summary_count summaries exist in DB"

    # Verify the first summary has valid content.
    local first_summary_id
    first_summary_id=$(query_db "SELECT summary_id FROM lcm_summaries WHERE session_id = '$current_sid' ORDER BY created_at DESC LIMIT 1" | jq -r '.[0].summary_id // empty')

    if [[ -n "$first_summary_id" ]]; then
      local summary_content_len
      summary_content_len=$(query_db "SELECT length(content) as len FROM lcm_summaries WHERE summary_id = '$first_summary_id' AND session_id = '$current_sid'" | jq '.[0].len // 0')
      if [[ "$summary_content_len" -gt 20 ]]; then
        pass "Scenario 2: Summary has substantive content (length=$summary_content_len)"
      else
        fail "Scenario 2: Summary content is too short (length=$summary_content_len)"
      fi

      local summary_kind
      summary_kind=$(query_db "SELECT kind FROM lcm_summaries WHERE summary_id = '$first_summary_id' AND session_id = '$current_sid'" | jq -r '.[0].kind // empty')
      case "$summary_kind" in
        leaf|condensed|session|archive_stub)
          pass "Scenario 2: Summary kind is valid: $summary_kind"
          ;;
        *)
          fail "Scenario 2: Unexpected summary kind: $summary_kind"
          ;;
      esac

      local summary_tokens
      summary_tokens=$(query_db "SELECT token_count FROM lcm_summaries WHERE summary_id = '$first_summary_id' AND session_id = '$current_sid'" | jq '.[0].token_count // 0')
      if [[ "$summary_tokens" -gt 0 ]]; then
        pass "Scenario 2: Summary token_count = $summary_tokens (> 0)"
      else
        fail "Scenario 2: Summary token_count is $summary_tokens, expected > 0"
      fi
    fi
  else
    # Summary count is zero — verify context items exist as an alternative signal.
    local ctx_items
    ctx_items=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$current_sid'" | jq '.[0].cnt // 0')
    if [[ "$ctx_items" -ge 2 ]]; then
      pass "Scenario 2: $ctx_items context items exist (>= 2)"
    else
      fail "Scenario 2: Insufficient context items: $ctx_items"
    fi
  fi
}

# ---------------------------------------------------------------------------
# Scenario 3: lcm_expand retrieves original messages after compaction
# ---------------------------------------------------------------------------
test_lcm_expand_after_compaction() {
  echo "=== Scenario 3: lcm_expand retrieves original content after compaction ==="
  WAVE=3
  SCENARIO="lcm-expand-compaction"

  setup_clean_crush
  cleanup_test() { cleanup_tui; restore_crush; }
  trap cleanup_test EXIT

  start_crush_tui 3
  focus_editor

  # Inject a distinctive marker for expand verification.
  send_tui_prompt "Please create a file called /tmp/lcm-expand-test-LCM_EXPAND_SENTINEL_55.txt with the content 'The answer to everything is LCM_EXPAND_SENTINEL_55'. Confirm once done."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 3: Crush did not become idle after first prompt"
    capture_tui_evidence "idle-timeout-prompt1"
    return
  fi

  # Primary gate: the sentinel must appear in the response.
  if assert_tui_contains "LCM_EXPAND_SENTINEL_55"; then
    pass "Scenario 3: TUI shows LCM_EXPAND_SENTINEL_55 after file creation"
  else
    fail "Scenario 3: TUI does not show LCM_EXPAND_SENTINEL_55 after file creation"
    capture_tui_evidence "sentinel-missing-prompt1"
    return
  fi

  # Drive context high enough for compaction to trigger.
  send_tui_prompt "Explain every file in internal/lcm/ in full detail. For each file, describe its purpose, exported types, key methods, and how it integrates with other LCM subsystems. Be exhaustive."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 3: Crush did not become idle after second prompt"
    capture_tui_evidence "idle-timeout-prompt2"
    return
  fi

  send_tui_prompt "Now describe the full compaction pipeline. Explain the 9-layer architecture, how layers are prioritized, the budget formula, and how the cache optimizer works. Include specific details about Anthropic prefix caching."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 3: Crush did not become idle after third prompt"
    capture_tui_evidence "idle-timeout-prompt3"
    return
  fi

  send_tui_prompt "Describe the repository map system. Explain PageRank scoring, tag extraction, file graph construction, and how the renderer works with token budgets."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 3: Crush did not become idle after fourth prompt"
    capture_tui_evidence "idle-timeout-prompt4"
    return
  fi

  # Ask Crush to use lcm_expand to retrieve the original sentinel message.
  send_tui_prompt "Use the lcm_expand tool to expand the summary that contains our earlier conversation about LCM_EXPAND_SENTINEL_55. Show me the original message content you find."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 3: Crush did not become idle after lcm_expand prompt"
    capture_tui_evidence "idle-timeout-prompt5"
    return
  fi

  # Primary gate: the expand result must include the sentinel.
  if assert_tui_contains "LCM_EXPAND_SENTINEL_55"; then
    pass "Scenario 3: TUI shows LCM_EXPAND_SENTINEL_55 after lcm_expand"
  else
    fail "Scenario 3: TUI does not show LCM_EXPAND_SENTINEL_55 after lcm_expand"
    capture_tui_evidence "sentinel-missing-expand"
    return
  fi

  capture_tui_evidence "lcm-expand-result"

  # --- Secondary DB checks ---
  local expand_sid
  expand_sid=$(get_session_id)
  if [[ -z "$expand_sid" ]]; then
    fail "Scenario 3: No session ID found"
    return
  fi

  local total_msgs
  total_msgs=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$expand_sid'" | jq '.[0].cnt // 0')
  if [[ "$total_msgs" -ge 4 ]]; then
    pass "Scenario 3: Session has $total_msgs messages (>= 4)"
  else
    fail "Scenario 3: Session has $total_msgs messages, expected >= 4"
  fi

  local expand_summary_count
  expand_summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$expand_sid'" | jq '.[0].cnt // 0')
  if [[ "$expand_summary_count" -ge 1 ]]; then
    pass "Scenario 3: $expand_summary_count summaries exist after multi-turn conversation"
  else
    fail "Scenario 3: No summaries found in DB"
  fi

  local sentinel_in_msgs
  sentinel_in_msgs=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$expand_sid' AND lower(parts) LIKE '%lcm_expand_sentinel_55%'" | jq '.[0].cnt // 0')
  if [[ "$sentinel_in_msgs" -ge 1 ]]; then
    pass "Scenario 3: $sentinel_in_msgs original messages contain expand sentinel in DB"
  else
    fail "Scenario 3: No messages contain expand sentinel in DB"
  fi

  local summary_msg_links
  summary_msg_links=$(query_db "
    SELECT COUNT(*) as cnt FROM lcm_summary_messages sm
    JOIN lcm_summaries ls ON ls.summary_id = sm.summary_id
    WHERE ls.session_id = '$expand_sid'" | jq '.[0].cnt // 0')
  if [[ "$summary_msg_links" -ge 1 ]]; then
    pass "Scenario 3: $summary_msg_links summary-message links exist (>= 1)"
  else
    fail "Scenario 3: No summary-message links found in DB"
  fi

  local expand_ctx_count
  expand_ctx_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$expand_sid'" | jq '.[0].cnt // 0')
  if [[ "$expand_ctx_count" -ge 1 ]]; then
    pass "Scenario 3: $expand_ctx_count context items exist"
  else
    fail "Scenario 3: No context items found in DB"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_lcm_grep_sentinel
test_lcm_describe_facts
test_lcm_expand_after_compaction

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
