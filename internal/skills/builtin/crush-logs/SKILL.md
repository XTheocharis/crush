---
name: crush-logs
description: Use when the user needs to find, review, or debug Crush logs — examining the current session's logs, investigating previous sessions, or checking logs for a different project directory.
---

# Crush Logs

Crush writes structured JSON logs to disk for every session. These logs
contain timestamped entries with severity levels, source locations, and
key-value context — useful for debugging provider errors, tool failures,
LSP issues, MCP connection problems, hook execution, and LCM compaction
events.

## Log File Location

Logs are written to:

```
<data_directory>/logs/crush.log
```

Where `<data_directory>` defaults to `~/.local/share/crush` and can be
overridden by `options.data_directory` in `crush.json` or the
`--data-dir` CLI flag.

Each log line is a JSON object with fields: `time`, `level`, `msg`,
`source` (file:line), plus arbitrary key-value pairs.

## Available Tools

### `crush_logs` (agent tool)

Reads the last N JSON log entries from the current session's log file.
Returns formatted log lines with sensitive values (API keys, tokens,
passwords) automatically redacted to `[REDACTED]`.

Parameters:
- `lines` (optional): Number of recent entries to return. Default: 50, max: 100.

Use this for quick diagnostics within an active session — provider errors,
tool failures, hook timeouts, compaction events, etc.

### `crush_info` (agent tool)

Returns runtime state including the active model, provider, LSP/MCP status,
and configured skills. Useful for confirming which log file path is active
and whether subsystems are healthy.

## Reviewing Logs by Scenario

### Current Session

Use the `crush_logs` tool directly. It reads from the log file associated
with the running Crush instance.

```
crush_logs  →  last 50 entries
crush_logs(lines=100)  →  last 100 entries (max)
```

### Previous Sessions in the Current Project

All sessions in the same project share the same `<data_directory>`, so the
log file contains entries from current and recent sessions. Use `grep` or
`rg` on the log file to filter by session ID or time range:

```bash
# Find the log file path
rg -n 'crush.log' <data_directory>/logs/

# Filter by session ID
rg '"session_id":"<SESSION_ID>"' <data_directory>/logs/crush.log

# Filter by ERROR level
rg '"level":"error"' <data_directory>/logs/crush.log

# Filter by time range
rg '"time":"2026-05-3' <data_directory>/logs/crush.log
```

Session IDs appear in most log entries. Find them with:
```bash
rg -o '"session_id":"[^"]*"' <data_directory>/logs/crush.log | sort -u
```

### Sessions in a Different Project Directory

When investigating logs for a different project, the log file may be in a
different data directory (if that project uses `options.data_directory` in
its `crush.json`). Steps:

1. Read the target project's `crush.json` to find its `data_directory`
   override (if any).
2. Default location if no override: `~/.local/share/crush/logs/crush.log`.
3. Use `bash` to read and filter the log file at that path.

```bash
# Check for a custom data directory
cat /path/to/project/crush.json | jq '.options.data_directory'

# Then read from the resolved path
rg '"level":"error"' /path/to/data/logs/crush.log | tail -50
```

## Log Levels

| Level | Description |
|-------|-------------|
| `debug` | Verbose internal details. Only present when `options.debug: true` or `--debug` flag is set. |
| `info` | Normal operational messages (session start, tool calls, compaction events). |
| `warn` | Recoverable issues (hook timeouts, LSP restarts, rate limit backoff). |
| `error` | Failures (provider errors, tool execution errors, MCP disconnections). |

## Sensitive Data

The `crush_logs` tool automatically redacts values for keys containing:
`authorization`, `api-key`, `api_key`, `apikey`, `token`, `secret`,
`password`, `credential`. Redacted values appear as `[REDACTED]`.

When reading the log file directly via `bash`, be aware that raw logs
contain unredacted values. Avoid displaying credentials in conversation
output — pipe through `jq` and strip sensitive fields first:

```bash
cat <data_directory>/logs/crush.log | jq 'del(.api_key, .token, .authorization)'
```

## Common Investigations

### Provider errors
```bash
rg '"level":"error"' <data_directory>/logs/crush.log | rg 'provider\|LLM\|fantasy'
```

### LSP issues
```bash
rg 'lsp\|LSP' <data_directory>/logs/crush.log | tail -30
```

### MCP server connection problems
```bash
rg 'mcp\|MCP' <data_directory>/logs/crush.log | tail -30
```

### Hook execution failures
```bash
rg 'hook\|Hook' <data_directory>/logs/crush.log | tail -30
```

### LCM compaction events
```bash
rg 'compact\|Compact\|LCM\|lcm' <data_directory>/logs/crush.log | tail -30
```

### Rate limiting
```bash
rg 'rate.limit\|429\|ratelimit' <data_directory>/logs/crush.log | tail -20
```

## CLI Alternative

Users can also view logs outside the agent via the CLI:

```bash
crush logs             # last 1000 lines
crush logs -t 50       # last 50 lines
crush logs -f          # follow mode (stream new entries)
crush logs -f -t 100   # follow after showing last 100
```

The CLI pretty-prints JSON log lines with timestamps, levels, and source
locations.
