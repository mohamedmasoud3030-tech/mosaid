# Mosaid

Mosaid is a planned lightweight, extensible general personal AI agent for coding workflows, safe local tools, repository work, scheduled assistance, and future modular integrations.

## Current status

**Phases 1–13 complete (product core, hardening, CI) and Phase 14 complete (Android/Termux product package). Phase 15 (final documentation/handoff) is the remaining planned phase.**

The initial runtime target is an unused Android/ARM64 phone running Termux, controlled from an iPhone through a private Telegram bot. PicoClaw `v0.3.1` was the **Phase 0 conditional qualification candidate only**; the product runtime is the Mosaid codebase itself (`cmd/mosaid`), and Phase 14 packages it for the phone.

Physical-phone testing remains intentionally deferred by the owner.

## What exists today

- Phases 1–13: durable Telegram inbox/outbox, fail-closed policy and approvals, structured exec without shell, draft-only GitHub operations, FTS5 memory, durable scheduler, skills/MCP with strict contracts, bounded research fetch, approval-gated image generation, Instagram prepare-only workflows, and Phase 13 hardening (secrets/redaction, flood and execution budgets, DB integrity/backup/restore, deterministic SBOM, active hardened CI).
- Phase 14: a fail-closed Android/Termux product package — installer, preflight, runit supervision, wake lock, Termux:Boot hook, log redaction, health sampling, diagnostics collector with leak refusal, and uninstaller — built as a deterministic phone-kit tarball by the packaging script (`phase14-android-package/`; a CI package job is preserved locally pending workflow-permission activation).
- A pinned and reproducible Phase 0 Android ARM64 qualification build (historical; PicoClaw v0.3.1).
- Numeric acceptance criteria for 30-minute, 2-hour, 12-hour, and 24-hour tests plus network, reboot, crash, memory, battery, thermal, duplicate, and continuity scenarios (product version in the Phase 14 checklist).

## What does not exist

- A physical-phone qualification report (Phase 0 or product).
- Real integration credentials for any external service (Telegram/model/GitHub/MCP/images/Meta remain test-only pending).
- A published product release.
- Phase 15 final documentation/handoff.

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

The Phase 14 product phone package is under [`phase14-android-package/`](phase14-android-package/README.md).

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
