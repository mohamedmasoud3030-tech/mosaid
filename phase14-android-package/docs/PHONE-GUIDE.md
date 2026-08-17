# Mosaid product — phone guide

Target: an unused Android phone (arm64-v8a) running official Termux, controlled from an iPhone through a private Telegram bot.

## Before you start

1. Prepare **dedicated low-quota test credentials**:
   - A Telegram bot token from @BotFather (a separate test bot, not a production bot).
   - An OpenAI-compatible model API key with a small spending limit.
   - Your numeric Telegram user ID (e.g. from @userinfobot).
2. Install **Termux** from F-Droid or the official Termux GitHub releases (same source/signing family).
3. Install **Termux:Boot** from the same source, and open it once manually.
4. Set Android battery usage for both apps to **Unrestricted**.

## Install

1. Copy the phone kit tarball to the phone (for example `termux-setup-storage` is NOT needed: transfer over USB/`adb push` into the app directory, or a private channel, or `termux-share` from the opposite direction).
2. Verify and extract inside Termux:

```bash
cd ~/downloads   # wherever you placed it
sha256sum -c phase14-phone-kit.tar.gz.sha256
tar xzf phase14-phone-kit.tar.gz
bash phase14-phone-kit/phone/install-product.sh
```

The installer asks for: owner ID, bot token, model base URL, model name, API key. It writes `~/.local/share/mosaid` (binary, scripts, data, logs) and `~/.config/mosaid` (secrets, mode 0600).

3. Run the network preflight and read the output:

```bash
bash ~/.local/share/mosaid/scripts/preflight.sh --network
```

A passing preflight shows `telegram_api=pass` and `model_api=pass` and ends with `Preflight: PASS`.

4. In Telegram, send `/status` to the bot. A healthy reply looks like:

```text
Mosaid phase14 status=running uptime=… messages=…
```

## Daily operation

| Command | What it does |
|---|---|
| `/help` | Lists commands |
| `/status` | Process status, uptime, message count |
| `/stop` | Stops the active in-flight request |
| `/approve` / `/deny` | Answer a pending approval request |
| `/memory <query>` | Search memory |
| `/remember <text>` | Store a memory |
| `/forget <id>` | Remove a memory |
| `/export` | Export memories |

- Service control: `sv status mosaid`, `sv restart mosaid`, `sv down mosaid`, `sv up mosaid`.
- Logs (already redacted): `ls ~/.local/share/mosaid/logs/`.
- Live health snapshot: `cat ~/.local/share/mosaid/runtime/health.json`.
- Restart counter: `cat ~/.local/share/mosaid/runtime/restart.count`.

## Diagnostics

```bash
bash ~/.local/share/mosaid/scripts/collect-diagnostics.sh
```

This refuses to package anything if a secret ever appears in the logs, then produces `mosaid-diagnostics-*.tar.gz` + `.sha256` under `~/.local/share/mosaid/reports/`. Share with `termux-share -a send <file>` (no storage permission needed).

## Uninstall

```bash
bash ~/.local/share/mosaid/scripts/uninstall-product.sh          # keep data
bash ~/.local/share/mosaid/scripts/uninstall-product.sh --purge-data
```

## Known limits

- OEM battery optimizations can still kill Termux despite the wake lock; this is exactly what the qualification checklist measures.
- Force-stop of the Termux app halts everything until Termux is opened again.
- All Termux processes share one app UID; do not run untrusted code in the same Termux.
