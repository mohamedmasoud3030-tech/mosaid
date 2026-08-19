#!/data/data/com.termux/files/usr/bin/bash
# Sample system/process health every 60s while the Mosaid binary runs.
# Usage: health-sampler.sh <pid> <cycle>
set -euo pipefail
umask 077

PID="${1:?pid required}"
CYCLE="${2:-1}"
M_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
RUNTIME="$M_HOME/runtime"
OUT="$RUNTIME/health.ndjson"
mkdir -p "$RUNTIME"
touch "$OUT" && chmod 600 "$OUT"

battery_json() {
  command -v termux-battery-status >/dev/null 2>&1 || { echo '{}'; return; }
  termux-battery-status 2>/dev/null | jq -c '{percentage,status,temperature,plugged}' 2>/dev/null || echo '{}'
}

while kill -0 "$PID" 2>/dev/null; do
  ts="$(date -u +%FT%TZ)"
  mem_total="$(awk '/^MemTotal:/{print $2}' /proc/meminfo 2>/dev/null || echo 0)"
  mem_avail="$(awk '/^MemAvailable:/{print $2}' /proc/meminfo 2>/dev/null || echo 0)"
  load1="$(cut -d' ' -f1 /proc/loadavg 2>/dev/null || echo 0)"
  rss_kb="$(awk '/^VmRSS:/{print $2}' "/proc/$PID/status" 2>/dev/null || echo 0)"
  battery="$(battery_json)"
  jq -nc \
    --arg ts "$ts" \
    --arg pid "$PID" \
    --arg cycle "$CYCLE" \
    --arg load1 "$load1" \
    --argjson mem_total "$mem_total" \
    --argjson mem_avail "$mem_avail" \
    --argjson rss_kb "$rss_kb" \
    --argjson battery "$battery" \
    '{timestamp:$ts,pid:($pid|tonumber),cycle:($cycle|tonumber),
      load1:($load1|tonumber),mem_total_kb:$mem_total,mem_available_kb:$mem_avail,
      rss_kb:$rss_kb,battery:$battery}' >> "$OUT"
  sleep 60
done
