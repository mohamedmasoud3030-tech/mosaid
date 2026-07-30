# Mosaid

Mosaid is Mohamed's personal digital-work agent: an Arabic-first product that helps a beginner understand, learn, execute, verify and deliver real digital work.

## Current status

The security-hardened Go product foundation through Phase 13 is merged in PR #2 and preserved on `main`.

The project is now pivoting to use [`NousResearch/hermes-agent`](https://github.com/NousResearch/hermes-agent) as the **single general-purpose runtime**, hosted on Oracle Cloud Compute. Mosaid remains the differentiated product layer: identity, Arabic behavior, safety and approvals, Work Packs, portfolio, opportunity and client workflows, and product-specific evaluation.

The earlier Phase 14–18 runtime prototype is preserved in closed, unmerged PR #3. It is not the active runtime path.

The active work is Draft PR #4 on:

```text
pivot/hermes-oracle-runtime-20260729
```

## Target architecture

```text
Telegram / phone
      |
      v
Oracle Cloud Compute            <- hosting and compute
      |
      v
Hermes Agent                    <- the single agent runtime
      |
      +-- Mosaid Arabic identity
      +-- Mosaid safety and approval profile
      +-- Mosaid Skills and Work Packs
      +-- portfolio, opportunity and client workflows
      +-- verified-free or self-hosted model endpoint
```

There will not be two agent runtimes running beside each other.

### Layers

| Layer | Role | Choice |
|---|---|---|
| Hosting and compute | The billed machine that exists | **Oracle Cloud Compute** |
| Agent runtime | Agent loop, gateways, memory, Skills, tools | **Hermes Agent** (pinned) |
| Product | Identity, Arabic behavior, policy, approvals, Work Packs | **Mosaid** |
| Execution sandbox | Isolation for untrusted code | **none in the first release** |

Oracle is the host and is not replaced by any runtime or sandbox technology. **There is no fourth layer active today.** The first release runs with `terminal`, `file` and `code_execution` globally disabled, so no sandbox is needed.

If isolated execution becomes necessary after launch, the first option is the **Docker backend already built into Hermes**. [**AgentENV**](https://github.com/kvcache-ai/AgentENV) is a deferred, optional later stage only — it is not a cloud provider, not a replacement for Oracle or Hermes, and not a dependency of this repository. The reasoning, security requirements and adoption gate are in the [AgentENV execution-backend decision](docs/pivot/AGENTENV-EXECUTION-BACKEND-DECISION.md).

## Completed foundation

The merged Go foundation contains evidence and tested implementations for:

- durable Telegram inbox/outbox;
- policy and short-lived approvals;
- safe tools;
- Git/GitHub contracts;
- memory;
- scheduler;
- Skills;
- MCP;
- web/documents;
- image generation contracts;
- Instagram official API contracts;
- security, backup, SBOM, license and CI hardening.

This foundation remains preserved until the Hermes-based Oracle deployment passes end-to-end acceptance.

## Pivot assets

- [Runtime decision](docs/pivot/HERMES-RUNTIME-DECISION.md)
- [AgentENV execution-backend decision (deferred)](docs/pivot/AGENTENV-EXECUTION-BACKEND-DECISION.md)
- [Hermes upstream candidate pin](docs/pivot/HERMES-UPSTREAM-PIN.md)
- [Migration map](docs/pivot/MIGRATION-MAP.md)
- [Oracle deployment plan](docs/pivot/ORACLE-DEPLOYMENT-PLAN.md)
- [Oracle provisioning runbook](deploy/oracle/PROVISIONING-RUNBOOK.md)
- [Deployment entrypoint](deploy/hermes/README.md)
- [Mosaid identity](product/identity/MOSAID.md)
- [Mosaid safety policy](product/policies/SAFETY.md)
- [First Research Skill](product/skills/research/SKILL.md)
- [Implementation status](docs/roadmap/IMPLEMENTATION-STATUS.md)

## Security and secrets

Never commit or paste:

- Oracle Auth Tokens;
- SSH private keys;
- Telegram bot tokens;
- model/API keys;
- populated server environment files.

External Skills are not auto-enabled, self-improvement creates drafts only, high-risk actions require owner approval, and the default billing policy is free-only with no paid fallback.

The first release also enables **no code execution at all**: `terminal`, `file`, `code_execution`, `browser`, `computer_use`, `delegation`, `cronjob`, `image_gen`, `video_gen`, `x_search` and the chat/home integrations are globally disabled. Telegram is limited to `web`, `skills`, `todo`, `memory`, `session_search` and `clarify`. These invariants are enforced in CI by `scripts/verify-hermes-pivot.py`.

## Next gate

The next technical gate is a pinned Hermes deployment on the Oracle Compute instance, followed by owner-only Telegram, model, approval, restart, rollback and no-secret validation.
