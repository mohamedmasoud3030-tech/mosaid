#!/data/data/com.termux/files/usr/bin/bash
# Verify the installed Mosaid product configuration, secrets and binary.
# The checks mirror the bounds enforced by internal/config.Validate,
# so a config that passes here also passes the product's own loader.
set -euo pipefail
umask 077

MOSAID_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
CONFIG_HOME="${MOSAID_CONFIG_HOME:-$HOME/.config/mosaid}"
CONFIG="$MOSAID_HOME/config.json"
BINARY="$MOSAID_HOME/bin/mosaid"
CHECKSUM_FILE="$MOSAID_HOME/BINARY.sha256"

fail() { printf 'VERIFY FAIL: %s\n' "$*" >&2; exit 1; }
pass() { printf 'VERIFY PASS: %s\n' "$*"; }

[[ -f "$CONFIG" ]] || fail "missing config.json"
[[ -x "$BINARY" ]] || fail "missing executable binary"
[[ -f "$CHECKSUM_FILE" ]] || fail "missing binary checksum"

for cmd in jq sha256sum stat; do command -v "$cmd" >/dev/null 2>&1 || fail "missing command: $cmd"; done

expected="$(awk 'NR==1{print $1}' "$CHECKSUM_FILE")"
actual="$(sha256sum "$BINARY" | awk '{print $1}')"
[[ "$actual" == "$expected" ]] || fail "binary SHA-256 mismatch"
pass "binary SHA-256 $actual"

config_mode="$(stat -c '%a' "$CONFIG")"
[[ "$config_mode" == "600" ]] || fail "config mode must be 600, got $config_mode"
pass "config.json permissions are 0600"

jq -e . "$CONFIG" >/dev/null || fail "config is not valid JSON"

owner="$(jq -r '.owner_telegram_id // 0' "$CONFIG")"
[[ "$owner" =~ ^[0-9]+$ ]] && (( owner > 0 )) || fail "owner_telegram_id must be a positive integer"
pass "owner_telegram_id=$owner"

data_dir="$(jq -r '.data_dir // ""' "$CONFIG")"
[[ -n "$data_dir" && "$data_dir" == /* ]] || fail "data_dir must be absolute"
[[ "$data_dir" == /data/* ]] || fail "data_dir must live inside the Termux app directory"
pass "data_dir=$data_dir"

token_file="$(jq -r '.telegram.token_file // ""' "$CONFIG")"
key_file="$(jq -r '.model.api_key_file // ""' "$CONFIG")"
[[ "$token_file" == /* ]] || fail "telegram.token_file must be absolute"
[[ "$key_file" == /* ]] || fail "model.api_key_file must be absolute"

for f in "$token_file" "$key_file"; do
  [[ -f "$f" ]] || fail "missing secret file: $f"
  [[ ! -L "$f" ]] || fail "secret must not be a symlink: $f"
  mode="$(stat -c '%a' "$f")"
  [[ "$mode" == "600" ]] || fail "secret mode must be 600, got $mode for $f"
  lines="$(wc -l < "$f")"
  [[ "$lines" == "1" ]] || fail "secret must be one line: $f"
  [[ -s "$f" ]] || fail "secret is empty: $f"
done
pass "secrets exist with mode 0600"

token="$(cat "$token_file")"
[[ "$token" =~ ^[0-9]{7,12}:[A-Za-z0-9_-]{30,}$ ]] || fail "Telegram token format is invalid"

base="$(jq -r '.model.base_url // ""' "$CONFIG")"
[[ "$base" == https://* && "$base" != *"@"* && "$base" != *"#"* ]] || fail "model.base_url must be HTTPS without credentials or fragment"
model_name="$(jq -r '.model.name // ""' "$CONFIG")"
[[ -n "$model_name" && "${#model_name}" -le 128 ]] || fail "model.name required and at most 128 characters"
pass "model endpoint=$base model=$model_name"

poll="$(jq -r '.telegram.poll_timeout_seconds // 0' "$CONFIG")"
model_timeout="$(jq -r '.model.timeout_seconds // 0' "$CONFIG")"
[[ "$poll" =~ ^[0-9]+$ ]] && (( poll >= 1 && poll <= 60 )) || fail "poll_timeout_seconds must be 1..60"
[[ "$model_timeout" =~ ^[0-9]+$ ]] && (( model_timeout >= 1 && model_timeout <= 300 )) || fail "model.timeout_seconds must be 1..300"
pass "network timeouts bounded"

bound() { # bound <json-value> <min> <max> <label>
  local v="$1" lo="$2" hi="$3" label="$4"
  [[ "$v" =~ ^[0-9]+$ ]] && (( v >= lo && v <= hi )) || fail "$label must be $lo..$hi"
}

bound "$(jq -r '.limits.max_message_bytes // 0' "$CONFIG")" 1 1048576 "limits.max_message_bytes"
bound "$(jq -r '.limits.max_response_bytes // 0' "$CONFIG")" 1 4194304 "limits.max_response_bytes"
bound "$(jq -r '.limits.max_model_steps // 0' "$CONFIG")" 1 32 "limits.max_model_steps"
bound "$(jq -r '.limits.max_tool_calls // 0' "$CONFIG")" 1 128 "limits.max_tool_calls"
bound "$(jq -r '.limits.max_tokens // 0' "$CONFIG")" 1 1000000 "limits.max_tokens"
bound "$(jq -r '.limits.max_retries // 0' "$CONFIG")" 1 20 "limits.max_retries"
bound "$(jq -r '.limits.messages_per_minute // 0' "$CONFIG")" 1 600 "limits.messages_per_minute"
bound "$(jq -r '.limits.message_burst // 0' "$CONFIG")" 1 600 "limits.message_burst"

cost="$(jq -r '.limits.max_cost_usd // 0' "$CONFIG")"
[[ "$cost" =~ ^[0-9]+([.][0-9]+)?$ ]] || fail "limits.max_cost_usd must be numeric"
awk -v c="$cost" 'BEGIN{exit !(c > 0 && c <= 100)}' || fail "limits.max_cost_usd must be > 0 and <= 100"

burst="$(jq -r '.limits.message_burst' "$CONFIG")"
per_min="$(jq -r '.limits.messages_per_minute' "$CONFIG")"
(( burst <= per_min )) || fail "limits.message_burst must not exceed limits.messages_per_minute"
pass "limits bounded and fail-closed"

echo "VERIFY: all product configuration checks passed"
