#!/usr/bin/env bash
# provider-setup.sh — Generate wave-specific crush.json configs for xrush-qa.
#
# Usage: bash provider-setup.sh <wave-number> [output-file]
#   wave-number : 1–5
#   output-file : defaults to test/xrush-qa/wave<N>.json

set -euo pipefail

# ── Constants ────────────────────────────────────────────────────────────────

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly QA_DIR="$(dirname "$SCRIPT_DIR")"

readonly USER_CONFIG="${HOME}/.config/crush/crush.json"

# ── Helpers ──────────────────────────────────────────────────────────────────

die() { echo "ERROR: $*" >&2; exit 1; }

# Check for jq availability.
if command -v jq &>/dev/null; then
  HAS_JQ=1
else
  HAS_JQ=0
fi

# ── Validate args ────────────────────────────────────────────────────────────

WAVE="${1:-}"
OUTPUT="${2:-}"

[[ -z "$WAVE" ]] && die "Usage: bash provider-setup.sh <wave-number> [output-file]"
[[ "$WAVE" =~ ^[1-5]$ ]] || die "Wave must be 1-5, got: $WAVE"

if [[ -z "$OUTPUT" ]]; then
  OUTPUT="${QA_DIR}/wave${WAVE}.json"
fi

# ── Read user-level providers ────────────────────────────────────────────────

if [[ -f "$USER_CONFIG" ]]; then
  PROVIDERS_JSON=$(python3 -c "
import json, sys
with open('$USER_CONFIG') as f:
    cfg = json.load(f)
providers = cfg.get('providers', {})
# Only keep zai and zai-2
kept = {k: v for k, v in providers.items() if k in ('zai', 'zai-2')}
print(json.dumps(kept, indent=2))
" 2>/dev/null || echo "{}")
else
  [[ -n "${ZAI_API_KEY:-}" ]] || die "Missing $USER_CONFIG and ZAI_API_KEY"
  [[ -n "${ZAI_2_API_KEY:-}" ]] || die "Missing $USER_CONFIG and ZAI_2_API_KEY"
  PROVIDERS_JSON='{
  "zai": {
    "id": "zai",
    "name": "Z.AI Coding Plan (Primary)",
    "base_url": "https://api.z.ai/api/coding/paas/v4",
    "type": "openai-compat",
    "api_key": "'"$ZAI_API_KEY"'",
    "disable": false,
    "flat_rate": true,
    "models": [
      {"id": "glm-5.1", "name": "GLM-5.1", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-5-turbo", "name": "GLM-5-Turbo", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-5", "name": "GLM-5", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-4.7", "name": "GLM-4.7", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-4.6", "name": "GLM-4.6", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-4.5", "name": "GLM-4.5", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-4.5-air", "name": "GLM-4.5-Air", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true}
    ]
  },
  "zai-2": {
    "id": "zai-2",
    "name": "Z.AI Coding Plan (Secondary)",
    "base_url": "https://api.z.ai/api/coding/paas/v4",
    "type": "openai-compat",
    "api_key": "'"$ZAI_2_API_KEY"'",
    "disable": false,
    "flat_rate": true,
    "models": [
      {"id": "glm-5.1", "name": "GLM-5.1", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-5-turbo", "name": "GLM-5-Turbo", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-5", "name": "GLM-5", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-4.7", "name": "GLM-4.7", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-4.6", "name": "GLM-4.6", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-4.5", "name": "GLM-4.5", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true},
      {"id": "glm-4.5-air", "name": "GLM-4.5-Air", "cost_per_1m_in": 0, "cost_per_1m_out": 0, "context_window": 200000, "default_max_tokens": 131072, "can_reason": true}
    ]
  }
}'
fi

# ── Build config via python3 ─────────────────────────────────────────────────

python3 - "$WAVE" "$PROVIDERS_JSON" <<'PYEOF' > "$OUTPUT"
import json, sys

wave = int(sys.argv[1])
providers = json.loads(sys.argv[2])

# ── Base config (all waves) ──────────────────────────────────────────────

config = {
    "providers": providers,
    "models": {
        "large": {"model": "glm-5.1", "provider": "zai"},
        "small": {"model": "glm-5-turbo", "provider": "zai-2"}
    },
    "options": {
        "architect_model": {"model": "glm-5.1", "provider": "zai-2"},
        "editor_model": {"model": "glm-5-turbo", "provider": "zai-2"},
        "lcm": {
            "summarizer_model": {"model": "glm-5-turbo", "provider": "zai-2"}
        }
    }
}

# ── Wave-specific overrides ──────────────────────────────────────────────

if wave == 2:
    config["options"]["repo_map"] = {"disabled": False}

elif wave == 3:
    config["options"]["lcm"].update({
        "ctx_cutoff_threshold": 0.1,
        "auto_memory_interval": 1,
        "operational_memory_enabled": True,
        "observation": {"enabled": True},
        "large_tool_output_token_threshold": 100,
        "disable_large_tool_output": False,
    })

elif wave == 4:
    config["options"].update({
        "architect": {"approval_required": False},
        "validation": {
            "enabled": True,
            "auto_fix": True,
            "autofix_loop_enabled": True,
        },
        "doom_loop_intervention": "full",
    })

elif wave == 5:
    config["options"]["processors"] = {
        "enabled": True,
        "list": ["token_limiter", "system_prompt_scrubber", "pii_detector"],
    }

print(json.dumps(config, indent=2))
PYEOF

echo "Generated: $OUTPUT"
