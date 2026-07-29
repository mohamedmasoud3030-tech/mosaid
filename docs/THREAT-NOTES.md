# Phase 0 threat notes

This is a qualification harness, not a hardened personal-agent product.

## Trust boundaries

### What is enforced

- One explicit numeric Telegram user ID.
- Telegram private chats only; groups, supergroups, and channels are rejected before the agent bus.
- `/status` reveals only a generic OK state and process uptime.
- `/echo` returns at most 512 Unicode code points and invokes no model or tool.
- All PicoClaw callable tools are disabled twice:
  - every tool `enabled` flag is false in the config;
  - the active turn profile has `tools.mode=off` and rejects tool calls again at execution time.
- Skills, MCP, cron commands, heartbeat, hooks, evolution, web, images, GitHub, and every non-Telegram channel are disabled.
- Config verification fails closed before each process start.
- The supervisor verifies the binary SHA-256 before starting it.
- Secrets live only in `.security.yml` with mode 0600 and are not passed as command-line arguments.
- Logs are passed through exact-value and pattern redaction before `svlogd` rotation.

### What is not an enforced security boundary

- `chmod 600` does not isolate one Termux process from another process under the same Android app UID.
- The Termux app sandbox protects its private directory from ordinary unrelated Android apps, but not from root, device compromise, adb/debug configurations, backups outside the sandbox, or another process with the Termux UID.
- `restrict_to_workspace`, disabled tools, and model instructions are application controls—not an OS sandbox against arbitrary code execution.
- Runit, wake locks, and Termux:Boot improve availability, not confidentiality or integrity.
- Long polling does not provide durable exactly-once message processing. Phase 0 instruments duplicate message IDs but does not implement a durable inbox.

## Main threats

| Threat | Phase 0 control | Residual risk |
|---|---|---|
| Unknown Telegram user | Numeric allowlist checked before media download or agent bus | Bot existence is visible; generic denial may be sent |
| Owner adds bot to group | Qualification ingress rejects every non-private chat | Telegram still delivers the update to the bot process |
| Prompt injection through owner text | No tools, web, files, MCP, or publish capabilities are callable | Model may produce misleading text, but cannot call a tool |
| Secret in process list | Secrets are read from a 0600 local file, not argv | Same-UID process can read file or process environment used by redactor |
| Secret in logs | Exact and pattern redaction; diagnostic packaging refuses if an exact secret is found | Novel encodings/partial secrets may bypass pattern redaction |
| Malicious upstream artifact | Pinned commit/tree, custom build, two-build hash comparison, SBOM | Upstream source and Go module supply chain are still trusted inputs |
| Compromised dependency | `go mod verify`, pinned versions, `govulncheck`, license inventory | One documented module-level advisory remains; 93 linked modules is too broad |
| Duplicate Telegram update | Message ID instrumentation and test harness | No durable deduplication; a crash can still cause duplicate processing |
| Android kills Termux | wake lock, Unrestricted battery setting, runit, Termux:Boot | OEM/Android force-stop policies can prevent unattended recovery |
| Log/storage exhaustion | 1 MB log files, ten rotations, disk metrics | Model/session state can still grow; Phase 0 has no quota manager |
| Model endpoint interception | HTTPS-only API base; CA/TLS preflight | A compromised device CA store can still intercept TLS |

## Secret handling rules

- Use a dedicated test Telegram bot token.
- Use a test model key with the smallest practical quota and no access to unrelated services.
- Do not reuse future production credentials.
- Do not paste tokens into shell command lines, issues, chat messages, screenshots, or diagnostics.
- Revoke both credentials after Phase 0.
- `.security.yml` is deliberately excluded from diagnostics and backups.

## Safe failure behavior

- Invalid/missing config, wrong owner shape, enabled tools, wrong checksum, or permissive channel config prevents startup.
- A second supervisor refuses to start with exit code 73.
- Network/API failure causes supervised restart with exponential backoff up to 300 seconds.
- Process crashes are counted and preserved across reboot.
- A diagnostic package is not created if an exact current secret is found in logs/results.

## Qualification-only vulnerability exception

`govulncheck` reports `GO-2026-5932` at module level for `golang.org/x/crypto/openpgp`. The actual package is absent from the `go list -deps` graph for the built command. This exception is recorded rather than hidden. A future product build must eliminate unnecessary provider/channel dependencies and re-run the scan with no broad exception.
