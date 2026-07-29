#!/usr/bin/env bash
set -euo pipefail

readonly EXPECTED_REF="b8ceba97ed0b2bf0255cc5c8c61c9110a026cda4"
readonly MOSAID_ROOT="/opt/mosaid"
readonly MOSAID_DATA="/var/lib/mosaid"
readonly HERMES_HOME="${MOSAID_DATA}/hermes"
readonly ENV_FILE="${HERMES_HOME}/.env"
readonly CONFIG_FILE="${HERMES_HOME}/config.yaml"
readonly HERMES_BIN="${MOSAID_ROOT}/current/.venv/bin/hermes"

fail() {
  printf 'mosaid-preflight: %s\n' "$*" >&2
  exit 1
}

read_env() {
  local key="$1"
  local value
  value="$(sed -n "s/^${key}=//p" "${ENV_FILE}" | tail -n 1)"
  [[ -n "${value}" ]] || fail "missing environment value: ${key}"
  printf '%s' "${value}"
}

[[ -L "${MOSAID_ROOT}/current" ]] || fail "current release symlink is missing"
[[ "$(basename "$(readlink -f "${MOSAID_ROOT}/current")")" == "${EXPECTED_REF}" ]] || fail "current release is not the reviewed Hermes commit"
[[ -x "${HERMES_BIN}" ]] || fail "Hermes console script is missing or not executable"

[[ -f "${ENV_FILE}" && ! -L "${ENV_FILE}" ]] || fail ".env must be a regular non-symlink file"
[[ -f "${CONFIG_FILE}" && ! -L "${CONFIG_FILE}" ]] || fail "config.yaml must be a regular non-symlink file"

[[ "$(stat -c '%a' "${ENV_FILE}")" == "600" ]] || fail ".env mode must be 0600"
[[ "$(stat -c '%a' "${CONFIG_FILE}")" == "600" ]] || fail "config.yaml mode must be 0600"
[[ "$(stat -c '%U' "${ENV_FILE}")" == "mosaid" ]] || fail ".env must be owned by mosaid"
[[ "$(stat -c '%U' "${CONFIG_FILE}")" == "mosaid" ]] || fail "config.yaml must be owned by mosaid"

if grep -Eq 'REPLACE_|CHANGE_ME|YOUR_[A-Z_]+' "${ENV_FILE}" "${CONFIG_FILE}"; then
  fail "unresolved deployment placeholder found"
fi

[[ "$(read_env HERMES_REF)" == "${EXPECTED_REF}" ]] || fail "HERMES_REF mismatch"
[[ "$(read_env MOSAID_BILLING_MODE)" == "free_only" ]] || fail "billing mode is not free_only"
[[ "$(read_env MOSAID_MAX_SPEND_USD)" == "0" ]] || fail "maximum spend is not zero"
[[ "$(read_env MOSAID_ALLOW_PAID_FALLBACK)" == "false" ]] || fail "paid fallback is enabled"
[[ "$(read_env MOSAID_UNKNOWN_COST_POLICY)" == "deny" ]] || fail "unknown cost policy is not deny"
[[ "$(read_env MOSAID_PUBLIC_DASHBOARD)" == "false" ]] || fail "public dashboard is enabled"

telegram_user="$(read_env TELEGRAM_ALLOWED_USERS)"
[[ "${telegram_user}" =~ ^[0-9]+$ ]] || fail "first gate requires exactly one numeric Telegram owner"
telegram_token="$(read_env TELEGRAM_BOT_TOKEN)"
[[ "${telegram_token}" =~ ^[0-9]{6,}:[A-Za-z0-9_-]{20,}$ ]] || fail "Telegram token format is invalid"

model_url="$(read_env MODEL_BASE_URL)"
case "${model_url}" in
  https://*|http://127.0.0.1:*|http://localhost:*) ;;
  *) fail "model endpoint must use HTTPS or loopback HTTP" ;;
esac

[[ -n "$(read_env MODEL_NAME)" ]] || fail "model name is empty"
[[ -n "$(read_env OPENAI_API_KEY)" ]] || fail "model API key is empty"

grep -q '^  telegram:$' "${CONFIG_FILE}" || fail "Telegram toolset profile missing"
grep -q '^  write_approval: true$' "${CONFIG_FILE}" || fail "write approvals are not enabled"
grep -q '^    - /opt/mosaid/product/skills$' "${CONFIG_FILE}" || fail "Mosaid Skill directory missing"
grep -q '^  disabled_toolsets:$' "${CONFIG_FILE}" || fail "global tool denials missing"

for toolset in terminal file code_execution browser computer_use delegation cronjob image_gen video_gen x_search; do
  grep -q "^    - ${toolset}$" "${CONFIG_FILE}" || fail "dangerous toolset not disabled: ${toolset}"
done

[[ -f "${HERMES_HOME}/SOUL.md" ]] || fail "SOUL.md missing"
[[ -f "${MOSAID_DATA}/workspaces/.hermes.md" ]] || fail "project policy context missing"
[[ -f "${MOSAID_ROOT}/product/skills/research/SKILL.md" ]] || fail "Mosaid Research Skill missing"

if find "${MOSAID_ROOT}/product" -perm /022 -print -quit | grep -q .; then
  fail "Mosaid product assets are group/world writable"
fi

printf 'mosaid-preflight: verified Hermes %s\n' "${EXPECTED_REF}"
