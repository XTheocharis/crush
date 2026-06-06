-- Crush DB Verification Queries
-- Parameterized with __SESSION_ID__ for session-scoped queries.
-- Usage: sed 's/__SESSION_ID__/<id>/g' db-queries.sql | sqlite3 -json .crush/crush.db

-- ============================================================================
-- Session queries
-- ============================================================================

-- name: session_count
-- Session count
SELECT COUNT(*) as count FROM sessions;

-- name: session_by_id
-- Session by ID
SELECT * FROM sessions WHERE id = '__SESSION_ID__';

-- name: child_sessions
-- Child sessions (forked from parent)
SELECT id, title, created_at FROM sessions WHERE parent_session_id = '__SESSION_ID__';

-- name: session_title
-- Session title
SELECT title FROM sessions WHERE id = '__SESSION_ID__';

-- name: session_stats
-- Session token/cost stats
SELECT prompt_tokens, completion_tokens, cost, message_count FROM sessions WHERE id = '__SESSION_ID__';

-- ============================================================================
-- Message queries
-- ============================================================================

-- name: message_count
-- Message count by session
SELECT COUNT(*) as count FROM messages WHERE session_id = '__SESSION_ID__';

-- name: message_parts_by_type
-- Message parts by type
SELECT part_type, COUNT(*) as count FROM message_parts WHERE session_id = '__SESSION_ID__' GROUP BY part_type;

-- name: messages_with_timestamps
-- Messages with timestamps (chronological)
SELECT id, role, created_at, seq, token_count FROM messages WHERE session_id = '__SESSION_ID__' AND created_at > 0 ORDER BY created_at;

-- name: tool_use_parts
-- Tool use parts (sample)
SELECT content_json FROM message_parts WHERE session_id = '__SESSION_ID__' AND part_type = 'tool_call' LIMIT 5;

-- name: message_roles
-- Message role distribution
SELECT role, COUNT(*) as count FROM messages WHERE session_id = '__SESSION_ID__' GROUP BY role;

-- name: message_timing
-- Message timing (latency analysis)
SELECT id, role, submitted_at, sent_to_llm_at, first_token_at, completed_at FROM messages WHERE session_id = '__SESSION_ID__' AND completed_at > 0 ORDER BY created_at LIMIT 20;

-- ============================================================================
-- LCM queries
-- ============================================================================

-- name: lcm_session_config
-- LCM session config
SELECT * FROM lcm_session_config WHERE session_id = '__SESSION_ID__';

-- name: lcm_context_items
-- Context items with tokens
SELECT position, item_type, token_count, message_id, summary_id FROM lcm_context_items WHERE session_id = '__SESSION_ID__' ORDER BY position;

-- name: lcm_total_context_tokens
-- Total context tokens (messages only)
SELECT SUM(token_count) as total_tokens FROM lcm_context_items WHERE session_id = '__SESSION_ID__' AND item_type = 'message';

-- name: lcm_summaries_by_kind
-- Summaries by kind
SELECT kind, COUNT(*) as count, SUM(token_count) as total_tokens FROM lcm_summaries WHERE session_id = '__SESSION_ID__' GROUP BY kind;

-- name: lcm_auto_memory
-- Auto-memory entries
SELECT memory_type, content, confidence, priority FROM lcm_auto_memory WHERE session_id = '__SESSION_ID__';

-- name: lcm_operational_memory
-- Operational memory
SELECT key, value, priority FROM session_operational_memory WHERE session_id = '__SESSION_ID__';

-- name: lcm_observation_buffer_count
-- Observation buffer count
SELECT COUNT(*) as count FROM lcm_observation_buffer WHERE session_id = '__SESSION_ID__';

-- name: lcm_large_files
-- Large files
SELECT file_id, token_count, original_path FROM lcm_large_files WHERE session_id = '__SESSION_ID__';

-- name: lcm_content_replacements_count
-- Content replacements count
SELECT COUNT(*) as count FROM lcm_content_replacements WHERE session_id = '__SESSION_ID__';

-- name: lcm_reversible_state_count
-- Reversible state count (global)
SELECT COUNT(*) as count FROM lcm_reversible_state;

-- name: lcm_map_runs
-- Map runs
SELECT tool_type, status, input_path FROM lcm_map_runs WHERE session_id = '__SESSION_ID__';

-- name: lcm_map_items
-- Map items (via run_id join)
SELECT mi.item_index, mi.status, mi.output_json FROM lcm_map_items mi WHERE mi.run_id IN (SELECT run_id FROM lcm_map_runs WHERE session_id = '__SESSION_ID__');

-- name: lcm_compacted_items
-- Compacted context items (have summary_id)
SELECT COUNT(*) as count FROM lcm_context_items WHERE session_id = '__SESSION_ID__' AND summary_id IS NOT NULL;

-- ============================================================================
-- Repo Map queries
-- ============================================================================

-- name: repo_map_file_cache_count
-- File cache count (global)
SELECT COUNT(*) as count FROM repo_map_file_cache;

-- name: repo_map_session_rankings
-- Session rankings (top files)
SELECT rel_path, rank FROM repo_map_session_rankings WHERE session_id = '__SESSION_ID__' ORDER BY rank DESC LIMIT 20;

-- name: repo_map_tags_by_language
-- Tag count by language (global)
SELECT language, COUNT(*) as count FROM repo_map_tags GROUP BY language;

-- name: repo_map_imports_count
-- Import count (global)
SELECT COUNT(*) as count FROM repo_map_imports;

-- name: repo_map_imports_from_agent
-- Top imports from agent dir
SELECT path, import_path FROM repo_map_imports WHERE path LIKE '%/agent/%' LIMIT 10;

-- ============================================================================
-- Rewind queries
-- ============================================================================

-- name: turn_snapshots_count
-- Turn snapshots count
SELECT COUNT(*) as count FROM turn_snapshots WHERE session_id = '__SESSION_ID__';

-- name: turn_snapshot_files
-- Snapshot files (joined)
SELECT tsf.snapshot_id, tsf.path, tsf.version FROM turn_snapshot_files tsf JOIN turn_snapshots ts ON ts.id = tsf.snapshot_id WHERE ts.session_id = '__SESSION_ID__';

-- ============================================================================
-- File tracking queries
-- ============================================================================

-- name: read_files
-- Read files
SELECT path FROM read_files WHERE session_id = '__SESSION_ID__' ORDER BY read_at DESC LIMIT 20;

-- name: written_files
-- Written files
SELECT path FROM written_files WHERE session_id = '__SESSION_ID__' ORDER BY written_at DESC LIMIT 20;

-- ============================================================================
-- Eval queries
-- ============================================================================

-- name: eval_runs_count
-- Eval runs count (global)
SELECT COUNT(*) as count FROM eval_runs;

-- name: scorer_results_count
-- Scorer results count (global)
SELECT COUNT(*) as count FROM scorer_results;
