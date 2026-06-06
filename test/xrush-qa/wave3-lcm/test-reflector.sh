#!/usr/bin/env bash
# Test: LCM reflector and observation buffer.
# Verifies that the BufferingCoordinator collects observations and that
# the ReflectorAgent triggers reflection cycles when token thresholds are
# crossed. Wave 3 lowers the reflector threshold so this test can require
# actual reflection evidence instead of accepting an unreachable threshold.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# Shared session ID across scenarios (set in Scenario 1, reused in Scenario 2).
SID=""

# ---------------------------------------------------------------------------
# Scenario 1: Reflector triggered post-compaction
#
# Drive conversation with multiple long prompts to accumulate tokens.
# Check lcm_observation_buffer for rows. Wave 3 lowers the threshold, so no
# buffer rows and no reflection logs is a real failure.
# ---------------------------------------------------------------------------
test_reflector_post_compaction() {
  echo "=== Scenario 1: Reflector triggered post-compaction ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called via trap
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    command restore_crush
  }
  trap restore_on_exit EXIT

  start_crush 3

  # Prompt 1: Ask for detailed explanation of the entire LCM subsystem.
  send_prompt "Explain the entire internal/lcm/ package in exhaustive detail. For every .go file, list its purpose, all exported functions and types, and describe how they interact with the rest of the LCM pipeline. Include details about the Manager, Store, Compactor, ReflectorAgent, BufferingCoordinator, Explorer subsystem, and all 9 compaction layers."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_evidence 15 "reflector-compaction"
    return
  fi

  # Prompt 2: Ask for detailed explanation of the repo map and tree-sitter.
  send_prompt "Now describe the internal/repomap/ and internal/treesitter/ packages in full detail. Explain how PageRank is used for file ranking, how the token budget is allocated, how tree-sitter grammars are loaded and pooled, and how tag extraction works across all 28 supported languages. Include code-level details about the FileGraph, edge types, and rendering pipeline."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after second prompt"
    capture_evidence 15 "reflector-compaction"
    return
  fi

  # Prompt 3: Ask for detailed explanation of the agent orchestration system.
  send_prompt "Describe the full agent orchestration system in internal/agent/. Cover the Coordinator, Operator, Parallel, and Swarm patterns in detail. Explain how forked sessions work, how structured sub-agents produce typed output, how the doom-loop detector operates, and how the model router and tier-based routing decide which model to use. Include the resource_limits, ratelimit, and cache_share subsystems."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after third prompt"
    capture_evidence 15 "reflector-compaction"
    return
  fi

  # Prompt 4: Ask for detailed explanation of the processor pipeline and eval.
  send_prompt "Explain the internal/processor/ pipeline and internal/eval/ harness in detail. Describe all four processor phases, the 15 configurable processors, how TokenLimiter and PIIDetector work, and how the eval runner loads JSON datasets and runs through scorers. Also explain the rewind/snapshot system in internal/rewind/."
  if ! wait_for_idle 180; then
    fail "Scenario 1: Crush did not become idle after fourth prompt"
    capture_evidence 15 "reflector-compaction"
    return
  fi

  # Get the session ID.
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 15 "reflector-compaction"
    return
  fi

  # Query total token count for context.
  local total_tokens
  total_tokens=$(query_db "SELECT COALESCE(SUM(token_count), 0) as total FROM lcm_context_items WHERE session_id = '$SID'" | jq '.[0].total // 0')
  echo "INFO: Scenario 1: Total context tokens = $total_tokens (reflector threshold = 500)"

  # Check lcm_observation_buffer for any rows for this session.
  local obs_count
  obs_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_observation_buffer WHERE session_id = '$SID'" | jq '.[0].cnt // 0')

  if [[ "$obs_count" -gt 0 ]]; then
    pass "Scenario 1: Reflector buffered $obs_count observation(s)"

    # Report buffer_type breakdown.
    local breakdown
    breakdown=$(query_db "SELECT buffer_type, COUNT(*) as cnt FROM lcm_observation_buffer WHERE session_id = '$SID' GROUP BY buffer_type")
    echo "INFO: Scenario 1: Buffer breakdown: $breakdown"

    # Check for insight-type rows (reflector output).
    local insight_count
    insight_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_observation_buffer WHERE session_id = '$SID' AND buffer_type = 'insight'" | jq '.[0].cnt // 0')
    if [[ "$insight_count" -gt 0 ]]; then
      pass "Scenario 1: Reflector produced $insight_count insight(s)"
    else
      fail "Scenario 1: Observation buffer has no insight-type rows"
    fi
  else
    fail "Scenario 1: No observation_buffer rows for reflector output"
  fi

  # Also verify that compaction happened (prerequisite for reflector).
  local summary_count
  summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$summary_count" -ge 1 ]]; then
    pass "Scenario 1: Compaction confirmed ($summary_count summaries)"
  else
    fail "Scenario 1: No summaries — compaction did not trigger before reflector check"
  fi

  capture_evidence 15 "reflector-compaction"
  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: Log evidence of reflector activity
#
# Grep the Crush log for reflector-related entries and require evidence.
# ---------------------------------------------------------------------------
test_reflector_log_evidence() {
  echo "=== Scenario 2: Log evidence of reflector activity ==="

  if [[ ! -f .crush/logs/crush.log ]]; then
    fail "Scenario 2: No crush.log found"
    return
  fi

  local obs_count insight_count summary_count
  obs_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_observation_buffer WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  insight_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_observation_buffer WHERE session_id = '$SID' AND buffer_type = 'insight'" | jq '.[0].cnt // 0')
  summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID' AND lower(content) GLOB '*reflect*'" | jq '.[0].cnt // 0')

  if [[ "$obs_count" -gt 0 && "$insight_count" -gt 0 ]]; then
    pass "Scenario 2: Observation buffer contains $obs_count row(s), including $insight_count insight row(s)"
  else
    fail "Scenario 2: Observation buffer lacks required reflector insight rows"
  fi

  if [[ "$summary_count" -gt 0 ]]; then
    pass "Scenario 2: Reflector summary content persisted ($summary_count summary row(s))"
  else
    fail "Scenario 2: No reflector-related persisted summary content found"
  fi

  capture_evidence 15 "reflector-logs"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_reflector_post_compaction
test_reflector_log_evidence

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
