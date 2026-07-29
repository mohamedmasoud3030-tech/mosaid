#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
umask 077

active="${1:?active test json required}"
P0_HOME="${PICOCLAW_PHASE0_HOME:-$HOME/.local/share/picoclaw-phase0}"
id="$(jq -r '.scenario' "$active")"
dir="$P0_HOME/tests/$id"
csv="$dir/samples.csv"
[[ -f "$csv" ]] || { echo "No samples for $id" >&2; exit 1; }

start="$(jq -r '.start_epoch' "$active")"
end="$(jq -r '.end_epoch' "$active")"
duration=$((end-start))
expected_echo="$(jq -r '.scenario_definition.expected_echo' "$active")"
restart_min="$(jq -r '.scenario_definition.restart_min' "$active")"
restart_max="$(jq -r '.scenario_definition.restart_max' "$active")"
baseline_restart="$(jq -r '.baseline_restart_count' "$active")"
baseline_mid="$(jq -r '.baseline_message_id' "$active")"
now="$(date +%s)"

stats="$dir/stats.env"
awk -F, '
NR==1 {next}
{
 n++
 epoch=$2+0; rss=$6+0; cpu=$7+0
 if (prev>0 && epoch-prev>maxgap) maxgap=epoch-prev
 prev=epoch
 sumrss+=rss; sumcpu+=cpu
 if (rss>maxrss) maxrss=rss
 if (cpu>maxcpu) maxcpu=cpu
 if ($10!="null" && $10!="") { if (bn==0) bfirst=$10+0; blast=$10+0; bn++ }
 if ($11!="null" && $11!="") { t=$11+0; sumtemp+=t; tn++; if(t>maxtemp) maxtemp=t }
 if (n==1) firstepoch=epoch
 lastepoch=epoch
}
END {
 printf "samples=%d\n",n
 printf "first_epoch=%d\n",firstepoch
 printf "last_epoch=%d\n",lastepoch
 printf "max_gap=%d\n",maxgap
 printf "avg_rss_kb=%.2f\n",n?sumrss/n:0
 printf "max_rss_kb=%.2f\n",maxrss
 printf "avg_cpu=%.2f\n",n?sumcpu/n:0
 printf "max_cpu=%.2f\n",maxcpu
 printf "temp_samples=%d\n",tn
 printf "avg_temp=%.2f\n",tn?sumtemp/tn:0
 printf "max_temp=%.2f\n",maxtemp
 printf "battery_samples=%d\n",bn
 printf "battery_first=%.2f\n",bfirst
 printf "battery_last=%.2f\n",blast
}' "$csv" > "$stats"

declare -A S
while IFS='=' read -r k v; do S[$k]="$v"; done < "$stats"
samples="${S[samples]:-0}"
expected_samples=$((duration/60))
(( expected_samples < 1 )) && expected_samples=1
coverage="$(awk -v a="$samples" -v b="$expected_samples" 'BEGIN{printf "%.2f",100*a/b}')"

current_restart="$(cat "$P0_HOME/runtime/restart.count" 2>/dev/null || echo "$baseline_restart")"
restart_delta=$((current_restart-baseline_restart))

# Extract Phase 0 message events after the baseline Telegram message id.
events="$dir/phase0-events.tsv"
: > "$events"
grep -h 'Qualification message handled' "$P0_HOME"/logs/* 2>/dev/null | awk -v base="$baseline_mid" '
{
 id=""; kind=""; replies=""; latency=""
 for(i=1;i<=NF;i++) {
   if($i ~ /^message_id=/){split($i,a,"="); id=a[2]}
   if($i ~ /^kind=/){split($i,a,"="); kind=a[2]}
   if($i ~ /^reply_count=/){split($i,a,"="); replies=a[2]}
   if($i ~ /^latency_ms=/){split($i,a,"="); latency=a[2]}
 }
 if((id+0)>base) print id "\t" kind "\t" replies "\t" latency
}' > "$events"

echo_ids="$dir/echo-message-ids.txt"
awk -F '\t' '$2=="echo"{print $1}' "$events" | sort -n > "$echo_ids"
echo_total="$(wc -l < "$echo_ids" | awk '{print $1}')"
echo_unique="$(sort -u "$echo_ids" | wc -l | awk '{print $1}')"
duplicates=$((echo_total-echo_unique))
lost="$(awk -F '\t' '$2=="echo" && $3!=1{n++} END{print n+0}' "$events")"
latencies="$dir/echo-latencies-ms.txt"
awk -F '\t' '$2=="echo" && $4 ~ /^[0-9]+$/{print $4}' "$events" | sort -n > "$latencies"
latency_n="$(wc -l < "$latencies" | awk '{print $1}')"
if (( latency_n > 0 )); then
  pidx=$(((95*latency_n+99)/100))
  p95="$(sed -n "${pidx}p" "$latencies")"
else p95=0; fi

secret_leaks=0
bot_token="$(jq -r '.channels.telegram.settings.token // .channel_list.telegram.settings.token // ""' "$P0_HOME/.security.yml")"
api_key="$(jq -r '.model_list["phase0-model"].api_keys[0] // ""' "$P0_HOME/.security.yml")"
for secret in "$bot_token" "$api_key"; do
  [[ -n "$secret" ]] || continue
  if grep -Fq "$secret" "$P0_HOME"/logs/* 2>/dev/null; then secret_leaks=$((secret_leaks+1)); fi
done
panic_count="$({ grep -hEi 'panic|fatal' "$P0_HOME"/logs/* 2>/dev/null || true; } | wc -l | awk '{print $1}')"

failures=()
incomplete=()
(( now >= end )) || failures+=("test finalized before planned duration")
awk -v x="$coverage" 'BEGIN{exit !(x>=90)}' || failures+=("sample coverage below 90%")
max_gap_limit=180
case "$id" in 10-reboot|11-kill-termux) max_gap_limit=600;; esac
(( ${S[max_gap]:-999999} <= max_gap_limit )) || failures+=("health sample gap exceeds ${max_gap_limit}s")
(( restart_delta >= restart_min && restart_delta <= restart_max )) || failures+=("restart delta outside expected range ${restart_min}-${restart_max}")
awk -v x="${S[avg_rss_kb]:-999999}" 'BEGIN{exit !(x<=76800)}' || failures+=("average RSS exceeds 75 MiB")
awk -v x="${S[max_rss_kb]:-999999}" 'BEGIN{exit !(x<=102400)}' || failures+=("peak RSS exceeds 100 MiB")
cpu_limit=10
case "$id" in 02-screen-locked-2h|03-run-12h|04-run-24h|14-on-charger) cpu_limit=5;; esac
awk -v x="${S[avg_cpu]:-999}" -v y="$cpu_limit" 'BEGIN{exit !(x<=y)}' || failures+=("average CPU exceeds ${cpu_limit}%")
if (( ${S[temp_samples]:-0} == 0 )); then incomplete+=("battery temperature unavailable");
else awk -v x="${S[max_temp]:-999}" 'BEGIN{exit !(x<=45)}' || failures+=("battery temperature exceeds 45 C"); fi
(( echo_unique == expected_echo )) || failures+=("unique echo replies $echo_unique != expected $expected_echo")
(( duplicates == 0 )) || failures+=("duplicate handling count is $duplicates")
(( lost == 0 )) || failures+=("echo events without exactly one reply: $lost")
(( latency_n == 0 || p95 <= 3000 )) || failures+=("echo p95 latency exceeds 3000 ms")
(( secret_leaks == 0 )) || failures+=("secret found in logs")
(( panic_count == 0 )) || failures+=("panic/fatal entries found in logs")

battery_drain_per_hour=null
if (( ${S[battery_samples]:-0} >= 2 )); then
  battery_drain_per_hour="$(awk -v a="${S[battery_first]}" -v b="${S[battery_last]}" -v d="$duration" 'BEGIN{printf "%.3f",(a-b)/(d/3600)}')"
  case "$id" in
    02-screen-locked-2h|13-low-battery)
      awk -v x="$battery_drain_per_hour" 'BEGIN{exit !(x<=4)}' || failures+=("battery drain exceeds 4 percentage points/hour")
      ;;
    03-run-12h|04-run-24h)
      awk -v x="$battery_drain_per_hour" 'BEGIN{exit !(x<=3)}' || failures+=("battery drain exceeds 3 percentage points/hour")
      ;;
    14-on-charger)
      awk -v a="${S[battery_first]}" -v b="${S[battery_last]}" 'BEGIN{exit !((a-b)<=2)}' || failures+=("battery declined by more than 2 percentage points on charger")
      ;;
  esac
else incomplete+=("battery percentage unavailable"); fi

result=PASS
(( ${#failures[@]} == 0 )) || result=FAIL
if [[ "$result" == PASS && ${#incomplete[@]} -gt 0 ]]; then result=INCOMPLETE; fi
fail_json="$(printf '%s\n' "${failures[@]:-}" | jq -Rsc 'split("\n") | map(select(length>0))')"
inc_json="$(printf '%s\n' "${incomplete[@]:-}" | jq -Rsc 'split("\n") | map(select(length>0))')"

jq -n --arg scenario "$id" --arg result "$result" --arg generated_at "$(date -u +%FT%TZ)" \
  --argjson planned_duration_seconds "$duration" --argjson samples "$samples" --argjson expected_samples "$expected_samples" --argjson sample_coverage_percent "$coverage" \
  --argjson max_sample_gap_seconds "${S[max_gap]:-0}" --argjson restart_delta "$restart_delta" \
  --argjson avg_rss_kb "${S[avg_rss_kb]:-0}" --argjson max_rss_kb "${S[max_rss_kb]:-0}" \
  --argjson avg_cpu_percent "${S[avg_cpu]:-0}" --argjson max_cpu_percent "${S[max_cpu]:-0}" \
  --argjson temperature_samples "${S[temp_samples]:-0}" --argjson avg_temperature_c "${S[avg_temp]:-0}" --argjson max_temperature_c "${S[max_temp]:-0}" \
  --argjson battery_samples "${S[battery_samples]:-0}" --argjson battery_start "${S[battery_first]:-0}" --argjson battery_end "${S[battery_last]:-0}" --argjson battery_drain_per_hour "$battery_drain_per_hour" \
  --argjson expected_echo "$expected_echo" --argjson echo_total "$echo_total" --argjson echo_unique "$echo_unique" --argjson duplicates "$duplicates" --argjson lost "$lost" --argjson echo_p95_latency_ms "$p95" \
  --argjson secret_leaks "$secret_leaks" --argjson panic_fatal_count "$panic_count" --argjson failures "$fail_json" --argjson incomplete "$inc_json" \
  '{scenario:$scenario,result:$result,generated_at:$generated_at,planned_duration_seconds:$planned_duration_seconds,health:{samples:$samples,expected_samples:$expected_samples,coverage_percent:$sample_coverage_percent,max_gap_seconds:$max_sample_gap_seconds,restart_delta:$restart_delta},resources:{avg_rss_kb:$avg_rss_kb,max_rss_kb:$max_rss_kb,avg_cpu_percent:$avg_cpu_percent,max_cpu_percent:$max_cpu_percent,temperature_samples:$temperature_samples,avg_temperature_c:$avg_temperature_c,max_temperature_c:$max_temperature_c,battery_samples:$battery_samples,battery_start:$battery_start,battery_end:$battery_end,battery_drain_percent_per_hour:$battery_drain_per_hour},telegram:{expected_echo:$expected_echo,total_echo_events:$echo_total,unique_echo_message_ids:$echo_unique,duplicate_handling:$duplicates,lost_or_zero_reply:$lost,p95_echo_latency_ms:$echo_p95_latency_ms},security:{secret_leaks:$secret_leaks,panic_fatal_count:$panic_fatal_count},failures:$failures,incomplete:$incomplete}' > "$dir/result.json"

cat > "$dir/report.md" <<EOF
# Phase 0 test result — $id

- Result: **$result**
- Planned duration: $duration seconds
- Health coverage: $coverage% ($samples/$expected_samples samples)
- Restarts during test: $restart_delta (allowed $restart_min-$restart_max)
- RSS average / peak: ${S[avg_rss_kb]:-0} / ${S[max_rss_kb]:-0} KiB
- CPU average / peak: ${S[avg_cpu]:-0}% / ${S[max_cpu]:-0}%
- Temperature average / peak: ${S[avg_temp]:-0} / ${S[max_temp]:-0} C (${S[temp_samples]:-0} samples)
- Battery start / end: ${S[battery_first]:-0}% / ${S[battery_last]:-0}%
- Echo unique / expected: $echo_unique / $expected_echo
- Duplicate handling: $duplicates
- Lost or zero-reply echo events: $lost
- Echo p95 latency: $p95 ms
- Secret leaks detected: $secret_leaks
- Panic/fatal entries: $panic_count

## Failures
$(if ((${#failures[@]})); then printf -- '- %s\n' "${failures[@]}"; else echo '- None'; fi)

## Incomplete evidence
$(if ((${#incomplete[@]})); then printf -- '- %s\n' "${incomplete[@]}"; else echo '- None'; fi)

Evidence files: samples.csv, phase0-events.tsv, echo-message-ids.txt, echo-latencies-ms.txt, and notes.md.
EOF
chmod 600 "$dir/result.json" "$dir/report.md" "$events" "$stats"
cat "$dir/result.json"
