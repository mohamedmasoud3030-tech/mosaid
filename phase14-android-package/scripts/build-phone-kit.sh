#!/usr/bin/env bash
# Build the Mosaid product phone kit: Android ARM64 binary, installer
# scripts, config template, licenses and manifests, packed into a
# deterministic tarball with checksums.
#
# Usage: build-phone-kit.sh [OUT_DIR]
#   OUT_DIR defaults to ./build/kit
#
# Determinism: the Go build is CGO_ENABLED=0 with -trimpath and fixed
# ldflags; the tarball uses sorted entries, fixed mtime and numeric
# owner/group. Rebuilding from the same commit produces identical bytes.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PACKAGE="$ROOT/phase14-android-package"
OUT_DIR="${1:-$ROOT/build/kit}"

VERSION="phase14"
COMMIT="$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(git -C "$ROOT" show -s --format=%cI HEAD 2>/dev/null || echo unknown)"

mkdir -p "$OUT_DIR"
BINARY="$OUT_DIR/mosaid-android-arm64"
(
  cd "$ROOT"
  CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -trimpath \
    -ldflags "-X main.version=$VERSION -X main.commit=$COMMIT -X main.buildTime=$BUILD_TIME" \
    -o "$BINARY" ./cmd/mosaid
)

STAGE="$OUT_DIR/phase14-phone-kit"
rm -rf "$STAGE"
mkdir -p "$STAGE"/{bin,config,phone,licenses,manifests,docs}
install -m 0755 "$BINARY" "$STAGE/bin/mosaid"
cp "$PACKAGE/config/product.template.json" "$STAGE/config/"
cp "$PACKAGE"/phone/*.sh "$STAGE/phone/"
cp "$PACKAGE"/phone/*.boot "$STAGE/phone/"
chmod 0755 "$STAGE"/phone/*.sh "$STAGE"/phone/*.boot
cp "$ROOT/LICENSE" "$STAGE/licenses/MIT-LICENSE"
cp "$ROOT/THIRD_PARTY_NOTICES.md" "$STAGE/licenses/"
cp "$ROOT/security/product-sbom.cdx.json" "$STAGE/manifests/"
cp "$ROOT/security/product-license-report.tsv" "$STAGE/manifests/"
cp "$PACKAGE/docs/PHONE-GUIDE.md" "$STAGE/docs/"
cp "$PACKAGE/docs/PRODUCT-CHECKLIST.md" "$STAGE/docs/"
( cd "$STAGE" && sha256sum bin/mosaid > BINARY.sha256 )
cat > "$STAGE/README-FIRST.txt" <<EOF
MOSAID PRODUCT PHONE KIT ($VERSION)
commit: $COMMIT

Requirements:
- Android arm64-v8a
- Current official Termux from F-Droid or Termux GitHub releases
- Termux:Boot from the same source/signing family, opened once
- A Telegram bot token and an OpenAI-compatible model API key

Run inside Termux after extracting:
  bash phone/install-product.sh

No shared-storage permission is requested. Start with low-quota test
credentials and revoke them after the first qualification runs.
EOF
(
  cd "$STAGE"
  find . -type f -not -name FILES.sha256 -print0 | sort -z | xargs -0 sha256sum > FILES.sha256
)

mkdir -p "$OUT_DIR"
TARBALL="$OUT_DIR/phase14-phone-kit.tar.gz"
tar -C "$OUT_DIR" --sort=name --mtime='UTC 2026-08-17 00:00:00' --owner=0 --group=0 --numeric-owner -czf "$TARBALL" phase14-phone-kit
sha256sum "$TARBALL" > "$TARBALL.sha256"

echo "phone kit: $TARBALL"
cat "$TARBALL.sha256"
