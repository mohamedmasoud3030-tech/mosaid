#!/usr/bin/env bash
# Local/CI self-test for a staged Mosaid phone kit.
# Usage: selftest.sh <staged_kit_dir> [linux_binary_for_smoke]
# Checks: script syntax, checksums, template JSON, secret-pattern scan,
# and (optionally) fail-closed config rejection by the binary.
set -euo pipefail

STAGE="${1:?usage: selftest.sh <staged_kit_dir> [linux_binary]}"
SMOKE_BIN="${2:-}"
[[ -d "$STAGE" ]] || { echo "stage dir not found: $STAGE" >&2; exit 1; }
if [[ -n "$SMOKE_BIN" ]]; then
  SMOKE_BIN="$(cd "$(dirname "$SMOKE_BIN")" && pwd)/$(basename "$SMOKE_BIN")"
fi
cd "$STAGE"

echo "== script syntax =="
for f in phone/*.sh phone/*.boot; do
  bash -n "$f" || { echo "syntax error in $f" >&2; exit 1; }
done
echo "ok"

echo "== binary checksum =="
sha256sum -c BINARY.sha256

echo "== staged file checksums =="
sha256sum -c FILES.sha256

echo "== config template is valid JSON with pinned tripwires =="
jq -e . config/config.phone.template.json >/dev/null
[[ "$(jq -r '.model.base_url' config/config.phone.template.json)" == https://generativelanguage.googleapis.com/v1beta/openai ]] || { echo "base_url drift" >&2; exit 1; }
[[ "$(jq -r '.limits.max_cost_usd' config/config.phone.template.json)" == "0.01" ]] || { echo "cost tripwire drift" >&2; exit 1; }
[[ "$(jq -r '.limits.messages_per_minute' config/config.phone.template.json)" == "10" ]] || { echo "flood limit drift" >&2; exit 1; }

echo "== secret-pattern scan (kit must contain no secrets) =="
if grep -rEn '[0-9]{7,12}:[A-Za-z0-9_-]{30,}|AIza[0-9A-Za-z_-]{30,}' \
    --exclude FILES.sha256 --exclude '*.sha256' . ; then
  echo "secret-like pattern found in kit" >&2
  exit 1
fi
echo "ok"

echo "== kit must never include a secrets directory =="
if find . -type d -name secrets | grep -q .; then
  echo "secrets dir leaked into kit" >&2
  exit 1
fi
echo "ok"

echo "== supervisor lifecycle regression (TERM exit, no orphan) =="
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/selftest-supervisor.sh" "$(pwd)/phone"
echo "ok"

if [[ -n "$SMOKE_BIN" && -f "$SMOKE_BIN" ]]; then
  echo "== smoke: binary must reject the unpersonalized template (fail-closed) =="
  if "$SMOKE_BIN" --config "$STAGE/config/config.phone.template.json" >/dev/null 2>&1; then
    echo "binary accepted an invalid config — fail-closed broken" >&2
    exit 1
  fi
  echo "ok (rejected as expected)"
fi

echo "SELFTEST PASS"
