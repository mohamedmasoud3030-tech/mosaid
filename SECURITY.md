# Security policy

## Product status

Mosaid is a security-hardened **product foundation preview**, not a production service. Software gates run on Linux CI and Android ARM64 is cross-compiled. Physical Android/Termux behavior, real Telegram/model credentials, real MCP servers, image-provider credentials, Meta credentials, and an Instagram Professional account remain `PENDING_EXTERNAL_VALIDATION`.

No real Instagram publishing has been performed.

## Reporting

Report suspected vulnerabilities privately to the repository owner. Do not open a public issue containing tokens, Authorization headers, private paths, device identifiers, staging URLs, personal data, or unredacted diagnostics.

## Trust model

- One trusted numeric Telegram owner in a private chat.
- Model output, web/documents, repositories, MCP servers, provider responses, and generated artifacts are untrusted.
- Every Tool side effect is policy gated; high-risk and publishing actions require bound, expiring, single-use approval.
- No free-form shell, browser automation, dynamic Go plugins, MCP auto-discovery, self-modification, or automatic self-update.
- Official Meta Graph API only; no passwords, cookies, or unofficial Instagram API.

See [the product threat model](docs/security/THREAT-MODEL.md) and [hardening controls](docs/security/HARDENING.md).

## Secret rules

Never commit or attach:

- `.env`, `.security.yml`, credential stores, private keys, or access tokens;
- Telegram, model, GitHub, MCP, image-provider, or Meta credentials;
- live media-staging URLs;
- private diagnostics, owner messages, memory exports, or personal data.

Runtime secrets must be one-line regular non-symlink files, mode `0600` or stricter. Use dedicated low-quota test credentials and revoke them after external validation.

## Termux limitation

Termux does **not** provide a strong sandbox between processes running under its Android app UID. A malicious repository test or MCP executable under the same UID may read Mosaid files and secrets despite application-level path and environment controls. Do not run arbitrary untrusted code on the credential-bearing phone. Prefer a separate remote sandbox for such work.

## Continuous controls

The Phase 13 local gate verified formatting, module checksums, 140 unit tests, race tests, vet, staticcheck, govulncheck, Linux and Android builds, binary checksums, secret scan, deterministic CycloneDX SBOM, and license classification. The existing Product CI also passed on the Phase 13 commit.

Activation of the expanded CI definition is still pending because the temporary GitHub token lacked Workflows write permission. The exact reviewed workflow is preserved at [`docs/handoff/PRODUCT-CI-HARDENING.pending.yml`](docs/handoff/PRODUCT-CI-HARDENING.pending.yml); see the adjacent blocker note. Tracked SBOM/license evidence is under [`security/`](security/).

Security scans are point-in-time evidence, not proof that the system is vulnerability-free.

## Historical Phase 0

The `phase0-harness-v1` tag, release, harness, reports, and their earlier advisory analysis are immutable historical qualification evidence. Product hardening does not claim that the archived PicoClaw binary passed physical-phone testing.
