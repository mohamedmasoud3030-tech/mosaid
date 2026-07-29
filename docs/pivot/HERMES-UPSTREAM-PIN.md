# Hermes Upstream Candidate Pin

Date recorded: 2026-07-29
Repository: `NousResearch/hermes-agent`
Commit: `b8ceba97ed0b2bf0255cc5c8c61c9110a026cda4`
Status: Candidate pin selected; deployment and full supply-chain review remain pending.

## Verified at this commit

- Project name: `hermes-agent`.
- Project version: `0.19.0`.
- License declaration: MIT.
- Supported Python range: `>=3.11,<3.14`.
- Core direct dependencies are intentionally exact-pinned where declared in `pyproject.toml`; some bounded ranges remain for platform/framework dependencies.
- The selected commit is an upstream web/PTY sanitizer fix with expanded tests; it is not a Mosaid modification.

## Why an exact commit is recorded

A moving `main` branch is not reproducible and can silently change dependencies or behavior. The Oracle deployment must clone this repository and check out this exact SHA before installation.

## Required checks before production deployment

1. Verify the fetched repository HEAD exactly equals the recorded commit.
2. Verify the repository origin is `https://github.com/NousResearch/hermes-agent.git`.
3. Review `LICENSE`, `pyproject.toml`, the lockfile and installation scripts at the pinned commit.
4. Generate a dependency inventory for only the selected installation extras.
5. Run the upstream test subset relevant to CLI, Telegram/gateway, Skills, memory and provider configuration.
6. Run Mosaid's owner-only, approval, restart, rollback and no-secret acceptance tests.
7. Record hashes for the deployed source tree and built/installed environment.

## Not yet claimed

- The pinned commit has not been installed on the Oracle instance.
- No Oracle Compute instance has been accessed.
- No live Telegram or model credential has been used.
- No claim is made that every optional Hermes dependency or feature is approved.
- The pin may be changed only through a reviewed commit that updates this document, the environment template and deployment evidence together.
