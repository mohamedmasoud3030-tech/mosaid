# Product hardening controls

## Startup and secrets

Configuration decoding rejects unknown fields and trailing documents. Every timeout and message/model/tool/token/cost/retry/flood limit is explicit and bounded; missing values fail startup. Secret files must be regular non-symlink files, mode `0600` or stricter, one line, non-empty, and no larger than 64 KiB. Secret values are registered with structured-log redaction and cleared on shutdown on a best-effort basis.

Termux processes under the same app UID can still read each other's files. Put only dedicated, low-quota credentials on the phone and revoke them after testing.

## Runtime limits

- Telegram message byte cap plus per-owner token bucket.
- Per-turn model-step, Tool-call, approximate-token, cost, and retry budgets.
- Provider and Tool timeouts, output caps, and bounded retry/backoff.
- Exact path/network/tool allowlists at each integration boundary.
- Policy and approval checks remain authoritative even when model or external content asks otherwise.

## Database maintenance

Startup runs `PRAGMA integrity_check`. `DB.Backup` checkpoints WAL, creates a SQLite `VACUUM INTO` snapshot, sets mode `0600`, validates all migrations and full integrity, fsyncs, then atomically renames. `RestoreBackup` verifies before and after copying and refuses to overwrite an existing database. Stop Mosaid before restore and retain the original database separately.

Migrations are numbered, transactional, and recorded only after their body succeeds. Tests prove a failed migration leaves neither its table nor migration marker.

## Continuous verification

The Phase 13 local gate ran formatting, `go mod verify`, 140 unit tests, race tests, vet, staticcheck, govulncheck, Linux/Android builds, checksums, secret scan, deterministic CycloneDX SBOM regeneration/verification, and license verification. The pre-existing Product CI subset passed on commit `0dfba13`.

The expanded CI workflow could not be activated with the temporary token because GitHub requires Workflows write permission. Its exact definition is preserved at `docs/handoff/PRODUCT-CI-HARDENING.pending.yml` for a normal later commit with an approved credential.

Tracked evidence:

- [`security/product-sbom.cdx.json`](../../security/product-sbom.cdx.json)
- [`security/product-license-report.tsv`](../../security/product-license-report.tsv)
- [`VULNERABILITY-STATUS.md`](VULNERABILITY-STATUS.md)
- [`THREAT-MODEL.md`](THREAT-MODEL.md)
