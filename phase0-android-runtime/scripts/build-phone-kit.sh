#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BINARY="${1:-$ROOT/evidence/artifacts/picoclaw-phase0-v0.3.1-android-arm64}"
OUT_DIR="${2:-$ROOT/release}"
[[ -f "$BINARY" ]] || { echo "binary not found: $BINARY" >&2; exit 1; }

STAGE="$OUT_DIR/phase0-phone-kit"
rm -rf "$STAGE"
mkdir -p "$STAGE"/{bin,config,phone,licenses,manifests}
install -m 0755 "$BINARY" "$STAGE/bin/picoclaw-phase0"
cp "$ROOT/config/config.phase0.template.json" "$STAGE/config/"
cp "$ROOT/config/test-plan.json" "$STAGE/config/"
cp "$ROOT"/phone/* "$STAGE/phone/"
chmod 0755 "$STAGE"/phone/*.sh "$STAGE"/phone/*.boot
cp "$ROOT/evidence/upstream/LICENSE" "$STAGE/licenses/PicoClaw-LICENSE"
cp "$ROOT/manifests/sbom.cdx.json" "$STAGE/manifests/"
cp "$ROOT/manifests/license-report.tsv" "$STAGE/manifests/"
cp "$ROOT/manifests/linked-modules.tsv" "$STAGE/manifests/"
( cd "$STAGE" && sha256sum bin/picoclaw-phase0 > BINARY.sha256 )
cat > "$STAGE/README-FIRST.txt" <<'EOF'
PHASE 0 QUALIFICATION KIT — NOT A PRODUCT BUILD

Requirements:
- Android arm64-v8a
- Current official Termux from F-Droid or Termux GitHub releases
- Termux:Boot from the same source/signing family, opened once
- A separate test Telegram bot token and a low-privilege test model API key

Run inside Termux after extracting:
  bash phone/install-phone.sh

No shared-storage permission is requested. Do not place real secrets in this kit.
EOF
(
  cd "$STAGE"
  find . -type f -not -name FILES.sha256 -print0 | sort -z | xargs -0 sha256sum > FILES.sha256
)
mkdir -p "$OUT_DIR"
tar -C "$OUT_DIR" --sort=name --mtime='UTC 2026-06-30 09:42:07' --owner=0 --group=0 --numeric-owner -czf "$OUT_DIR/phase0-phone-kit.tar.gz" phase0-phone-kit
sha256sum "$OUT_DIR/phase0-phone-kit.tar.gz" > "$OUT_DIR/phase0-phone-kit.tar.gz.sha256"
echo "$OUT_DIR/phase0-phone-kit.tar.gz"
cat "$OUT_DIR/phase0-phone-kit.tar.gz.sha256"
