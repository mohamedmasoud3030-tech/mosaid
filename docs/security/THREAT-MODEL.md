# Mosaid product threat model

Status: product-foundation review, 2026-07-29. This document does not supersede the archived Phase 0 threat notes.

## Assets

- Telegram bot token, model/provider tokens, future Meta token, and MCP credentials.
- Owner messages, session history, long-term memory, SQLite state, generated artifacts, repositories, and publishing drafts.
- Approval tokens and the integrity of the policy/audit chain.
- Availability and cost budgets for model, search, image, and social APIs.

## Trust boundaries

1. The single numeric owner in a private Telegram chat is trusted. Telegram delivery and all network paths are external.
2. Model output, web/search results, documents, repository content, generated patches, image-provider output, MCP schemas/output, and social-provider responses are untrusted.
3. Builtin Go code and manually reviewed configuration are trusted. Declarative Skills are integrity checked but remain constrained declarations, not new authority.
4. Android/Termux is an application boundary from ordinary unrelated apps, **not** a strong boundary between processes under the same Termux UID.
5. GitHub Actions and pinned third-party modules/actions are supply-chain dependencies.

## Threats and controls

| Threat | Controls | Residual risk |
|---|---|---|
| Telegram impersonation/group use | Numeric owner and private-chat checks before durable ingestion | Compromise of the owner's Telegram account or bot token |
| Flood/cost exhaustion | Message token bucket, byte limits, retry budget, model-step/tool/token/cost budgets, provider timeouts and output caps | Distributed upstream outages and billing semantics vary by provider |
| Prompt injection | Web/search content is tagged `UNTRUSTED_EXTERNAL_CONTENT` with no policy, secret, approval, tool, or automatic-memory authority | A model can still produce poor advice; trusted code must keep enforcing boundaries |
| Malicious repository | Canonical workspace boundary, symlink/secret-path denial, structured executable profiles, no shell text, no destructive Git operations | Repository tests execute under the Termux UID when explicitly approved; no strong OS sandbox |
| Approval bypass/replay | Random short-lived token, stored hash only, user/tool/args/resource binding, expiry at resolution and use, single-use receipt, hash-chain audit | A compromised owner session can approve harmful work |
| Scheduler duplication/overlap | SQLite idempotency, transactional claim, expiring locks, stale recovery, bounded retries | External APIs may provide weaker idempotency guarantees |
| Malicious Skill | Strict JSON, duplicate/unknown-field rejection, schema/version/integrity checks, fixed capability scopes, Tool Registry routing | Builtin Go Skills are trusted core code and require code review |
| Malicious MCP server | Manual registration only, pinned executable checksum or TLS identity, exact tool allowlist, filtered environment, bounded cwd/time/output, schema checks, policy and audit | A stdio process under the same UID cannot be strongly network- or filesystem-sandboxed on normal Termux |
| SSRF/DNS rebinding | HTTP(S) only, hostname and resolved-IP checks, all-address fail closed, DNS-pinned dialing, redirect revalidation, metadata/private/link-local denial | Public services can proxy private content; content remains untrusted |
| Image payload abuse | Base64/response caps, MIME magic, decoder/dimension/hash validation, atomic non-executable artifact store | Image decoders remain a parsing surface |
| Unauthorized social publish | Official Meta Graph API only; immutable account/asset/caption/time approval binding; expiry, single use, persisted authorization for bounded idempotent retries | Real-account behavior requires external validation; staging URLs are sensitive while live |
| Secret/log leakage | Non-symlink mode-0600 file source, single-line/size checks, exact and pattern/nested log redaction, secret scan, staging URLs omitted from JSON | Same-UID processes can read secret files; scanners are not proof of absence |
| Database corruption | WAL/FULL sync, transactional migrations, startup integrity check, verified SQLite backup/restore, atomic file writes | Device/storage failure can still lose the latest writes |
| Supply-chain compromise | Exact Go versions, module sums and `go mod verify`, pinned MCP SDK/action SHAs, staticcheck, govulncheck, CycloneDX SBOM, license report | New advisories and compromised upstream accounts remain possible |

## Explicitly prohibited

No free-form shell, `sh -c`, `bash -c`, `eval`, dynamic Go plugins, MCP auto-discovery, `npx -y`, download-and-run, browser automation, cookies/password Instagram login, self-modification, automatic self-update, force-push, direct merge, or unapproved publishing.

## Security claims not made

Mosaid does not claim strong process isolation inside Termux, safe arbitrary untrusted code execution, exactly-once delivery across every external system, physical-phone reliability, or production readiness for external credentials. Those gates remain `PENDING_EXTERNAL_VALIDATION`.
