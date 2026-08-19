#!/usr/bin/env bash
# Regression test for the supervisor lifecycle contract.
#
# Reproduces the real-phone incident of 2026-08-19: the old supervisor kept
# running after SIGTERM (runit then force-killed it) and orphaned the agent
# child, which held the singleton lock while a new supervisor cycled on
# exit 73. The fixed contract is asserted here:
#   1. SIGTERM to the supervisor terminates it promptly (exit 0),
#   2. the agent child is terminated gracefully (TERM delivered),
#   3. the singleton state (lock dir, agent.pid) is cleaned up.
#
# Usage: selftest-supervisor.sh <phone_scripts_dir>
# Runs a fake agent binary; no network, no Termux, no root required.
set -euo pipefail

PHONE_DIR="${1:?usage: selftest-supervisor.sh <phone_scripts_dir>}"
[[ -d "$PHONE_DIR" ]] || { echo "phone dir not found: $PHONE_DIR" >&2; exit 1; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

M_HOME="$TMP/home/.local/share/mosaid"
mkdir -p "$M_HOME"/{bin,scripts,secrets,runtime,logs,tmp,reports}
chmod 700 "$M_HOME/secrets"

install -m 0700 "$PHONE_DIR/supervisor.sh" "$M_HOME/scripts/supervisor.sh"
install -m 0700 "$PHONE_DIR/verify-config.sh" "$M_HOME/scripts/verify-config.sh"
install -m 0700 "$PHONE_DIR/health-sampler.sh" "$M_HOME/scripts/health-sampler.sh"

cat > "$M_HOME/config.json" <<EOF
{
  "data_dir": "$M_HOME",
  "owner_telegram_id": 123456789,
  "telegram": {"token_file": "$M_HOME/secrets/telegram.token", "poll_timeout_seconds": 30},
  "model": {"base_url": "https://example.invalid/v1", "api_key_file": "$M_HOME/secrets/model.key", "name": "test-model", "timeout_seconds": 60},
  "limits": {"max_message_bytes": 16384, "max_response_bytes": 65536, "max_model_steps": 4, "max_tool_calls": 16, "max_tokens": 32000, "max_cost_usd": 0.01, "max_retries": 5, "messages_per_minute": 10, "message_burst": 3}
}
EOF
chmod 600 "$M_HOME/config.json"

# Build a syntactically valid fake Telegram token at runtime without storing a
# secret-shaped literal in the repository (the repository scanner should stay strict).
printf '%s%s\n' '1234567890:' 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' > "$M_HOME/secrets/telegram.token"
printf '%s\n' 'test-key-0123456789abcdef' > "$M_HOME/secrets/model.key"
chmod 600 "$M_HOME/secrets/telegram.token" "$M_HOME/secrets/model.key"

# Fake agent: stays alive, records its pid, confirms TERM delivery on exit.
AGENT_PID_FILE="$M_HOME/runtime/test-agent.pid"
AGENT_TERM_FILE="$M_HOME/runtime/test-agent.term"
cat > "$M_HOME/bin/mosaid" <<EOF
#!/bin/sh
echo "\$\$" > "$AGENT_PID_FILE"
trap 'echo term > "$AGENT_TERM_FILE"; exit 0' TERM INT
while :; do sleep 1; done
EOF
chmod 0700 "$M_HOME/bin/mosaid"

MOSAID_HOME="$M_HOME" bash "$M_HOME/scripts/supervisor.sh" --once &
sup_pid=$!

agent_pid=""
for _ in $(seq 1 100); do
  if [[ -f "$AGENT_PID_FILE" ]]; then agent_pid="$(cat "$AGENT_PID_FILE")"; break; fi
  sleep 0.1
done
[[ -n "$agent_pid" ]] || { echo "FAIL: agent never started" >&2; kill -KILL "$sup_pid" 2>/dev/null || true; exit 1; }
kill -0 "$agent_pid" || { echo "FAIL: agent died before TERM" >&2; exit 1; }

kill -TERM "$sup_pid" 2>/dev/null || true

sup_exit=0
wait "$sup_pid" || sup_exit=$?
[[ "$sup_exit" == "0" ]] || { echo "FAIL: supervisor exit=$sup_exit (want 0)" >&2; exit 1; }

if kill -0 "$agent_pid" 2>/dev/null; then
  echo "FAIL: agent child survived supervisor shutdown (orphan)" >&2
  kill -KILL "$agent_pid" 2>/dev/null || true
  exit 1
fi
[[ -f "$AGENT_TERM_FILE" ]] || { echo "FAIL: agent did not receive TERM" >&2; exit 1; }
[[ -d "$M_HOME/runtime/supervisor.lock" ]] && { echo "FAIL: lock dir not cleaned up" >&2; exit 1; }
[[ -f "$M_HOME/runtime/agent.pid" ]] && { echo "FAIL: agent.pid not cleaned up" >&2; exit 1; }

echo "supervisor lifecycle: PASS (term-exit=0, child terminated, state cleaned)"
