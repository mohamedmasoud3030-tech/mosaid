#!/data/data/com.termux/files/usr/bin/bash
# Package redacted Mosaid product diagnostics for review.
# Refuses to package anything when a secret is found outside the
# protected secret files.
set -euo pipefail
umask 077

MOSAID_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
CONFIG_HOME="${MOSAID_CONFIG_HOME:-$HOME/.config/mosaid}"
OUT="$MOSAID_HOME/reports/mosaid-diagnostics-$(date -u +%Y%m%dT%H%M%SZ).tar.gz"
STAGE="$MOSAID_HOME/tmp/diagnostics.$$"
mkdir -p "$STAGE" "$MOSAID_HOME/reports"
trap 'rm -rf "$STAGE"' EXIT

bot_token="$(cat "$CONFIG_HOME/telegram.token" 2>/dev/null || true)"
api_key="$(cat "$CONFIG_HOME/model.key" 2>/dev/null || true)"
for secret in "$bot_token" "$api_key"; do
  [[ -n "$secret" ]] || continue
  if grep -RFq "$secret" "$MOSAID_HOME/logs" "$MOSAID_HOME/reports" "$MOSAID_HOME/tests" 2>/dev/null; then
    echo "Refusing to package diagnostics: a secret was found outside the protected secret files" >&2
    exit 1
  fi
done
unset bot_token api_key

mkdir -p "$STAGE"/{reports,tests,logs,runtime,meta}
cp -a "$MOSAID_HOME"/reports/. "$STAGE/reports/" 2>/dev/null || true
cp -a "$MOSAID_HOME"/tests/. "$STAGE/tests/" 2>/dev/null || true
cp -a "$MOSAID_HOME"/logs/. "$STAGE/logs/" 2>/dev/null || true
cp "$MOSAID_HOME/config.json" "$STAGE/meta/config.json"
cp "$MOSAID_HOME/BINARY.sha256" "$STAGE/meta/BINARY.sha256"
[[ -f "$MOSAID_HOME/runtime/health.json" ]] && cp "$MOSAID_HOME/runtime/health.json" "$STAGE/runtime/health.json"
[[ -f "$MOSAID_HOME/runtime/restart.count" ]] && cp "$MOSAID_HOME/runtime/restart.count" "$STAGE/runtime/restart.count"
sha256sum "$MOSAID_HOME/bin/mosaid" > "$STAGE/meta/installed-binary.sha256"
{
  echo "generated_at=$(date -u +%FT%TZ)"
  echo "uname=$(uname -a 2>/dev/null || true)"
  echo "android_release=$(getprop ro.build.version.release 2>/dev/null || true)"
  echo "android_sdk=$(getprop ro.build.version.sdk 2>/dev/null || true)"
  echo "android_abi=$(getprop ro.product.cpu.abi 2>/dev/null || true)"
  echo "timezone=$(getprop persist.sys.timezone 2>/dev/null || true)"
  echo "termux_version=${TERMUX_VERSION:-unknown}"
} > "$STAGE/meta/device.txt"
termux-info > "$STAGE/meta/termux-info.txt" 2>&1 || true
dpkg-query -W > "$STAGE/meta/packages.tsv" 2>/dev/null || true
find "$STAGE" -type f -not -path "$STAGE/meta/files.sha256" -print0 | sort -z | xargs -0 sha256sum > "$MOSAID_HOME/tmp/files.sha256.$$"
mv "$MOSAID_HOME/tmp/files.sha256.$$" "$STAGE/meta/files.sha256"

tar -C "$STAGE" -czf "$OUT" .
sha256sum "$OUT" > "$OUT.sha256"
chmod 0600 "$OUT" "$OUT.sha256"
echo "Diagnostics: $OUT"
cat "$OUT.sha256"
if command -v termux-share >/dev/null 2>&1; then
  echo "To share without granting general storage access: termux-share -a send '$OUT'"
fi
