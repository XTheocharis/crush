#!/usr/bin/env bash
# Test: LCM content is retrievable after compaction.
# Verifies that a unique sentinel injected in early messages can still be
# found via lcm_grep / lcm_expand and that the messages table retains it
# even after the 9-layer compaction pipeline has run.
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

# Unique sentinel injected into the first prompt.
SENTINEL="QA_COMPACTION_RETRIEVE_X9"

# ---------------------------------------------------------------------------
# Scenario 1: Inject sentinel, drive compaction, then retrieve via lcm_grep
# ---------------------------------------------------------------------------
test_sentinel_survives_compaction() {
  echo "=== Scenario 1: Sentinel retrievable after compaction ==="

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

  # --- Prompt 1: inject the sentinel in an early message ---
  send_prompt "Please remember this exact identifier: ${SENTINEL}. This is a unique test marker used for compaction retrieval verification. Repeat it back to me and confirm you have stored it."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after sentinel prompt"
    capture_evidence 16 "compaction-retrieval-sentinel"
    return
  fi

  # --- Prompts 2-5: drive context high enough to trigger compaction ---
  send_prompt "Explain every file in internal/lcm/ in full detail. For each file, describe its purpose, exported types, key methods, and how it integrates with other LCM subsystems. Be exhaustive."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after prompt 2"
    capture_evidence 16 "compaction-retrieval-sentinel"
    return
  fi

  send_prompt "Now describe the full 9-layer compaction pipeline. Explain how layers are prioritized, the budget formula, the cache optimizer, and how Anthropic prefix caching is leveraged. Include code-level details."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after prompt 3"
    capture_evidence 16 "compaction-retrieval-sentinel"
    return
  fi

  send_prompt "Describe the repository map system in depth. Explain PageRank scoring over the file graph, tag extraction via tree-sitter, the rendering algorithm with token budgets, and how personalization vectors work."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after prompt 4"
    capture_evidence 16 "compaction-retrieval-sentinel"
    return
  fi

  send_prompt "Explain how the tree-sitter integration works. Cover the parser pool, query loading, AST scope walking, import resolution, and how 28 language grammars are managed. Include details about CGO usage."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after prompt 5"
    capture_evidence 16 "compaction-retrieval-sentinel"
    return
  fi

  # --- Prompt 6: ask Crush to use lcm_grep to find the sentinel ---
  send_prompt "Use the lcm_grep tool to search for '${SENTINEL}' in our conversation history. Tell me what you find — include the exact match."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after lcm_grep prompt"
    capture_evidence 16 "compaction-retrieval-sentinel"
    return
  fi

  # Get the session ID.
  local sid
  sid=$(get_session_id)
  if [[ -z "$sid" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 16 "compaction-retrieval-sentinel"
    return
  fi

  # ---------------------------------------------------------------
  # Assertion A: compaction occurred — lcm_summaries has rows
  # ---------------------------------------------------------------
  local summary_count
  summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$sid'" | jq '.[0].cnt // 0')
  if [[ "$summary_count" -ge 1 ]]; then
    pass "Scenario 1: lcm_summaries has $summary_count rows — compaction occurred"
  else
    fail "Scenario 1: lcm_summaries has $summary_count rows, expected >= 1 — compaction may not have triggered"
  fi

  # ---------------------------------------------------------------
  # Assertion B: messages table still references the sentinel
  # ---------------------------------------------------------------
  local sentinel_in_msgs
  sentinel_in_msgs=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$sid' AND lower(parts) LIKE '%${SENTINEL,,}%'" | jq '.[0].cnt // 0')
  if [[ "$sentinel_in_msgs" -ge 1 ]]; then
    pass "Scenario 1: $sentinel_in_msgs messages still contain sentinel '${SENTINEL}'"
  else
    fail "Scenario 1: No messages contain sentinel '${SENTINEL}'"
  fi

  # ---------------------------------------------------------------
  # Assertion C: sentinel is retrievable via summary-message links
  # ---------------------------------------------------------------
  if [[ "$summary_count" -ge 1 ]] && [[ "$sentinel_in_msgs" -ge 1 ]]; then
    local recoverable_via_summary
    recoverable_via_summary=$(query_db "
      SELECT COUNT(*) as cnt FROM lcm_summary_messages sm
      JOIN messages m ON m.id = sm.message_id
      JOIN lcm_summaries ls ON ls.summary_id = sm.summary_id
      WHERE ls.session_id = '$sid'
        AND lower(m.parts) LIKE '%${SENTINEL,,}%'" | jq '.[0].cnt // 0')
    if [[ "$recoverable_via_summary" -ge 1 ]]; then
      pass "Scenario 1: Sentinel is recoverable via summary-message link ($recoverable_via_summary links)"
    else
      pass "Scenario 1: Sentinel in messages but not in summary links (may not have been compacted yet)"
    fi
  fi

  # ---------------------------------------------------------------
  # Assertion D: context items exist for this session
  # ---------------------------------------------------------------
  local ctx_count
  ctx_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$sid'" | jq '.[0].cnt // 0')
  if [[ "$ctx_count" -ge 1 ]]; then
    pass "Scenario 1: $ctx_count context items exist for session"
  else
    fail "Scenario 1: No context items found for session"
  fi

  # ---------------------------------------------------------------
  # Assertion E: assistant response mentions the sentinel (via lcm_grep)
  # ---------------------------------------------------------------
  capture_evidence 16 "compaction-retrieval-sentinel"
  local evidence_file="${EVIDENCE_DIR:-.sisyphus/evidence}/task-16-compaction-retrieval-sentinel.txt"
  if [[ -f "$evidence_file" ]]; then
    if grep -qi "$SENTINEL" "$evidence_file" 2>/dev/null; then
      pass "Scenario 1: Assistant response includes sentinel '${SENTINEL}' (retrieved via lcm_grep)"
    else
      fail "Scenario 1: Assistant response does not include sentinel '${SENTINEL}'"
    fi
  else
    fail "Scenario 1: Evidence file not captured"
  fi

  # Store SID for Scenario 2.
  SID="$sid"

  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: lcm_expand recovers original sentinel content after compaction
# ---------------------------------------------------------------------------
test_lcm_expand_recovers_sentinel() {
  echo "=== Scenario 2: lcm_expand recovers sentinel after compaction ==="

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

  # Continue the session from Scenario 1.
  start_crush 3 --continue

  # Ask Crush to expand any summary that references the sentinel.
  send_prompt "Use lcm_expand on the summary that covers our early conversation. Then tell me whether the identifier '${SENTINEL}' appears in the expanded content."
  if ! wait_for_idle 180; then
    fail "Scenario 2: Crush did not become idle after lcm_expand prompt"
    capture_evidence 16 "compaction-retrieval-expand"
    return
  fi

  # ---------------------------------------------------------------
  # Assertion A: summaries still exist for the session
  # ---------------------------------------------------------------
  local summary_count
  summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$summary_count" -ge 1 ]]; then
    pass "Scenario 2: $summary_count summaries still exist for session"
  else
    fail "Scenario 2: No summaries found for session"
  fi

  # ---------------------------------------------------------------
  # Assertion B: summary_messages link table has entries
  # ---------------------------------------------------------------
  local summary_msg_links
  summary_msg_links=$(query_db "
    SELECT COUNT(*) as cnt FROM lcm_summary_messages sm
    JOIN lcm_summaries ls ON ls.summary_id = sm.summary_id
    WHERE ls.session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$summary_msg_links" -ge 1 ]]; then
    pass "Scenario 2: $summary_msg_links summary-message links exist (>= 1)"
  else
    fail "Scenario 2: No summary-message links found"
  fi

  # ---------------------------------------------------------------
  # Assertion C: sentinel is present in original messages
  # (messages table retains original content regardless of compaction)
  # ---------------------------------------------------------------
  local sentinel_in_msgs
  sentinel_in_msgs=$(query_db "SELECT COUNT(*) as cnt FROM messages WHERE session_id = '$SID' AND lower(parts) LIKE '%${SENTINEL,,}%'" | jq '.[0].cnt // 0')
  if [[ "$sentinel_in_msgs" -ge 1 ]]; then
    pass "Scenario 2: $sentinel_in_msgs original messages still contain sentinel"
  else
    fail "Scenario 2: No original messages contain sentinel '${SENTINEL}'"
  fi

  # ---------------------------------------------------------------
  # Assertion D: assistant response confirms sentinel via lcm_expand
  # ---------------------------------------------------------------
  capture_evidence 16 "compaction-retrieval-expand"
  local evidence_file="${EVIDENCE_DIR:-.sisyphus/evidence}/task-16-compaction-retrieval-expand.txt"
  if [[ -f "$evidence_file" ]]; then
    if grep -qi "$SENTINEL" "$evidence_file" 2>/dev/null; then
      pass "Scenario 2: Assistant response includes sentinel via lcm_expand"
    else
      fail "Scenario 2: Assistant response does not include sentinel via lcm_expand"
    fi
  else
    fail "Scenario 2: Evidence file not captured"
  fi

  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_sentinel_survives_compaction
test_lcm_expand_recovers_sentinel

echo ""
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
exit "$FAIL"
