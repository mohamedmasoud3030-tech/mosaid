#!/usr/bin/env bash
set -euo pipefail

# Stages the reviewed Hermes commit and Mosaid product assets.
# It never creates secrets and never starts or enables a service.

readonly EXPECTED_HERMES_REF="b8ceba97ed0b2bf0255cc5c8c61c9110a026cda4"
readonly EXPECTED_REPOSITORY="https://github.com/NousResearch/hermes-agent.git"

MOSAID_SOURCE="${MOSAID_SOURCE:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
MOSAID_ROOT="${MOSAID_ROOT:-/opt/mosaid}"
MOSAID_DATA="${MOSAID_DATA:-/var/lib/mosaid}"
HERMES_HOME="${HERMES_HOME:-${MOSAID_DATA}/hermes}"
HERMES_REF="${HERMES_REF:-${EXPECTED_HERMES_REF}}"
HERMES_REPOSITORY="${HERMES_REPOSITORY:-${EXPECTED_REPOSITORY}}"

fail() {
  printf 'stage-release: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

[[ "${EUID}" -eq 0 ]] || fail "run as root on the Oracle instance"
[[ "${HERMES_REF}" == "${EXPECTED_HERMES_REF}" ]] || fail "unreviewed Hermes ref: ${HERMES_REF}"
[[ "${HERMES_REPOSITORY}" == "${EXPECTED_REPOSITORY}" ]] || fail "unexpected Hermes repository: ${HERMES_REPOSITORY}"
[[ -f "${MOSAID_SOURCE}/product/hermes/SOUL.md" ]] || fail "Mosaid SOUL.md not found"
[[ -f "${MOSAID_SOURCE}/product/hermes/.hermes.md" ]] || fail "Mosaid .hermes.md not found"
[[ -f "${MOSAID_SOURCE}/deploy/hermes/config.yaml.example" ]] || fail "Hermes config template not found"
[[ -f "${MOSAID_SOURCE}/product/skills/research/SKILL.md" ]] || fail "Mosaid Skills not found"

require_command git
require_command uv
require_command python3
require_command install

python3 - <<'PY'
import sys
if not ((3, 11) <= sys.version_info[:2] < (3, 14)):
    raise SystemExit(f"Python 3.11-3.13 required; found {sys.version.split()[0]}")
PY

release_dir="${MOSAID_ROOT}/releases/${HERMES_REF}"
temporary_dir="${release_dir}.tmp.$$"

install -d -m 0755 "${MOSAID_ROOT}/releases" "${MOSAID_ROOT}/bin"
install -d -m 0700 "${HERMES_HOME}" "${HERMES_HOME}/memories" "${HERMES_HOME}/pending"
install -d -m 0750 "${MOSAID_DATA}/workspaces" "${MOSAID_DATA}/outputs" "${MOSAID_DATA}/backups"

if [[ ! -d "${release_dir}/.git" ]]; then
  rm -rf "${temporary_dir}"
  git clone --filter=blob:none --no-checkout "${HERMES_REPOSITORY}" "${temporary_dir}"
  git -C "${temporary_dir}" fetch --depth=1 origin "${HERMES_REF}"
  git -C "${temporary_dir}" checkout --detach "${HERMES_REF}"

  [[ "$(git -C "${temporary_dir}" rev-parse HEAD)" == "${HERMES_REF}" ]] || fail "fetched Hermes SHA mismatch"
  [[ "$(git -C "${temporary_dir}" remote get-url origin)" == "${HERMES_REPOSITORY}" ]] || fail "fetched Hermes origin mismatch"
  [[ -f "${temporary_dir}/LICENSE" ]] || fail "Hermes LICENSE missing"
  grep -qx 'MIT License' <(head -n 1 "${temporary_dir}/LICENSE") || fail "unexpected Hermes license header"
  grep -q 'version = "0.19.0"' "${temporary_dir}/pyproject.toml" || fail "unexpected Hermes version"
  grep -q 'requires-python = ">=3.11,<3.14"' "${temporary_dir}/pyproject.toml" || fail "unexpected Hermes Python range"

  uv sync --project "${temporary_dir}" --frozen --no-dev --extra messaging
  mv "${temporary_dir}" "${release_dir}"
else
  [[ "$(git -C "${release_dir}" rev-parse HEAD)" == "${HERMES_REF}" ]] || fail "existing release SHA mismatch"
  [[ "$(git -C "${release_dir}" remote get-url origin)" == "${HERMES_REPOSITORY}" ]] || fail "existing release origin mismatch"
fi

product_tmp="${MOSAID_ROOT}/product.tmp.$$"
rm -rf "${product_tmp}"
install -d -m 0755 "${product_tmp}/hermes" "${product_tmp}/skills"
cp -a "${MOSAID_SOURCE}/product/hermes/." "${product_tmp}/hermes/"
cp -a "${MOSAID_SOURCE}/product/skills/." "${product_tmp}/skills/"
find "${product_tmp}" -type d -exec chmod 0555 {} +
find "${product_tmp}" -type f -exec chmod 0444 {} +
rm -rf "${MOSAID_ROOT}/product"
mv "${product_tmp}" "${MOSAID_ROOT}/product"

install -m 0444 "${MOSAID_ROOT}/product/hermes/SOUL.md" "${HERMES_HOME}/SOUL.md"
install -m 0444 "${MOSAID_ROOT}/product/hermes/.hermes.md" "${MOSAID_DATA}/workspaces/.hermes.md"

if [[ ! -e "${HERMES_HOME}/config.yaml" ]]; then
  install -m 0600 "${MOSAID_SOURCE}/deploy/hermes/config.yaml.example" "${HERMES_HOME}/config.yaml"
fi

# Start from a blank bundled-skill profile. Mosaid Skills are loaded from the
# read-only external directory configured in config.yaml.
install -m 0444 /dev/null "${HERMES_HOME}/.no-bundled-skills"

ln -sfn "${release_dir}" "${MOSAID_ROOT}/current.new"
mv -Tf "${MOSAID_ROOT}/current.new" "${MOSAID_ROOT}/current"

printf '%s\n' \
  "Staged Hermes ${HERMES_REF}" \
  "Release: ${release_dir}" \
  "Product: ${MOSAID_ROOT}/product" \
  "Hermes home: ${HERMES_HOME}" \
  "Service was not started." \
  "Next: create ${HERMES_HOME}/.env with mode 0600, validate config, then install the systemd unit."
