# Phase 14 phone test plan

Adapted from the Phase 0 acceptance criteria for the Phase 1–13 product
runtime. A final GO requires real-phone evidence for all mandatory scenarios.

## Platform and installation

| Criterion | Pass threshold |
|---|---|
| CPU architecture | `uname -m=aarch64` and Android ABI `arm64-v8a` |
| Native execution | Android binary starts with `/system/bin/linker64`; no proot/chroot |
| Reinstallability | Fresh install from the phone kit succeeds with documented steps; no shared-storage permission |
| Config security | Config and secrets mode 0600; one positive numeric owner; HTTPS model endpoint; `max_cost_usd=0.01` tripwire unchanged |
| TLS/DNS | Telegram and model endpoint reachable in `preflight.sh --network` |
| Duplicate process | Second supervisor attempt exits 73; one Telegram poller |
| Secret leakage | Zero exact Telegram/model keys in logs, reports, process argv, and the diagnostics archive |

## 24-hour soak

| Metric | Pass threshold |
|---|---|
| Planned duration | At least 86,400 seconds |
| Health sample coverage | ≥90% at 60-second interval |
| Maximum sample gap | ≤180 seconds outside an intentional reboot/force-stop test |
| Unforced restarts | 0 |
| Average RSS | ≤75 MiB |
| Peak RSS | ≤100 MiB |
| Average idle CPU | ≤5% of one core |
| Battery temperature | Every available sample ≤45.0 °C |
| Panic/fatal entries | 0 |
| `/status` p95 latency | ≤3,000 ms while network is available |
| Duplicate handling | Zero duplicate replies for the same Telegram message ID (durable inbox) |

## Recovery

| Scenario | Pass threshold |
|---|---|
| Wi-Fi reconnect | `/status` works within 120 seconds; no restart required |
| Wi-Fi/mobile transition | `/status` works within 120 seconds if mobile data exists |
| Airplane-mode recovery | `/status` works within 120 seconds after connectivity returns |
| Telegram temporary outage | Polling resumes within 120 seconds; no duplicates |
| Model temporary outage | `/status` stays available; model reply works within 120 seconds after restoration |
| Android reboot | Service restored within 120 seconds after boot, unlock, and network availability |
| Child killed | Supervisor restores it within 60 seconds; exactly one restart |
| Termux force-stop | Must recover without manual Termux launch to pass; otherwise record FAIL rather than masking it |
| Memory pressure | No crash preferred; at most one explained restart |

## Battery

- Screen-off idle, unplugged: drain ≤4 percentage points/hour.
- Intended charger: battery must not decline by more than 2 percentage points over the scenario.
- Temperature >45 °C at any sample: FAIL and stop load-generating tests.

## Conversation continuity

- Messages use unique IDs, e.g. `M14-01-001`; the agent's model reply must
  return within the configured model timeout.
- Scenario 01: at least 6 unique numbered messages over 30 minutes, all
  answered in order, zero lost and zero duplicate replies.
- `/remember <text>` followed by `/memory <query>` must return the stored item.
- Crash tests compare pre/post-crash message instrumentation.

## Scenarios

| ID | Scenario | Minimum duration |
|---|---|---|
| 01 | initial-30m | 30 minutes |
| 02 | two-hour | 2 hours |
| 03 | twelve-hour | 12 hours |
| 04 | twenty-four-hour | 24 hours |
| 05 | wifi-reconnect | event-driven |
| 06 | network-transition | event-driven |
| 07 | airplane-mode | event-driven |
| 08 | telegram-outage | event-driven |
| 09 | model-outage | event-driven |
| 10 | reboot | event-driven |
| 11 | child-killed | event-driven |
| 12 | force-stop | event-driven |
| 13 | memory-pressure | event-driven |
| 14 | battery-charger | event-driven |
| 15 | battery-thermal | event-driven |
| 16 | duplicate-message | event-driven |
| 17 | continuity-100 | 100 numbered messages |

## Evidence

Run `collect-diagnostics.sh` after each scenario. The archive contains the
preflight report, health samples, restart counts, redacted logs, config
(no secrets), binary checksum check, and version string. Keep the archives
out of Git (`**/reports/` is ignored); share them only with the owner.

## Decision mapping

- **GO:** every mandatory scenario PASS including the 24-hour soak.
- **GO conditional:** runtime/Telegram/resource tests pass with one bounded,
  documented issue that does not change the platform decision.
- **NO-GO:** native binary fails; background recovery is unreliable;
  reboot/force-stop cannot recover; soak exceeds resource/thermal limits;
  or secrets leak.
