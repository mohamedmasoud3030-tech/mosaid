# Mosaid product — physical phone qualification checklist

Run the scenarios in order on the real phone. Record each result in the scenario directory. The health sampler writes one sample per minute to `~/.local/share/mosaid/runtime/health.json` and, while a scenario is active, to `~/.local/share/mosaid/tests/<scenario>/samples.csv`.

To start a scenario, write an `active.json` (the sampler picks it up within a minute):

```bash
cat > ~/.local/share/mosaid/tests/active.json <<'EOF'
{"scenario":"01-install-30m","end_epoch":<unix epoch when the scenario ends>}
EOF
# epoch example: date -d "+30 minutes" +%s
```

To finish: `rm ~/.local/share/mosaid/tests/active.json`.

Provisional pass targets (to be confirmed or tightened from the first measured run):

| Metric | Target |
|---|---|
| `/status` reply after idle | < 5 seconds |
| Idle RSS | < 300 MB |
| RSS growth over 24 h | < 50 MB |
| Restart counter | 0 unexpected increments |
| Secrets in logs/reports | none (diagnostics refuse otherwise) |
| Battery drain idle 1 h | < 5% |
| Battery temperature | < 42 °C |
| Disk free floor | > 500 MB |

## Scenarios

### 01 — Install and 30-minute idle (first run)
1. Install per `PHONE-GUIDE.md`; run `preflight.sh --network` → all `pass`.
2. Send `/status`, `/help`, `/remember test hello`, `/memory test`, `/forget <id>`, `/export`.
3. Start scenario 01 for 30 minutes; leave the phone idle and locked.
4. Pass: `/status` works throughout; `health.json` has ~30 samples; RSS stable; no restart increment.

### 02 — Model conversation
1. Send a normal multi-turn conversation (3–5 messages), including one long message near `max_message_bytes`.
2. Send `/stop` during a reply.
3. Pass: replies arrive, `/stop` ends the turn, no crash, budgets honored (no runaway loop).

### 03 — Flood and duplicates
1. Send 6 messages within a few seconds (above `message_burst`).
2. Pass: extras are rejected politely; agent keeps responding to normal traffic afterwards; no restart.

### 04 — 2-hour soak
1. Start scenario 04 for 2 hours; send `/status` every 15 minutes.
2. Pass: all replies < 5 s; RSS growth < 20 MB; no restart increment; battery drain < 10%.

### 05 — 12-hour overnight soak
1. Start scenario 05 before sleep; check `/status` in the morning.
2. Pass: continuous `health.json` samples; no restart increment; RSS growth < 40 MB.

### 06 — 24-hour soak
1. Start scenario 06 for 24 hours with periodic `/status` and one conversation every few hours.
2. Pass: targets above; no crash; scheduler/approval features untouched (they remain off until configured).

### 07 — Reboot recovery
1. With Termux:Boot installed and opened once: reboot the phone.
2. Wait 5 minutes; send `/status`.
3. Pass: service restarted by itself, `/status` answers, `restart.count` incremented at most by the boot cycle.

### 08 — Network transition
1. Switch Wi-Fi → mobile data → Wi-Fi while chatting.
2. Pass: after short retries replies resume; no restart storm (counter may increment at most once if the network outage exceeds retry budgets).

### 09 — Force-stop recovery
1. Force-stop Termux from Android settings; wait 1 minute; reopen Termux.
2. Pass: `sv up mosaid` (or the boot hook) restarts the supervisor; `/status` answers; data intact.

### 10 — Memory pressure and battery/thermal watch
1. While scenario 10 is active, open a few heavy apps for 15 minutes; observe `health.json` RSS and battery temperature.
2. Pass: process survives; temperature < 42 °C; free disk > 500 MB; no secret appears in any log.

## Final step

```bash
bash ~/.local/share/mosaid/scripts/collect-diagnostics.sh
```

Attach the tarball to the qualification report. A passing run of 01–10 plus the diagnostics archive is the GO evidence for the phone runtime; any failure becomes a tracked issue before Phase 15 sign-off.
