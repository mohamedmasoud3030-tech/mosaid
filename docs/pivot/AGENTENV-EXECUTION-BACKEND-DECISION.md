# ADR: AgentENV as a deferred, optional execution backend

- Status: **Accepted — deferred, not adopted**
- Date: 2026-07-30
- Supersedes: nothing
- Related: [`HERMES-RUNTIME-DECISION.md`](HERMES-RUNTIME-DECISION.md), [`ORACLE-DEPLOYMENT-PLAN.md`](ORACLE-DEPLOYMENT-PLAN.md), [`MIGRATION-MAP.md`](MIGRATION-MAP.md), [`HERMES-UPSTREAM-PIN.md`](HERMES-UPSTREAM-PIN.md)
- Scope: sandboxed code/command execution for Mosaid
- Decision owner: Mosaid project owner

This ADR is the **single source of truth** for the AgentENV question. Other documents link here instead of restating the rationale.

---

## 1. What AgentENV is

AgentENV (AENV) is an open-source platform from `kvcache-ai` for running **agent execution environments at scale**. Observed facts at the time of review:

| Property | Observed value |
|---|---|
| Repository | `https://github.com/kvcache-ai/AgentENV` |
| License | MIT |
| Primary language | Rust |
| First public release | `v0.1.0` (`8f028b1369a4ce9a66d140174d0a646e72398517`) |
| Repository age at review | days (initial open-source release 2026-07-23) |
| Stated purpose | Sandbox fleet powering agentic RL training for Kimi K3 |

Technical model, as described by upstream:

- **Firecracker microVMs** as the isolation unit, not containers.
- **OCI-compatible images** loaded on demand through `overlaybd`, with local disk as a bounded cache.
- **Snapshot/resume in <50 ms, pause in <100 ms**, so idle sandboxes release CPU and RAM.
- **Native snapshot and fork**: a running environment forks into independent parallel sandboxes; snapshots persist to S3-compatible storage or a shared distributed filesystem.
- **ublk-backed I/O** with shared host page cache, plus memory ballooning for overcommit.
- An **E2B-compatible HTTP API**, so E2B SDKs work by repointing `E2B_API_URL`.
- A CLI (`aenv`) for templates and sandbox lifecycle.

Upstream prerequisites:

- Linux kernel **6.8+**; the install script additionally requires **Ubuntu 24.04**.
- **`/dev/kvm`** access for Firecracker microVM execution.

Upstream security warning, quoted in substance: **AgentENV currently does not support authorization.** Its API must not be exposed to a public network; it should run on a trusted network or behind an authorization proxy.

**Category:** AgentENV is *sandbox execution infrastructure*. It is not a cloud provider, not a host, and not an agent.

---

## 2. Why AgentENV does not replace Oracle

Oracle Cloud Compute is the **host**: the billed, provisioned machine with an OS, CPU, RAM, disk, network identity, firewall and lifecycle. AgentENV is software that would have to **run on such a machine**. It provisions nothing, bills nothing, and owns no network identity.

Replacing Oracle with AgentENV is a category error. If Mosaid adopted AgentENV, it would run **on Oracle**, not instead of it:

```text
Oracle Compute instance (host)
        └── AgentENV daemon (sandbox fleet manager)   ← still needs the host
                └── Firecracker microVMs
```

The layer map stays fixed:

| Layer | Role | Current choice |
|---|---|---|
| Hosting / compute | The machine that exists and is billed | **Oracle Cloud Compute** |
| Agent runtime | The agent loop, gateways, memory, skills, tools | **Hermes Agent** (pinned) |
| Product | Identity, Arabic behavior, policy, approvals, Work Packs | **Mosaid** |
| Execution sandbox | Isolation for untrusted code | **none in First Gate**; Docker later; AgentENV only if justified |

---

## 3. Why AgentENV does not replace Hermes

Hermes is the **agent runtime**: it holds the reasoning loop, the Telegram gateway, provider plumbing, memory, session search, the Skills loader, the scheduler and MCP. AgentENV holds **none** of these. It exposes sandbox lifecycle primitives (`pull`, `start`, `exec`, `pause`, `resume`, `fork`, `delete`) behind an E2B-compatible API.

The two are not substitutes and not competitors. At most, AgentENV is a **backend that a runtime calls** when it needs to execute untrusted code:

```text
Hermes (decides what to run)  ──calls──▶  execution backend (runs it in isolation)
```

Adopting AgentENV would therefore **add** a component, never remove Hermes. It also would not reduce the Hermes attack surface unless the `terminal`/`code_execution` toolsets were enabled — and in the First Gate they are globally disabled, so there is nothing for a sandbox to isolate.

The one-runtime rule from [`HERMES-RUNTIME-DECISION.md`](HERMES-RUNTIME-DECISION.md) stands: no second agent loop runs beside Hermes, and AgentENV does not create an exception to it.

---

## 4. Where AgentENV could fit in the future

The only defensible future position for AgentENV in Mosaid is as an **isolated execution backend behind a typed Hermes tool boundary**, reached only after a policy check and an owner approval:

```text
Telegram (owner only)
      │
      ▼
Hermes Agent on Oracle
      │
      ├── Mosaid identity, policy, approvals, Skills, Work Packs
      │
      └── execution tool  ── policy gate ── owner approval
                 │
                 ▼
        execution backend  ──▶  Docker (step 2)  or  AgentENV microVM (step 3)
                                                      on 127.0.0.1 / private network
```

Realistic triggering use cases:

- Heavy execution of **untrusted** third-party or model-generated code.
- **Many parallel sandboxes**, e.g. batch evaluation of Work Packs.
- **Snapshot / resume / fork** semantics for long, resumable jobs.
- Large **programmatic Work Packs** that outgrow a single container.
- **Multi-user** or wide-load operation.
- A requirement for **microVM isolation stronger than containers** (kernel-level boundary rather than namespaces).

None of these describe a single-owner, Telegram-only, read-only First Gate.

---

## 5. Why AgentENV is not added now

1. **No demand.** The First Gate ships with `terminal`, `file`, `code_execution`, `browser` and `computer_use` globally disabled. Mosaid executes no untrusted code, so there is nothing to sandbox. Buying isolation for a capability that does not exist is pure cost.
2. **Very likely blocked by the target host.** AgentENV requires `/dev/kvm`. Oracle Always Free Ampere (`VM.Standard.A1.Flex`) instances are widely reported not to expose a KVM module, and Oracle's own KVM images target x86 (Intel/AMD). *This has not been verified on Mosaid's own instance and must be checked with the gate in §8 before any adoption work begins.* If `/dev/kvm` is absent, AgentENV is simply not installable on the intended host.
3. **Maturity.** The project is days old at review, at `v0.1.0`, with a small commit history. It is designed for RL-training fleets, not for a single personal agent. Mosaid's First Gate is the wrong place to absorb that risk.
4. **No built-in authorization.** Upstream states plainly that AgentENV does not support authorization. Adding an unauthenticated control plane that can start VMs on the same host as the agent is a serious increase in blast radius.
5. **Resource cost.** A microVM fleet manager, overlaybd storage, ublk devices and snapshot storage consume RAM, disk and operational attention that a Free Tier instance can ill afford — resources Hermes itself needs.
6. **Installation path conflicts with policy.** The documented quick start is `curl -fsSL … | sudo bash` and `docker run -d --privileged -v /dev:/dev -p 8000:8000`. Both are prohibited in Mosaid's production path: unpinned remote script execution as root, `--privileged`, host `/dev` mounting, and a world-facing port.
7. **Simplicity rule.** Mosaid's own policy is to choose the simplest maintainable solution and avoid duplicated systems and unrequested complexity.

---

## 6. Accepted execution order

Isolation is adopted in **three ordered steps**. Each step requires the previous one to be in production and to have proven insufficient.

### Step 1 — First Gate: no code execution *(current)*

- `terminal`, `file`, `code_execution`, `browser`, `computer_use`, `delegation`, `cronjob`, `image_gen`, `video_gen`, `x_search`, `homeassistant`, `spotify`, `discord`, `discord_admin` are globally disabled.
- Telegram is limited to `web`, `skills`, `todo`, `memory`, `session_search`, `clarify`.
- No Docker daemon requirement, no sandbox, no `/dev/kvm`, no AgentENV.
- The `terminal:` block in `deploy/hermes/config.yaml.example` is **inert configuration staged for Step 2**; it takes effect only if `terminal` is removed from `agent.disabled_toolsets`, which the CI contract forbids.

### Step 2 — Docker backend, after a successful launch

- Use the Docker execution backend **already supported inside Hermes**. No new project dependency.
- Preconditions: First Gate stable in production; a concrete, owner-approved need for code execution; `docker_network: false`; non-root container user; no host `cwd` mount; CPU/RAM/disk caps; execution behind approval.
- Requires a separate ADR and a separate CI contract change enabling the toolset.

### Step 3 — AgentENV, only on proven need

- Entered only when Docker isolation is demonstrably insufficient for a documented workload (see §4) **and** the §8 conditions all hold.
- Requires a new ADR with a threat model, a pinned AgentENV commit, and a rollback plan.

---

## 7. Security requirements for any future AgentENV use

These are binding preconditions, not recommendations. If AgentENV is ever deployed:

1. **Never expose the API to the public internet.** Bind to `127.0.0.1` or a private network only.
2. **Never open port 8000 to the world.** No `0.0.0.0:8000` publish, no security-list or firewall rule admitting `0.0.0.0/0` to that port.
3. If remote access is genuinely required, place it **behind a hardened authorization proxy** with TLS, an independent service credential, rate limits and request-size limits.
4. **No `--privileged`** without a written, reviewed threat analysis. Prefer explicit, minimal device grants over `-v /dev:/dev`.
5. **No `curl | sudo bash` in the production path.** Any installation must be pinned to a reviewed commit or signed artifact, checksum-verified, and reviewed for supply-chain risk (Rust crate provenance and build reproducibility included).
6. **Pin an exact AgentENV commit**, mirroring the Hermes pin discipline: recorded SHA, license review, dependency inventory, no floating `main`, no unattended self-update.
7. **Network-deny by default inside sandboxes.** Egress is allowlisted per Work Pack, never open by default.
8. **No secrets reachable from a sandbox.** No forwarded environment, no host credential mounts, no Oracle tenancy credentials, no Telegram or model tokens.
9. **Sandbox lifecycle stays owner-approved.** A model cannot start, fork, extend, or widen a sandbox on its own authority.
10. **Snapshot storage is treated as sensitive data**, encrypted at rest and covered by the same retention and secret-scanning rules as the rest of the deployment.
11. **Resource ceilings are mandatory**: bounded CPU, RAM, disk, TTL and concurrent sandbox count, so a runaway workload cannot exhaust the host running Hermes.
12. **Separate the blast radius.** Prefer a dedicated host over co-locating an unauthenticated sandbox control plane with the live Mosaid runtime.

---

## 8. Conditions for future adoption

All of the following must hold, with recorded evidence, before AgentENV work may begin:

**Host capability gate**

- [ ] Ubuntu 24.04 or another upstream-supported environment
- [ ] Linux kernel **6.8+**
- [ ] `/dev/kvm` present and usable by the service user
- [ ] `/dev/ublk-control` present
- [ ] Nested virtualization available on the instance shape
- [ ] `CAP_NET_ADMIN` grantable
- [ ] `CAP_SYS_ADMIN` grantable (with a documented justification, since it is broad)
- [ ] Sufficient spare CPU, RAM and disk **beyond** what Hermes needs, with headroom measured, not assumed
- [ ] Architecture confirmed compatible (x86_64 vs. Ampere ARM verified against upstream support)

**Product and process gate**

- [ ] First Gate is live and stable in production
- [ ] Docker isolation (Step 2) is deployed and **documented as insufficient** for a real workload
- [ ] A concrete triggering use case from §4 is written down with volume figures
- [ ] Upstream maturity is re-assessed: release cadence, issue quality, security policy, breaking-change history
- [ ] **Authorization exists upstream**, or an approved proxy design is implemented and reviewed
- [ ] License re-verified as MIT (or otherwise acceptable) at the pinned commit
- [ ] Supply-chain review of the pinned commit completed
- [ ] A written threat model covering sandbox escape, host compromise and data exfiltration
- [ ] A tested rollback plan that removes AgentENV without breaking Hermes
- [ ] Cost impact confirmed to remain within the zero-spend policy
- [ ] Owner approval recorded in a new ADR

---

## 9. Rejection and rollback conditions

Adoption is **rejected**, or an existing deployment is **rolled back**, if any of the following becomes true:

- `/dev/kvm` or nested virtualization is unavailable on the host and no supported alternative exists.
- Upstream still lacks authorization and no reviewed proxy is in place.
- The API would have to be reachable from the public internet.
- `--privileged` or host-wide `/dev` mounting would be required without an approved threat analysis.
- Installation cannot be pinned, checksum-verified and reviewed.
- The project becomes unmaintained, changes to a non-permissive license, or accumulates unresolved security advisories.
- Resource consumption threatens Hermes availability or the zero-cost policy.
- Docker isolation turns out to be sufficient after all.
- A sandbox escape, secret leak or unauthorized-execution incident is observed.
- Operational complexity exceeds the maintenance capacity of a single-owner project.

Rollback procedure: disable the execution toolset in Hermes config → stop and disable the AgentENV service → remove sandboxes, templates and snapshot storage → revert to the Docker backend or to no execution → confirm Hermes preflight and owner-only Telegram still pass → record evidence.

---

## 10. Architecture diagrams

### Current — First Gate (in effect now)

```text
Telegram / phone  (owner only)
        │
        ▼
Oracle Cloud Compute            ← hosting and compute
        │
        ▼
Hermes Agent (pinned b8ceba97…) ← the one and only agent runtime
        │
        ├── Mosaid identity and Arabic behavior     (SOUL.md)
        ├── Mosaid policy and approvals             (.hermes.md)
        ├── Mosaid Skills and Work Packs            (read-only, /opt/mosaid/product/skills)
        └── verified-free or self-hosted model endpoint

Enabled via Telegram : web · skills · todo · memory · session_search · clarify
Globally disabled    : terminal · file · code_execution · browser · computer_use ·
                       delegation · cronjob · image_gen · video_gen · x_search ·
                       homeassistant · spotify · discord · discord_admin

No sandbox layer. No Docker requirement. No AgentENV. No /dev/kvm.
```

### Possible future — only if a real need is proven

```text
Telegram / phone  (owner only)
        │
        ▼
Oracle Cloud Compute (or a successor host)
        │
        ▼
Hermes Agent (pinned)
        │
        ├── Mosaid identity, policy, approvals, Skills, Work Packs
        │
        └── execution tool (typed boundary)
                 │
                 ├── policy check
                 ├── owner approval  ← a model cannot approve itself
                 │
                 ▼
        ┌────────────────────────────────────────────┐
        │ Step 2: Docker backend (Hermes built-in)   │  ← first choice after launch
        │   network off · caps · no host mounts      │
        └────────────────────────────────────────────┘
                 │  only if proven insufficient
                 ▼
        ┌────────────────────────────────────────────┐
        │ Step 3: AgentENV (Firecracker microVMs)    │  ← deferred, conditional
        │   127.0.0.1 or private network only        │
        │   never public · never port 8000 exposed   │
        │   pinned commit · authz proxy · no secrets │
        └────────────────────────────────────────────┘
```

The layered order never changes: **Oracle hosts, Hermes runs, Mosaid defines, and a sandbox — if it ever exists — only executes.**

---

## 11. Consequences

**Accepted now**

- Mosaid ships the First Gate with no execution sandbox and no new dependency.
- The `terminal:` block in the Hermes config template stays staged and inert for Step 2.
- No AgentENV package, service, port, environment variable or install command enters the production path.
- `scripts/verify-hermes-pivot.py` enforces these invariants in CI so the decision cannot erode silently.

**Deliberately not done**

- AgentENV is not installed, not vendored, not added as a dependency, and not connected to any service.
- No `/dev/kvm` capability check has been run against a real Oracle instance; §8 remains unverified.
- No Docker execution backend is enabled.

**Revisiting**

This ADR is revisited when a Step 2 → Step 3 trigger from §4 occurs, or when the host-capability facts change. Any change requires a **new ADR with written technical evidence**; this file is then marked superseded rather than edited in place.
