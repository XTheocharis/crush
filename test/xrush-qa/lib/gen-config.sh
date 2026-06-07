#!/usr/bin/env bash
set -euo pipefail

# gen-config.sh — Generate crush.json config for xrush-qa wave tests.
#
# Usage:
#   bash gen-config.sh <wave-number> [--output <file>] [--override key=value ...]
#
# Waves 1–5 each apply layer-specific overrides on top of the base config.
# Additional --override key=value pairs are merged last (highest priority).
# Does NOT include a providers block — providers come from global config.

usage() {
  cat <<'EOF'
Usage: bash gen-config.sh <wave-number> [options]

Arguments:
  wave-number    Wave 1-5 (each applies different overrides)

Options:
  --output FILE  Write JSON to FILE instead of stdout
  --override K=V Apply additional override (repeatable, dot-notation paths)
  --help         Show this message

Wave overrides:
  1  Base config only
  2  options.repo_map.disabled=false
  3  LCM overrides (ctx_cutoff_threshold, auto_memory, observation, etc.)
  4  Orchestration overrides (validation, doom_loop, architect)
  5  Processor overrides (token_limiter, system_prompt_scrubber, pii_detector)
EOF
}

WAVE=""
OUTPUT=""
OVERRIDES=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --help|-h)
      usage
      exit 0
      ;;
    --output)
      OUTPUT="$2"
      shift 2
      ;;
    --override)
      OVERRIDES+=("$2")
      shift 2
      ;;
    -*)
      echo "error: unknown flag: $1" >&2
      usage >&2
      exit 1
      ;;
    *)
      if [[ -z "$WAVE" ]]; then
        WAVE="$1"
        shift
      else
        echo "error: unexpected argument: $1" >&2
        usage >&2
        exit 1
      fi
      ;;
  esac
done

if [[ -z "$WAVE" ]]; then
  echo "error: wave number is required" >&2
  usage >&2
  exit 1
fi
if ! [[ "$WAVE" =~ ^[1-5]$ ]]; then
  echo "error: wave number must be 1-5, got: $WAVE" >&2
  exit 1
fi

read -r -d '' BASE_JSON <<'JSONEOF' || true
{
  "models": {
    "large": "glm-5.1/zai",
    "small": "glm-5-turbo/zai-2"
  },
  "options": {
    "architect_model": "glm-5.1/zai",
    "editor_model": "glm-5.1/zai",
    "lcm": {
      "summarizer_model": "glm-5-turbo/zai-2"
    }
  }
}
JSONEOF

wave_patch_json() {
  local wave="$1"
  case "$wave" in
    1) echo '{}' ;;
    2) echo '{"options":{"repo_map":{"disabled":false}}}' ;;
    3) echo '{"options":{"lcm":{"ctx_cutoff_threshold":0.1,"auto_memory_interval":1,"operational_memory_enabled":true,"observation":{"enabled":true},"large_tool_output_token_threshold":100}}}' ;;
    4) echo '{"options":{"validation":{"enabled":true,"auto_fix":true,"autofix_loop_enabled":true},"doom_loop_intervention":"full","architect":{"approval_required":false}}}' ;;
    5) echo '{"options":{"processors":{"enabled":true,"list":["token_limiter","system_prompt_scrubber","pii_detector"]}}}' ;;
  esac
}

override_to_json() {
  local pair="$1"
  python3 -c "
import sys, json

def set_deep(d, keys, value):
    for k in keys[:-1]:
        d = d.setdefault(k, {})
    d[keys[-1]] = value

pair = sys.argv[1]
path, raw = pair.split('=', 1)
keys = path.split('.')
try:
    value = json.loads(raw)
except (json.JSONDecodeError, ValueError):
    value = raw
result = {}
set_deep(result, keys, value)
print(json.dumps(result))
" "$pair"
}

deep_merge() {
  python3 -c "
import sys, json

def merge(base, override):
    for k, v in override.items():
        if k in base and isinstance(base[k], dict) and isinstance(v, dict):
            merge(base[k], v)
        else:
            base[k] = v
    return base

args = sys.argv[1:]
result = json.loads(args[0])
for patch_json in args[1:]:
    merge(result, json.loads(patch_json))
print(json.dumps(result))
" "$@"
}

PATCHES=()
PATCHES+=("$(wave_patch_json "$WAVE")")

for ov in "${OVERRIDES[@]+"${OVERRIDES[@]}"}"; do
  PATCHES+=("$(override_to_json "$ov")")
done

RESULT=$(deep_merge "$BASE_JSON" "${PATCHES[@]}")

if [[ -n "$OUTPUT" ]]; then
  echo "$RESULT" | jq '.' > "$OUTPUT"
else
  echo "$RESULT" | jq '.'
fi
