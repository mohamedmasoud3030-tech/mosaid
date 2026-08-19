# Mosaid

Mosaid is a lightweight, extensible general personal AI agent for coding workflows, safe local tools, repository work, scheduled assistance, and future modular integrations. The runtime is implemented and hardened (Phases 1–13); a fail-closed Android/Termux phone package (Phase 14) is ready, and real-phone qualification is pending the owner's test run.

## Current status

**Phase 14 complete: the Android/Termux phone kit is built, checksum-pinned, and self-tested. The first physical-phone run has not happened yet.**

The intended first device is an unused Android/ARM64 phone running Termux, controlled from an iPhone through a private Telegram bot. The default model provider is the Google Gemini free tier via its officially documented OpenAI-compatible endpoint; the owner chose a free-only model policy.

## What exists today

- The implemented, hardened Go runtime (Phases 1–13): durable Telegram inbox/outbox, fail-closed policy and approvals, structured-argv tools (no shell), draft-only GitHub integration, FTS5 memory, scheduler, Skills, official MCP SDK, DNS-pinned fetch, approval-gated image generation, and a recoverable Instagram container workflow.
- Phase 14 phone kit: Termux installer, preflight, runit supervision, wake lock, Termux:Boot, health sampling, log rotation, redacted diagnostics collector, self-test, SBOM/license manifests, numeric acceptance criteria, an Arabic owner guide, and a free-only model policy with a pinned cost tripwire.
- The pinned Phase 0 qualification harness (archived, historical).

## What does not exist

- A passed real-phone qualification report (30-minute, 2-hour, 12-hour, 24-hour, network, reboot, battery, thermal, duplicate, and continuity scenarios).
- Real Telegram/model runtime credentials in this repository (the owner enters them on the phone; the installer writes them as 0600 single-line files).

## Start with the evidence

- [Full runtime/framework evaluation](docs/research/2026-07-29-agent-runtime-evaluation.md)
- [ADR-0001: conditional PicoClaw selection](docs/decisions/ADR-0001-picoclaw-conditional-selection.md)
- [Proposed architecture](docs/architecture/proposed-architecture.md)
- [Implementation status by phase](docs/roadmap/IMPLEMENTATION-STATUS.md)
- [Phase 14 phone package](phase14-android-package/README.md)
- [Phase 14 test plan](phase14-android-package/docs/TEST-PLAN.md)
- [Phase 14 free-only model policy](phase14-android-package/docs/MODEL-FREE-TIER.md)
- [دليل التشغيل بالعربية](phase14-android-package/docs/PHONE-GUIDE.ar.md)

The archived Phase 0 harness is under [`phase0-android-runtime/`](phase0-android-runtime/README.md).

## Phase 14 pinned identity

```text
Source commit:   b6b0a9b53820842fe9e5b42e3a9c0a9545eeefc3
Version:         v0.14.0
Qualification Go: 1.25.13 (built from official golang/go source)
Android target:  arm64-v8a
Binary SHA-256:  4f8679caa0271051835d4016ab003a4dd24e44e13b1d8169af9fb20e985dba43
Phone-kit SHA-256: 8402f949822fe29cc8eb22989bf1213248573a95228935fadb5fa2e24ba89c21
```

## Warning

Do **not** use the phone kit with production Telegram bots, production model keys, sensitive repositories, public groups, or publishing accounts. Use a dedicated owner-only bot and a free-tier key you can revoke. The Gemini free tier may use conversations to improve its models — do not share secrets in chats.

`chmod 600`, owner allowlisting, redaction, and Termux's app directory are defense in depth. They are not a substitute for an OS sandbox, and arbitrary untrusted code must not run under the same Termux UID as secrets.

## Licensing

Original Mosaid material is MIT-licensed. The Phase 0 qualification binary and patches derive from MIT-licensed PicoClaw and retain its attribution. See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and the SBOM/license reports.
