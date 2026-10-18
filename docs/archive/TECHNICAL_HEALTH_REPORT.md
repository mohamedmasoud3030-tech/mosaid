# Technical Health Report

Date: 2026-08-19 · Branch: `arena/01a01769-mosaid` (commit `a03fc21` base, updated this session)
Prepared by: Arena orchestrator (owner-authorized direct execution; no specialist-model claims — see session compliance note).

## Verification environment

The sandbox blocks all non-GitHub egress. Verification was therefore performed with:

- Go 1.25.13 toolchain built from official `golang/go` source (bootstrap chain 1.4.3 → 1.17.13 → 1.20.14 → 1.22.12 → 1.25.13).
- Module graph staged locally from GitHub mirrors whose zip content hashes match the pinned `go.sum` entries exactly (`modernc.org/sqlite` v1.53.0 tree `6b32d1e`, `modernc.org/libc` v1.73.4 tree `70624da7`, mathutil/memory from published proxy zips, compile-time-only stand-ins for tool modules; the repo's own `go.mod`/`go.sum` are untouched).
- govulncheck@v1.6.0 against a vulnerability database reconstructed from the official `golang/vulndb` repository per the published database API.
- GitHub Actions as independent evidence (same steps, real toolchain/proxy).

## Baseline results (verified this session)

| Check | Result |
|---|---|
| gofmt (cmd, internal) | clean |
| go vet | clean |
| Unit tests | 145/145 pass (was 140; +5 this session) |
| Race tests | clean |
| Linux AMD64 build | OK |
| Android ARM64 build | OK |
| staticcheck@v0.7.0 | clean |
| govulncheck@v1.6.0 (Go 1.25.13, official DB data) | No vulnerabilities found |
| govulncheck control (Go 1.25.12) | exit 3, 4 stdlib findings (net/http etc., fixed in 1.25.13) — confirms the scan is effective and explains the CI gate |
| Phone kit self-test | PASS (syntax, checksums, tripwires, secret scan, fail-closed smoke, supervisor lifecycle regression) |
| Kit reproducibility | deterministic SHA-256; binary pinned `4f8679ca…`, kit `eaac4ff0…` |

## Assessment by area

### 1. Build, structure, dependencies, configuration
Sound. Single Go module, pinned Go 1.25.12 + `toolchain go1.25.13`, pinned
actions in CI, reproducible CycloneDX SBOM (36 modules) and license report
enforced by `cmp` in CI, secret scan on every push, clean-clone verification.
Config loading is strict (unknown fields rejected, single document, bounds on
every limit). Secrets are single-line 0600 files rejected otherwise.

### 2. Frontend / PWA
Not applicable. Mosaid has no web frontend; the interface is a private
Telegram bot. No browser storage, cookies, CORS, or service-worker surfaces.

### 3. Backend rules, jobs, failure handling
Phases 1–13 implemented: durable Telegram inbox/outbox (at-least-once +
idempotent outbox keyed by update ID), fail-closed policy engine,
short-lived approvals, structured-argv tools (no shell), draft-only GitHub
integration, FTS5 memory, durable scheduler with missed-run policy and
locks, declarative Skills, official MCP SDK client (allowlisted config),
DNS-pinned fetch, approval-gated image generation, staged Instagram
container workflow. Failure paths are explicit (retries, backoff, dead
states) and covered by tests. Gateway rejects non-private chats and
non-owner users before any handler runs; flood guard runs before the
handler (test-verified).

### 4. Authentication / authorization
Single numeric owner; ownership enforced per update; approval tokens are
hashed, short-lived, single-use, bound to tool+args hash+resource; audit
entries are hash-chained. No roles beyond owner (product is single-owner by
design). Minor: unauthorized users receive "Access denied." (fine for a
private bot).

### 5. Database
SQLite with WAL, synchronous=FULL, foreign_keys=ON, busy_timeout, single
connection, versioned migrations, startup integrity check, in-band
backup/restore and maintenance tooling with migration rollback tests. No
external backup automation (see risks).

### 6. Integrations
MCP: official Go SDK v1.7.0, allowlisted server config, timeouts, output
caps, no discovery/download/run, no shell launcher. Research fetch:
private-IP/IPv6/metadata/redirect/DNS-rebinding protections, size/type
caps, content tagged `UNTRUSTED_EXTERNAL_CONTENT`. Images: approval-gated,
atomic artifact store, no publishing. Instagram: official Graph API
contract tests, immutable bindings, recoverable container workflow — no
real credentials used. GitHub: mock-contract tests, main/force/destructive
denied. All external gates remain `PENDING_EXTERNAL_VALIDATION`.

### 7. Security, privacy, secrets
Verified on the real device this session: the bot token is redacted in
runtime logs. No shared-storage permission is requested on the phone.
Free-tier model privacy is documented (Google may train on free-tier
conversations). Known, documented limitation: no OS sandbox — everything
runs under one Termux UID; untrusted code must not run there.

### 8. Performance / resource limits
Per-request budgets (model steps, tool calls, tokens, cost, retries),
Telegram flood control matched to the free tier (10 msg/min), response
size caps, bounded timeouts, 60-second health sampling, log rotation.
Android soak criteria (RSS/CPU/battery) are defined but pending the real
phone run.

### 9. Tests, CI, operations
145 unit tests + race + vet + staticcheck + govulncheck + secret scan +
SBOM/license comparison + clean-clone check. CI currently shows one red
step (`Vulnerability scan`) on Go 1.25.12 database drift; the Phase 0
preservation CI shows the same drift plus its hardcoded advisory set.
Both fixes are staged and blocked only on a Workflows-write credential
(owner gate). Diagnostics collector produces redacted archives.

## Risk register (prioritized)

| ID | Class | Finding | Status |
|---|---|---|---|
| R1 | Outage | Phone bot failed DNS (`[::1]:53`) and `sv restart` orphaned the agent (lock held, exit-73 loop) | FIXED in repo (proot DNS fix + supervisor TERM contract + regression test). Awaiting owner device verification. |
| R2 | CI health | govulncheck gate fails on Go 1.25.12 DB drift; fix is one CI line | BLOCKED: workflow credential (owner-approved, not yet delivered). Activation package ready. |
| R3 | Money | Cost limit was advisory only (`UseCost` never called) | FIXED: per-request cost accounting enforced (zero-price default = free tier; nonzero price trips the $0.01 cap fail-closed). Residual: no cumulative cross-request spend ledger (M4). |
| R4 | Ops | Budget/other handler failures were silent to the owner | PARTIAL: budget-exceeded now replies visibly; other dead-lettered inbox items still only logged (M5). |
| R5 | Data | No automated off-device backup | OPEN: documented manual backup/restore; off-device target is an owner decision. |
| R6 | Info | Telegram token regex pins a legacy format | LOW: validation only; actual token is what the API accepts. |

## Verified vs unverified

- VERIFIED (this session, local): everything in the baseline table plus the
  two remediation milestones' regression tests.
- UNVERIFIED (external): real-phone behavior after the DNS/proot fix; real
  Telegram/model/GitHub/MCP/image/Instagram credentials; 30-minute → 24-hour
  phone scenarios; CI green state (blocked by R2 credential).
