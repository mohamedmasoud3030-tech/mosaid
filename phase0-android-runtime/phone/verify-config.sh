#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
umask 077

P0_HOME="${PICOCLAW_PHASE0_HOME:-$HOME/.local/share/picoclaw-phase0}"
CONFIG="$P0_HOME/config.json"
SECRETS="$P0_HOME/.security.yml"
BINARY="$P0_HOME/bin/picoclaw-phase0"
CHECKSUM_FILE="$P0_HOME/BINARY.sha256"

fail() { printf 'VERIFY FAIL: %s\n' "$*" >&2; exit 1; }
pass() { printf 'VERIFY PASS: %s\n' "$*"; }

[[ -f "$CONFIG" ]] || fail "missing config.json"
[[ -f "$SECRETS" ]] || fail "missing .security.yml"
[[ -x "$BINARY" ]] || fail "missing executable binary"
[[ -f "$CHECKSUM_FILE" ]] || fail "missing binary checksum"

for cmd in jq sha256sum stat; do command -v "$cmd" >/dev/null 2>&1 || fail "missing command: $cmd"; done

expected="$(awk 'NR==1{print $1}' "$CHECKSUM_FILE")"
actual="$(sha256sum "$BINARY" | awk '{print $1}')"
[[ "$actual" == "$expected" ]] || fail "binary SHA-256 mismatch"
pass "binary SHA-256 $actual"

config_mode="$(stat -c '%a' "$CONFIG")"
secret_mode="$(stat -c '%a' "$SECRETS")"
[[ "$config_mode" == "600" ]] || fail "config mode must be 600, got $config_mode"
[[ "$secret_mode" == "600" ]] || fail "secrets mode must be 600, got $secret_mode"
pass "config and secrets permissions are 0600"

jq -e . "$CONFIG" >/dev/null || fail "config is not valid JSON"
jq -e . "$SECRETS" >/dev/null || fail ".security.yml must contain the installer-generated JSON/YAML subset"

owner="$(jq -r '.channel_list.telegram.allow_from[0] // ""' "$CONFIG")"
[[ "$owner" =~ ^[0-9]+$ ]] || fail "owner must be one numeric Telegram user ID"
[[ "$(jq '.channel_list.telegram.allow_from | length' "$CONFIG")" == "1" ]] || fail "exactly one owner is required"
[[ "$(jq -r '.channel_list.telegram.enabled' "$CONFIG")" == "true" ]] || fail "Telegram must be enabled"

other_enabled="$(jq -r '[.channel_list | to_entries[] | select(.key != "telegram" and .value.enabled == true)] | length' "$CONFIG")"
[[ "$other_enabled" == "0" ]] || fail "a non-Telegram channel is enabled"

[[ "$(jq -r '.agents.defaults.turn_profile.enabled' "$CONFIG")" == "true" ]] || fail "turn profile must be enabled"
[[ "$(jq -r '.agents.defaults.turn_profile.tools.mode' "$CONFIG")" == "off" ]] || fail "turn profile tools must be off"
[[ "$(jq -r '.agents.defaults.turn_profile.skills.mode' "$CONFIG")" == "off" ]] || fail "turn profile skills must be off"
[[ "$(jq -r '.heartbeat.enabled' "$CONFIG")" == "false" ]] || fail "heartbeat must be disabled"
[[ "$(jq -r '.evolution.enabled' "$CONFIG")" == "false" ]] || fail "evolution must be disabled"
[[ "$(jq -r '.hooks.enabled' "$CONFIG")" == "false" ]] || fail "hooks must be disabled"

# Every tool-level enabled field in the Phase 0 config must be false.
enabled_paths="$(jq -r '
  paths(scalars) as $p
  | select(($p[-1] == "enabled") and ($p[0] == "tools") and (getpath($p) == true))
  | $p | map(tostring) | join(".")
' "$CONFIG")"
[[ -z "$enabled_paths" ]] || fail "dangerous tool flags enabled: $enabled_paths"
[[ "$(jq -r '.tools.exec.allow_remote' "$CONFIG")" == "false" ]] || fail "remote exec must be false"
[[ "$(jq -r '.tools.cron.allow_command' "$CONFIG")" == "false" ]] || fail "cron commands must be false"
[[ "$(jq -r '.tools.filter_sensitive_data' "$CONFIG")" == "true" ]] || fail "sensitive-data filtering must be enabled"

bot_token="$(jq -r '.channels.telegram.settings.token // .channel_list.telegram.settings.token // ""' "$SECRETS")"
api_key="$(jq -r '.model_list["phase0-model"].api_keys[0] // ""' "$SECRETS")"
[[ -n "$bot_token" && "$bot_token" != "null" ]] || fail "Telegram token missing from secrets"
[[ -n "$api_key" && "$api_key" != "null" ]] || fail "model API key missing from secrets"

if grep -Fq "$bot_token" "$CONFIG" || grep -Fq "$api_key" "$CONFIG"; then
  fail "a secret appears in config.json"
fi

# PicoClaw currently exits zero for some config-load errors, so inspect its
# status output rather than trusting the exit code alone. This command performs
# no model or Telegram request.
status_output="$(PICOCLAW_PHASE0_QUALIFICATION=1 PICOCLAW_CONFIG="$CONFIG" PICOCLAW_HOME="$P0_HOME" "$BINARY" status 2>&1 || true)"
if grep -Eqi 'failed to load config|error loading config|unknown field' <<<"$status_output"; then
  fail "PicoClaw rejected the generated configuration schema"
fi
if grep -Fq "$bot_token" <<<"$status_output" || grep -Fq "$api_key" <<<"$status_output"; then
  fail "PicoClaw status output exposed a secret"
fi
pass "one-owner private qualification configuration; tools, skills, MCP, cron, heartbeat and evolution are off"
