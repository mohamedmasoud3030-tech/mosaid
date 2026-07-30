# Hermes Runtime Deployment for Mosaid

This directory contains the pinned, fail-closed deployment path for running Mosaid as a Hermes Agent profile on Oracle Cloud Compute.

## Current state

The repository pivot and deployment assets are implemented. No Oracle instance has been accessed and no live credential has been used. Live deployment remains pending until the Compute IP, SSH access and non-secret instance facts are available outside Git.

## Runtime rule

Run one agent runtime only:

```text
Hermes Agent + Mosaid product assets
```

Do not run the historical Go Mosaid agent beside Hermes.

## Implemented mapping

- `product/hermes/SOUL.md` → `$HERMES_HOME/SOUL.md`
- `product/hermes/.hermes.md` → `/var/lib/mosaid/workspaces/.hermes.md`
- `product/skills/` → read-only `/opt/mosaid/product/skills`
- `deploy/hermes/config.yaml.example` → `$HERMES_HOME/config.yaml`
- `deploy/hermes/preflight.sh` → `/opt/mosaid/bin/preflight-hermes`
- `deploy/hermes/mosaid-hermes.service` → `/etc/systemd/system/mosaid-hermes.service`

The exact Hermes upstream candidate is pinned to:

```text
b8ceba97ed0b2bf0255cc5c8c61c9110a026cda4
```

## Required external facts

- instance IP;
- OS and version;
- architecture;
- OCPU and RAM;
- disk size;
- confirmed billing tier;
- SSH public-key access.

Never commit or paste the SSH private key, Oracle Auth Token, Telegram token or model key.

## Server prerequisites

The first gate expects:

- Linux with systemd;
- Python 3.11, 3.12 or 3.13;
- `git`;
- `uv` installed from a reviewed package/source;
- a system user named `mosaid`;
- **no** Docker, `/dev/kvm`, AgentENV or any other sandbox runtime. Terminal and code execution are globally disabled during the first gate, so no isolation layer is installed. Docker is the first isolation option to evaluate *after* launch, under a separate ADR; see [`../../docs/pivot/AGENTENV-EXECUTION-BACKEND-DECISION.md`](../../docs/pivot/AGENTENV-EXECUTION-BACKEND-DECISION.md).

Create the service user with the OS-supported system-user command. On common Oracle Linux/Ubuntu images:

```bash
sudo useradd --system --home-dir /var/lib/mosaid --create-home --shell /usr/sbin/nologin mosaid
```

Do not add `mosaid` to the Docker group during the read-only first gate.

## Stage the pinned release

Clone this Mosaid branch on the server, inspect it, and run:

```bash
python3 scripts/verify-hermes-pivot.py
bash -n deploy/hermes/stage-release.sh deploy/hermes/preflight.sh
sudo MOSAID_SOURCE="$PWD" bash deploy/hermes/stage-release.sh
```

The staging script:

- fetches only the reviewed Hermes commit;
- verifies repository URL, SHA, license header, version and Python range;
- installs the locked `messaging` extra with `uv --frozen`;
- builds the virtual environment in its final path;
- makes runtime source and Mosaid product assets read-only;
- installs the identity, project policy, Skills, preflight and systemd unit;
- does **not** create secrets;
- does **not** start or enable the service.

## Configure secrets locally on Oracle

Install the example as Hermes' real `.env`, then edit it only on the server:

```bash
sudo install -o mosaid -g mosaid -m 0600 \
  deploy/hermes/mosaid.env.example \
  /var/lib/mosaid/hermes/.env
sudo -u mosaid editor /var/lib/mosaid/hermes/.env
```

Replace all `REPLACE_*` values. Required first-gate values:

- one numeric `TELEGRAM_ALLOWED_USERS` owner;
- Telegram bot token;
- model name;
- HTTPS or loopback model endpoint;
- provider key or a non-secret dummy value only when the local endpoint requires no authentication;
- verified Oracle billing metadata.

The Auth Token page in Oracle Cloud is not used for SSH and is not a substitute for the Compute IP or SSH key.

## Validate before start

```bash
sudo -u mosaid /opt/mosaid/bin/preflight-hermes
sudo systemctl daemon-reload
sudo systemctl start mosaid-hermes
sudo systemctl status --no-pager mosaid-hermes
sudo journalctl -u mosaid-hermes -n 100 --no-pager
```

Start manually first. Enable at boot only after owner-only Telegram, model, restart and no-secret tests pass:

```bash
sudo systemctl enable mosaid-hermes
```

## First acceptance gate

1. Only the configured numeric Telegram owner can receive a response.
2. The reply follows the Arabic Mosaid identity.
3. `/mosaid-research` is discoverable.
4. Telegram has no terminal, file, code execution, browser, delegation, cron or publishing toolset.
5. No sandbox or execution service is running and no execution port is listening.
6. Skill and memory writes are staged for approval.
7. The selected model endpoint has no paid fallback.
8. Restarting the service preserves the Hermes session state.
9. Logs and the repository pass secret scans.

## Rollback

Do not delete the previous release. To roll back:

```bash
sudo systemctl stop mosaid-hermes
sudo ln -sfn /opt/mosaid/releases/<previous-reviewed-sha> /opt/mosaid/current.new
sudo mv -Tf /opt/mosaid/current.new /opt/mosaid/current
sudo -u mosaid /opt/mosaid/bin/preflight-hermes
sudo systemctl start mosaid-hermes
```

The current preflight intentionally accepts only the reviewed SHA recorded in this branch. A rollback to another SHA therefore requires restoring the matching preflight, config and evidence bundle together; do not bypass the check.

## Repository verification

Before every push:

```bash
python3 scripts/verify-hermes-pivot.py
bash -n deploy/hermes/stage-release.sh deploy/hermes/preflight.sh
bash phase0-android-runtime/scripts/scan-secrets.sh .
git diff --check
git status --short
```

Populated `.env` files and all live credentials must remain outside the repository workspace.
