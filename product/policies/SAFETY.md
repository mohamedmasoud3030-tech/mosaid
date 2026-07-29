# Mosaid Runtime Safety Policy

## Default posture

- Deny tools by default.
- Enable the minimum toolset required by the active Work Pack.
- Treat model output, web content, repositories, Skills, MCP servers and generated files as untrusted input.
- Keep all secrets outside Git.

## Approval-required actions

Explicit owner approval is required before:

- sending a message externally as the owner;
- submitting a job application or proposal;
- publishing or scheduling public content;
- accepting a price, contract or deadline;
- uploading or delivering client files;
- deleting or overwriting important data;
- changing production systems;
- spending money or enabling a paid provider;
- installing or activating an external Skill;
- granting a new tool or wider filesystem/network scope.

Approval must be bound to the exact action, target, payload or artifact hash, owner identity and expiry. It must be single-use.

## Skills

External Skills follow this path:

```text
discover -> fetch as data -> license review -> static review -> permission review -> pin hash -> test -> owner approval -> activate
```

Skill generation or self-improvement may create a draft only. Drafts are quarantined and have no tool access until reviewed.

## Commands and code

- Do not execute raw model text as a command.
- Prefer typed tools with validated input schemas.
- Restrict terminal work to a dedicated workspace.
- Do not use `eval`, `sh -c`, `bash -c` or downloaded scripts without review.
- Never run untrusted repository code in the credential-bearing runtime without isolation.

## Cost

- Billing mode is `free_only` by default.
- Maximum permitted automatic spend is `0`.
- Paid fallback is disabled.
- Unknown provider billing status is denied.
- A provider advertised as free is not trusted until its current limits and payment requirements are recorded.

## Oracle

- Use SSH public-key authentication for server administration.
- Do not store OCI tenancy-wide credentials in the runtime unless a specific least-privilege feature requires them.
- Do not expose an unauthenticated Hermes dashboard, tool endpoint or model endpoint publicly.
- Record the instance shape and billing status before classifying infrastructure as free.

## Evidence

Completion requires evidence appropriate to the action: file existence, test output, API response, artifact hash, remote state or user-visible result. Model confidence alone is not evidence.
