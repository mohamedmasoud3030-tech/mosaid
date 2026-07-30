# Oracle Cloud Deployment Plan for Mosaid

Date: 2026-07-29
Region observed from owner link: `ap-singapore-1`
Status: Deployment preparation; instance details and credentials remain external.

## Important distinction

The Oracle Cloud Auth Tokens page is not the server endpoint. An Auth Token is not required for normal SSH access to a Compute instance and must not be pasted into chat or committed.

The actual deployment requires a running Compute instance with:

- instance public or private IP;
- operating system and architecture;
- OCPU and RAM;
- SSH public-key access;
- confirmed billing/Free Tier status;
- firewall and network rules.

## Runtime topology

Only one product runtime runs on Oracle:

```text
Telegram -> Hermes Agent with Mosaid extensions -> model/tools
```

The phone is the user interface. It does not run a second Mosaid agent runtime.

Oracle is the **host** and is not replaced by any runtime, container platform or sandbox technology. The first gate installs exactly one service on the instance: the pinned Hermes gateway under `mosaid-hermes.service`.

No execution sandbox is installed in the first gate:

- no Docker daemon requirement;
- no `/dev/kvm` requirement;
- no AgentENV service, port, image or environment variable;
- no `--privileged` container;
- no publicly reachable execution API.

Docker — already supported inside Hermes — is the first isolation option to evaluate after a successful launch. AgentENV remains deferred and conditional. See [`AGENTENV-EXECUTION-BACKEND-DECISION.md`](AGENTENV-EXECUTION-BACKEND-DECISION.md).

## Access model

### Server administration

- SSH public-key authentication only.
- Disable password SSH login where the image supports it.
- Do not store the SSH private key in the repository.
- Use a non-root deployment user.
- Restrict inbound SSH by source IP or private VPN when practical.

### Telegram

- Dedicated bot token stored in a `0600` secret file or system service secret.
- Owner numeric Telegram ID allowlisted.
- Private chat only for the first release gate.

### Model providers

- Prefer a self-hosted or verified-free endpoint.
- Store provider keys outside Git.
- No paid fallback.
- Unknown billing status is denied by default.

### Oracle APIs

Hermes does not need tenancy-wide Oracle API credentials to run on a Compute instance. Add OCI API access only when a concrete feature requires it, using a dedicated least-privilege identity. Do not place tenancy admin credentials in the runtime.

## Executable runbook

The steps below are implemented as an executable runbook and two scripts:

- [`deploy/oracle/PROVISIONING-RUNBOOK.md`](../../deploy/oracle/PROVISIONING-RUNBOOK.md) — shape and billing check, SSH key, instance creation, SSH-only ingress, hardening.
- `deploy/oracle/collect-instance-facts.sh` — read-only; produces the non-secret fact report required below.
- `deploy/oracle/bootstrap-host.sh` — idempotent host preparation; installs a pinned, checksum-verified `uv`, creates the `mosaid` user and directory layout, and refuses to run if Docker, AgentENV or a listening port 8000 is present.

Neither script clones Hermes, creates a secret, or starts a service.

## Installation policy

Do not use an unreviewed `curl | bash` deployment in production.

Deployment steps:

1. Record OS, architecture, CPU, RAM and disk.
2. Install supported Python `>=3.11,<3.14` and required system packages.
3. Clone `NousResearch/hermes-agent` into a versioned release directory.
4. Pin an exact reviewed upstream commit.
5. Verify the MIT license, `pyproject.toml`, lockfile and dependency hashes available to the selected install path.
6. Create a dedicated virtual environment with `uv` or an equivalent locked install.
7. Install only required extras for the first gate, primarily messaging/Telegram; do not install all optional providers and voice/browser dependencies.
8. Create the Mosaid product extension directory.
9. Inject secret file paths through the service environment.
10. Start under a system service with restart limits and bounded logs.
11. Run owner-only Telegram, restart and rollback acceptance tests.

## Filesystem proposal

```text
/opt/mosaid/
├── releases/
│   └── <hermes-commit>/
├── current -> releases/<hermes-commit>/
├── product/
│   ├── identity/
│   ├── policies/
│   ├── skills/
│   ├── workpacks/
│   └── benchmarks/
└── bin/

/var/lib/mosaid/
├── state/
├── sessions/
├── workspaces/
└── backups/

/etc/mosaid/
├── mosaid.env
├── telegram.token
└── model.key
```

Secret files must be regular non-symlink files with owner-only permissions.

## Network exposure

First preference:

- Telegram outbound polling or a tightly controlled webhook.
- No public model API.
- No public Hermes dashboard.
- No public execution or sandbox API; port `8000` is not opened to the internet.
- SSH restricted to a trusted source or private network.

If an HTTP endpoint is later required:

- terminate TLS;
- require an independent service credential;
- apply rate limits and request-size limits;
- bind the endpoint to the smallest necessary network scope;
- never expose an unauthenticated tool endpoint.

## Oracle billing gate

Before deployment, record evidence for:

- instance shape;
- boot volume size;
- public IPv4 charges, if any;
- outbound bandwidth policy;
- region availability;
- Always Free or trial status;
- budget alerts and quota limits.

Do not classify the instance as `verified_free` solely because the account was created successfully. If billing is unclear, mark it `unknown` and keep automatic deployment disabled.

## First acceptance test

1. Send an Egyptian-Arabic request through Telegram.
2. Confirm owner-only access.
3. Confirm Mosaid identity and beginner guidance are loaded.
4. Execute the Research Work Pack using only allowlisted tools.
5. Require approval before any external side effect.
6. Restart the service during an unfinished task.
7. Confirm the conversation/task state recovers without duplicate side effects.
8. Export sanitized evidence.
9. Scan the repository, service environment and logs for secrets.

## Rollback

- Keep the previous pinned release directory.
- Stop the service.
- Switch `/opt/mosaid/current` to the previous release.
- Restore the pre-deployment state backup if a migration occurred.
- Start and verify owner-only `/status` before resuming work.

## External inputs still required

Only non-secret instance facts are needed for the next deployment step:

- instance state;
- public/private IP;
- OS image;
- CPU architecture;
- OCPU;
- RAM;
- disk size;
- confirmed billing tier.

The following facts are **not** needed for the first gate and are recorded only if a future execution backend is evaluated: kernel version, `/dev/kvm` presence, `/dev/ublk-control` presence and nested-virtualization support. Oracle Always Free Ampere shapes are reported not to expose KVM, which is one reason AgentENV is deferred rather than planned.

Never provide the SSH private key, Auth Token, API secret or Telegram token in chat.
