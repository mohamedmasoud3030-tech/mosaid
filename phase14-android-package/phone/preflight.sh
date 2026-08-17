#!/data/data/com.termux/files/usr/bin/bash
# Mosaid product preflight: platform baseline always, plus optional
# network/API checks with --network. Writes a JSON report and exits
# non-zero when a baseline check fails.
set -euo pipefail
umask 077

MOSAID_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
CONFIG_HOME="${MOSAID_CONFIG_HOME:-$HOME/.config/mosaid}"
CONFIG="$MOSAID_HOME/config.json"
REPORT_DIR="$MOSAID_HOME/reports"
NETWORK=0
[[ "${1:-}" == "--network" ]] && NETWORK=1
mkdir -p "$REPORT_DIR" "$MOSAID_HOME/tmp"
REPORT="$REPORT_DIR/preflight-$(date -u +%Y%m%dT%H%M%SZ).json"
TMP="$MOSAID_HOME/tmp/preflight.$$"
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing required command: $1" >&2; exit 1; }; }
for cmd in jq curl sha256sum getprop uname df awk sed stat; do need "$cmd"; done
"$(dirname "$0")/verify-config.sh" >/dev/null

arch_uname="$(uname -m 2>/dev/null || echo unknown)"
arch_abi="$(getprop ro.product.cpu.abi 2>/dev/null || echo unknown)"
android_release="$(getprop ro.build.version.release 2>/dev/null || echo unknown)"
android_sdk="$(getprop ro.build.version.sdk 2>/dev/null || echo unknown)"
timezone="$(getprop persist.sys.timezone 2>/dev/null || true)"
[[ -n "$timezone" ]] || timezone="$(date +%Z 2>/dev/null || echo unknown)"
termux_version="${TERMUX_VERSION:-unknown}"
termux_prefix="${PREFIX:-unknown}"

case "$termux_prefix" in
  /data/data/com.termux/files/usr|/data/user/0/com.termux/files/usr) termux_runtime=true ;;
  *) termux_runtime=false ;;
esac

storage_kb="$(df -Pk "$MOSAID_HOME" | awk 'NR==2{print $4}')"
mem_total_kb="$(awk '/^MemTotal:/{print $2}' /proc/meminfo 2>/dev/null || echo 0)"
mem_available_kb="$(awk '/^MemAvailable:/{print $2}' /proc/meminfo 2>/dev/null || echo 0)"

write_ok=false
probe="$MOSAID_HOME/tmp/write-probe.$$"
if (umask 077; printf 'mosaid\n' > "$probe") 2>/dev/null && [[ "$(cat "$probe")" == mosaid ]]; then write_ok=true; fi
rm -f "$probe"

wake_lock_request="not-requested"
if command -v termux-wake-lock >/dev/null 2>&1; then
  if termux-wake-lock >/dev/null 2>&1; then wake_lock_request="requested-ok"; else wake_lock_request="request-failed"; fi
else
  wake_lock_request="command-missing"
fi

battery_optimization="unknown"
if command -v dumpsys >/dev/null 2>&1 && command -v timeout >/dev/null 2>&1; then
  if timeout 5 dumpsys deviceidle whitelist 2>/dev/null | grep -q 'com.termux'; then
    battery_optimization="whitelisted"
  else
    battery_optimization="not-whitelisted-or-unobservable"
  fi
fi

battery_json='null'
if command -v termux-battery-status >/dev/null 2>&1 && command -v timeout >/dev/null 2>&1; then
  raw_battery="$(timeout 8 termux-battery-status 2>/dev/null || true)"
  if jq -e . >/dev/null 2>&1 <<<"$raw_battery"; then
    battery_json="$(jq '{percentage,status,plugged,temperature,current,health}' <<<"$raw_battery")"
  fi
fi

ca_file=""
for p in "$PREFIX/etc/tls/cert.pem" "$PREFIX/etc/ssl/cert.pem" "$PREFIX/etc/ssl/certs/ca-certificates.crt"; do
  [[ -r "$p" ]] && { ca_file="$p"; break; }
done

dns_telegram="not-tested"
tls_telegram="not-tested"
telegram_api="not-tested"
model_api="not-tested"
model_latency_ms=null
if (( NETWORK )); then
  if getent hosts api.telegram.org > "$TMP/dns.txt" 2>&1; then dns_telegram="pass"; else dns_telegram="fail"; fi
  http_code="$(curl -sS --connect-timeout 10 --max-time 20 -o /dev/null -w '%{http_code}' https://api.telegram.org 2>"$TMP/tls.err" || true)"
  if [[ "$http_code" =~ ^(200|301|302|404)$ ]]; then tls_telegram="pass"; else tls_telegram="fail:$http_code"; fi

  bot_token="$(cat "$CONFIG_HOME/telegram.token")"
  tg_response="$TMP/telegram.json"
  {
    printf 'silent\nshow-error\nconnect-timeout = 10\nmax-time = 20\n'
    printf 'url = "https://api.telegram.org/bot%s/getMe"\n' "$bot_token"
  } | curl --config - > "$tg_response" 2>"$TMP/telegram.err" || true
  if jq -e '.ok == true' "$tg_response" >/dev/null 2>&1; then telegram_api="pass"; else telegram_api="fail"; fi

  api_key="$(cat "$CONFIG_HOME/model.key")"
  api_base="$(jq -r '.model.base_url' "$CONFIG")"
  model_id="$(jq -r '.model.name' "$CONFIG")"
  jq -nc --arg m "$model_id" '{model:$m,messages:[{role:"user",content:"Reply exactly MOSAID_PRODUCT_OK"}],max_tokens:16,temperature:0}' > "$TMP/model-request.json"
  start_ms="$(($(date +%s%N)/1000000))"
  {
    printf 'silent\nshow-error\nconnect-timeout = 10\nmax-time = 60\nrequest = "POST"\n'
    printf 'url = "%s/chat/completions"\n' "${api_base%/}"
    printf 'header = "Authorization: Bearer %s"\n' "$api_key"
    printf 'header = "Content-Type: application/json"\n'
  } | curl --config - --data-binary @"$TMP/model-request.json" > "$TMP/model-response.json" 2>"$TMP/model.err" || true
  end_ms="$(($(date +%s%N)/1000000))"
  model_latency_ms=$((end_ms-start_ms))
  model_text="$(jq -r '.choices[0].message.content // ""' "$TMP/model-response.json" 2>/dev/null || true)"
  if [[ "$model_text" == *MOSAID_PRODUCT_OK* ]]; then model_api="pass"; else model_api="fail"; fi
  unset bot_token api_key
fi

jq -n \
  --arg generated_at "$(date -u +%FT%TZ)" \
  --arg arch_uname "$arch_uname" --arg arch_abi "$arch_abi" \
  --arg android_release "$android_release" --arg android_sdk "$android_sdk" \
  --arg termux_version "$termux_version" --arg prefix "$termux_prefix" \
  --arg timezone "$timezone" --argjson storage_kb "${storage_kb:-0}" \
  --argjson mem_total_kb "${mem_total_kb:-0}" --argjson mem_available_kb "${mem_available_kb:-0}" \
  --argjson termux_runtime "$termux_runtime" --argjson write_ok "$write_ok" \
  --arg wake_lock_request "$wake_lock_request" --arg battery_optimization "$battery_optimization" \
  --argjson battery "$battery_json" --arg ca_file "$ca_file" \
  --arg dns_telegram "$dns_telegram" --arg tls_telegram "$tls_telegram" \
  --arg telegram_api "$telegram_api" --arg model_api "$model_api" \
  --argjson model_latency_ms "$model_latency_ms" \
  '{generated_at:$generated_at,device:{uname_arch:$arch_uname,android_abi:$arch_abi,android_release:$android_release,android_sdk:$android_sdk,termux_version:$termux_version,prefix:$prefix,timezone:$timezone,storage_free_kb:$storage_kb,mem_total_kb:$mem_total_kb,mem_available_kb:$mem_available_kb},checks:{is_termux_runtime:$termux_runtime,app_private_write:$write_ok,wake_lock_request:$wake_lock_request,battery_optimization:$battery_optimization,ca_file:$ca_file,dns_telegram:$dns_telegram,tls_telegram:$tls_telegram,telegram_api:$telegram_api,model_api:$model_api,model_latency_ms:$model_latency_ms},battery:$battery}' > "$REPORT"
chmod 600 "$REPORT"
cat "$REPORT"

if [[ "$arch_uname" != "aarch64" || "$arch_abi" != arm64-v8a || "$termux_runtime" != true || "$write_ok" != true ]]; then
  echo "Preflight: FAIL (platform baseline)" >&2
  exit 1
fi
if (( NETWORK )) && [[ "$dns_telegram" != pass || "$tls_telegram" != pass || "$telegram_api" != pass || "$model_api" != pass ]]; then
  echo "Preflight: FAIL (network/API baseline)" >&2
  exit 1
fi
echo "Preflight: PASS"
