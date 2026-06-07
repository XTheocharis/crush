#!/usr/bin/env bash
# Test: LCM reflector and observation buffer via TUI.
# Verifies that the BufferingCoordinator collects observations and that
# the ReflectorAgent triggers reflection cycles when token thresholds are
# crossed. Uses deterministic sentinels for TUI-based assertion.
set -euo pipefail

WAVE=3
SCENARIO="reflector-init"
source "$(dirname "$0")/../lib/common.sh"
source "$(dirname "$0")/../lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

cleanup_test() { cleanup_tui; restore_crush; }
trap cleanup_test EXIT

# Shared session ID across scenarios (set in Scenario 1, reused in Scenario 2).
SID=""

# ---------------------------------------------------------------------------
# Scenario 1: Reflector triggered post-compaction
# ---------------------------------------------------------------------------
test_reflector_post_compaction() {
  SCENARIO="reflector-s1"
  echo "=== Scenario 1: Reflector triggered post-compaction ==="

  setup_clean_crush
  start_crush_tui 3

  # Prompt 1: Long prompt to accumulate tokens past the reflector threshold.
  focus_editor
  send_tui_prompt "Explain the entire internal/lcm/ package in exhaustive detail. For every .go file, list its purpose, all exported functions and types, and describe how they interact with the rest of the LCM pipeline. Include details about the Manager, Store, Compactor, ReflectorAgent, BufferingCoordinator, Explorer subsystem, and all 9 compaction layers. End your reply with the sentinel REFLECTOR_SENTINEL_42"
  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after first prompt"
    capture_tui_evidence "reflector-prompt1-timeout"
    return
  fi

  if assert_tui_contains "REFLECTOR_SENTINEL_42"; then
    pass "Scenario 1: TUI contains sentinel REFLECTOR_SENTINEL_42 from first prompt"
  else
    fail "Scenario 1: TUI missing sentinel REFLECTOR_SENTINEL_42"
    capture_tui_evidence "reflector-prompt1-missing"
    return
  fi

  # Prompt 2: More token accumulation via repomap and tree-sitter.
  focus_editor
  send_tui_prompt "Now describe the internal/repomap/ and internal/treesitter/ packages in full detail. Explain how PageRank is used for file ranking, how the token budget is allocated, how tree-sitter grammars are loaded and pooled, and how tag extraction works across all 28 supported languages. Include code-level details about the FileGraph, edge types, and rendering pipeline."
  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after second prompt"
    capture_tui_evidence "reflector-prompt2-timeout"
    return
  fi

  # Prompt 3: Agent orchestration details.
  focus_editor
  send_tui_prompt "Describe the full agent orchestration system in internal/agent/. Cover the Coordinator, Operator, Parallel, and Swarm patterns in detail. Explain how forked sessions work, how structured sub-agents produce typed output, how the doom-loop detector operates, and how the model router and tier-based routing decide which model to use."
  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after third prompt"
    capture_tui_evidence "reflector-prompt3-timeout"
    return
  fi

  # Prompt 4: Processor pipeline and eval.
  focus_editor
  send_tui_prompt "Explain the internal/processor/ pipeline and internal/eval/ harness in detail. Describe all four processor phases, the 15 configurable processors, how TokenLimiter and PIIDetector work, and how the eval runner loads JSON datasets and runs through scorers. Also explain the rewind/snapshot system in internal/rewind/."
  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle after fourth prompt"
    capture_tui_evidence "reflector-prompt4-timeout"
    return
  fi

  # Get the session ID.
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_tui_evidence "reflector-no-sid"
    return
  fi

  # SECONDARY: check lcm_observation_buffer for reflector output.
  local obs_count
  obs_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_observation_buffer WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$obs_count" -gt 0 ]]; then
    pass "Scenario 1: Reflector buffered $obs_count observation(s)"
  else
    fail "Scenario 1: No observation_buffer rows for reflector output"
  fi

  # SECONDARY: check for insight-type rows (reflector output).
  local insight_count
  insight_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_observation_buffer WHERE session_id = '$SID' AND buffer_type = 'insight'" | jq '.[0].cnt // 0')
  if [[ "$insight_count" -gt 0 ]]; then
    pass "Scenario 1: Reflector produced $insight_count insight(s)"
  else
    fail "Scenario 1: Observation buffer has no insight-type rows"
  fi

  # SECONDARY: verify compaction happened (prerequisite for reflector).
  local summary_count
  summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID'" | jq '.[0].cnt // 0')
  if [[ "$summary_count" -ge 1 ]]; then
    pass "Scenario 1: Compaction confirmed ($summary_count summaries)"
  else
    fail "Scenario 1: No summaries — compaction did not trigger before reflector check"
  fi

  capture_tui_evidence "reflector-compaction-final"
}

# ---------------------------------------------------------------------------
# Scenario 2: Reflection analysis via sentinel
# ---------------------------------------------------------------------------
test_reflector_analysis() {
  SCENARIO="reflector-s2"
  echo "=== Scenario 2: Reflection analysis via sentinel ==="

  if [[ -z "$SID" ]]; then
    fail "Scenario 2: No session ID from Scenario 1 — skipping"
    return
  fi

  focus_editor
  send_tui_prompt "Reflect on our conversation so far and provide a concise analysis of what topics we covered. End your reflection with the sentinel REFLECTOR_ANALYSIS_77"
  if ! wait_for_tui_idle 180; then
    fail "Scenario 2: Crush did not become idle within 180s"
    capture_tui_evidence "reflector-analysis-timeout"
    return
  fi

  # PRIMARY: assert TUI contains the reflection sentinel.
  if assert_tui_contains "REFLECTOR_ANALYSIS_77"; then
    pass "Scenario 2: TUI contains sentinel REFLECTOR_ANALYSIS_77"
  else
    fail "Scenario 2: TUI missing sentinel REFLECTOR_ANALYSIS_77"
    capture_tui_evidence "reflector-analysis-missing"
    return
  fi

  # SECONDARY: verify reflector insight rows persisted.
  local insight_count
  insight_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_observation_buffer WHERE session_id = '$SID' AND buffer_type = 'insight'" | jq '.[0].cnt // 0')
  if [[ "$insight_count" -gt 0 ]]; then
    pass "Scenario 2: Reflector insight rows persisted ($insight_count)"
  else
    fail "Scenario 2: No reflector insight rows found"
  fi

  # SECONDARY: check for reflector-related summary content.
  local summary_count
  summary_count=$(query_db "SELECT COUNT(*) as cnt FROM lcm_summaries WHERE session_id = '$SID' AND lower(content) GLOB '*reflect*'" | jq '.[0].cnt // 0')
  if [[ "$summary_count" -gt 0 ]]; then
    pass "Scenario 2: Reflector summary content persisted ($summary_count summary row(s))"
  else
    fail "Scenario 2: No reflector-related persisted summary content found"
  fi

  capture_tui_evidence "reflector-analysis-final"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_reflector_post_compaction
test_reflector_analysis

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
