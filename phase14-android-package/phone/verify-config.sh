#!/data/data/com.termux/files/usr/bin/bash
# Fail-closed config and secret verification for the Mosaid phone runtime.
set -euo pipefail

M_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
CONFIG="$M_HOME/config.json"
TOKEN_FILE="$M_HOME/secrets/telegram.token"
KEY_FILE="$M_HOME/secrets/model.key"

fail() { echo "VERIFY FAIL: $*" >&2; exit 1; }
for cmd in jq stat readlink; do command -v "$cmd" >/dev/null 2>&1 || fail "missing command: $cmd"; done

[[ -f "$CONFIG" ]] || fail "config not found: $CONFIG"
[[ "$(stat -c '%a' "$CONFIG")" == "600" ]] || fail "config must be mode 600"

check_secret_file() {
  local path="$1" label="$2"
  [[ -f "$path" ]] || fail "$label not found: $path"
  [[ -L "$path" ]] && fail "$label must not be a symlink"
  [[ "$(stat -c '%a' "$path")" == "600" ]] || fail "$label must be mode 600"
  local value
  value="$(tr -d '\r' < "$path")"
  [[ -n "$value" ]] || fail "$label is empty"
  [[ "$value" == *$'\n'* ]] && fail "$label must be a single line"
  [[ "${#value}" -le 65536 ]] || fail "$label is too long"
}

check_secret_file "$TOKEN_FILE" "telegram.token"
check_secret_file "$KEY_FILE" "model.key"

jq -e . "$CONFIG" >/dev/null || fail "config is not valid JSON"
[[ "$(jq -r '.owner_telegram_id' "$CONFIG")" =~ ^[1-9][0-9]*$ ]] || fail "owner_telegram_id must be a positive integer"
[[ "$(jq -r '.data_dir' "$CONFIG")" == /* ]] || fail "data_dir must be absolute"
[[ "$(jq -r '.telegram.token_file' "$CONFIG")" == /* ]] || fail "telegram.token_file must be absolute"
[[ "$(jq -r '.model.api_key_file' "$CONFIG")" == /* ]] || fail "model.api_key_file must be absolute"
[[ "$(jq -r '.model.base_url' "$CONFIG")" == https://* ]] || fail "model.base_url must be HTTPS"
[[ -n "$(jq -r '.model.name // empty' "$CONFIG")" ]] || fail "model.name is required"
[[ "$(jq -r '.limits.max_cost_usd' "$CONFIG")" == "0.01" ]] || fail "free-only tripwire changed: limits.max_cost_usd must stay 0.01"

TOKEN_VALUE="$(tr -d '\r' < "$TOKEN_FILE")"
KEY_VALUE="$(tr -d '\r' < "$KEY_FILE")"
[[ "$TOKEN_VALUE" =~ ^[0-9]{7,12}:[A-Za-z0-9_-]{30,}$ ]] || fail "telegram.token format is invalid"
[[ "${#KEY_VALUE}" -ge 16 ]] || fail "model.key is too short"

echo "verify-config: OK"
