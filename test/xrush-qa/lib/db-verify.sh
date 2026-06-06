#!/usr/bin/env bash
set -euo pipefail

CRUSH_DB="${CRUSH_DB:-.crush/crush.db}"
XRUSH_QA_LIB="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DB_QUERIES_SQL="$XRUSH_QA_LIB/db-queries.sql"

_db_query() {
  sqlite3 -json "$CRUSH_DB" "$1"
}

_db_query_with_session() {
  local query="$1"
  local session_id="$2"
  local expanded
  expanded=$(sed "s/__SESSION_ID__/${session_id}/g" <<< "$query")
  sqlite3 -json "$CRUSH_DB" "$expanded"
}

run_query() {
  local query_name="$1"
  local session_id="${2:-}"
  local query
  query=$(awk -v qn="$query_name" '
    /^-- name:/ {
      if (found) { found=0 }
      sub(/^-- name: /, "")
      if ($0 == qn) { found=1; next }
      next
    }
    found && /^--/ { next }
    found && /^$/ { next }
    found { buf = buf ? buf " " $0 : $0 }
    END { print buf }
  ' "$DB_QUERIES_SQL")
  if [[ -z "$query" ]]; then
    echo "ERROR: query '$query_name' not found in $DB_QUERIES_SQL" >&2
    return 1
  fi
  if [[ -n "$session_id" ]]; then
    _db_query_with_session "$query" "$session_id"
  else
    _db_query "$query"
  fi
}

assert_table_has_rows() {
  local table_name="$1"
  local min_rows="$2"
  local session_id="${3:-}"
  local count
  if [[ -n "$session_id" ]]; then
    count=$(_db_query "SELECT COUNT(*) as count FROM ${table_name} WHERE session_id = '${session_id}';" | jq '.[0].count')
  else
    count=$(_db_query "SELECT COUNT(*) as count FROM ${table_name};" | jq '.[0].count')
  fi
  if (( count < min_rows )); then
    echo "FAIL: ${table_name} has ${count} rows, expected at least ${min_rows}${session_id:+ (session: ${session_id})}" >&2
    return 1
  fi
  echo "OK: ${table_name} has ${count} rows (>= ${min_rows})"
}

assert_table_count() {
  local table_name="$1"
  local expected_count="$2"
  local session_id="${3:-}"
  local count
  if [[ -n "$session_id" ]]; then
    count=$(_db_query "SELECT COUNT(*) as count FROM ${table_name} WHERE session_id = '${session_id}';" | jq '.[0].count')
  else
    count=$(_db_query "SELECT COUNT(*) as count FROM ${table_name};" | jq '.[0].count')
  fi
  if (( count != expected_count )); then
    echo "FAIL: ${table_name} has ${count} rows, expected exactly ${expected_count}${session_id:+ (session: ${session_id})}" >&2
    return 1
  fi
  echo "OK: ${table_name} has ${count} rows (== ${expected_count})"
}

assert_table_empty() {
  local table_name="$1"
  local session_id="${2:-}"
  local count
  if [[ -n "$session_id" ]]; then
    count=$(_db_query "SELECT COUNT(*) as count FROM ${table_name} WHERE session_id = '${session_id}';" | jq '.[0].count')
  else
    count=$(_db_query "SELECT COUNT(*) as count FROM ${table_name};" | jq '.[0].count')
  fi
  if (( count > 0 )); then
    echo "FAIL: ${table_name} is not empty (${count} rows)${session_id:+ (session: ${session_id})}" >&2
    return 1
  fi
  echo "OK: ${table_name} is empty"
}

dump_session_state() {
  local session_id="$1"
  local tables
  tables=$(sqlite3 "$CRUSH_DB" "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE '%_fts%' AND name NOT LIKE 'goose_%' ORDER BY name;")
  local first=1
  echo "{"
  while IFS= read -r table; do
    [[ -z "$table" ]] && continue
    local has_session_col
    has_session_col=$(sqlite3 "$CRUSH_DB" "SELECT COUNT(*) FROM pragma_table_info('${table}') WHERE name='session_id';")
    local data
    if [[ "$has_session_col" -eq 1 ]]; then
      data=$(_db_query "SELECT * FROM ${table} WHERE session_id = '${session_id}';")
    else
      data=$(_db_query "SELECT COUNT(*) as count FROM ${table};")
    fi
    if [[ "$first" -eq 1 ]]; then
      first=0
    else
      echo ","
    fi
    printf '  "%s": %s' "$table" "$data"
  done <<< "$tables"
  echo ""
  echo "}"
}
