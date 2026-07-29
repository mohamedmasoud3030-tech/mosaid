#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
umask 077

P0_HOME="${PICOCLAW_PHASE0_HOME:-$HOME/.local/share/picoclaw-phase0}"
OUT="$P0_HOME/reports/phase0-diagnostics-$(date -u +%Y%m%dT%H%M%SZ).tar.gz"
STAGE="$P0_HOME/tmp/diagnostics.$$"
mkdir -p "$STAGE" "$P0_HOME/reports"
trap 'rm -rf "$STAGE"' EXIT

bot_token="$(jq -r '.channels.telegram.settings.token // .channel_list.telegram.settings.token // ""' "$P0_HOME/.security.yml")"
api_key="$(jq -r '.model_list["phase0-model"].api_keys[0] // ""' "$P0_HOME/.security.yml")"
for secret in "$bot_token" "$api_key"; do
  [[ -n "$secret" ]] || continue
  if grep -RFq "$secret" "$P0_HOME/logs" "$P0_HOME/reports" "$P0_HOME/tests" 2>/dev/null; then
    echo "Refusing to package diagnostics: a secret was found outside .security.yml" >&2
    exit 1
  fi
done
unset bot_token api_key

mkdir -p "$STAGE"/{reports,tests,logs,runtime,meta}
cp -a "$P0_HOME"/reports/. "$STAGE/reports/" 2>/dev/null || true
cp -a "$P0_HOME"/tests/. "$STAGE/tests/" 2>/dev/null || true
cp -a "$P0_HOME"/logs/. "$STAGE/logs/" 2>/dev/null || true
cp "$P0_HOME/config.json" "$STAGE/meta/config.json"
cp "$P0_HOME/BINARY.sha256" "$STAGE/meta/BINARY.sha256"
[[ -f "$P0_HOME/runtime/health.json" ]] && cp "$P0_HOME/runtime/health.json" "$STAGE/runtime/health.json"
[[ -f "$P0_HOME/runtime/restart.count" ]] && cp "$P0_HOME/runtime/restart.count" "$STAGE/runtime/restart.count"
sha256sum "$P0_HOME/bin/picoclaw-phase0" > "$STAGE/meta/installed-binary.sha256"
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
find "$STAGE" -type f -not -path "$STAGE/meta/files.sha256" -print0 | sort -z | xargs -0 sha256sum > "$P0_HOME/tmp/files.sha256.$$"
mv "$P0_HOME/tmp/files.sha256.$$" "$STAGE/meta/files.sha256"

tar -C "$STAGE" -czf "$OUT" .
sha256sum "$OUT" > "$OUT.sha256"
chmod 0600 "$OUT" "$OUT.sha256"
echo "Diagnostics: $OUT"
cat "$OUT.sha256"
if command -v termux-share >/dev/null 2>&1; then
  echo "To share without granting general storage access: termux-share -a send '$OUT'"
fi
