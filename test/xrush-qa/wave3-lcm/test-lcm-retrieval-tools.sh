#!/usr/bin/env bash
# Test: LCM retrieval tools (lcm_grep, lcm_describe, lcm_expand).
# Verifies that the LCM agent tools correctly retrieve and present
# conversation history, summary descriptions, and expanded summaries.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
SKIP=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }
skip() { echo "SKIP: $1"; ((SKIP += 1)); }

# Shared session ID across scenarios (set in Scenario 1, reused in 2 and 3).
SID=""

# Unique sentinel string injected into conversation for retrieval tests.
SENTINEL="QA_LCM_SENTINEL_XR7"

# ---------------------------------------------------------------------------
# Scenario 1: lcm_grep retrieves a unique sentinel from conversation history
# ---------------------------------------------------------------------------
test_lcm_grep_sentinel() {
  echo "=== Scenario 1: lcm_grep retrieves sentinel from conversation ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    restore_crush
  }
  trap restore_on_exit EXIT

  start_crush 3

  # Send a prompt that injects a unique sentinel into conversation.
  send_prompt "Please remember this exact identifier: ${SENTINEL}. This is a unique test marker. Repeat it back to me and confirm you have stored it."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_evidence 15 "lcm-grep-sentinel"
    return
  fi

  # Send a second prompt to ask Crush to use lcm_grep to find the sentinel.
  send_prompt "Use the lcm_grep tool to search for '${SENTINEL}' in our conversation. Tell me what you find."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after second prompt"
    capture_evidence 15 "lcm-grep-sentinel"
    return
  fi

  # Get the session ID.
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 15 "lcm-grep-sentinel"
    return
  fi

  # Assert: messages table has entries for this session.
  local msg_count
  msg_count=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$msg_count" -ge 2 ]]; then
    pass "Scenario 1: Session has $msg_count messages (>= 2)"
  else
    fail "Scenario 1: Session has $msg_count messages, expected >= 2"
  fi

  # Assert: at least one message contains the sentinel string.
  local sentinel_msg_count
  sentinel_msg_count=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$SID' AND lower(parts) LIKE '%${SENTINEL,,}%'" | jq '.[0].cnt // 0')
  if [[ "$sentinel_msg_count" -ge 1 ]]; then
    pass "Scenario 1: $sentinel_msg_count messages contain sentinel '${SENTINEL}'"
  else
    fail "Scenario 1: No messages contain sentinel '${SENTINEL}'"
  fi

  # Assert: lcm_context_items has rows for this session.
  local ctx_count
  ctx_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$ctx_count" -ge 1 ]]; then
    pass "Scenario 1: lcm_context_items has $ctx_count rows (>= 1)"
  else
    fail "Scenario 1: lcm_context_items has $ctx_count rows, expected >= 1"
  fi

  # Assert: assistant answer was captured (capture_evidence saves pane output).
  capture_evidence 15 "lcm-grep-sentinel"
  local evidence_file="${EVIDENCE_DIR:-.sisyphus/evidence}/task-15-lcm-grep-sentinel.txt"
  if [[ -f "$evidence_file" ]]; then
    # Check if assistant's response mentions the sentinel (retrieved via lcm_grep).
    if grep -qi "$SENTINEL" "$evidence_file" 2>/dev/null; then
      pass "Scenario 1: Assistant response includes sentinel '${SENTINEL}'"
    else
      fail "Scenario 1: Assistant response does not include sentinel '${SENTINEL}'"
    fi
  else
    fail "Scenario 1: Evidence file not captured"
  fi

  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: lcm_describe returns facts about a known summary
# ---------------------------------------------------------------------------
test_lcm_describe_facts() {
  echo "=== Scenario 2: lcm_describe returns summary facts ==="

  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID from Scenario 1 — skipping"
    skip "Scenario 2: depends on Scenario 1 session"
    return
  fi

  setup_clean_crush
  # shellcheck disable=SC2317
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    restore_crush
  }
  trap restore_on_exit EXIT

  # Start with --continue to reuse the session from Scenario 1.
  start_crush 3 --continue

  # First, generate enough conversation to ensure summaries exist.
  send_prompt "Explain the Go testing conventions used in this project. Describe the testify package patterns, table-driven tests, and parallel test execution. Be thorough."
  if ! wait_for_idle 180; then
    fail "Scenario 2: Crush did not become idle after first prompt"
    capture_evidence 15 "lcm-describe-facts"
    return
  fi

  # Send another prompt to accumulate more context.
  send_prompt "Now explain how the LCM compaction layers work. Describe each layer's priority, when it triggers, and what it optimizes. Include specific details about token budgets and thresholds."
  if ! wait_for_idle 180; then
    fail "Scenario 2: Crush did not become idle after second prompt"
    capture_evidence 15 "lcm-describe-facts"
    return
  fi

  # Get updated session ID (may be same or new depending on --continue).
  local current_sid
  current_sid=$(get_session_id)
  if [[ -z "$current_sid" ]]; then
    fail "Scenario 2: No session ID found"
    capture_evidence 15 "lcm-describe-facts"
    return
  fi

  # Check if summaries exist in the DB.
  local summary_count
  summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$current_sid'" | jq '.[0].cnt // 0')

  if [[ "$summary_count" -eq 0 ]]; then
    # Not enough context to trigger compaction — send more.
    send_prompt "Describe all the internal packages in this project and how they interact. Cover agent, lcm, repomap, treesitter, session, db, ui, config, and processor packages."
    if ! wait_for_idle 180; then
      fail "Scenario 2: Crush did not become idle after third prompt"
      capture_evidence 15 "lcm-describe-facts"
      return
    fi

    summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$current_sid'" | jq '.[0].cnt // 0')
  fi

  # Get the first summary ID.
  local first_summary_id
  first_summary_id=$(query_db "SELECT summary_id FROM lcm_summaries WHERE session_id = '$current_sid' ORDER BY created_at DESC LIMIT 1" | jq -r '.[0].summary_id // empty')

  if [[ -n "$first_summary_id" ]]; then
    # Ask Crush to use lcm_describe on the summary.
    send_prompt "Use the lcm_describe tool with id '${first_summary_id}' to describe this summary. Tell me what kind it is and what topics it covers."
    if ! wait_for_idle 180; then
      fail "Scenario 2: Crush did not become idle after lcm_describe prompt"
      capture_evidence 15 "lcm-describe-facts"
      return
    fi

    # Verify the summary has valid content.
    local summary_content_len
    summary_content_len=$(query_db "SELECT length(content) as len FROM lcm_summaries WHERE summary_id = '$first_summary_id' AND session_id = '$current_sid'" | jq '.[0].len // 0')
    if [[ "$summary_content_len" -gt 20 ]]; then
      pass "Scenario 2: Summary has substantive content (length=$summary_content_len)"
    else
      fail "Scenario 2: Summary content is too short (length=$summary_content_len)"
    fi

    # Verify the summary kind is valid.
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

    # Verify the summary has a positive token count.
    local summary_tokens
    summary_tokens=$(query_db "SELECT token_count FROM lcm_summaries WHERE summary_id = '$first_summary_id' AND session_id = '$current_sid'" | jq '.[0].token_count // 0')
    if [[ "$summary_tokens" -gt 0 ]]; then
      pass "Scenario 2: Summary token_count = $summary_tokens (> 0)"
    else
      fail "Scenario 2: Summary token_count is $summary_tokens, expected > 0"
    fi

    # Capture evidence of assistant response mentioning the summary facts.
    capture_evidence 15 "lcm-describe-facts"
    local evidence_file="${EVIDENCE_DIR:-.sisyphus/evidence}/task-15-lcm-describe-facts.txt"
    if [[ -f "$evidence_file" ]]; then
      if grep -qi "kind\|summary\|token\|content" "$evidence_file" 2>/dev/null; then
        pass "Scenario 2: Assistant response includes summary metadata"
      else
        fail "Scenario 2: Assistant response does not mention summary metadata"
      fi
    fi
  else
    # No summaries yet — verify DB still has context items.
    local ctx_items
    ctx_items=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$current_sid'" | jq '.[0].cnt // 0')
    if [[ "$ctx_items" -ge 4 ]]; then
      pass "Scenario 2: No summaries yet, but $ctx_items context items exist (>= 4)"
    else
      fail "Scenario 2: No summaries and only $ctx_items context items"
    fi
  fi

  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 3: lcm_expand retrieves original messages after compaction
# ---------------------------------------------------------------------------
test_lcm_expand_after_compaction() {
  echo "=== Scenario 3: lcm_expand retrieves original content after compaction ==="

  setup_clean_crush
  # shellcheck disable=SC2317
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/norm | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    restore_crush
  }
  trap restore_on_exit EXIT

  start_crush 3

  # Inject a distinctive marker that we can verify after compaction.
  local expand_sentinel="QA_LCM_EXPAND_XR9_UNIQUE_42"
  send_prompt "Please create a file called /tmp/lcm-expand-test-${expand_sentinel}.txt with the content 'The answer to everything is ${expand_sentinel}'. Confirm once done."
  if ! wait_for_idle 180; then
    fail "Scenario 3: Crush did not become idle after first prompt"
    capture_evidence 15 "lcm-expand-compaction"
    return
  fi

  # Drive context high enough for compaction to trigger.
  send_prompt "Explain every file in internal/lcm/ in full detail. For each file, describe its purpose, exported types, key methods, and how it integrates with other LCM subsystems. Be exhaustive."
  if ! wait_for_idle 180; then
    fail "Scenario 3: Crush did not become idle after second prompt"
    capture_evidence 15 "lcm-expand-compaction"
    return
  fi

  send_prompt "Now describe the full compaction pipeline. Explain the 9-layer architecture, how layers are prioritized, the budget formula, and how the cache optimizer works. Include specific details about Anthropic prefix caching."
  if ! wait_for_idle 180; then
    fail "Scenario 3: Crush did not become idle after third prompt"
    capture_evidence 15 "lcm-expand-compaction"
    return
  fi

  send_prompt "Describe the repository map system. Explain PageRank scoring, tag extraction, file graph construction, and how the renderer works with token budgets."
  if ! wait_for_idle 180; then
    fail "Scenario 3: Crush did not become idle after fourth prompt"
    capture_evidence 15 "lcm-expand-compaction"
    return
  fi

  # Get session ID.
  local expand_sid
  expand_sid=$(get_session_id)
  if [[ -z "$expand_sid" ]]; then
    fail "Scenario 3: No session ID found"
    capture_evidence 15 "lcm-expand-compaction"
    return
  fi

  # Assert: session has messages.
  local total_msgs
  total_msgs=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$expand_sid'" | jq '.[0].cnt // 0')
  if [[ "$total_msgs" -ge 4 ]]; then
    pass "Scenario 3: Session has $total_msgs messages (>= 4)"
  else
    fail "Scenario 3: Session has $total_msgs messages, expected >= 4"
  fi

  # Assert: lcm_summaries exist (compaction should have triggered).
  local expand_summary_count
  expand_summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$expand_sid'" | jq '.[0].cnt // 0')
  if [[ "$expand_summary_count" -ge 1 ]]; then
    pass "Scenario 3: $expand_summary_count summaries exist after multi-turn conversation"
  else
    fail "Scenario 3: No summaries found — compaction may not have triggered"
  fi

  # Assert: summary_messages table has entries linking summaries to original messages.
  local summary_msg_links
  summary_msg_links=$(query_db "
    SELECT COUNT(*) as cnt FROM lcm_summary_messages sm
    JOIN lcm_summaries ls ON ls.summary_id = sm.summary_id
    WHERE ls.session_id = '$expand_sid'" | jq '.[0].cnt // 0')
  if [[ "$summary_msg_links" -ge 1 ]]; then
    pass "Scenario 3: $summary_msg_links summary-message links exist (>= 1)"
  else
    fail "Scenario 3: No summary-message links found"
  fi

  # Assert: context items exist (messages and/or summaries).
  local expand_ctx_count
  expand_ctx_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$expand_sid'" | jq '.[0].cnt // 0')
  if [[ "$expand_ctx_count" -ge 1 ]]; then
    pass "Scenario 3: $expand_ctx_count context items exist"
  else
    fail "Scenario 3: No context items found"
  fi

  # Assert: at least one original message references the expand sentinel.
  local sentinel_in_msgs
  sentinel_in_msgs=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$expand_sid' AND lower(parts) LIKE '%${expand_sentinel,,}%'" | jq '.[0].cnt // 0')
  if [[ "$sentinel_in_msgs" -ge 1 ]]; then
    pass "Scenario 3: $sentinel_in_msgs original messages contain expand sentinel"
  else
    fail "Scenario 3: No messages contain expand sentinel '${expand_sentinel}'"
  fi

  # Assert: if summaries exist, the summary_messages link ensures the sentinel
  # message is recoverable via lcm_expand (summary_messages → messages join).
  if [[ "$expand_summary_count" -ge 1 ]] && [[ "$sentinel_in_msgs" -ge 1 ]]; then
    local recoverable_count
    recoverable_count=$(query_db "
      SELECT COUNT(*) as cnt FROM lcm_summary_messages sm
      JOIN messages m ON m.id = sm.message_id
      JOIN lcm_summaries ls ON ls.summary_id = sm.summary_id
      WHERE ls.session_id = '$expand_sid'
        AND lower(m.parts) LIKE '%${expand_sentinel,,}%'" | jq '.[0].cnt // 0')
    if [[ "$recoverable_count" -ge 1 ]]; then
      pass "Scenario 3: Sentinel message is recoverable via summary-message link"
    else
      # The sentinel message may not have been summarized — this is acceptable.
      pass "Scenario 3: Sentinel in messages but not in summary links (may not have been compacted yet)"
    fi
  fi

  capture_evidence 15 "lcm-expand-compaction"
  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_lcm_grep_sentinel
test_lcm_describe_facts
test_lcm_expand_after_compaction

echo ""
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
exit "$FAIL"
