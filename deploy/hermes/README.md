# Hermes Runtime Deployment for Mosaid

This directory prepares the single-runtime deployment of Mosaid on an Oracle Cloud Compute instance.

## Current state

The repository pivot is implemented, but no Oracle instance has been accessed and no live credential has been used. Deployment remains pending until the non-secret instance facts and SSH access are available outside Git.

## Runtime rule

Run one agent runtime only:

```text
Hermes Agent + Mosaid product assets
```

Do not run the historical Go Mosaid agent beside Hermes.

## Required external facts

- instance IP;
- OS and version;
- architecture;
- OCPU and RAM;
- disk size;
- confirmed billing tier;
- SSH public-key access.

Never commit or paste the SSH private key, Oracle Auth Token, Telegram token or model key.

## Deployment sequence

1. Review `docs/pivot/HERMES-RUNTIME-DECISION.md`.
2. Review `docs/pivot/ORACLE-DEPLOYMENT-PLAN.md`.
3. Pin a reviewed Hermes commit in the server-side environment.
4. Copy `mosaid.env.example` to `/etc/mosaid/mosaid.env` and populate it locally on the server.
5. Install only the required Hermes extras for the first Telegram gate.
6. Link or copy `product/identity`, `product/policies` and approved Skills into the Hermes-supported context/Skills locations.
7. Configure owner-only Telegram access.
8. Configure one verified-free or self-hosted model endpoint.
9. Start the service without a public dashboard or public tool endpoint.
10. Run the acceptance and rollback tests documented in the deployment plan.

## Initial product assets

- `product/identity/MOSAID.md`
- `product/policies/SAFETY.md`
- `product/skills/research/SKILL.md`

These files are product source assets. The adapter that maps them into the exact pinned Hermes version must be implemented and tested after the upstream commit is selected.

## No-secret verification

Before every push:

```bash
bash phase0-android-runtime/scripts/scan-secrets.sh .
git diff --check
git status --short
```

The populated `/etc/mosaid/mosaid.env` and secret files must never exist in the repository workspace.
