# Mosaid product phone kit (Phase 14)

> **Status: ARCHIVED (2026-08-17).** The product pivoted to a Hermes-on-Oracle-Cloud runtime (see PR #4 and `docs/pivot/`). The Go runtime packaged here is no longer the product runtime and is preserved as a fallback/legacy artifact until the Hermes deployment passes its qualification gates. This directory is kept for reference and is not part of the forward path.

This directory holds the packaging for installing the Mosaid Go product binary on an Android phone running Termux, controlled from a private Telegram bot.

## Status

Packaging complete. **Physical-phone validation is pending** (owner-deferred). Nothing here has been run on real hardware yet; treat the kit as qualified-by-construction until the checklist in `docs/PRODUCT-CHECKLIST.md` is executed on the target phone.

## What is included

| Path | Purpose |
|---|---|
| `config/product.template.json` | Product config template; installer fills in owner ID, model endpoint, and paths |
| `phone/install-product.sh` | Interactive Termux installer (packages, secrets, config, runit service, boot hook) |
| `phone/verify-config.sh` | Mirrors `internal/config.Validate` bounds; fail-closed config/secrets/binary checks |
| `phone/preflight.sh` | Platform baseline plus optional `--network` Telegram/model API checks; JSON report |
| `phone/supervisor.sh` | Singleton lock, wake lock, restart backoff, log redaction, health sampling |
| `phone/redact-stream.sh` | Filters the Telegram token and model key out of all log output |
| `phone/health-sampler.sh` | Per-minute `/proc` and battery samples; scenario CSV support |
| `phone/collect-diagnostics.sh` | Redacted diagnostics tarball; refuses to package if a secret leaks into logs |
| `phone/uninstall-product.sh` | Removes service and boot hook; data kept unless `--purge-data` |
| `phone/10-mosaid.boot` | Termux:Boot hook (wake lock + start services) |
| `scripts/build-phone-kit.sh` | Reproducible kit build: Android ARM64 binary + deterministic tarball + SHA-256 |

## Build

```bash
bash phase14-android-package/scripts/build-phone-kit.sh
# -> build/kit/phase14-phone-kit.tar.gz + .sha256
```

The tarball is deterministic for a given commit: the binary is built with `CGO_ENABLED=0 -trimpath` and fixed `ldflags`, and the archive uses sorted entries, fixed mtime and numeric owner/group. The CI `package` job rebuilds the kit twice and compares bytes.

## Install (on the phone)

```bash
# inside Termux, after extracting the tarball
bash phone/install-product.sh
bash ~/.local/share/mosaid/scripts/preflight.sh --network
```

Then follow `docs/PHONE-GUIDE.md` and `docs/PRODUCT-CHECKLIST.md`.

## Safety notes

- No shared-storage permission is requested at any point.
- Secrets are written only under `$HOME/.config/mosaid` (mode 0600) inside the Termux app directory.
- All log output passes through the redaction filter before it reaches disk.
- Diagnostics refuse to package when a secret is detected outside the protected files.
- Termux provides no strong OS sandbox between processes of the same app UID. The supervisor, redaction and 0600 modes are defense in depth, not isolation. Use test credentials for the first qualification runs and revoke them afterwards.

## Explicitly not included

- No shared-storage access, no Termux:API beyond battery status, no webhook port, no group chats.
- The kit tarball is not committed to Git; CI builds it as an artifact and the official copy belongs in a GitHub release when the owner approves one.
