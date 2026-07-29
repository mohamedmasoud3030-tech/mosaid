# Short phone guide

## Before Termux

Install from one official signing family—prefer F-Droid for all three:

1. Termux.
2. Termux:Boot.
3. Termux:API (recommended for battery/temperature evidence and safe file picking).

Do not use the obsolete Google Play Termux build. Open Termux:Boot and Termux:API once after installation. In Android settings set Termux and Termux:Boot battery use to **Unrestricted** and allow background/autostart behavior offered by the phone vendor.

## Import the kit without broad storage permission

If the kit is downloaded in Android's browser, use Android's file picker rather than `termux-setup-storage`:

```bash
pkg update
pkg install termux-api
termux-storage-get phase0-phone-kit.tar.gz
```

Choose `phase0-phone-kit.tar.gz` in the system picker. Alternatively, after a CI run, download the artifact directly with `curl` from its authenticated GitHub URL.

Verify the outer checksum supplied beside the kit, then:

```bash
mkdir -p ~/phase0-kit
cd ~/phase0-kit
tar -xzf ~/phase0-phone-kit.tar.gz
cd phase0-phone-kit
bash phone/install-phone.sh
```

The installer:

- verifies Android ARM64 and the binary checksum;
- installs only runtime/measurement packages;
- prompts privately for one numeric owner ID, a test bot token, and a test model key;
- writes secrets to Termux-private `.security.yml` mode 0600;
- installs runit supervision, log rotation, wake lock, and Termux:Boot startup;
- never requests broad shared-storage access.

## First qualification

Run:

```bash
~/.local/share/picoclaw-phase0/scripts/preflight.sh --network
```

Then, from the owner's private Telegram chat only:

```text
/status
/echo hello
```

Expected examples:

```text
phase0_status=ok uptime_seconds=...
echo: hello
```

Start the first test:

```bash
~/.local/share/picoclaw-phase0/scripts/test-harness.sh arm 01-initial-30m
```

Send the ten numbered echoes described by the command. After 30 minutes:

```bash
~/.local/share/picoclaw-phase0/scripts/test-harness.sh finalize
~/.local/share/picoclaw-phase0/scripts/collect-diagnostics.sh
```

Return the generated diagnostics archive and its `.sha256` file. Do not send `.security.yml`.

## Useful local commands

```bash
sv status picoclaw-phase0
sv restart picoclaw-phase0
sv down picoclaw-phase0
sv up picoclaw-phase0

cat ~/.local/share/picoclaw-phase0/runtime/health.json
~/.local/share/picoclaw-phase0/scripts/test-harness.sh status
~/.local/share/picoclaw-phase0/scripts/test-harness.sh list
```

## Safety stop

Stop immediately if battery temperature exceeds 45 °C, the battery swells, the phone becomes unusually hot, or log/output growth consumes the remaining storage:

```bash
sv down picoclaw-phase0
termux-wake-unlock
```
