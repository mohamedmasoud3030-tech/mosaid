# Manual steps that cannot be completed in this environment

These are the only owner/device actions still required.

1. Install current official Termux, Termux:Boot, and preferably Termux:API from one matching source/signature family; open Boot/API once.
2. Set Android battery mode for Termux and Termux:Boot to Unrestricted; enable vendor autostart/background permission if present.
3. Import `phase0-phone-kit.tar.gz` through the Android file picker (`termux-storage-get`) or download a CI artifact directly. Do not grant broad storage access.
4. Verify the supplied SHA-256, extract the kit, and run `bash phone/install-phone.sh` once.
5. At the hidden prompts enter:
   - one numeric Telegram owner ID;
   - a dedicated Phase 0 bot token;
   - an HTTPS OpenAI-compatible base URL, model ID, and low-quota test key.
6. Run `preflight.sh --network` and return the JSON if it fails.
7. In the owner's private chat send `/status` and `/echo hello`. Also test one unauthorized user and one group; they must not reach the agent bus or reveal status.
8. Run the twenty scenarios in `test-plan.json`, especially the 24-hour soak, network transitions, reboot, Termux force-stop, duplicate/singleton, battery, and thermal tests.
9. After each scenario run `test-harness.sh finalize` and review `result.json`.
10. At the end run `collect-diagnostics.sh` and return only the diagnostics archive plus its checksum. Never return `.security.yml`.
11. Revoke the test Telegram bot token and model key after Phase 0.

No claim about Android runtime success is made until these results are returned.
