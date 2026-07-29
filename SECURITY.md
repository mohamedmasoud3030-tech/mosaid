# Security policy

## Project status

Mosaid is pre-product research and an Android runtime qualification harness. It is not ready for production credentials, sensitive repositories, public Telegram groups, or unattended publishing.

## Reporting

Until a dedicated private advisory process is configured, report suspected vulnerabilities privately to the repository owner. Do not create a public issue containing a token, exploit, device identifier, private path, or unredacted diagnostic archive.

## Current trust model

- One trusted owner and one private Telegram chat.
- All Phase 0 tools, Skills, MCP, cron commands, web, filesystem tools, Shell, remote execution, publishing, and self-update are disabled.
- Android/Termux provides an app UID boundary from ordinary unrelated apps; it does not isolate processes running under the Termux UID from one another.
- `chmod 600`, redaction, allowlists, and application path checks are defense in depth, not an OS sandbox against malicious code.
- The Phase 0 binary must not be used with production accounts or keys.

## Secret rules

Never commit or attach:

- `.env` or `.security.yml`;
- Telegram bot tokens;
- model-provider or GitHub tokens;
- Authorization headers;
- private keys;
- phone diagnostics before running the collector/redaction checks;
- future phone results containing personal identifiers.

Use dedicated low-quota test credentials and revoke them after qualification. The repository CI performs pattern-based secret scanning, but scanners do not replace manual review.

## Phase 0 controls

The qualification harness enforces:

- one numeric Telegram owner;
- private chat only;
- bounded `/echo` and non-sensitive `/status`;
- config and binary checksum verification before process start;
- tool and Skill disablement at config and turn-profile layers;
- exact-value log redaction and rotated logs;
- singleton supervision and clean shutdown;
- diagnostics packaging that refuses to proceed when an exact active secret is found outside the secret file.

See:

- `docs/phase0/THREAT-NOTES.md`
- `docs/phase0/ACCEPTANCE-CRITERIA.md`
- `phase0-android-runtime/manifests/sbom.cdx.json`
- `THIRD_PARTY_NOTICES.md`

## Unsupported security claims

The project does not currently claim:

- exactly-once Telegram processing;
- safe arbitrary Shell execution;
- safe execution of untrusted repository tests;
- multi-user isolation;
- Android background-service reliability;
- production-ready secret storage;
- safe external Skill/MCP installation.

These claims require future design and testing; Phase 1 has not started.
