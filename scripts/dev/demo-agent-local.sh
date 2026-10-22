#!/usr/bin/env bash
# Run the real Mosaid binary against a scripted local HTTPS provider.
#
# No API key, no Telegram bot, no network egress: this proves the agent loop,
# the policy engine, the approval engine, the durable stores and the workspace
# tools all work together. It is a demonstration harness, not a test suite.
#
# Usage: scripts/dev/demo-agent-local.sh
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
work="$(mktemp -d)"
port="${DEMO_PORT:-18443}"
cleanup() {
  [[ -n "${provider_pid:-}" ]] && kill "$provider_pid" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

mkdir -p "$work/workspace/notes" "$work/data" "$work/secrets"
printf 'demo-key\n' > "$work/secrets/model.key"
chmod 600 "$work/secrets/model.key"
printf 'demo-token\n' > "$work/secrets/telegram.token"
chmod 600 "$work/secrets/telegram.token"
printf -- '- شراء الحليب\n' > "$work/workspace/notes/groceries.md"

echo "== building mosaid-cli =="
(cd "$repo" && go build -o "$work/mosaid-cli" ./cmd/mosaid-cli)

echo "== generating a throwaway TLS certificate =="
openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
  -keyout "$work/key.pem" -out "$work/cert.pem" \
  -subj "/CN=127.0.0.1" -addext "subjectAltName=IP:127.0.0.1" >/dev/null 2>&1

echo "== starting the scripted provider =="
python3 "$repo/scripts/dev/fake_provider.py" "$work/cert.pem" "$work/key.pem" "$port" > "$work/provider.log" 2>&1 &
provider_pid=$!
for _ in $(seq 1 50); do
  grep -q ready "$work/provider.log" && break
  sleep 0.1
done

cat > "$work/config.json" <<JSON
{
  "data_dir": "$work/data",
  "workspace_dir": "$work/workspace",
  "owner_telegram_id": 42,
  "telegram": {"token_file": "$work/secrets/telegram.token", "poll_timeout_seconds": 30},
  "model": {
    "base_url": "https://127.0.0.1:$port/v1",
    "api_key_file": "$work/secrets/model.key",
    "name": "demo-model",
    "timeout_seconds": 30,
    "input_price_per_1m": 0,
    "output_price_per_1m": 0
  },
  "agent": {"max_steps": 4, "max_calls_per_step": 4, "max_tool_result_bytes": 8192, "max_tokens": 512},
  "limits": {
    "max_message_bytes": 16384, "max_response_bytes": 65536, "max_model_steps": 6,
    "max_tool_calls": 16, "max_tokens": 32000, "max_cost_usd": 0.01, "max_retries": 5,
    "messages_per_minute": 30, "message_burst": 5
  }
}
JSON

echo "== running a real session =="
# The CLI trusts the system root store; point Go at our throwaway certificate.
SSL_CERT_FILE="$work/cert.pem" python3 "$repo/scripts/dev/demo_driver.py" \
  "$work/mosaid-cli" --config "$work/config.json"

echo
echo "== resulting file on disk =="
cat "$work/workspace/notes/todo.md"
