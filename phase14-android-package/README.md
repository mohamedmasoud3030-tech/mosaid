# Phase 14 — Android/Termux phone package

Self-contained, fail-closed phone kit for the Phase 1–13 Mosaid product
runtime on an unused Android arm64-v8a phone running official Termux.

## Kit layout

```
mosaid-phone-kit/
├── bin/mosaid                         Android ARM64 binary (statically linked)
├── BINARY.sha256                      Binary checksum, verified by the installer
├── config/config.phone.template.json  Fail-closed template (personalized by installer)
├── phone/                             Termux installer, preflight, supervisor, health,
│                                      diagnostics, foreground run, Termux:Boot hook
├── manifests/                         SBOM, license report, notices, LICENSE
├── FILES.sha256                       Per-file integrity manifest
└── README-FIRST.txt                   Requirements and warnings
```

## How it runs on the phone

- `install-phone.sh` personalizes the config, writes secrets as single-line
  0600 files, installs a runit service (`sv`), an svlogd rotation config,
  and a Termux:Boot hook. No shared-storage permission is requested.
- `supervisor.sh` adds singleton locking, wake lock, backoff restarts
  (2 s → 300 s), health sampling, redacted JSON events, and clean shutdown.
- The binary itself holds its own singleton lock, redacts logs internally,
  enforces the owner-only Telegram boundary, and fails closed on config or
  secret errors.
- `collect-diagnostics.sh` produces a redacted archive (never the secrets
  directory; logs scrubbed for token/key formats).

## Free-only model policy

Default model provider is the Google Gemini free tier through the
OpenAI-compatible endpoint. See [docs/MODEL-FREE-TIER.md](docs/MODEL-FREE-TIER.md).
`limits.max_cost_usd` is pinned to `0.01` as a fail-closed tripwire and
`verify-config.sh` refuses any change to it.

## Test plan

Real-phone scenarios and numeric acceptance criteria:
[docs/TEST-PLAN.md](docs/TEST-PLAN.md). Owner guide in Arabic:
[docs/PHONE-GUIDE.ar.md](docs/PHONE-GUIDE.ar.md).

## Building

Requires the repo's pinned Go toolchain (1.25.13) and the built binary:

```sh
# from the repository root, with the product Android build available:
bash phase14-android-package/scripts/build-phone-kit.sh build/mosaid-android-arm64
bash phase14-android-package/scripts/selftest.sh \
  phase14-android-package/release/mosaid-phone-kit \
  build/mosaid-linux-amd64   # optional fail-closed smoke test
```

## Pinned identity

```text
Qualification Go: 1.25.13 (built from official golang/go source)
Source commit:    b6b0a9b53820842fe9e5b42e3a9c0a9545eeefc3
Version:          v0.14.0
Android target:   arm64-v8a (GOOS=android GOARCH=arm64 CGO_ENABLED=0, -trimpath)
Binary SHA-256:   4f8679caa0271051835d4016ab003a4dd24e44e13b1d8169af9fb20e985dba43
Kit SHA-256:      f1b67b5b33ea92abd866d756913d77415a08e77950a80195b6ff9ce6629b42c6
```

The binary records its own identity via `mosaid --version`
(`version`, `commit`, `buildTime` linker flags).

## Execution note (Arena session compliance)

This package was implemented and verified directly by the Arena orchestrator
agent in session `arena/01a01769-mosaid` on 2026-08-19. The owner authorized
a documented deviation from the approved-team routing policy because model
routing could not be selected or verified by Arena tooling in this session.
No claims are made that any named specialist model performed this work; all
statements here are backed by local build, test, and checksum evidence
recorded in this repository.
