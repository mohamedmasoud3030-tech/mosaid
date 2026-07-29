# Phase 0 — Android Runtime Qualification execution report

Status at 2026-07-29: **all environment-local work completed; physical-phone qualification pending**.

No claim is made that the binary has run on the owner's phone.

## 1. Work completed here

### Upstream/source verification

- Resolved `v0.3.1` annotated tag object to `9fba4cec050cbfe3d73dfcfe015d7960447b9c7f`.
- Peeled and pinned commit `2cf030d2fd3b871d7ec17e3be34c24688aac76da` and tree `79530d185c4c5eb30719fd45cf323217d2a9f5c5`.
- Recorded GitHub API release, tag, commit, workflow, Makefile, `go.mod`, `go.sum`, and license evidence.
- Confirmed the tag object is unsigned and release API reports `immutable=false`.
- Downloaded and verified official Android, Linux ARM64, and checksum assets.
- Inspected archive paths before safe extraction; no path traversal or unexpected scripts were found.
- Confirmed the official Android core embeds the exact pinned commit and Go 1.25.11.
- Confirmed the official Android artifact is not an APK; it is two Android ARM64 executables staged with `.so` names.
- Attempted independent reproduction; hashes did not match, so upstream reproducibility is not claimed.

### Qualification build

A controlled derivative was built only for Phase 0. It does not constitute the future product.

Minimum isolated changes:

1. Exactly one numeric Telegram owner.
2. Private-chat-only ingress.
3. Generic unauthorized/group denial with no paths/config details.
4. Safe `/status` and bounded `/echo` commands.
5. Message-ID, reply-count, and latency instrumentation without message content.
6. Tools and skills remain blocked by the effective config/turn profile.
7. The out-of-scope Kagi client was prevented from linking because its generated upstream repository has no license file.
8. Security-only dependency floors and Go 1.25.12 were used.

Two controlled Android builds produced identical output:

```text
b68746ddeeb341c291da5f93f59f857cdd892d8fe76940367604a2ec1c729a4f
```

Target inspection:

```text
ELF64, AArch64, Android PIE
interpreter /system/bin/linker64
Go 1.25.12
CGO_ENABLED=0
```

### Local tests

Passed:

- Telegram qualification policy tests.
- Unauthorized and group messages do not reach the agent bus.
- Authorized private owner messages reach the agent bus.
- `/echo` is bounded to 512 Unicode code points.
- Existing Telegram and agent scoped tests under the pinned source.
- Build/compile checks after Go security overrides.
- Config fail-closed verifier self-test.
- Exact-value log redaction self-test.
- Repository secret scan.
- Patch clean-apply test against the pinned source archive.
- Phone kit inner and outer checksum verification.

Kagi-specific upstream tests are explicitly skipped because Kagi is disabled and removed from the linked Phase 0 surface. This is recorded rather than presented as an unmodified green upstream test suite.

### SBOM, licenses and vulnerability scan

- CycloneDX 1.5 SBOM generated from modules actually linked into the binary.
- 93 linked modules remain—too many for the future product but acceptable for runtime qualification measurement.
- Best-effort license classification found no missing/unknown linked-module license after Kagi was disabled.
- The report includes MIT, Apache-2.0, BSD, ISC, MPL-2.0, and EPL-2.0/EDL-1.0 components.
- Go module contents passed `go mod verify` before qualification patches.
- Security floors reduced `govulncheck` findings from 19 to one module-level advisory: `GO-2026-5932`.
- `go list -deps` confirms the flagged `golang.org/x/crypto/openpgp` package itself is not imported. This is a documented Phase 0 exception, not a blanket waiver for a product release.

## 2. Phone runtime prepared

The generated phone kit includes:

- Native Android/ARM64 qualification binary.
- SHA-256 files.
- Config generator and fail-closed verifier.
- Secrets file creation with mode 0600.
- Offline and network preflight.
- Foreground runner.
- runit/termux-services supervision.
- Atomic singleton lock and PID files.
- Exponential restart backoff: 2 seconds doubling to 300 seconds.
- `termux-wake-lock` and Termux:Boot startup.
- `svlogd` rotation: ten 1 MB files with timestamps.
- Health sampling: process uptime, restarts, RSS, CPU, file descriptors, disk, battery, and temperature when Termux:API is available.
- Persistent test arming across process/reboot restarts.
- Twenty-scenario test plan.
- Automated result summary and diagnostics packaging.
- Refusal to package diagnostics if an exact active secret is found outside the secret file.

Final phone-kit SHA-256:

```text
72a10827f7adbc9c743e8b9cddac89b5eebadcb349e1dbcf811acfb787bf4a63
```

## 3. What remains impossible here

The following require the owner's physical phone and cannot be inferred from a successful cross-build:

- Native loader/runtime compatibility on that Android version.
- OEM battery and background-process policy.
- Screen-off two-hour behavior.
- 12-hour and 24-hour continuity.
- Real Wi-Fi/mobile/airplane transitions.
- Telegram and model-provider outage recovery.
- Termux:Boot after a real reboot.
- Whether force-stopping Termux prevents unattended restoration.
- Actual RAM, CPU, battery drain, and thermal behavior.
- Message loss/duplication around real process/network interruption.

## 4. Current red flags

1. The upstream tag is unsigned and the release is mutable.
2. The Android bundle is omitted from the official checksums text file.
3. The official Android core embeds `Version=nightly` despite release `v0.3.1`.
4. Upstream `v0.3.1` defaults enable many tools, including remote exec; Phase 0 must never run with defaults.
5. Empty upstream `allow_from` means allow everyone in this version; the verifier requires exactly one numeric ID.
6. The binary still links 93 modules and many out-of-scope integrations.
7. There is no durable Telegram inbox or exactly-once guarantee. Duplicate-update tests are mandatory.
8. Termux has no OS sandbox separating the model runtime, child processes, and secrets under the same app UID.
9. Android force-stop may prevent Termux:Boot/runit recovery until the app is opened again.

## 5. Current decision

### **GO مشروط — إلى الاختبار على الهاتف فقط**

This is **not** yet “GO with PicoClaw” and is not permission to begin Phase 1.

The build/source gate passed sufficiently to justify installing the qualification kit on one test phone. Final classification remains pending:

- Final **GO with PicoClaw** only if every mandatory numeric criterion passes.
- Final **GO conditional** if Android/runtime/resource tests pass but durable dedupe is the only bounded next-phase requirement.
- **NO-GO** if native execution, 24-hour stability, reboot/force-stop recovery, resources, temperature, or secret isolation fail.

## 6. Recommended next phase—do not start

If and only if the phone returns a passing/acceptable Phase 0 report, recommend:

> Phase 1: create the experimental minimal fork, remove every non-Telegram channel and out-of-scope provider/tool at compile time, implement a durable Telegram inbox/outbox and explicit permission layer, then repeat SBOM/vulnerability/license/resource measurements.

No part of Phase 1 has been started.
