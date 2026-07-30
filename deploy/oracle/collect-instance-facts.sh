#!/usr/bin/env bash
set -euo pipefail

# Collects non-secret facts about an Oracle Compute instance and prints a
# Markdown report. Read-only: it installs nothing, changes nothing and
# contacts no third party. Safe to run as an unprivileged user.
#
# Deliberately never prints: IP addresses, hostnames, SSH keys, instance OCIDs,
# environment variables or file contents. Those are either secret or
# identifying, and this output is meant to be committed.

fail() {
  printf 'collect-instance-facts: %s\n' "$*" >&2
  exit 1
}

have() { command -v "$1" >/dev/null 2>&1; }

value_or() {
  local value="$1"
  local fallback="${2:-unknown}"
  if [[ -z "${value}" ]]; then printf '%s' "${fallback}"; else printf '%s' "${value}"; fi
}

# --- operating system -------------------------------------------------------

os_name="unknown"
os_version="unknown"
os_id="unknown"
if [[ -r /etc/os-release ]]; then
  # shellcheck disable=SC1091
  os_name="$(. /etc/os-release && value_or "${PRETTY_NAME:-}")"
  os_version="$(. /etc/os-release && value_or "${VERSION_ID:-}")"
  os_id="$(. /etc/os-release && value_or "${ID:-}")"
fi

kernel="$(uname -r 2>/dev/null || echo unknown)"
arch="$(uname -m 2>/dev/null || echo unknown)"

init_system="unknown"
if [[ -d /run/systemd/system ]]; then
  init_system="systemd"
elif have systemctl; then
  init_system="systemd (not booted)"
fi

# --- python -----------------------------------------------------------------

python_version="not found"
python_supported="no"
for candidate in python3.13 python3.12 python3.11 python3; do
  if have "${candidate}"; then
    python_version="$("${candidate}" --version 2>&1 | awk '{print $2}')"
    if "${candidate}" -c 'import sys; raise SystemExit(0 if (3,11) <= sys.version_info[:2] < (3,14) else 1)' 2>/dev/null; then
      python_supported="yes (${candidate})"
    else
      python_supported="no (${candidate} is out of the 3.11-3.13 range)"
    fi
    break
  fi
done

# --- cpu / memory / disk ----------------------------------------------------

cpu_count="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo unknown)"
cpu_model="unknown"
if [[ -r /proc/cpuinfo ]]; then
  cpu_model="$(awk -F': ' '/^model name|^Model|^CPU implementer/{print $2; exit}' /proc/cpuinfo 2>/dev/null || true)"
  cpu_model="$(value_or "${cpu_model}")"
fi

mem_total_kb=""
[[ -r /proc/meminfo ]] && mem_total_kb="$(awk '/^MemTotal:/{print $2}' /proc/meminfo)"
if [[ -n "${mem_total_kb}" ]]; then
  mem_total_gib="$(awk -v k="${mem_total_kb}" 'BEGIN{printf "%.1f", k/1048576}')"
else
  mem_total_gib="unknown"
fi

swap_total_kb=""
[[ -r /proc/meminfo ]] && swap_total_kb="$(awk '/^SwapTotal:/{print $2}' /proc/meminfo)"
if [[ -n "${swap_total_kb}" ]]; then
  swap_total_gib="$(awk -v k="${swap_total_kb}" 'BEGIN{printf "%.1f", k/1048576}')"
else
  swap_total_gib="unknown"
fi

root_size="unknown"; root_avail="unknown"; root_used_pct="unknown"
if have df; then
  read -r root_size root_used_pct root_avail <<<"$(df -h --output=size,pcent,avail / 2>/dev/null | tail -n 1 | awk '{print $1, $2, $3}')" || true
  root_size="$(value_or "${root_size}")"
  root_avail="$(value_or "${root_avail}")"
  root_used_pct="$(value_or "${root_used_pct}")"
fi

# --- readiness for the Mosaid first gate -----------------------------------

user_exists="no"
id mosaid >/dev/null 2>&1 && user_exists="yes"

uv_present="no"
uv_version="-"
if have uv; then
  uv_present="yes"
  uv_version="$(uv --version 2>/dev/null | awk '{print $2}')"
  uv_version="$(value_or "${uv_version}")"
fi

git_present="no"
have git && git_present="yes"

opt_mosaid="absent"
[[ -d /opt/mosaid ]] && opt_mosaid="present"
var_mosaid="absent"
[[ -d /var/lib/mosaid ]] && var_mosaid="present"

service_state="not installed"
if [[ -f /etc/systemd/system/mosaid-hermes.service ]]; then
  if have systemctl; then
    active="$(systemctl is-active mosaid-hermes 2>/dev/null || true)"
    enabled="$(systemctl is-enabled mosaid-hermes 2>/dev/null || true)"
    service_state="installed (active=$(value_or "${active}" unknown), enabled=$(value_or "${enabled}" unknown))"
  else
    service_state="installed"
  fi
fi

# --- things that must NOT be present in the first gate ---------------------

docker_present="no"
have docker && docker_present="yes"
docker_running="no"
if have systemctl && systemctl is-active docker >/dev/null 2>&1; then docker_running="yes"; fi

agentenv_present="no"
{ have aenv || [[ -f /etc/systemd/system/aenv.service ]]; } && agentenv_present="yes"

kvm_present="no"
[[ -e /dev/kvm ]] && kvm_present="yes"
ublk_present="no"
[[ -e /dev/ublk-control ]] && ublk_present="yes"

# Listening TCP ports, without revealing addresses: report port numbers only.
listening_ports="unavailable"
if have ss; then
  listening_ports="$(ss -H -ltn 2>/dev/null | awk '{print $4}' | sed 's/.*://' | sort -un | tr '\n' ' ' | sed 's/ $//')"
  listening_ports="$(value_or "${listening_ports}" none)"
fi
port_8000="no"
case " ${listening_ports} " in *" 8000 "*) port_8000="yes";; esac

# --- ssh posture ------------------------------------------------------------

ssh_password_auth="unknown"
ssh_root_login="unknown"
if have sshd; then
  sshd_out="$(sshd -T 2>/dev/null || true)"
  if [[ -n "${sshd_out}" ]]; then
    ssh_password_auth="$(printf '%s\n' "${sshd_out}" | awk '/^passwordauthentication/{print $2; exit}')"
    ssh_root_login="$(printf '%s\n' "${sshd_out}" | awk '/^permitrootlogin/{print $2; exit}')"
    ssh_password_auth="$(value_or "${ssh_password_auth}")"
    ssh_root_login="$(value_or "${ssh_root_login}")"
  fi
fi

# --- verdict ----------------------------------------------------------------

blockers=()
# Blockers are conditions the bootstrap cannot fix. Anything the bootstrap
# installs is a warning, not a blocker.
[[ "${init_system}" == systemd* ]] || blockers+=("systemd is not the init system")
[[ "${arch}" == "aarch64" || "${arch}" == "x86_64" ]] || blockers+=("unexpected architecture: ${arch}")
[[ "${port_8000}" == "no" ]] || blockers+=("port 8000 is listening; the first gate must not expose an execution API")
[[ "${docker_running}" == "no" ]] || blockers+=("Docker is running; the first gate installs no sandbox runtime")
[[ "${agentenv_present}" == "no" ]] || blockers+=("AgentENV detected; it is deferred and must not be present")

warnings=()
[[ "${ssh_password_auth}" == "no" ]] || warnings+=("SSH password authentication is '${ssh_password_auth}'; it should be 'no'")
case "${python_supported}" in
  yes*) ;;
  *) warnings+=("no Python in the 3.11-3.13 range yet; bootstrap-host.sh will install one") ;;
esac
[[ "${git_present}" == "yes" ]] || warnings+=("git is not installed; bootstrap-host.sh will install it")
[[ "${uv_present}" == "yes" ]] || warnings+=("uv is not installed; bootstrap-host.sh will install it pinned")
if [[ "${mem_total_gib}" != "unknown" ]] && awk -v m="${mem_total_gib}" 'BEGIN{exit !(m < 1.9)}'; then
  warnings+=("only ${mem_total_gib} GiB RAM; a Python agent runtime will be tight")
fi

# --- report -----------------------------------------------------------------

cat <<REPORT
# Oracle Instance Facts

Collected: $(date -u '+%Y-%m-%dT%H:%M:%SZ') (UTC)
Collector: \`deploy/oracle/collect-instance-facts.sh\` (read-only)

> No IP address, hostname, OCID or secret is included below, so this report is safe to commit.

## Operating system

| Fact | Value |
|---|---|
| OS | ${os_name} |
| OS ID / version | ${os_id} / ${os_version} |
| Kernel | ${kernel} |
| Architecture | ${arch} |
| Init system | ${init_system} |

## Compute and storage

| Fact | Value |
|---|---|
| Logical CPUs | ${cpu_count} |
| CPU model | ${cpu_model} |
| RAM | ${mem_total_gib} GiB |
| Swap | ${swap_total_gib} GiB |
| Root filesystem size | ${root_size} |
| Root filesystem available | ${root_avail} |
| Root filesystem used | ${root_used_pct} |

## Runtime prerequisites

| Fact | Value |
|---|---|
| Python | ${python_version} |
| Python in 3.11-3.13 | ${python_supported} |
| git installed | ${git_present} |
| uv installed | ${uv_present} (${uv_version}) |
| \`mosaid\` user exists | ${user_exists} |
| /opt/mosaid | ${opt_mosaid} |
| /var/lib/mosaid | ${var_mosaid} |
| mosaid-hermes service | ${service_state} |

## First-gate isolation posture

These must all be absent or disabled. See the AgentENV decision record.

| Fact | Value | Required |
|---|---|---|
| Docker installed | ${docker_present} | not required |
| Docker running | ${docker_running} | must be no |
| AgentENV present | ${agentenv_present} | must be no |
| /dev/kvm | ${kvm_present} | not required |
| /dev/ublk-control | ${ublk_present} | not required |
| Listening TCP ports | ${listening_ports} | ideally 22 only |
| Port 8000 listening | ${port_8000} | must be no |

## SSH posture

| Fact | Value |
|---|---|
| PasswordAuthentication | ${ssh_password_auth} |
| PermitRootLogin | ${ssh_root_login} |

## Billing (fill in manually from the console)

| Fact | Value |
|---|---|
| Shape | REPLACE_AFTER_CONSOLE_CHECK |
| Always Free-eligible badge shown | REPLACE_AFTER_CONSOLE_CHECK |
| Region | REPLACE_AFTER_CONSOLE_CHECK |
| Boot volume size | REPLACE_AFTER_CONSOLE_CHECK |
| Billing status | unknown until visually confirmed |

REPORT

if ((${#blockers[@]})); then
  printf '## Blockers\n\n'
  for item in "${blockers[@]}"; do printf -- '- %s\n' "${item}"; done
  printf '\n'
fi

if ((${#warnings[@]})); then
  printf '## Warnings\n\n'
  for item in "${warnings[@]}"; do printf -- '- %s\n' "${item}"; done
  printf '\n'
fi

if ((${#blockers[@]} == 0)); then
  printf '## Verdict\n\nNo blockers detected. Proceed to `sudo bash deploy/oracle/bootstrap-host.sh`.\n'
else
  printf '## Verdict\n\nResolve the blockers above before running the bootstrap.\n'
  exit 1
fi
