#!/data/data/com.termux/files/usr/bin/bash
# Mosaid supervisor: singleton lock, wake lock, backoff restarts,
# health sampling, redacted event log, clean shutdown.
# Logs are redacted inside the binary; no extra redactor stage is needed.
set -euo pipefail
umask 077

MODE="${1:-loop}"
[[ "$MODE" == "loop" || "$MODE" == "--once" ]] || { echo "usage: supervisor.sh [--once]" >&2; exit 2; }

M_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
BIN="$M_HOME/bin/mosaid"
SCRIPTS="$M_HOME/scripts"
RUNTIME="$M_HOME/runtime"
CONFIG="$M_HOME/config.json"
mkdir -p "$RUNTIME" "$M_HOME/logs" "$M_HOME/tmp"

bash "$SCRIPTS/verify-config.sh"

# DNS guard: some Termux builds ship $PREFIX/etc/resolv.conf pointing at an
# inactive local resolver (nameserver ::1); pure-Go binaries then fail to
# resolve (curl works because Android resolves through netd for it). Ensure
# a working public DNS is configured before each start.
if ! grep -qE '^nameserver[[:space:]]+[0-9]+(\.[0-9]+){3}[[:space:]]*$' "$PREFIX/etc/resolv.conf" 2>/dev/null; then
  printf 'nameserver 1.1.1.1\nnameserver 8.8.8.8\n' > "$PREFIX/etc/resolv.conf" 2>/dev/null || true
fi

LOCK="$RUNTIME/supervisor.lock"
if ! mkdir "$LOCK" 2>/dev/null; then
  old_pid="$(cat "$LOCK/pid" 2>/dev/null || true)"
  if [[ "$old_pid" =~ ^[0-9]+$ ]] && kill -0 "$old_pid" 2>/dev/null; then
    echo "Mosaid supervisor is already running (pid=$old_pid)" >&2
    exit 73
  fi
  rm -rf "$LOCK"
  mkdir "$LOCK" || exit 73
fi
echo $$ > "$LOCK/pid"
echo $$ > "$RUNTIME/supervisor.pid"

child_pid=""
sampler_pid=""
clean_cycle() {
  [[ -n "$sampler_pid" ]] && kill "$sampler_pid" 2>/dev/null || true
  [[ -n "$sampler_pid" ]] && wait "$sampler_pid" 2>/dev/null || true
  rm -f "$RUNTIME/agent.pid"
  child_pid=""; sampler_pid=""
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

  "$BIN" --config "$CONFIG" &
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
