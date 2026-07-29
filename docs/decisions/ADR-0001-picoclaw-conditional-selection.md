# ADR-0001: Conditional selection of PicoClaw for runtime qualification

- Status: **Accepted conditionally for Phase 0 only**
- Date: 2026-07-29
- Decision owners: Mosaid project owner
- Scope: runtime qualification and foundation selection

## Context

Mosaid targets a lightweight general personal agent controlled through Telegram and hosted initially on an unused Android phone through Termux. The system must start with coding/repository workflows and later allow optional documents, web research, memory, scheduling, Skills, MCP, image generation, and official Instagram publishing.

The foundation must be small, understandable, maintainable, permissively licensed, multi-provider capable, and realistic on Android ARM64 without Docker.

Candidates evaluated included PicoClaw, Hermes Agent, nanobot, OpenClaw, PydanticAI, LangGraph, Open Interpreter, OpenHands, Agent Zero, AutoGen, CrewAI, and several newer lightweight runtimes.

## Decision

Use **PicoClaw v0.3.1 only as the current candidate for Android Runtime Qualification**.

This is a **conditional GO**, not a final product dependency decision and not authorization to begin Phase 1.

Pinned source:

- Repository: `sipeed/picoclaw`
- Tag: `v0.3.1`
- Tag object: `9fba4cec050cbfe3d73dfcfe015d7960447b9c7f`
- Commit: `2cf030d2fd3b871d7ec17e3be34c24688aac76da`
- Tree: `79530d185c4c5eb30719fd45cf323217d2a9f5c5`

## Why PicoClaw was selected conditionally

- It has an official Android ARM64 build path and release artifact.
- Go allows a single low-overhead runtime binary and avoids Python native-extension and Node runtime friction on Termux.
- Telegram long polling, model providers, sessions, tools, MCP, memory, and scheduling already exist.
- The code is MIT licensed and has unit/integration/security CI.
- The source can theoretically be pruned into a smaller product after platform qualification.

## Security constraints

PicoClaw's current Shell implementation is **not acceptable for the Mosaid product**:

- it accepts shell text;
- it relies substantially on regex deny patterns;
- v0.3.1 defaults enable many tools and remote execution behavior;
- application-level path restrictions are not OS containment;
- Termux does not provide Docker/bubblewrap isolation as a normal supported boundary.

Any product phase must replace free-form Shell with structured argv execution, explicit executable/argument policies, environment filtering, workspace boundaries, approvals, audit, and remote sandboxing for untrusted repositories.

The Phase 0 harness disables every callable tool twice—configuration and turn-profile execution—and permits only Telegram private owner messages, `/status`, `/echo`, and model chat without tools.

## Gate before Phase 1

**Phase 1 is prohibited until the owner completes real-phone qualification and the results satisfy the numeric acceptance criteria.**

Mandatory evidence includes:

- native Android ARM64 execution;
- Telegram long-poll stability;
- one-owner/private-chat authorization;
- network reconnect;
- Termux:Boot recovery after reboot;
- singleton process behavior;
- 24-hour soak with measured restarts;
- RAM/CPU/battery/thermal limits;
- zero secret leakage;
- no unacceptable lost or duplicate Telegram handling.

Successful cross-compilation is not sufficient.

## Largest uncertainty

Android/Termux is the largest unresolved risk:

- OEM background killers may ignore wake locks.
- Android force-stop may block unattended recovery.
- Termux runs all child processes under one app UID.
- thermal and battery behavior is device-specific.
- Telegram delivery across crashes lacks a durable exactly-once inbox in upstream v0.3.1.

## Consequences

### Positive

- Runtime feasibility is tested before committing to a large fork.
- The candidate is pinned and reproducible enough for controlled qualification.
- Research, patches, SBOM, license evidence, and phone harness are archived.

### Negative

- The Phase 0 binary still links many out-of-scope modules.
- A future fork will diverge significantly once security and compile-time pruning begin.
- If Android qualification fails, work on PicoClaw integration must stop and the runtime target must be reconsidered.

## Reversal conditions

Change this ADR to NO-GO if any of these occur:

- native Android execution fails;
- 24-hour stability or reboot recovery fails materially;
- resource/thermal limits are unacceptable;
- secrets cannot be isolated sufficiently for the intended threat model;
- duplicate/lost message behavior cannot be bounded;
- the amount of required pruning/rewrite exceeds the value of the inherited core.

## Related documents

- `docs/research/2026-07-29-agent-runtime-evaluation.md`
- `docs/phase0/EXECUTION-REPORT.md`
- `docs/phase0/ACCEPTANCE-CRITERIA.md`
- `docs/phase0/THREAT-NOTES.md`
