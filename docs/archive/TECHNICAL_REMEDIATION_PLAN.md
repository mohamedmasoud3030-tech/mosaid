# Technical Remediation Plan

Milestones ordered by risk class (credentials/access, data loss, money,
outage, correctness, regression protection, operations). Status reflects
actual evidence, not intentions.

## M1 — Phone runtime: DNS + supervisor lifecycle (outage) — DONE in repo
- Problem (reproduced from on-device logs): `lookup api.telegram.org on
  [::1]:53 … connection refused` while curl succeeded; `sv restart`
  orphaned the agent child which held the singleton lock (exit-73 loop).
- Root cause: stock-Go on Android reads `/etc/resolv.conf` and falls back to
  localhost:53; the old supervisor's TERM trap cleaned up but did not exit.
- Fix: proot bind of `$PREFIX/etc/resolv.conf` over `/etc/resolv.conf`
  (D-03), Termux-only DNS guard, supervisor TERM contract (D-04).
- Evidence: `selftest-supervisor.sh` PASS (term-exit=0, child terminated,
  state cleaned); kit selftest PASS; binary unchanged `4f8679ca…`; kit
  re-pinned `eaac4ff0…`.
- Remaining: owner device verification (UNVERIFIED externally) — run the
  updated phone steps and report `/status` result.

## M2 — Cost accounting (money) — DONE in repo
- Problem: `UseCost` was never called; `max_cost_usd` could not stop spend.
- Fix: per-request estimated cost against the cap (D-02), visible
  budget-exceeded reply (D-09).
- Evidence: 5 new regression tests (TokenCostUSD, config price validation,
  agent cost trip, zero-price passthrough, gateway budget reply);
  145/145 tests, race, vet, staticcheck, govulncheck all green locally.
- Remaining: cumulative cross-request spend ledger (M4); CI green pending M3.

## M3 — CI vulnerability gate + phone-kit job (CI health) — BLOCKED (owner)
- Problem: govulncheck gate fails on Go 1.25.12 DB drift (4 stdlib findings
  fixed in 1.25.13); setup-go pins GOTOOLCHAIN=local so the `toolchain`
  directive cannot switch CI; the App credential lacks `workflows`
  permission (GitHub rejected the push, same mechanism as Phase 13).
- Fix: staged in `docs/handoff/activation/` — `product-ci.yml`
  (go-version 1.25.13 + `phone-kit` job), `phase0-ci.yml`
  (go-version 1.25.13 + pinned `expected-govulncheck.txt`), temporary
  `measure-phase0.yml` to re-pin the Phase 0 artifacts on the real proxy,
  and the activation procedure.
- Required from owner: an approved credential with Workflows write via
  Arena's secure channel (one yes/no, already requested).

## M4 — Cumulative cost ledger (money, depth) — PLANNED (next safe milestone)
- Residual risk from M2: the cap is per-request; there is no cross-request
  accumulated spend guard for paid providers.
- Planned: `cost_entries` table + per-day accumulation column, checked
  against a new optional `limits.max_cost_usd_day` (0 = disabled), enforced
  before each model call, surfaced like D-09. Migration v6 with rollback
  test; smallest addition, free-tier unaffected.

## M5 — Dead-letter visibility and metrics (operations) — PLANNED
- Problem: non-budget handler errors retire to `dead` silently; the owner
  only discovers them via logs/diagnostics.
- Planned: health counters for inbox/outbox dead states exposed in `/status`
  (e.g. `dead_inbox=1`), plus a redacted last-error line in the diagnostics
  archive.

## M6 — Real-phone qualification (owner-side) — OWNER ACTION
- Run the updated phone steps (download via the private-repo raw URL,
  copy from Downloads, verify `eaac4ff0…`, copy the new scripts, install
  proot/resolv-conf, `pkill -f mosaid`, `/status`), then scenario
  01-initial-30m and collect diagnostics after each scenario.

## M7 — Off-device backup (data) — OWNER DECISION
- Backup/restore exists in-band; an off-device target (cloud/SFTP) is a
  product/ops choice requiring the owner. No action until the owner asks.

## Out of scope (no evidence of a problem)
- Frontend/PWA work (no web surface), database engine change, framework
  change, architecture rewrite.
