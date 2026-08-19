#!/data/data/com.termux/files/usr/bin/bash
# Mosaid Phase 14 preflight: local facts plus optional network reachability.
# Produces a redacted JSON report; never sends secrets anywhere.
set -euo pipefail
umask 077

M_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
REPORT_DIR="$M_HOME/reports"
NETWORK=0
[[ "${1:-}" == "--network" ]] && NETWORK=1
mkdir -p "$REPORT_DIR" "$M_HOME/tmp"
REPORT="$REPORT_DIR/preflight-$(date -u +%Y%m%dT%H%M%SZ).json"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing required command: $1" >&2; exit 1; }; }
for cmd in jq curl sha256sum getprop uname df awk stat; do need "$cmd"; done
bash "$M_HOME/scripts/verify-config.sh" >/dev/null

arch_uname="$(uname -m 2>/dev/null || echo unknown)"
arch_abi="$(getprop ro.product.cpu.abi 2>/dev/null || echo unknown)"
android_release="$(getprop ro.build.version.release 2>/dev/null || echo unknown)"
android_sdk="$(getprop ro.build.version.sdk 2>/dev/null || echo unknown)"
timezone="$(getprop persist.sys.timezone 2>/dev/null || true)"
[[ -n "$timezone" ]] || timezone="unknown"
termux_version="${TERMUX_VERSION:-unknown}"
storage_kb="$(df -Pk "$M_HOME" | awk 'NR==2{print $4}')"
mem_total_kb="$(awk '/^MemTotal:/{print $2}' /proc/meminfo 2>/dev/null || echo 0)"
mem_available_kb="$(awk '/^MemAvailable:/{print $2}' /proc/meminfo 2>/dev/null || echo 0)"
battery_api="no"
command -v termux-battery-status >/dev/null 2>&1 && battery_api="yes"

network_telegram="skipped"
network_model="skipped"
if [[ "$NETWORK" == "1" ]]; then
  if curl -fsS -m 15 -o /dev/null https://api.telegram.org/ 2>/dev/null; then
    network_telegram="reachable"
  else
    network_telegram="unreachable"
  fi
  model_base="$(jq -r '.model.base_url' "$M_HOME/config.json")"
  model_key="$(tr -d '\r' < "$M_HOME/secrets/model.key")"
  model_http="$(curl -sS -m 15 -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $model_key" "$model_base/models" 2>/dev/null || echo 000)"
  case "$model_http" in
    200) network_model="reachable" ;;
    401|403) network_model="reachable-auth-check-failed" ;;
    *) network_model="unreachable" ;;
  esac
fi

jq -n \
  --arg arch_uname "$arch_uname" \
  --arg arch_abi "$arch_abi" \
  --arg android_release "$android_release" \
  --arg android_sdk "$android_sdk" \
  --arg timezone "$timezone" \
  --arg termux_version "$termux_version" \
  --arg battery_api "$battery_api" \
  --arg network_telegram "$network_telegram" \
  --arg network_model "$network_model" \
  --arg storage_kb "$storage_kb" \
  --arg mem_total_kb "$mem_total_kb" \
  --arg mem_available_kb "$mem_available_kb" \
  '{
    timestamp: (now | todate),
    arch_uname: $arch_uname,
    arch_abi: $arch_abi,
    android_release: $android_release,
    android_sdk: $android_sdk,
    timezone: $timezone,
    termux_version: $termux_version,
    battery_api: $battery_api,
    storage_free_kb: ($storage_kb | tonumber),
    mem_total_kb: ($mem_total_kb | tonumber),
    mem_available_kb: ($mem_available_kb | tonumber),
    network_telegram: $network_telegram,
    network_model: $network_model
  }' > "$REPORT"
chmod 600 "$REPORT"
echo "preflight report: $REPORT"
jq . "$REPORT"
