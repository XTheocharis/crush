#!/usr/bin/env bash
# Test: LCM retrieval after compaction AND routing (cross-feature interaction).
# Verifies that after compaction occurs, a large/context-heavy request triggers
# routing and the model still recovers old sentinels from compacted context.
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
# Scenario 1: Build context, trigger compaction, then route a large request
#             that must recover early sentinels from compacted context.
# ---------------------------------------------------------------------------
test_compaction_then_routing() {
  echo "=== Scenario 1: Compaction followed by routing with sentinel recovery ==="
  WAVE=3
  SCENARIO="compaction-routing-recovery"

  setup_clean_crush
  cleanup_test() {
    cleanup_tui
    restore_crush
  }
  trap cleanup_test EXIT

  start_crush_tui 3
  focus_editor

  # --- Prompt 1: Establish early sentinel in conversation. ---
  send_tui_prompt "Explain the purpose of each file in internal/lcm/ in detail. Describe the Manager, Compactor, Store, and summarizer components. For each, list key functions and data flow. Somewhere in your reply include the exact token LCM_ROUTE_SENTINEL_EARLY_42."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_tui_evidence "idle-timeout-p1"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "LCM_ROUTE_SENTINEL_EARLY_42"; then
    pass "Scenario 1: TUI shows LCM_ROUTE_SENTINEL_EARLY_42 sentinel"
  else
    fail "Scenario 1: TUI does not show LCM_ROUTE_SENTINEL_EARLY_42 sentinel"
    capture_tui_evidence "sentinel-missing-p1"
    return
  fi

  # --- Prompt 2: Build more context to push toward compaction. ---
  focus_editor
  send_tui_prompt "Now explain the 9-layer compaction pipeline in internal/lcm/compaction_layers.go. For each layer describe its priority, trigger condition, and what it compacts. Include code-level detail. Somewhere in your reply include the exact token LCM_ROUTE_SENTINEL_MID_77."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after second prompt"
    capture_tui_evidence "idle-timeout-p2"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "LCM_ROUTE_SENTINEL_MID_77"; then
    pass "Scenario 1: TUI shows LCM_ROUTE_SENTINEL_MID_77 sentinel"
  else
    fail "Scenario 1: TUI does not show LCM_ROUTE_SENTINEL_MID_77 sentinel"
    capture_tui_evidence "sentinel-missing-p2"
    return
  fi

  # --- Prompt 3: More context to increase likelihood of compaction. ---
  focus_editor
  send_tui_prompt "Describe the LCM explorer subsystem in internal/lcm/explorer/, including the Registry, dispatch logic, per-language stdlib membership, and how tree-sitter code exploration works. Provide detailed examples. Somewhere in your reply include the exact token LCM_ROUTE_SENTINEL_MID_99."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after third prompt"
    capture_tui_evidence "idle-timeout-p3"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "LCM_ROUTE_SENTINEL_MID_99"; then
    pass "Scenario 1: TUI shows LCM_ROUTE_SENTINEL_MID_99 sentinel"
  else
    fail "Scenario 1: TUI does not show LCM_ROUTE_SENTINEL_MID_99 sentinel"
    capture_tui_evidence "sentinel-missing-p3"
    return
  fi

  # --- Prompt 4: Large/context-heavy request to trigger routing. ---
  focus_editor
  send_tui_prompt "Read AGENTS.md, internal/agent/router_tier.go, internal/lcm/manager.go, internal/repomap/repomap.go, and internal/treesitter/treesitter.go. Synthesize a comprehensive summary of how LCM compaction, model routing, repository mapping, and tree-sitter analysis work together as an integrated system. Then answer: what was the exact token I asked you to include in your very first reply in this conversation? End your reply with exactly: LCM_ROUTE_SENTINEL_RECOVERED_88"

  if ! wait_for_tui_idle 240; then
    fail "Scenario 1: Crush did not become idle after routing prompt"
    capture_tui_evidence "idle-timeout-p4"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: TUI must contain the recovery sentinel.
  if assert_tui_contains "LCM_ROUTE_SENTINEL_RECOVERED_88"; then
    pass "Scenario 1: TUI contains LCM_ROUTE_SENTINEL_RECOVERED_88"
  else
    fail "Scenario 1: TUI does not contain LCM_ROUTE_SENTINEL_RECOVERED_88"
    capture_tui_evidence "recovered-sentinel-missing"
  fi

  # Primary gate: TUI must also recover the early sentinel from compacted context.
  if assert_tui_contains "LCM_ROUTE_SENTINEL_EARLY_42"; then
    pass "Scenario 1: TUI recovered early sentinel LCM_ROUTE_SENTINEL_EARLY_42 from compacted context"
  else
    fail "Scenario 1: TUI did not recover early sentinel LCM_ROUTE_SENTINEL_EARLY_42"
    capture_tui_evidence "early-sentinel-not-recovered"
  fi

  capture_tui_evidence "routing-recovery-final"

  # --- Secondary: log grep for compaction and routing evidence. ---
  local compaction_entries
  compaction_entries=$(grep -ciE "compact|lcm.*summar|layer.*compact|summary.*creat" .crush/logs/crush.log 2>/dev/null ) || compaction_entries=0
  if [[ "$compaction_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $compaction_entries compaction log entries"
  else
    echo "  NOTE: No compaction log entries found"
  fi

  local routing_entries
  routing_entries=$(grep -ciE "router|tier|routing|model.*select|route.*request|escalat" .crush/logs/crush.log 2>/dev/null ) || routing_entries=0
  if [[ "$routing_entries" -ge 1 ]]; then
    pass "Scenario 1: Found $routing_entries routing/tier log entries"
  else
    echo "  NOTE: No routing/tier log entries found"
  fi

  echo "--- Compaction log evidence ---"
  grep -iE "compact|lcm.*summar|layer.*compact|summary.*creat" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- Routing log evidence ---"
  grep -iE "router|tier|routing|model.*select|route.*request|escalat" .crush/logs/crush.log 2>/dev/null | head -10 || true
  echo "--- End evidence ---"

  # --- Secondary: DB checks for lcm_summaries. ---
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    return
  fi

  local summary_count
  summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$summary_count" -ge 1 ]]; then
    pass "Scenario 1: lcm_summaries has $summary_count rows (>= 1)"
  else
    fail "Scenario 1: lcm_summaries has $summary_count rows, expected >= 1"
  fi

  # Verify summary kinds are valid.
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

  # Verify context items reference summaries.
  local summary_items
  summary_items=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$SID' AND summary_id IS NOT NULL" | jq '.[0].cnt // 0')
  if [[ "$summary_items" -ge 1 ]]; then
    pass "Scenario 1: $summary_items context items reference summaries (>= 1)"
  else
    fail "Scenario 1: No context items reference summaries, expected >= 1"
  fi

  # Verify no orphan summary references.
  local orphan_count
  orphan_count=$(query_db "
    SELECT COUNT(*) as cnt FROM lcm_context_items ci
    WHERE ci.session_id = '$SID'
      AND ci.summary_id IS NOT NULL
      AND NOT EXISTS (
        SELECT 1 FROM lcm_summaries s WHERE s.summary_id = ci.summary_id
      )" | jq '.[0].cnt // 0')
  if [[ "$orphan_count" -eq 0 ]]; then
    pass "Scenario 1: No orphan summary references in context items"
  else
    fail "Scenario 1: $orphan_count context items reference non-existent summaries"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: Post-compaction routing with cross-session sentinel persistence
# ---------------------------------------------------------------------------
test_post_compaction_routing_persistence() {
  echo "=== Scenario 2: Post-compaction routing with session persistence ==="
  WAVE=3
  SCENARIO="post-compaction-routing-persist"

  setup_clean_crush
  cleanup_test() {
    cleanup_tui
    restore_crush
  }
  trap cleanup_test EXIT

  start_crush_tui 3
  focus_editor

  # --- Prompt 1: Establish sentinel. ---
  send_tui_prompt "Explain the repository map system in internal/repomap/. Cover repomap.go, tags.go, graph.go, pagerank.go, render.go, and tiktoken.go. Describe the PageRank algorithm as applied to code files. Somewhere in your reply include the exact token LCM_ROUTE_PERSIST_EARLY_11."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle after first prompt"
    capture_tui_evidence "idle-timeout-s2p1"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "LCM_ROUTE_PERSIST_EARLY_11"; then
    pass "Scenario 2: TUI shows LCM_ROUTE_PERSIST_EARLY_11 sentinel"
  else
    fail "Scenario 2: TUI does not show LCM_ROUTE_PERSIST_EARLY_11 sentinel"
    capture_tui_evidence "sentinel-missing-s2p1"
    return
  fi

  # --- Prompt 2: Heavy context build. ---
  focus_editor
  send_tui_prompt "Now explain every tool in internal/agent/tools/ that starts with 'lcm_'. For each tool, describe its purpose, parameters, return type, and how it integrates with the LCM manager. Provide implementation details. Somewhere in your reply include the exact token LCM_ROUTE_PERSIST_MID_33."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle after second prompt"
    capture_tui_evidence "idle-timeout-s2p2"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "LCM_ROUTE_PERSIST_MID_33"; then
    pass "Scenario 2: TUI shows LCM_ROUTE_PERSIST_MID_33 sentinel"
  else
    fail "Scenario 2: TUI does not show LCM_ROUTE_PERSIST_MID_33 sentinel"
    capture_tui_evidence "sentinel-missing-s2p2"
    return
  fi

  # --- Prompt 3: Heavy context build. ---
  focus_editor
  send_tui_prompt "Describe the complete message processing pipeline in internal/processor/. Cover all four phases, every registered processor, and how TokenLimiter, SystemPromptScrubber, and PIIDetector work internally. Somewhere in your reply include the exact token LCM_ROUTE_PERSIST_MID_55."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle after third prompt"
    capture_tui_evidence "idle-timeout-s2p3"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  if assert_tui_contains "LCM_ROUTE_PERSIST_MID_55"; then
    pass "Scenario 2: TUI shows LCM_ROUTE_PERSIST_MID_55 sentinel"
  else
    fail "Scenario 2: TUI does not show LCM_ROUTE_PERSIST_MID_55 sentinel"
    capture_tui_evidence "sentinel-missing-s2p3"
    return
  fi

  # --- Prompt 4: Large synthesis request to trigger routing + recover early sentinel. ---
  focus_editor
  send_tui_prompt "Read internal/agent/agent.go, internal/agent/coordinator.go, internal/lcm/manager.go, internal/processor/runner.go, and internal/repomap/repomap.go. Explain how the agent loop, LCM compaction, processor pipeline, and repomap generation interact during a multi-turn session. Then recall: what exact token did I ask you to include in your very first reply? End your reply with exactly: LCM_ROUTE_PERSIST_RECOVERED_99"

  if ! wait_for_tui_idle 240; then
    fail "Scenario 2: Crush did not become idle after routing prompt"
    capture_tui_evidence "idle-timeout-s2p4"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # Primary gate: recovery sentinel.
  if assert_tui_contains "LCM_ROUTE_PERSIST_RECOVERED_99"; then
    pass "Scenario 2: TUI contains LCM_ROUTE_PERSIST_RECOVERED_99"
  else
    fail "Scenario 2: TUI does not contain LCM_ROUTE_PERSIST_RECOVERED_99"
    capture_tui_evidence "recovered-sentinel-missing-s2"
  fi

  # Primary gate: early sentinel recovered from compacted context.
  if assert_tui_contains "LCM_ROUTE_PERSIST_EARLY_11"; then
    pass "Scenario 2: TUI recovered early sentinel LCM_ROUTE_PERSIST_EARLY_11 from compacted context"
  else
    fail "Scenario 2: TUI did not recover early sentinel LCM_ROUTE_PERSIST_EARLY_11"
    capture_tui_evidence "early-sentinel-not-recovered-s2"
  fi

  capture_tui_evidence "post-compaction-routing-final"

  # --- Secondary: DB checks. ---
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID found in DB"
    return
  fi

  local summary_count
  summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$summary_count" -ge 1 ]]; then
    pass "Scenario 2: lcm_summaries has $summary_count rows (>= 1)"
  else
    fail "Scenario 2: lcm_summaries has $summary_count rows, expected >= 1"
  fi

  # Verify summaries have positive token counts.
  local zero_tokens
  zero_tokens=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID' AND token_count <= 0" | jq '.[0].cnt // 0')
  if [[ "$zero_tokens" -eq 0 ]]; then
    pass "Scenario 2: All summaries have token_count > 0"
  else
    fail "Scenario 2: $zero_tokens summaries have token_count <= 0"
  fi

  # Verify at least one summary contains compaction/routing-relevant content.
  local relevant_summaries
  relevant_summaries=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID' AND (lower(content) LIKE '%lcm%' OR lower(content) LIKE '%compact%' OR lower(content) LIKE '%rout%' OR lower(content) LIKE '%summary%')" | jq '.[0].cnt // 0')
  if [[ "$relevant_summaries" -ge 1 ]]; then
    pass "Scenario 2: At least one summary contains LCM/compaction/routing-relevant content"
  else
    fail "Scenario 2: No summary content mentions LCM, compaction, or routing topics"
  fi

  # Verify summary-linked context items have correct item_type.
  local type_mismatch
  type_mismatch=$(query_db "SELECT COUNT(*) as cnt FROM lcm_context_items WHERE session_id = '$SID' AND summary_id IS NOT NULL AND item_type != 'summary'" | jq '.[0].cnt // 0')
  if [[ "$type_mismatch" -eq 0 ]]; then
    pass "Scenario 2: All summary-referencing items have item_type = 'summary'"
  else
    fail "Scenario 2: $type_mismatch items with summary_id have wrong item_type"
  fi

  # --- Log evidence for both compaction and routing. ---
  local compaction_log
  compaction_log=$(grep -ciE "compact|lcm.*summar|layer.*compact" .crush/logs/crush.log 2>/dev/null ) || compaction_log=0
  if [[ "$compaction_log" -ge 1 ]]; then
    pass "Scenario 2: Found $compaction_log compaction log entries"
  else
    echo "  NOTE: No compaction log entries found"
  fi

  local routing_log
  routing_log=$(grep -ciE "router|tier|routing|model.*select|route.*request" .crush/logs/crush.log 2>/dev/null ) || routing_log=0
  if [[ "$routing_log" -ge 1 ]]; then
    pass "Scenario 2: Found $routing_log routing/tier log entries"
  else
    echo "  NOTE: No routing/tier log entries found"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_compaction_then_routing
test_post_compaction_routing_persistence

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
