# Phase 0 numeric acceptance criteria

Phase 0 cannot receive a final GO before real-phone evidence covers all mandatory scenarios.

## Platform and installation

| Criterion | Pass threshold |
|---|---|
| CPU architecture | `uname -m=aarch64` and Android ABI `arm64-v8a` |
| Native execution | Android binary starts with `/system/bin/linker64`; no proot/chroot |
| Reinstallability | Fresh install from phone kit succeeds using documented steps; no shared-storage permission |
| Config security | Config and secrets mode 0600; exactly one numeric owner; all tool flags false |
| TLS/DNS | Telegram DNS, Telegram TLS, `getMe`, and one model completion all pass preflight |
| Duplicate process | Second supervisor attempt exits 73; exactly one Telegram poller |
| Secret leakage | Zero exact Telegram/model keys in logs, reports, process argv, and diagnostic archive |

## 24-hour soak

| Metric | Pass threshold |
|---|---|
| Planned duration | At least 86,400 seconds |
| Health sample coverage | ≥90% at 60-second interval |
| Maximum sample gap | ≤180 seconds outside an intentional reboot/force-stop test |
| Unforced agent restarts | 0 |
| Average RSS | ≤75 MiB (76,800 KiB) |
| Peak RSS | ≤100 MiB (102,400 KiB) |
| Average idle CPU | ≤5% of one core |
| Battery temperature | Every available sample ≤45.0 °C |
| Panic/fatal entries | 0 |
| `/echo` p95 latency | ≤3,000 ms while network is available |
| Lost numbered echoes | 0 |
| Duplicate handling of a Telegram message ID | 0 |

## Recovery

| Scenario | Pass threshold |
|---|---|
| Wi-Fi reconnect | `/echo` works within 120 seconds; no process restart required |
| Wi-Fi/mobile transition | `/echo` works within 120 seconds if mobile data exists |
| Airplane-mode recovery | `/echo` works within 120 seconds after connectivity returns |
| Telegram temporary outage | Polling resumes within 120 seconds; no duplicate numbered echo |
| Model temporary outage | `/echo` remains available; model reply works within 120 seconds after restoration |
| Android reboot | Service restored within 120 seconds after boot, unlock, and network availability; 1–2 starts recorded |
| Agent child killed | runit/supervisor restores it within 60 seconds; exactly one restart |
| Termux force-stop | Must recover without manual Termux launch to pass. If Android blocks this, record FAIL rather than masking it |
| Memory pressure | No crash preferred; at most one explained restart and no lost/duplicate numbered echo |

## Battery

Battery and temperature evidence is mandatory for the battery/charger/thermal scenarios. Missing Termux:API data makes those scenarios INCOMPLETE.

- Screen-off idle, unplugged: drain ≤4 percentage points/hour.
- 12/24-hour unplugged measurement, when performed: average drain ≤3 points/hour.
- Intended charger: battery must not decline by more than 2 percentage points over the scenario.
- Temperature >45 °C at any sample: FAIL and stop load-generating tests.
- Do not intentionally heat the device to test thermal behavior.

## Telegram message continuity

For scenarios 16, 17, and 20:

- Message text must use unique IDs, e.g. `/echo P0-16-001`.
- Scenario 16: exactly 100 unique echo message IDs and zero duplicate handling.
- Scenario 20: exactly 50 unique echo message IDs and zero lost/duplicate handling.
- Crash tests must compare pre/post-crash Telegram `message_id` instrumentation.
- The upstream transport has no durable inbox. Any demonstrated duplicate after crash blocks an unconditional GO.

## Decision mapping

- **GO with PicoClaw:** every mandatory scenario PASS, including 24-hour soak, reboot, singleton, no secrets, no duplicates/loss, and acceptable battery/thermal results.
- **GO conditional:** runtime/Telegram/resource tests pass, but one bounded issue remains that can be resolved in the next phase without changing the platform decision—for example durable dedupe still needs implementation.
- **NO-GO:** native binary fails; background recovery is unreliable; force-stop/reboot cannot recover under the intended device policy; 24-hour soak exceeds resource/thermal limits; or secrets/tools cannot be kept disabled.
