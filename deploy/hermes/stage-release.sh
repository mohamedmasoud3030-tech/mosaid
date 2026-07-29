#!/usr/bin/env bash
set -euo pipefail

# Stages the reviewed Hermes commit and Mosaid product assets.
# It never creates secrets and never starts or enables a service.

readonly EXPECTED_HERMES_REF="b8ceba97ed0b2bf0255cc5c8c61c9110a026cda4"
readonly EXPECTED_REPOSITORY="https://github.com/NousResearch/hermes-agent.git"
readonly EXPECTED_ROOT="/opt/mosaid"
readonly EXPECTED_DATA="/var/lib/mosaid"
readonly EXPECTED_HOME="/var/lib/mosaid/hermes"
readonly EXPECTED_USER="mosaid"

MOSAID_SOURCE="${MOSAID_SOURCE:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
MOSAID_ROOT="${MOSAID_ROOT:-${EXPECTED_ROOT}}"
MOSAID_DATA="${MOSAID_DATA:-${EXPECTED_DATA}}"
HERMES_HOME="${HERMES_HOME:-${EXPECTED_HOME}}"
MOSAID_USER="${MOSAID_USER:-${EXPECTED_USER}}"
HERMES_REF="${HERMES_REF:-${EXPECTED_HERMES_REF}}"
HERMES_REPOSITORY="${HERMES_REPOSITORY:-${EXPECTED_REPOSITORY}}"

release_dir=""
temporary_dir=""
product_tmp=""
created_release=false

fail() {
  printf 'stage-release: %s\n' "$*" >&2
  exit 1
}

cleanup() {
  local status=$?
  [[ -z "${temporary_dir}" ]] || rm -rf "${temporary_dir}"
  [[ -z "${product_tmp}" ]] || rm -rf "${product_tmp}"
  if [[ ${status} -ne 0 && "${created_release}" == true && -n "${release_dir}" ]]; then
    rm -rf "${release_dir}"
  fi
  exit "${status}"
}
trap cleanup EXIT

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

[[ "${EUID}" -eq 0 ]] || fail "run as root on the Oracle instance"
[[ "${MOSAID_ROOT}" == "${EXPECTED_ROOT}" ]] || fail "first gate requires MOSAID_ROOT=${EXPECTED_ROOT}"
[[ "${MOSAID_DATA}" == "${EXPECTED_DATA}" ]] || fail "first gate requires MOSAID_DATA=${EXPECTED_DATA}"
[[ "${HERMES_HOME}" == "${EXPECTED_HOME}" ]] || fail "first gate requires HERMES_HOME=${EXPECTED_HOME}"
[[ "${MOSAID_USER}" == "${EXPECTED_USER}" ]] || fail "first gate requires MOSAID_USER=${EXPECTED_USER}"
[[ "${HERMES_REF}" == "${EXPECTED_HERMES_REF}" ]] || fail "unreviewed Hermes ref: ${HERMES_REF}"
[[ "${HERMES_REPOSITORY}" == "${EXPECTED_REPOSITORY}" ]] || fail "unexpected Hermes repository: ${HERMES_REPOSITORY}"
[[ -f "${MOSAID_SOURCE}/product/hermes/SOUL.md" ]] || fail "Mosaid SOUL.md not found"
[[ -f "${MOSAID_SOURCE}/product/hermes/.hermes.md" ]] || fail "Mosaid .hermes.md not found"
[[ -f "${MOSAID_SOURCE}/deploy/hermes/config.yaml.example" ]] || fail "Hermes config template not found"
[[ -f "${MOSAID_SOURCE}/deploy/hermes/preflight.sh" ]] || fail "Hermes preflight not found"
[[ -f "${MOSAID_SOURCE}/deploy/hermes/mosaid-hermes.service" ]] || fail "systemd unit not found"
[[ -f "${MOSAID_SOURCE}/product/skills/research/SKILL.md" ]] || fail "Mosaid Skills not found"

require_command git
require_command uv
require_command python3
require_command install
require_command id
id "${MOSAID_USER}" >/dev/null 2>&1 || fail "system user '${MOSAID_USER}' does not exist"
MOSAID_GROUP="$(id -gn "${MOSAID_USER}")"

python3 - <<'PY'
import sys
if not ((3, 11) <= sys.version_info[:2] < (3, 14)):
    raise SystemExit(f"Python 3.11-3.13 required; found {sys.version.split()[0]}")
PY

release_dir="${MOSAID_ROOT}/releases/${HERMES_REF}"
temporary_dir="${release_dir}.tmp.$$"

install -d -m 0755 "${MOSAID_ROOT}/releases" "${MOSAID_ROOT}/bin"
install -d -o "${MOSAID_USER}" -g "${MOSAID_GROUP}" -m 0700 \
  "${HERMES_HOME}" "${HERMES_HOME}/memories" "${HERMES_HOME}/pending" "${HERMES_HOME}/skills"
install -d -o "${MOSAID_USER}" -g "${MOSAID_GROUP}" -m 0750 \
  "${MOSAID_DATA}/workspaces" "${MOSAID_DATA}/outputs" "${MOSAID_DATA}/backups"

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

  # Move source before creating .venv; console-script shebangs contain absolute
  # paths and would break if the completed virtual environment were moved.
  mv "${temporary_dir}" "${release_dir}"
  temporary_dir=""
  created_release=true
  uv sync --project "${release_dir}" --frozen --no-dev --extra messaging
  chown -R root:root "${release_dir}"
  find "${release_dir}" -type d -exec chmod go-w {} +
  find "${release_dir}" -type f -exec chmod go-w {} +
else
  [[ "$(git -C "${release_dir}" rev-parse HEAD)" == "${HERMES_REF}" ]] || fail "existing release SHA mismatch"
  [[ "$(git -C "${release_dir}" remote get-url origin)" == "${HERMES_REPOSITORY}" ]] || fail "existing release origin mismatch"
  [[ -x "${release_dir}/.venv/bin/hermes" ]] || fail "existing release is missing the Hermes console script"
fi

product_tmp="${MOSAID_ROOT}/product.tmp.$$"
rm -rf "${product_tmp}"
install -d -o root -g root -m 0755 "${product_tmp}/hermes" "${product_tmp}/skills"
cp -a "${MOSAID_SOURCE}/product/hermes/." "${product_tmp}/hermes/"
cp -a "${MOSAID_SOURCE}/product/skills/." "${product_tmp}/skills/"
chown -R root:root "${product_tmp}"
find "${product_tmp}" -type d -exec chmod 0555 {} +
find "${product_tmp}" -type f -exec chmod 0444 {} +
rm -rf "${MOSAID_ROOT}/product"
mv "${product_tmp}" "${MOSAID_ROOT}/product"
product_tmp=""

install -o root -g root -m 0444 "${MOSAID_ROOT}/product/hermes/SOUL.md" "${HERMES_HOME}/SOUL.md"
install -o root -g root -m 0444 "${MOSAID_ROOT}/product/hermes/.hermes.md" "${MOSAID_DATA}/workspaces/.hermes.md"
install -o root -g root -m 0555 "${MOSAID_SOURCE}/deploy/hermes/preflight.sh" "${MOSAID_ROOT}/bin/preflight-hermes"
install -o root -g root -m 0444 "${MOSAID_SOURCE}/deploy/hermes/mosaid-hermes.service" \
  "/etc/systemd/system/mosaid-hermes.service"

if [[ ! -e "${HERMES_HOME}/config.yaml" ]]; then
  install -o "${MOSAID_USER}" -g "${MOSAID_GROUP}" -m 0600 \
    "${MOSAID_SOURCE}/deploy/hermes/config.yaml.example" "${HERMES_HOME}/config.yaml"
fi

# Start from a blank bundled-skill profile. Mosaid Skills are loaded from the
# read-only external directory configured in config.yaml.
install -o root -g root -m 0444 /dev/null "${HERMES_HOME}/.no-bundled-skills"

ln -sfn "${release_dir}" "${MOSAID_ROOT}/current.new"
mv -Tf "${MOSAID_ROOT}/current.new" "${MOSAID_ROOT}/current"
created_release=false

printf '%s\n' \
  "Staged Hermes ${HERMES_REF}" \
  "Release: ${release_dir}" \
  "Product: ${MOSAID_ROOT}/product" \
  "Hermes home: ${HERMES_HOME}" \
  "Service unit: /etc/systemd/system/mosaid-hermes.service" \
  "Service was not started or enabled." \
  "Next: create ${HERMES_HOME}/.env with mode 0600, run the preflight, then explicitly enable the service."
