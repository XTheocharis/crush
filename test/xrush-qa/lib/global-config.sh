#!/usr/bin/env bash
# global-config.sh — Resolve and validate user-scoped provider config for xrush-qa.
#
# Provides:
#   resolve_global_config()  — find + validate global crush.json, export CRUSH_GLOBAL_CONFIG
#   validate_provider_config() — check a config file has at least one usable provider
#   generate_project_config() — write wave-specific project-local config WITHOUT providers
#
# Source this file: source test/xrush-qa/lib/global-config.sh
set -euo pipefail

readonly _GLOBAL_CONFIG_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Resolve the user-scoped provider config file path.
# Sets CRUSH_GLOBAL_CONFIG to the directory containing the config if non-default.
# Exports CRUSH_QA_GLOBAL_CONFIG_FILE with the resolved file path.
resolve_global_config() {
  local config_file="${CRUSH_QA_GLOBAL_CONFIG_FILE:-$HOME/.config/crush/crush.json}"
  export CRUSH_QA_GLOBAL_CONFIG_FILE="$config_file"

  if [[ ! -f "$config_file" ]]; then
    echo "ERROR: Global provider config not found: $config_file" >&2
    echo "  Set CRUSH_QA_GLOBAL_CONFIG_FILE or create $HOME/.config/crush/crush.json" >&2
    return 1
  fi

  validate_provider_config "$config_file" || return 1

  local default_dir="$HOME/.config/crush"

  # Only export CRUSH_GLOBAL_CONFIG if using a non-default location.
  if [[ "$(dirname "$config_file")" != "$default_dir" ]]; then
    export CRUSH_GLOBAL_CONFIG="$(dirname "$config_file")"
  else
    unset CRUSH_GLOBAL_CONFIG 2>/dev/null || true
  fi
}

# Validate that a provider config file has at least one usable provider.
validate_provider_config() {
  local config_file="$1"

  if [[ ! -f "$config_file" ]]; then
    echo "ERROR: Config file not found: $config_file" >&2
    return 1
  fi

  local provider_count
  provider_count=$(python3 -c "
import json, sys
with open('$config_file') as f:
    cfg = json.load(f)
providers = cfg.get('providers', {})
count = 0
for name, prov in providers.items():
    api_key = prov.get('api_key', '')
    has_oauth = 'oauth' in prov or 'client_id' in prov
    if api_key or has_oauth:
        count += 1
print(count)
" 2>/dev/null || echo "0")

  if [[ "$provider_count" -lt 1 ]]; then
    echo "ERROR: No usable providers found in $config_file" >&2
    echo "  At least one provider must have an api_key or oauth configuration." >&2
    return 1
  fi
}

# Generate wave-specific project-local config (no providers block).
# Providers come from the user-scoped global config at runtime.
# Hooks are preserved from the wave JSON source file if present.
# Usage: generate_project_config <wave_number> <output_file>
generate_project_config() {
  local wave="$1"
  local output="$2"

  [[ "$wave" =~ ^[1-5]$ ]] || { echo "ERROR: Wave must be 1-5, got: $wave" >&2; return 1; }

  local wave_json="${_GLOBAL_CONFIG_SCRIPT_DIR}/../wave${wave}.json"

  python3 - "$wave" "$wave_json" <<'PYEOF' > "$output"
import json, sys, os

wave = int(sys.argv[1])
wave_json_path = sys.argv[2]

config = {
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

# Preserve hooks from the wave JSON source file if present.
if os.path.isfile(wave_json_path):
    with open(wave_json_path) as f:
        wave_config = json.load(f)
    if "hooks" in wave_config:
        config["hooks"] = wave_config["hooks"]

print(json.dumps(config, indent=2))
PYEOF

  echo "Generated: $output"
}
