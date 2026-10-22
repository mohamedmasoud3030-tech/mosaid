# Mosaid — Documentation Index

One canonical set (portfolio rule, 2026-10-22): the files below are the only
project documentation. No archive folders — Git history is the archive.
Session handoffs, preservation checksums, Phase-0 report copies and the
dated runtime evaluation were removed; standing conclusions live in the
canonical files.

- [`../README.md`](../README.md) — product overview and current status.
- [`STATUS.md`](./STATUS.md) — implementation status by phase.
- [`../AGENTS.md`](../AGENTS.md) — (if present) agent working rules.
- [`TECHNICAL_DECISIONS.md`](./TECHNICAL_DECISIONS.md) + [`decisions/`](./decisions) — decisions register (ADR-0001 + technical decisions).
- [`SECURITY.md`](./SECURITY.md) · [`security/`](./security) — threat model, hardening, dependency review, vulnerability status.
- [`architecture/`](./architecture) — proposed architecture.
- [`specs/phone-kit/`](./specs/phone-kit) — Phase 14 phone kit, release assets, workflow activation (incl. `activation/` CI templates).
- Package docs stay with their packages: `phase0-android-runtime/` (pinned qualification harness) and `phase14-android-package/` (phone kit).
- [`THIRD_PARTY_NOTICES.md`](./THIRD_PARTY_NOTICES.md) — licenses/notices.

Runtime truth (not prose): Go module (`cmd/`, `internal/`), `skills/`,
tracked SBOM/license evidence under `security/` at repo root.
