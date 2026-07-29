#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
umask 077

MODE="${1:-loop}"
[[ "$MODE" == "loop" || "$MODE" == "--once" ]] || { echo "usage: supervisor.sh [--once]" >&2; exit 2; }

P0_HOME="${PICOCLAW_PHASE0_HOME:-$HOME/.local/share/picoclaw-phase0}"
BIN="$P0_HOME/bin/picoclaw-phase0"
SCRIPTS="$P0_HOME/scripts"
RUNTIME="$P0_HOME/runtime"
CONFIG="$P0_HOME/config.json"
mkdir -p "$RUNTIME" "$P0_HOME/logs" "$P0_HOME/tmp"

bash "$SCRIPTS/verify-config.sh"

LOCK="$RUNTIME/supervisor.lock"
if ! mkdir "$LOCK" 2>/dev/null; then
  old_pid="$(cat "$LOCK/pid" 2>/dev/null || true)"
  if [[ "$old_pid" =~ ^[0-9]+$ ]] && kill -0 "$old_pid" 2>/dev/null; then
    echo "Phase 0 supervisor is already running (pid=$old_pid)" >&2
    exit 73
  fi
  rm -rf "$LOCK"
  mkdir "$LOCK" || exit 73
fi
echo $$ > "$LOCK/pid"
echo $$ > "$RUNTIME/supervisor.pid"

child_pid=""
redactor_pid=""
sampler_pid=""
fifo=""
clean_cycle() {
  [[ -n "$sampler_pid" ]] && kill "$sampler_pid" 2>/dev/null || true
  [[ -n "$redactor_pid" ]] && kill "$redactor_pid" 2>/dev/null || true
  [[ -n "$sampler_pid" ]] && wait "$sampler_pid" 2>/dev/null || true
  [[ -n "$redactor_pid" ]] && wait "$redactor_pid" 2>/dev/null || true
  [[ -n "$fifo" ]] && rm -f "$fifo"
  rm -f "$RUNTIME/agent.pid"
  child_pid=""; redactor_pid=""; sampler_pid=""; fifo=""
}
shutdown() {
  trap - TERM INT HUP EXIT
  if [[ -n "$child_pid" ]] && kill -0 "$child_pid" 2>/dev/null; then
    kill -TERM "$child_pid" 2>/dev/null || true
    for _ in {1..20}; do kill -0 "$child_pid" 2>/dev/null || break; sleep 1; done
    kill -KILL "$child_pid" 2>/dev/null || true
    wait "$child_pid" 2>/dev/null || true
  fi
  clean_cycle
  rm -f "$RUNTIME/supervisor.pid"
  rm -rf "$LOCK"
  command -v termux-wake-unlock >/dev/null 2>&1 && termux-wake-unlock >/dev/null 2>&1 || true
}
trap shutdown TERM INT HUP EXIT

command -v termux-wake-lock >/dev/null 2>&1 && termux-wake-lock >/dev/null 2>&1 || true

restart_file="$RUNTIME/restart.count"
[[ -f "$restart_file" ]] || echo 0 > "$restart_file"
backoff=2

while :; do
  restart_count="$(cat "$restart_file" 2>/dev/null || echo 0)"
  [[ "$restart_count" =~ ^[0-9]+$ ]] || restart_count=0
  restart_count=$((restart_count+1))
  echo "$restart_count" > "$restart_file"
  started_epoch="$(date +%s)"

  fifo="$RUNTIME/agent-log.$$.$restart_count.fifo"
  rm -f "$fifo"
  mkfifo -m 600 "$fifo"
  bash "$SCRIPTS/redact-stream.sh" < "$fifo" &
  redactor_pid=$!

  export PICOCLAW_PHASE0_QUALIFICATION=1
  export PICOCLAW_CONFIG="$CONFIG"
  export PICOCLAW_HOME="$P0_HOME"
  "$BIN" gateway > "$fifo" 2>&1 &
  child_pid=$!
  echo "$child_pid" > "$RUNTIME/agent.pid"
  bash "$SCRIPTS/health-sampler.sh" "$child_pid" "$restart_count" &
  sampler_pid=$!

  printf '{"timestamp":"%s","event":"agent_start","pid":%s,"restart_count":%s}\n' \
    "$(date -u +%FT%TZ)" "$child_pid" "$restart_count"

  set +e
  wait "$child_pid"
  status=$?
  set -e
  ended_epoch="$(date +%s)"
  runtime=$((ended_epoch-started_epoch))
  printf '{"timestamp":"%s","event":"agent_exit","exit_code":%s,"runtime_seconds":%s,"restart_count":%s}\n' \
    "$(date -u +%FT%TZ)" "$status" "$runtime" "$restart_count"
  clean_cycle

  [[ "$MODE" == "--once" ]] && exit "$status"
  if (( runtime >= 600 )); then backoff=2; fi
  printf '{"timestamp":"%s","event":"restart_backoff","seconds":%s}\n' "$(date -u +%FT%TZ)" "$backoff"
  sleep "$backoff"
  if (( backoff < 300 )); then
    backoff=$((backoff*2))
    (( backoff > 300 )) && backoff=300
  fi
done
