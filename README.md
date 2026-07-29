# Mosaid

Mosaid is a planned lightweight, extensible general personal AI agent for coding workflows, safe local tools, repository work, scheduled assistance, and future modular integrations.

## Current status

**Research and Phase 0 qualification only. The product has not been built.**

The initial runtime target is an unused Android/ARM64 phone running Termux, controlled from an iPhone through a private Telegram bot. PicoClaw `v0.3.1` is the current **conditional candidate** for runtime qualification; it is not yet a confirmed permanent dependency or the final architecture.

Phase 1 has not started. Phone testing is intentionally deferred by the owner.

## What exists today

- A deep comparison of open-source personal-agent runtimes and frameworks.
- ADR-0001 recording the conditional PicoClaw decision.
- Proposed security-first Mosaid architecture.
- A pinned and reproducible Phase 0 Android ARM64 qualification build.
- Telegram private-owner controls with `/status` and `/echo` only.
- All dangerous tools disabled: Shell, filesystem tools, MCP, cron commands, web, GitHub, images, Instagram, dynamic Skills, remote execution, and self-update.
- Termux installation, preflight, runit supervision, wake lock, Termux:Boot, health sampling, log rotation, test harness, SBOM, license report, and diagnostics collector.
- Numeric acceptance criteria for 30-minute, 2-hour, 12-hour, and 24-hour tests plus network, reboot, crash, memory, battery, thermal, duplicate, and continuity scenarios.

## What does not exist

- A production-ready personal agent.
- A safe general Shell or Permission Engine.
- Durable exactly-once Telegram processing.
- Git/GitHub runtime integration.
- Skills or MCP runtime enablement.
- Image generation or Instagram publishing.
- A passed real-phone qualification report.

## Start with the evidence

- [Full runtime/framework evaluation](docs/research/2026-07-29-agent-runtime-evaluation.md)
- [ADR-0001: conditional PicoClaw selection](docs/decisions/ADR-0001-picoclaw-conditional-selection.md)
- [Proposed architecture](docs/architecture/proposed-architecture.md)
- [Phase 0 execution report](docs/phase0/EXECUTION-REPORT.md)
- [Pinned source verification](docs/phase0/SOURCE-VERIFICATION.md)
- [Numeric acceptance criteria](docs/phase0/ACCEPTANCE-CRITERIA.md)
- [Threat notes](docs/phase0/THREAT-NOTES.md)
- [Phone guide](docs/phase0/PHONE-GUIDE.md)
- [Next-session handoff](docs/handoff/NEXT-SESSION.md)

The self-contained harness is under [`phase0-android-runtime/`](phase0-android-runtime/README.md).

## Phase 0 pinned identity

```text
Upstream:         sipeed/picoclaw
Tag:              v0.3.1
Tag object:       9fba4cec050cbfe3d73dfcfe015d7960447b9c7f (unsigned)
Commit:           2cf030d2fd3b871d7ec17e3be34c24688aac76da
Tree:             79530d185c4c5eb30719fd45cf323217d2a9f5c5
Qualification Go: 1.25.12
Android target:   arm64-v8a
Binary SHA-256:   b68746ddeeb341c291da5f93f59f857cdd892d8fe76940367604a2ec1c729a4f
Phone-kit SHA-256: 78b9fd3c50b4d0a33e0d20066675491823574b480418fe54b29d662e76595b1e
```

## Warning

Do **not** use the Phase 0 kit with production Telegram bots, production model keys, sensitive repositories, public groups, or publishing accounts. Use dedicated low-quota test credentials and revoke them after testing.

`chmod 600`, allowlists, redaction, and Termux's app directory are defense in depth. They are not a substitute for an OS sandbox, and arbitrary untrusted code must not run under the same Termux UID as secrets.

## Licensing

Original Mosaid material is MIT-licensed. The qualification binary and patches derive from MIT-licensed PicoClaw and retain its attribution. See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and the Phase 0 SBOM/license report.
