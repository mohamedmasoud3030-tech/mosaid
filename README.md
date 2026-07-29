# Phase 0 — Android Runtime Qualification

A strictly isolated qualification harness for `sipeed/picoclaw` `v0.3.1` on Android/Termux.

**Current decision:** GO conditional **to physical-phone testing only**. No product/Fork/skills/MCP/GitHub/Instagram work has started, and no claim of phone execution is made.

## Pinned identity

```text
Tag:              v0.3.1
Tag object:       9fba4cec050cbfe3d73dfcfe015d7960447b9c7f (unsigned)
Commit:           2cf030d2fd3b871d7ec17e3be34c24688aac76da
Tree:             79530d185c4c5eb30719fd45cf323217d2a9f5c5
Qualification Go: 1.25.12
Android target:   arm64-v8a / GOOS=android / GOARCH=arm64
Binary SHA-256:   b68746ddeeb341c291da5f93f59f857cdd892d8fe76940367604a2ec1c729a4f
```

## Read first

1. [Execution report](docs/EXECUTION-REPORT.md)
2. [Source/release verification](docs/SOURCE-VERIFICATION.md)
3. [Numeric acceptance criteria](docs/ACCEPTANCE-CRITERIA.md)
4. [PoC threat notes](docs/THREAT-NOTES.md)
5. [Short phone guide](docs/PHONE-GUIDE.md)
6. [Manual phone-only steps](docs/MANUAL-STEPS.md)

## Phone deliverable

- `release/phase0-phone-kit.tar.gz`
- `release/phase0-phone-kit.tar.gz.sha256`
- Final kit SHA-256: `72a10827f7adbc9c743e8b9cddac89b5eebadcb349e1dbcf811acfb787bf4a63`

After importing and extracting the kit inside Termux, the single setup entry point is:

```bash
bash phone/install-phone.sh
```

It prompts for test credentials without placing them on the command line. Use a dedicated test bot and a low-quota test model key.

## Requested deliverables map

| Requested output | Location |
|---|---|
| Source/version report | `docs/SOURCE-VERIFICATION.md` |
| SHA/checksum/version manifest | `manifests/source-manifest.json` |
| SBOM | `manifests/sbom.cdx.json` |
| Final linked dependencies | `manifests/linked-modules.tsv` |
| License report | `manifests/license-report.tsv` |
| Threat notes | `docs/THREAT-NOTES.md` |
| Phone installer | `phone/install-phone.sh` |
| Preflight verification | `phone/preflight.sh`, `phone/verify-config.sh` |
| Foreground/supervised run | `phone/run-foreground.sh`, `phone/supervisor.sh` |
| Termux:Boot script | `phone/10-picoclaw-phase0.boot` |
| Health/watchdog/logging | `phone/health-sampler.sh`, runit + `svlogd` setup in installer |
| Test harness | `phone/test-harness.sh`, `config/test-plan.json` |
| Result collector | `phone/collect-results.sh` |
| Diagnostics return bundle | `phone/collect-diagnostics.sh` |
| Short user guide | `docs/PHONE-GUIDE.md` |
| Result report template | `templates/PHASE0-RESULT-REPORT.md` |
| CI build/SBOM/scan/provenance | `.github/workflows/phase0-build.yml` |
| Current decision/next recommendation | `docs/EXECUTION-REPORT.md` |

## Safety properties

- One numeric Telegram owner; private chat only.
- `/status` and `/echo` are the only local diagnostic commands added.
- Every callable tool is disabled in config and by turn-profile execution policy.
- No Shell, file tool, MCP, cron, memory tool, web, image, GitHub, Instagram, dynamic skill, remote exec, self-update, group, or non-Telegram channel.
- Secrets are excluded from Git/diagnostics and redacted from logs.
- No shared-storage permission is requested by the installer.
- `chmod 600` is documented as defense in depth, not an OS sandbox.

## Important limitation

The upstream Telegram transport does not provide a durable exactly-once inbox. The harness records message IDs and detects duplicate handling, but does not hide or repair it. Real crash/reconnect tests decide whether this remains a Phase 1 condition or blocks PicoClaw entirely.
