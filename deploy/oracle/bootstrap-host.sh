#!/usr/bin/env bash
set -euo pipefail

# Prepares an Oracle Compute instance to host the pinned Hermes runtime.
#
# Idempotent: safe to re-run. It does NOT clone Hermes, create secrets, install
# Docker or any sandbox runtime, or start/enable any service. Staging is
# deploy/hermes/stage-release.sh; this script only builds the host prerequisites.

readonly UV_VERSION="0.12.0"
readonly UV_SHA256_AARCH64="2c5d6e3092cc5223b10ff403880cc75121bf64e84644e7a0c69f643b0d89ac95"
readonly UV_SHA256_X86_64="eaf842262aa1c418d8ecc5605f02ee1ebfd369124fa48548e85f9481a47831a9"

readonly MOSAID_USER="mosaid"
readonly MOSAID_ROOT="/opt/mosaid"
readonly MOSAID_DATA="/var/lib/mosaid"
readonly HERMES_HOME="${MOSAID_DATA}/hermes"

work_dir=""

fail() {
  printf 'bootstrap-host: %s\n' "$*" >&2
  exit 1
}

info() { printf 'bootstrap-host: %s\n' "$*"; }

cleanup() {
  local status=$?
  [[ -z "${work_dir}" ]] || rm -rf "${work_dir}"
  exit "${status}"
}
trap cleanup EXIT

have() { command -v "$1" >/dev/null 2>&1; }

[[ "${EUID}" -eq 0 ]] || fail "run as root (sudo bash deploy/oracle/bootstrap-host.sh)"

# --- refuse to run on an unsuitable or already-contaminated host ------------

[[ -d /run/systemd/system ]] || fail "systemd is required but this host is not booted with it"

arch="$(uname -m)"
case "${arch}" in
  aarch64) uv_sha256="${UV_SHA256_AARCH64}"; uv_target="aarch64-unknown-linux-gnu" ;;
  x86_64)  uv_sha256="${UV_SHA256_X86_64}";  uv_target="x86_64-unknown-linux-gnu" ;;
  *) fail "unsupported architecture: ${arch} (expected aarch64 or x86_64)" ;;
esac

if have systemctl && systemctl is-active docker >/dev/null 2>&1; then
  fail "Docker is running; the first gate installs no sandbox runtime (see docs/pivot/AGENTENV-EXECUTION-BACKEND-DECISION.md)"
fi
if have aenv || [[ -f /etc/systemd/system/aenv.service ]]; then
  fail "AgentENV detected; it is deferred and must not be installed"
fi
if have ss && ss -H -ltn 2>/dev/null | awk '{print $4}' | sed 's/.*://' | grep -qx 8000; then
  fail "port 8000 is already listening; the first gate must not expose an execution API"
fi

os_id=""
os_version=""
if [[ -r /etc/os-release ]]; then
  # shellcheck disable=SC1091
  os_id="$(. /etc/os-release && printf '%s' "${ID:-}")"
  # shellcheck disable=SC1091
  os_version="$(. /etc/os-release && printf '%s' "${VERSION_ID:-}")"
fi
info "host: ${os_id:-unknown} ${os_version:-unknown} on ${arch}"

# --- packages ---------------------------------------------------------------

install_packages_apt() {
  info "installing base packages with apt-get"
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq --no-install-recommends \
    ca-certificates curl git coreutils >/dev/null
  # Ubuntu 24.04 ships Python 3.12, which is inside the supported range.
  if ! python_supported_path >/dev/null; then
    apt-get install -y -qq --no-install-recommends python3 python3-venv >/dev/null
  fi
}

install_packages_dnf() {
  info "installing base packages with dnf"
  dnf install -y -q ca-certificates curl git coreutils >/dev/null
  if ! python_supported_path >/dev/null; then
    # Oracle Linux 9 provides python3.11 as an explicit package.
    dnf install -y -q python3.11 >/dev/null || dnf install -y -q python3 >/dev/null
  fi
}

python_supported_path() {
  local candidate
  for candidate in python3.13 python3.12 python3.11 python3; do
    if have "${candidate}" \
      && "${candidate}" -c 'import sys; raise SystemExit(0 if (3,11) <= sys.version_info[:2] < (3,14) else 1)' 2>/dev/null; then
      command -v "${candidate}"
      return 0
    fi
  done
  return 1
}

if have apt-get; then
  install_packages_apt
elif have dnf; then
  install_packages_dnf
else
  fail "no supported package manager found (expected apt-get or dnf)"
fi

python_bin="$(python_supported_path)" || fail "no Python in the required 3.11-3.13 range is available"
info "python: ${python_bin} ($("${python_bin}" --version 2>&1 | awk '{print $2}'))"

# --- uv, pinned and checksum-verified --------------------------------------
# Deliberately not `curl ... | sh`: the archive is downloaded to a file, its
# SHA-256 is verified against a pinned value, and only then is it extracted.

install_uv() {
  if have uv && [[ "$(uv --version 2>/dev/null | awk '{print $2}')" == "${UV_VERSION}" ]]; then
    info "uv ${UV_VERSION} already installed"
    return 0
  fi

  local url="https://github.com/astral-sh/uv/releases/download/${UV_VERSION}/uv-${uv_target}.tar.gz"
  work_dir="$(mktemp -d)"
  local archive="${work_dir}/uv.tar.gz"

  info "downloading uv ${UV_VERSION} for ${uv_target}"
  curl -fsSL --proto '=https' --tlsv1.2 -o "${archive}" "${url}" \
    || fail "failed to download uv from ${url}"

  local actual
  actual="$(sha256sum "${archive}" | awk '{print $1}')"
  if [[ "${actual}" != "${uv_sha256}" ]]; then
    fail "uv checksum mismatch: expected ${uv_sha256}, got ${actual}"
  fi
  info "uv checksum verified"

  tar -xzf "${archive}" -C "${work_dir}"
  local extracted
  extracted="$(find "${work_dir}" -type f -name uv -perm -u+x | head -n 1)"
  [[ -n "${extracted}" ]] || fail "uv binary not found in the downloaded archive"

  install -o root -g root -m 0755 "${extracted}" /usr/local/bin/uv
  local uvx
  uvx="$(find "${work_dir}" -type f -name uvx -perm -u+x | head -n 1)"
  [[ -z "${uvx}" ]] || install -o root -g root -m 0755 "${uvx}" /usr/local/bin/uvx

  rm -rf "${work_dir}"
  work_dir=""
  info "uv installed: $(uv --version)"
}

install_uv

# --- service user -----------------------------------------------------------

nologin_shell="/usr/sbin/nologin"
[[ -x "${nologin_shell}" ]] || nologin_shell="/sbin/nologin"
[[ -x "${nologin_shell}" ]] || nologin_shell="/bin/false"

if id "${MOSAID_USER}" >/dev/null 2>&1; then
  info "user '${MOSAID_USER}' already exists"
else
  info "creating system user '${MOSAID_USER}'"
  useradd --system --home-dir "${MOSAID_DATA}" --create-home --shell "${nologin_shell}" "${MOSAID_USER}"
fi
mosaid_group="$(id -gn "${MOSAID_USER}")"

# The service user must not be able to reach a container runtime.
if getent group docker >/dev/null 2>&1 && id -nG "${MOSAID_USER}" | tr ' ' '\n' | grep -qx docker; then
  fail "user '${MOSAID_USER}' is in the docker group; remove it before continuing"
fi

# --- directories ------------------------------------------------------------

info "creating directory layout"
install -d -o root -g root -m 0755 "${MOSAID_ROOT}" "${MOSAID_ROOT}/releases" "${MOSAID_ROOT}/bin"
install -d -o "${MOSAID_USER}" -g "${mosaid_group}" -m 0750 "${MOSAID_DATA}"
install -d -o "${MOSAID_USER}" -g "${mosaid_group}" -m 0700 \
  "${HERMES_HOME}" "${HERMES_HOME}/memories" "${HERMES_HOME}/pending" "${HERMES_HOME}/skills"
install -d -o "${MOSAID_USER}" -g "${mosaid_group}" -m 0750 \
  "${MOSAID_DATA}/workspaces" "${MOSAID_DATA}/outputs" "${MOSAID_DATA}/backups"

# --- verification -----------------------------------------------------------

info "verifying result"
[[ -d "${MOSAID_ROOT}/releases" ]] || fail "release directory missing"
[[ "$(stat -c '%a' "${HERMES_HOME}")" == "700" ]] || fail "Hermes home has unexpected permissions"
[[ "$(stat -c '%U' "${HERMES_HOME}")" == "${MOSAID_USER}" ]] || fail "Hermes home has unexpected owner"
have uv || fail "uv is not on PATH after installation"
have git || fail "git is not installed"

cat <<SUMMARY

bootstrap-host: host prepared.

  OS            : ${os_id:-unknown} ${os_version:-unknown} (${arch})
  Python        : ${python_bin} ($("${python_bin}" --version 2>&1 | awk '{print $2}'))
  uv            : $(uv --version)
  Service user  : ${MOSAID_USER}:${mosaid_group}
  Root          : ${MOSAID_ROOT}
  Data          : ${MOSAID_DATA}
  Hermes home   : ${HERMES_HOME}

Not done, by design:
  - Hermes was not cloned or installed.
  - No secret was created.
  - No service was started or enabled.
  - No Docker, AgentENV or sandbox runtime was installed.

Next:
  python3 scripts/verify-hermes-pivot.py
  sudo MOSAID_SOURCE="\$PWD" bash deploy/hermes/stage-release.sh
SUMMARY
