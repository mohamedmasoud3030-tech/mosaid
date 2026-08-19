#!/usr/bin/env bash
# Build the Mosaid Phase 14 phone kit (tarball + checksums) from a built binary.
# Usage: build-phone-kit.sh [binary] [out_dir]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BINARY="${1:-$ROOT/../build/mosaid-android-arm64}"
OUT_DIR="${2:-$ROOT/release}"
[[ -f "$BINARY" ]] || { echo "binary not found: $BINARY" >&2; exit 1; }

STAGE="$OUT_DIR/mosaid-phone-kit"
rm -rf "$STAGE"
mkdir -p "$STAGE"/{bin,config,phone,manifests}
install -m 0755 "$BINARY" "$STAGE/bin/mosaid"
cp "$ROOT/config/config.phone.template.json" "$STAGE/config/"
cp "$ROOT"/phone/* "$STAGE/phone/"
chmod 0755 "$STAGE"/phone/*.sh "$STAGE"/phone/*.boot
cp "$ROOT/../security/product-sbom.cdx.json" "$STAGE/manifests/"
cp "$ROOT/../security/product-license-report.tsv" "$STAGE/manifests/"
cp "$ROOT/../THIRD_PARTY_NOTICES.md" "$STAGE/manifests/"
cp "$ROOT/../LICENSE" "$STAGE/manifests/"
( cd "$STAGE" && sha256sum bin/mosaid > BINARY.sha256 )
cat > "$STAGE/README-FIRST.txt" <<'EOF'
MOSAID PHASE 14 PHONE KIT — PRIVATE OWNER USE ONLY

Requirements:
- Android arm64-v8a
- Current official Termux from F-Droid or Termux GitHub releases
- Termux:Boot from the same source/signing family, opened once
- A private Telegram bot token (owner-only) and a model API key
- Default model endpoint: Google Gemini free tier via the OpenAI-compatible API

Run inside Termux after extracting:
  bash phone/install-phone.sh

Free-only tripwire: limits.max_cost_usd is pinned to 0.01 and the runtime has
no cost accounting; any paid model would be stopped by the budget tripwire.
The free tier may use conversations to improve the model. Do not share
secrets or private data in chats. No shared-storage permission is requested.
EOF
(
  cd "$STAGE"
  find . -type f -not -name FILES.sha256 -print0 | sort -z | xargs -0 sha256sum > FILES.sha256
)
mkdir -p "$OUT_DIR"
tar -C "$OUT_DIR" --sort=name --mtime='UTC 2026-08-19 00:00:00' --owner=0 --group=0 --numeric-owner -czf "$OUT_DIR/mosaid-phone-kit.tar.gz" mosaid-phone-kit
( cd "$OUT_DIR" && sha256sum mosaid-phone-kit.tar.gz > mosaid-phone-kit.tar.gz.sha256 )
[[ "$(awk '{print $2}' "$OUT_DIR/mosaid-phone-kit.tar.gz.sha256")" == "mosaid-phone-kit.tar.gz" ]] || { echo "sha256 file must use a relative filename" >&2; exit 1; }
echo "$OUT_DIR/mosaid-phone-kit.tar.gz"
cat "$OUT_DIR/mosaid-phone-kit.tar.gz.sha256"
