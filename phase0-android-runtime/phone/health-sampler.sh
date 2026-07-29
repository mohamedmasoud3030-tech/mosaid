#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
umask 077

child_pid="${1:?child pid required}"
restart_count="${2:-0}"
P0_HOME="${PICOCLAW_PHASE0_HOME:-$HOME/.local/share/picoclaw-phase0}"
RUNTIME="$P0_HOME/runtime"
TESTS="$P0_HOME/tests"
INTERVAL="${PHASE0_HEALTH_INTERVAL_SECONDS:-60}"
mkdir -p "$RUNTIME" "$TESTS"
start_epoch="$(date +%s)"

number_or_null() { [[ "${1:-}" =~ ^-?[0-9]+([.][0-9]+)?$ ]] && printf '%s' "$1" || printf 'null'; }

while kill -0 "$child_pid" 2>/dev/null; do
  now="$(date +%s)"
  ts="$(date -u +%FT%TZ)"
  uptime=$((now-start_epoch))
  rss_kb="$(awk '/^VmRSS:/{print $2}' "/proc/$child_pid/status" 2>/dev/null || echo 0)"
  cpu_pct="$(ps -p "$child_pid" -o %cpu= 2>/dev/null | awk '{print $1+0}' || echo 0)"
  fd_count="$(find "/proc/$child_pid/fd" -mindepth 1 -maxdepth 1 2>/dev/null | wc -l | awk '{print $1}' || echo 0)"
  disk_free_kb="$(df -Pk "$P0_HOME" | awk 'NR==2{print $4}')"

  battery_pct=null
  battery_temp_c=null
  battery_status=null
  battery_plugged=null
  if command -v termux-battery-status >/dev/null 2>&1 && command -v timeout >/dev/null 2>&1; then
    battery="$(timeout 8 termux-battery-status 2>/dev/null || true)"
    if jq -e . >/dev/null 2>&1 <<<"$battery"; then
      battery_pct="$(number_or_null "$(jq -r '.percentage // ""' <<<"$battery")")"
      battery_temp_c="$(number_or_null "$(jq -r '.temperature // ""' <<<"$battery")")"
      battery_status="$(jq -c '.status // null' <<<"$battery")"
      battery_plugged="$(jq -c '.plugged // null' <<<"$battery")"
    fi
  fi

  tmp="$RUNTIME/health.json.tmp.$$"
  jq -n \
    --arg ts "$ts" --argjson epoch "$now" --argjson pid "$child_pid" \
    --argjson uptime "$uptime" --argjson restart_count "$restart_count" \
    --argjson rss_kb "${rss_kb:-0}" --argjson cpu_pct "${cpu_pct:-0}" \
    --argjson fd_count "${fd_count:-0}" --argjson disk_free_kb "${disk_free_kb:-0}" \
    --argjson battery_pct "$battery_pct" --argjson battery_temp_c "$battery_temp_c" \
    --argjson battery_status "$battery_status" --argjson battery_plugged "$battery_plugged" \
    '{timestamp:$ts,epoch:$epoch,pid:$pid,process_uptime_seconds:$uptime,restart_count:$restart_count,rss_kb:$rss_kb,cpu_percent:$cpu_pct,fd_count:$fd_count,disk_free_kb:$disk_free_kb,battery:{percentage:$battery_pct,temperature_c:$battery_temp_c,status:$battery_status,plugged:$battery_plugged}}' > "$tmp"
  mv -f "$tmp" "$RUNTIME/health.json"

  active="$TESTS/active.json"
  if [[ -f "$active" ]] && jq -e . "$active" >/dev/null 2>&1; then
    scenario="$(jq -r '.scenario' "$active")"
    end_epoch="$(jq -r '.end_epoch' "$active")"
    if [[ "$scenario" =~ ^[A-Za-z0-9._-]+$ ]] && (( now <= end_epoch )); then
      dir="$TESTS/$scenario"
      mkdir -p "$dir"
      csv="$dir/samples.csv"
      if [[ ! -f "$csv" ]]; then
        echo 'timestamp,epoch,pid,process_uptime_seconds,restart_count,rss_kb,cpu_percent,fd_count,disk_free_kb,battery_percentage,battery_temperature_c,battery_status,battery_plugged' > "$csv"
      fi
      printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n' \
        "$ts" "$now" "$child_pid" "$uptime" "$restart_count" "${rss_kb:-0}" "${cpu_pct:-0}" "${fd_count:-0}" "${disk_free_kb:-0}" \
        "$battery_pct" "$battery_temp_c" "${battery_status//,/;}" "${battery_plugged//,/;}" >> "$csv"
    elif (( now > end_epoch )); then
      touch "$TESTS/$scenario/ready-to-finalize"
    fi
  fi

  sleep "$INTERVAL"
done
