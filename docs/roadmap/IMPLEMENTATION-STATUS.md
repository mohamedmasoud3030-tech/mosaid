# Mosaid implementation status

| Phase | Status | Commit | Tests | External gates | Notes |
|---|---|---|---|---|---|
| 0 | ARCHIVED | `phase0-harness-v1` | CI passed | Physical phone pending | Historical harness unchanged |
| 1 | COMPLETE | `832d548` | unit, race, vet, Linux/Android build, secret scan | Android hardware pending | Minimal product runtime |
| 2 | COMPLETE | `e2bea62` | SQLite recovery and dedupe tests, race, vet, Android build | Real Telegram outage pending | Durable inbox/outbox; practical at-least-once + idempotent outbox |
| 3 | COMPLETE | `fe6c885` | binding/replay/expiry/audit-chain tests, race, vet | Telegram inline-button UX pending | Fail-closed policy and short-lived approvals |
| 4 | COMPLETE | `43ed57c` | traversal/symlink/secret/atomic/process/approval tests, race, vet | Termux has no strong OS sandbox | Structured argv only; no shell |
| 5 | COMPLETE | `15b8567` | local repository and mock GitHub contract tests, race, vet | Real GitHub credentials pending | Draft PR only; main/force/destructive operations denied |
| 6 | COMPLETE | `d5d79d0` | FTS5 lifecycle and secret-rejection tests, race, vet | Model summarization quality pending | Explicit/provenance memory without vector DB |
| 7 | COMPLETE | `074e61b` | 18 scheduler tests plus migration, unit, race, vet, Linux/Android builds, secret scan | Physical-phone scheduler validation pending | Durable jobs and recovery; reconstructed after prior local-only loss |
| 8 | COMPLETE | `8ecf5f9` | strict loader, integrity/schema/version/scope/malicious-manifest tests, race, vet, builds, secret scan | Provider-backed examples pending | Declarative Skills and explicit tool routing |
| 9 | COMPLETE | `33fc32b` (`103128a` lifetime fix) | SDK stdio/HTTP mocks, allowlist/schema/timeout/output/env/path/restart/policy/audit tests | Real MCP identity/configuration pending | Official Go SDK; no auto-discovery or inherited environment |
| 10 | COMPLETE | `d3bf4a7` | SSRF/private-IP/redirect/rebinding/type/size/timeout/prompt-injection tests | Search credentials optional | Bounded untrusted web/document ingestion |
| 11 | COMPLETE | `6fc902f` | provider mock, request/reference/cost/MIME/hash/artifact/audit/policy tests | Image provider credentials pending | Approval-gated generation contracts |
| 12 | COMPLETE | `d65c5fd` | official Graph API mock, approval/tamper/replay/idempotency/restart tests | Meta credentials and Professional account pending | No real Instagram publish performed |
| 13 | COMPLETE | `0dfba13` (`9376d77` CI activation) | 140 tests, race, vet, staticcheck, govulncheck, builds, secret scan, SBOM/license/backup gates | Integration credentials only | Product foundation merged in PR #2 |
| Prototype 14–18 | SUPERSEDED | PR #3 head `1b9c119` | Product CI, Phase 16 CI and preservation CI passed | Not integrated into product runtime | Closed without merge; preserved for selective migration |
| 14 | IN PROGRESS | branch `pivot/hermes-oracle-runtime-20260729` | Documentation/secret scan gates pending Draft PR | None | Adopt Hermes as the single general runtime; no duplicate Go agent runtime |
| 15 | PENDING EXTERNAL | — | Oracle deployment acceptance suite | Compute IP, OS, architecture, OCPU, RAM, disk, SSH and billing status | Pin and deploy reviewed Hermes commit on Oracle Cloud |
| 16 | PLANNED | — | Product asset and Skill tests | Live pinned Hermes runtime | Migrate Mosaid identity, safety, Work Packs, portfolio, opportunity and client workflows |
| 17 | PLANNED | — | Owner-only Telegram, restart, approval, rollback and no-secret evidence | Real Telegram/model endpoint | End-to-end qualification on Oracle |
| 18 | PLANNED | — | Release and handoff gates | All previous gates | Production documentation, rollback package and operator handoff |

## Current decision

One runtime will run on Oracle Cloud: `NousResearch/hermes-agent` with Mosaid product assets. The historical Go foundation remains preserved until the Hermes-based deployment passes Phase 17. No destructive migration occurs before that gate.
