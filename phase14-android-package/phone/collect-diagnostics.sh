#!/data/data/com.termux/files/usr/bin/bash
# Collect a redacted diagnostics archive for the owner to review/share.
# Never includes the secrets directory. Scrubs known secret formats from logs.
set -euo pipefail
umask 077

M_HOME="${MOSAID_HOME:-$HOME/.local/share/mosaid}"
REPORT_DIR="$M_HOME/reports"
TMP="$M_HOME/tmp/diag.$$"
OUT="$REPORT_DIR/phase14-diagnostics-$(date -u +%Y%m%dT%H%M%SZ).tar.gz"
mkdir -p "$TMP" "$REPORT_DIR"
trap 'rm -rf "$TMP"' EXIT

bash "$M_HOME/scripts/verify-config.sh" >/dev/null

# Latest preflight report.
latest_preflight="$(ls -1t "$REPORT_DIR"/preflight-*.json 2>/dev/null | head -1 || true)"
[[ -n "$latest_preflight" ]] && cp "$latest_preflight" "$TMP/preflight.json"

# Health samples and restart events.
[[ -f "$M_HOME/runtime/health.ndjson" ]] && cp "$M_HOME/runtime/health.ndjson" "$TMP/health.ndjson"
[[ -f "$M_HOME/runtime/restart.count" ]] && cp "$M_HOME/runtime/restart.count" "$TMP/restart.count"

# Logs, scrubbed for known secret formats (Telegram tokens, Google keys).
if [[ -d "$M_HOME/logs" ]]; then
  mkdir -p "$TMP/logs"
  find "$M_HOME/logs" -maxdepth 1 -type f ! -name config -print0 |
    while IFS= read -r -d '' f; do
      sed -E \
        -e 's/[0-9]{7,12}:[A-Za-z0-9_-]{30,}/[REDACTED-TELEGRAM-TOKEN]/g' \
        -e 's/AIza[0-9A-Za-z_-]{30,}/[REDACTED-API-KEY]/g' \
        "$f" > "$TMP/logs/$(basename "$f")" 2>/dev/null || true
    done
fi

# Config (contains paths only; secrets live in separate files) and identity.
cp "$M_HOME/config.json" "$TMP/config.json"
( cd "$M_HOME" && sha256sum -c BINARY.sha256 > "$TMP/binary-sha256.check" 2>&1 || true )
"$M_HOME/bin/mosaid" --version > "$TMP/version.txt" 2>&1 || true

printf '%s\n' \
  "Mosaid Phase 14 diagnostics" \
  "Generated: $(date -u +%FT%TZ)" \
  "Secrets are never included in this archive." > "$TMP/README.txt"

tar -C "$TMP" --sort=name --owner=0 --group=0 --numeric-owner -czf "$OUT" .
chmod 600 "$OUT"
echo "diagnostics archive: $OUT"
