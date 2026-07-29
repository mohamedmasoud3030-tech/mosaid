# Phase 0 Android Runtime Qualification — Result

## Device

- Device model:
- Android release / SDK:
- CPU ABI:
- Termux version/source:
- Termux:Boot source:
- RAM total:
- Storage free at start/end:
- Battery mode:
- Vendor autostart/background settings:
- Intended charger:

## Pinned software

- Upstream tag: `v0.3.1`
- Tag object: `9fba4cec050cbfe3d73dfcfe015d7960447b9c7f`
- Commit: `2cf030d2fd3b871d7ec17e3be34c24688aac76da`
- Qualification binary SHA-256:
- Phone kit SHA-256:
- Go version: `1.25.12`
- SBOM attached: yes/no

## Preflight

| Check | Result | Evidence |
|---|---|---|
| Native Android ARM64 execution | | |
| App-private write | | |
| DNS | | |
| TLS/CA | | |
| Telegram getMe | | |
| OpenAI-compatible model call | | |
| Timezone/time | | |
| Wake-lock request | | |
| Battery optimization | | |
| Config/tools fail-closed | | |

## Scenario results

| ID | Scenario | PASS/FAIL/INCOMPLETE | Restarts | Avg/peak RSS MiB | Avg/peak CPU % | Peak °C | Battery delta | Echo p95 ms | Lost | Duplicate | Notes |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| 01 | Initial 30m | | | | | | | | | | |
| 02 | Screen locked 2h | | | | | | | | | | |
| 03 | 12h soak | | | | | | | | | | |
| 04 | 24h soak | | | | | | | | | | |
| 05 | Wi-Fi cycle | | | | | | | | | | |
| 06 | Wi-Fi/mobile switch | | | | | | | | | | |
| 07 | Airplane mode | | | | | | | | | | |
| 08 | Model outage | | | | | | | | | | |
| 09 | Telegram outage | | | | | | | | | | |
| 10 | Reboot | | | | | | | | | | |
| 11 | Kill/force-stop Termux | | | | | | | | | | |
| 12 | Memory pressure | | | | | | | | | | |
| 13 | Low battery | | | | | | | | | | |
| 14 | On charger | | | | | | | | | | |
| 15 | Thermal | | | | | | | | | | |
| 16 | 100 repeated messages | | | | | | | | | | |
| 17 | Duplicate update observation | | | | | | | | | | |
| 18 | Crash during message | | | | | | | | | | |
| 19 | Singleton | | | | | | | | | | |
| 20 | 50-message continuity | | | | | | | | | | |

## Security evidence

- Unauthorized user blocked:
- Group blocked:
- Administrative commands checked only after owner auth:
- Second process blocked with exit 73:
- Tools disabled in effective config:
- Secrets found in logs: 0 / other:
- Diagnostics excludes `.security.yml`:
- Panic/fatal events:

## Exceptions and unexplained behavior

1.
2.

## Decision

Choose one:

- [ ] GO with PicoClaw.
- [ ] GO conditional — conditions:
- [ ] NO-GO — reason and alternative trigger:

## Recommendation for next phase

(To be filled only after reviewing real-phone diagnostics. Do not start the next phase from this template.)
