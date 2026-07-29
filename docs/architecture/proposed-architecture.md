# Mosaid proposed architecture

Status: design only. No Phase 1 implementation exists.

## Objectives

- One lightweight always-on process where practical.
- Telegram-first, single-owner, private-chat default.
- General personal-agent core rather than a project-specific agent.
- Tools and Skills added without modifying the agent loop.
- Safe repository work before external publishing capabilities.
- Explicit recovery and audit instead of optimistic autonomous replay.

## Logical components

```text
Telegram / iPhone
        │
        ▼
Telegram Gateway (long polling, owner allowlist)
        │
        ▼
Durable Inbox ──► Session Coordinator
                        │
                        ▼
                   Agent Core
                  /     │      \
          Model Router  │   Context Builder
                        │
                 Policy / Approval Layer
                        │
                  Tool Registry
        ┌───────────────┼────────────────────┐
        ▼               ▼                    ▼
 Workspace Tools   Structured Exec       MCP Client
        │               │                    │
        ▼               ▼                    ▼
 Workspace Manager   Git Adapter       Approved MCP servers

 Memory / Scheduler / Task Ledger / Outbox / Audit Log
                        │
                        ▼
                Process Supervisor
                        │
                        ▼
              Android + Termux Runtime
```

## Telegram Gateway

- Telegram Bot API long polling; no public webhook port initially.
- Exactly one numeric owner ID and private chat only.
- Durable insertion of `update_id` before processing.
- Outbox with idempotency key and delivery status.
- Signed/opaque approval callback IDs bound to owner, session, call hash, and expiration.
- `/stop`, `/status`, `/mode`, `/approve`, `/deny`, `/tasks` only after authorization.
- Groups disabled until a later explicit threat review.

## Agent Core

- Small bounded tool-calling loop.
- Hard limits on iterations, runtime, tokens, and cost.
- No self-evolution or self-modification in the initial product.
- No multi-agent orchestration until a concrete use case proves it necessary.
- Every tool invocation crosses the Policy Layer; Skills cannot call implementation functions directly.

## Model Router

- OpenAI-compatible API first.
- Direct provider adapters only when compatibility differences require them.
- Per-model capability declaration: tools, vision, structured output, context, parallel calls.
- Fallback only before external side effects or with an idempotency-safe task ledger.
- Provider API hosts allowlisted; secrets injected at the final adapter boundary.

## Permission and Approval Layer

### Modes

- `read-only`: default; local reads and bounded external reads.
- `write`: task- and time-scoped workspace mutation.
- `publish`: never persistent; approval for one exact external transaction.

### Risk classes

| Class | Examples | Default |
|---|---|---|
| R0 | read file, git status/log | automatic inside scope |
| R1 | web search/fetch | automatic with SSRF/content limits |
| R2 | patch/write | requires write mode and audit |
| R3 | tests, commit, paid image | policy/approval dependent |
| R4 | push, PR, publish, delete, secrets, unknown MCP | per-call approval always |

## Tool Registry

Each tool declares:

- name/version and JSON input schema;
- risk class and allowed modes;
- path and network scopes;
- timeout/output/cost limits;
- approval rules;
- idempotency/retry semantics;
- secret references, never raw secret values.

Disabled is the default. A Skill can request a tool but cannot enable it beyond the owner's policy.

## Structured Exec

There is no general `sh -c` tool.

```yaml
executable: pytest
argv: ["-q", "tests/unit"]
cwd: "${workspace}"
timeout_seconds: 300
```

Controls:

- resolved executable allowlist;
- anchored argument policy;
- no pipes, redirects, substitutions, eval, or shell startup files;
- sanitized environment and per-task HOME/TMP;
- process-group termination and output cap;
- no package manager/network client through Shell;
- untrusted repository tests run in an external sandbox, not directly under the Termux UID.

## Workspace and Git

- One allowed root per repository.
- One worktree and `agent/<task-id>` branch per task.
- Atomic writes and symlink/TOCTOU defenses.
- no write while checked out on `main`/`master`;
- no force push, remote deletion, `.git/config` or hooks modification;
- push and Draft PR require separate approvals;
- merge-to-main is absent as a tool.

## Skills

A Skill is declarative or out-of-process:

```text
skills/<id>/
  skill.yaml
  SKILL.md
  schemas/
  references/
```

Runtime types:

1. declarative instructions/workflows using registered tools;
2. reviewed built-in adapter;
3. pinned MCP server with an explicit tool allowlist.

No Android dynamic Go plugins, automatic marketplace install, post-install scripts, or silent update. Every external Skill has version, SHA-256, provenance, and license review.

## MCP

- Disabled initially.
- Official Go SDK when introduced.
- stdio and Streamable HTTP only.
- No auto-discovery and no `npx -y`.
- Pinned command/package/hash, sanitized environment, workspace cwd.
- Explicit include list for tools; every MCP tool gets a Mosaid risk class.
- Unknown schema change disables the server pending review.

## Memory

Initial implementation:

- SQLite + FTS5, no vector database.
- near-term session messages;
- session summaries;
- long-term memory items with source/provenance;
- memory candidates requiring confirmation for sensitive/persistent facts.

Web pages, repository instructions, and tool output do not enter long-term memory automatically.

## Scheduler and recovery

- SQLite-backed jobs invoking Skill + structured input, never shell text.
- Explicit missed-run policy: skip or once.
- Scheduled publishing uses a preapproval bound to exact content hash/account/time.
- Task states: queued, running, waiting-approval, interrupted, completed, failed.
- After crash, only read/idempotent tasks auto-retry; write/publish return to review.

## Web and documents

- HTTPS only, DNS resolved and checked against private/loopback/link-local/metadata ranges.
- redirects revalidated; byte/time/content limits.
- fetched text tagged untrusted with URL, timestamp, and hash.
- macros/embedded executable content never run.
- heavy document conversion delegated to a reviewed external service/MCP only after Phase 1.

## Images and Instagram

Future adapters only:

- Image generation via allowlisted HTTPS provider with cost estimate and artifact hash.
- Instagram via Meta Graph API, not browser/password automation.
- Prepare/preview/publish are separate actions.
- Publish approval binds asset hash, caption hash, account, and expiration.
- Media staging uses short-lived object storage URLs and lifecycle deletion.
- Meta token is available only to the publisher adapter, never Shell or model context.

## Audit and secrets

- SQLite audit plus exportable JSONL with sequence/previous-hash tamper evidence.
- Log policy decision, approval identity, redacted parameters, result hash, remote IDs.
- Mosaid state directory 0700 and secret files 0600 on Termux.
- Secrets excluded from workspace, Git, model prompt, child environment, logs, and backups.
- This is defense in depth; same-UID Termux processes are not OS-isolated.

## Android runtime

- Native Android ARM64 binary preferred; Linux/proot is fallback evidence only.
- termux-services/runit for process supervision.
- Termux:Boot and wake lock.
- singleton lock, backoff, rotated logs, local health state.
- concurrency 1 and no local LLM/browser to control heat.
- real-phone Phase 0 metrics decide whether Android remains the target.
