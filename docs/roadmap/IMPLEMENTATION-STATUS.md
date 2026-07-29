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
| 7 | COMPLETE | `074e61b` | 18 scheduler tests plus migration, unit, race, vet, Linux/Android builds, secret scan | Physical-phone scheduler validation pending | Durable one-time/recurring jobs, missed-run policy, retries, locks, recovery, cancellation, policy-bound execution; reconstructed from remote after the prior local-only loss |
| 8 | COMPLETE | `8ecf5f9` | strict loader, integrity/schema/version/scope/malicious-manifest tests, race, vet, Linux/Android builds, secret scan | Provider-backed example Skills await their later integration phases | Declarative, builtin Go, and explicit MCP-backed contracts; all tool calls route through the core registry |
| 9 | COMPLETE | `33fc32b` (`103128a` lifetime fix) | official SDK stdio/Streamable HTTP mocks, allowlist/schema/timeout/output/env/path/restart/policy/audit tests, race, vet, Linux/Android builds, secret scan; CI passed after lifetime fix | Real MCP server identity/configuration pending | Official Go SDK v1.7.0; no discovery/download/run, shell launcher, inherited environment, or unpinned identity |
| 10 | COMPLETE | `d3bf4a7` | SSRF/private-IP/IPv6/metadata/redirect/rebinding/type/size/timeout/prompt-injection tests, race, vet, Linux/Android builds, secret scan | Search provider credentials optional | DNS-pinned public fetches and bounded UTF-8 text documents tagged `UNTRUSTED_EXTERNAL_CONTENT`; no automatic memory/tool authority |
| 11 | COMPLETE | `6fc902f` | provider mock/OpenAI contract, request/reference/cost/MIME/dimension/hash/artifact/symlink/audit/policy tests, race, vet, Linux/Android builds, secret scan | Image provider credentials pending | Approval-gated external generation with atomic artifact store and no publishing capability |
| 12 | COMPLETE | `d65c5fd` | official Graph API contract mock, prepare/preview/bound approval/tamper/replay/idempotency/retry/restart/poll/staging/cleanup/audit/migration tests, race, vet, Linux/Android builds, secret scan | Meta credentials and Instagram Professional account pending; no real publish performed | Official API only; immutable account/asset/caption/time binding and recoverable container workflow |
| 13 | IN PROGRESS | — | — | — | Hardening |
| 14 | PLANNED | — | — | Physical phone pending | Android package |
| 15 | PLANNED | — | — | — | Final docs/handoff |
