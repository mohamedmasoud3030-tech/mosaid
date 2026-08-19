# Phase 14 phone kit — handoff

Date: 2026-08-19
Branch: `arena/01a01769-mosaid` (owner-authorized direct execution; see note below)

## What was built

Phase 14 — the fail-closed Android/Termux package for the Phase 1–13 product
runtime — is complete and checksum-pinned:

- `phase14-android-package/phone/` — installer, verify-config, preflight,
  supervisor (backoff, wake lock, health sampling, singleton), foreground run,
  health sampler, redacted diagnostics collector, Termux:Boot hook.
- `phase14-android-package/config/config.phone.template.json` — fail-closed
  template; Gemini free-tier defaults; `max_cost_usd` pinned to `0.01`.
- `phase14-android-package/scripts/` — reproducible kit builder + self-test.
- `phase14-android-package/docs/` — Arabic owner guide, test plan, free-only
  model policy.
- `phase14-android-package/release/` — pinned kit tarball + checksums.
- CI: `product-ci.yml` gained a `phone-kit` job (build + kit + self-test).

## Pinned identity

```text
Source commit:   b6b0a9b53820842fe9e5b42e3a9c0a9545eeefc3
Version:         v0.14.0
Qualification Go: 1.25.13 (built from official golang/go source)
Android target:  arm64-v8a
Binary SHA-256:  4f8679caa0271051835d4016ab003a4dd24e44e13b1d8169af9fb20e985dba43
Phone-kit SHA-256: 8402f949822fe29cc8eb22989bf1213248573a95228935fadb5fa2e24ba89c21
```

## Evidence produced this session

- Full baseline re-verified locally with the pinned Go toolchain built from
  official source: `go build`, `go vet`, gofmt clean, 140/140 tests,
  `go test -race` clean, Linux AMD64 + Android ARM64 builds,
  `staticcheck@v0.7.0` clean.
- `govulncheck@v1.6.0` parity with a locally reconstructed vulnerability
  database (built from the official `golang/vulndb` repository per the
  published database API): **Go 1.25.12 → exit 3 with 4 stdlib
  vulnerabilities** (fixed in 1.25.13), **Go 1.25.13 → "No vulnerabilities
  found"**. This is why `go.mod` pins `toolchain go1.25.13`: the existing CI
  toolchain (1.25.12) fails the govulncheck gate since the database gained
  the stdlib entries — the same drift tracked in PR #6.
- Kit self-test passed: script syntax, binary + staged-file checksums,
  tripwire values, secret-pattern scan, and fail-closed smoke (binary rejects
  the unpersonalized template).

## Known CI limitations (owner action eventually needed)

- The `phone-kit` CI job is staged in `docs/handoff/phone-kit-ci-job.yml`,
  not activated: the session's GitHub App credential lacks the `workflows`
  permission and GitHub correctly rejected that update (same mechanism as
  Phase 13). Activation needs an owner-approved credential with Workflows
  write.
- The Phase 0 preservation CI pins `GOTOOLCHAIN: local` with Go 1.25.12 and
  hardcodes the expected advisory set, so its govulncheck step fails on the
  same database drift and cannot be fixed without a workflow edit
  (credential-blocked). Pre-existing drift, not caused by this PR; PR #6
  (draft) tracks the Go-version fix.

## Owner's next actions (no technical skill required)

1. Follow `phase14-android-package/docs/PHONE-GUIDE.ar.md` on the phone:
   create the free Gemini key, download the kit (direct repository link is
   in the guide; the GitHub release asset upload was blocked by the sandbox
   network, so the kit is distributed from the committed file), run
   `install-phone.sh`, answer the five prompts (bot token and model key
   are typed on the phone only and never leave it).
2. Send `/status` to the bot and run scenario `01-initial-30m`.
3. After each scenario, run `collect-diagnostics.sh` and keep the redacted
   archive for review.

## External gates

- Real Telegram bot credentials: owner-provided, entered on the phone.
- Free Gemini API key: owner-created in Google AI Studio.
- Physical-phone scenarios: none executed yet.
- Regional model availability: if Gemini is unreachable from the owner's
  network, switch provider per `docs/MODEL-FREE-TIER.md`.

## Arena compliance note

Per the session operating contract, the approved specialist team routing
could not be selected or verified by Arena tooling in this session. The
owner explicitly authorized direct, documented orchestrator execution.
Nothing in this repository claims that any named specialist model performed
this work.
