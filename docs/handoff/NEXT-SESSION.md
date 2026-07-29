# Mosaid — next-session handoff

## Objective

Mosaid is intended to become a lightweight, extensible general personal AI agent for Android/Termux with Telegram control, safe coding/repository tools, scheduled assistance, and optional future Skills and integrations.

The repository currently preserves research and Phase 0 qualification work only. It is not a built product.

## Repository and archive

- Repository: https://github.com/mohamedmasoud3030-tech/mosaid
- Visibility at handoff: Private
- Default branch: `main`
- Canonical current main: https://github.com/mohamedmasoud3030-tech/mosaid/commits/main
- Last content/reproducibility commit before this handoff document: `53f2127e4102a46f35fe9b9f09c81c284c6952c4`
- Phase 0 archival tag: `phase0-harness-v1`
- Release: https://github.com/mohamedmasoud3030-tech/mosaid/releases/tag/phase0-harness-v1
- Deferred tracking issue: https://github.com/mohamedmasoud3030-tech/mosaid/issues/1

The exact final main SHA after the handoff commit is recorded in the release body and transfer report; a Git-tracked file cannot contain its own commit SHA without creating another commit.

## Why PicoClaw was selected conditionally

PicoClaw v0.3.1 was the best evidence-based candidate for Android qualification because it has an official Android ARM64 build path, a Go single-binary runtime, Telegram long polling, model-provider support, tests, and an MIT license.

This is not final adoption. The current Shell/default tool model is unacceptable for a product, the compile-time surface remains large, and Termux has no strong same-UID isolation.

Pinned upstream identity:

```text
Tag:        v0.3.1
Tag object: 9fba4cec050cbfe3d73dfcfe015d7960447b9c7f (unsigned)
Commit:     2cf030d2fd3b871d7ec17e3be34c24688aac76da
Tree:       79530d185c4c5eb30719fd45cf323217d2a9f5c5
```

## Phase 0 status

### Completed

- Source/tag/release/workflow verification.
- Official Android and Linux artifact inspection.
- Pinned qualification patches.
- Reproducible Android ARM64 build with Go 1.25.12.
- Owner/private-chat Telegram qualification tests.
- `/status` and bounded `/echo` diagnostics.
- Tools, Skills, MCP, cron, web, filesystem, Shell, remote exec, publishing, and self-update disabled.
- Termux installer, preflight, runit supervisor, wake lock, Boot script, backoff, singleton lock, log rotation, health sampler, result collector, and diagnostics package.
- Twenty phone test scenarios and numeric acceptance criteria.
- CycloneDX SBOM, linked dependency list, license report, build/source manifests, checksums, vulnerability evidence, and secret-scan audit.
- Original local Phase 0 Git refs and complete important working-tree output archived in the prerelease.

### Not completed

- No physical Android phone test.
- No 30-minute, 2-hour, 12-hour, or 24-hour soak.
- No real reboot, network transition, battery, thermal, memory-pressure, crash, duplicate, or continuity evidence.
- No final GO/NO-GO for PicoClaw.
- No Phase 1 code.

## Open risks

1. Android/OEM background killing despite wake lock.
2. Force-stop may block unattended recovery until Termux is opened.
3. No durable Telegram exactly-once inbox in upstream v0.3.1.
4. Termux processes and secrets share one Android app UID.
5. The qualification binary still links 93 modules.
6. Current Shell is denylist/text based and must not be reused as product security.
7. One documented module-level `govulncheck` advisory remains; the affected OpenPGP package is not imported according to `go list -deps`.
8. Upstream tag is unsigned; release Android asset was omitted from the official checksums text file.

## Owner decision

Phone testing is deliberately deferred. Issue #1 is a record, not an instruction to start automatically.

**Phase 1 has not started and is forbidden until the owner explicitly resumes the project and Phase 0 evidence is reviewed.**

## First suggested step when resuming

Do not change architecture or code first. Review the preserved evidence, download the prerelease phone kit, verify its SHA-256, then—only with an explicit owner instruction—perform the first `01-initial-30m` phone scenario and return the redacted diagnostics archive.

## Files to review first

1. `README.md`
2. `docs/research/2026-07-29-agent-runtime-evaluation.md`
3. `docs/decisions/ADR-0001-picoclaw-conditional-selection.md`
4. `docs/phase0/EXECUTION-REPORT.md`
5. `docs/phase0/SOURCE-VERIFICATION.md`
6. `docs/phase0/ACCEPTANCE-CRITERIA.md`
7. `docs/phase0/THREAT-NOTES.md`
8. `docs/phase0/SECRET-SCAN-AUDIT.md`
9. `phase0-android-runtime/manifests/source-manifest.json`
10. `phase0-android-runtime/manifests/build-metadata.json`
11. `phase0-android-runtime/sbom/sbom.cdx.json`
12. `archive/PHASE0-LOCAL-HISTORY-RESTORE.md`

## Explicit scope boundary

Do not add Permission Engine, Shell, Skills, MCP, GitHub runtime, Instagram, image generation, or any product feature until a separate owner instruction begins the appropriate phase.
