# Mosaid agent core

The agent core turns Mosaid from a chat responder into an actor: the model can
request tools, the runtime executes them through the existing policy and
approval engine, and the results are fed back until the model answers or a
budget stops it.

Before this change the runtime was an engine without a drivetrain. The tool
registry, policy engine, approval engine, workspace sandbox, memory store and
prompt scaffolding all existed and were tested, but `cmd/mosaid` never
constructed a tool registry and `model.Client` only exposed
`Complete(ctx, []Message) (string, error)`. The model could talk; it could not
act.

## What runs now

```text
Telegram update -> gateway -> durable inbox -> agent.Agent.Handle
                                                   |
                                    commands (/tools, /approve, /deny, ...)
                                                   |
                                    agentloop.Run(config)
                                                   |
              model.CompleteWithTools  <---->  tools.Registry.Execute
                  (tools, tool_calls)            (policy + approvals)
                                                   |
                                        workspace.* / memory.*
```

1. `internal/model` — native OpenAI-compatible tool calling (`tools`,
   `tool_choice`, `tool_calls`, `tool_call_id`, provider `usage`), typed HTTP
   errors, bounded retries with `Retry-After` support. The old
   `Client.Complete` path is untouched, so plain-chat clients still work.
2. `internal/agentloop` — the bounded loop: model steps, tool calls per step,
   tool-result size, tokens, cost, retries, and a forced final-answer turn so a
   chatty model cannot dead-end the request.
3. `internal/tools` — `Catalog()` derives the model-facing tool list from the
   same `policy.Tool` declaration the execution path enforces, and fails closed
   if a tool lacks a description or a valid JSON Schema object. Workspace tools
   now carry schemas and model-facing descriptions; `memory.search` and
   `memory.remember` are new.
4. `internal/agent` — dispatch for `/tools`, `/approve`, `/deny`; an in-memory
   continuation so an approved call executes exactly the arguments that were
   hashed and displayed; a compact Arabic approval prompt and a per-turn tool
   footer; `prompt.go` for the operating contract.
5. `internal/console` + `cmd/mosaid-cli` — the same agent in a terminal.
6. `scripts/dev/demo-agent-local.sh` — a credential-free end-to-end demo against
   a scripted local HTTPS provider.

## Approval semantics

* A write tool never executes speculatively. `tools.Registry.Execute` returns an
  `Approval` and the loop stops.
* The owner sees the tool name, the target resource, a bounded preview of the
  arguments, and the token.
* `/approve <token>` resolves the approval (bound to user, tool, argument hash
  and resource) and then executes the frozen argument JSON. Replays fail because
  `approval_uses` is primary-keyed and the continuation is consumed.
* `/deny <token>` resolves the approval and discards the continuation; nothing
  executes.
* Continuations are deliberately in-memory with a 15-minute TTL and an
  eight-entry cap. A restart loses the continuation, and `/approve` then answers
  "the call details are no longer saved" rather than replaying anything.

## Budgets

`agentloop.Config` is bounded by `security.BudgetLimits` (model steps, tool
calls, tokens, cost, retries) and by its own limits (`MaxSteps`,
`MaxCallsPerStep`, `MaxToolResultBytes`, `MaxTokens`). Provider-reported usage is
charged when present; otherwise tokens are estimated so a provider that omits
usage cannot silently bypass the cost tripwire.

## Configuration

All new keys are optional; an existing config file keeps working and simply
runs without file tools.

```json
{
  "workspace_dir": "/data/data/com.termux/files/home/mosaid-workspace",
  "agent": {
    "max_steps": 4,
    "max_calls_per_step": 4,
    "max_tool_result_bytes": 8192,
    "max_tokens": 1024,
    "system_prompt": "optional replacement for the built-in contract"
  }
}
```

Rules enforced by `config.Validate`:

* `workspace_dir` must be an existing absolute directory, must not be `/`, and
  must not contain or be contained by `data_dir` (which holds the database and
  secret files).
* `agent.max_steps` must not exceed `limits.max_model_steps`.
* `max_tool_result_bytes` is capped at 64 KiB, `max_tokens` at 8192.

## Laptop / no phone

Yes. The phone is only one possible host. Three modes, in ascending order of setup:

| Mode | Needs | What you get |
|---|---|---|
| `mosaid-cli` | a model key | the same agent, tools, policy and approvals in a terminal; **no bot, no phone, no inbound port** |
| `mosaid` daemon on the laptop | model key + Telegram bot token | identical behaviour to the phone, reachable from your phone's Telegram app while the laptop is on |
| `mosaid` daemon on the phone | the Phase 14 kit | always-on without a laptop |

The Telegram bot is created from BotFather, which works from Telegram Desktop or
web — no phone required. `telegram` is now an optional config section: omit it
and `mosaid-cli` runs with no chat at all, while the daemon refuses to start with
`telegram.token_file is required to run the Telegram daemon; use mosaid-cli for a
chat-free session` (exit 2), so a misconfigured daemon still fails closed.

Laptop config (`config/mosaid.laptop.example.json`) is the smallest one — no
Telegram block:

```bash
mkdir -p ~/mosaid/.data ~/mosaid/.secrets ~/mosaid/notes
printf '%s\n' 'YOUR_GEMINI_KEY' > ~/mosaid/.secrets/model.key
chmod 600 ~/mosaid/.secrets/model.key
sed "s#/HOME#$HOME#g" config/mosaid.laptop.example.json > ~/mosaid/config.json
go run ./cmd/mosaid-cli --config ~/mosaid/config.json
```

Verified in this session: config with **no** `telegram` key → `mosaid-cli` ran a
full session (list → approval → write → read) against the scripted provider, and
the daemon exited 2 with the message above.

Platform matrix (cross-compiled from the same source, no CGO):

| OS | Daemon | CLI | Notes |
|---|---|---|---|
| Linux amd64/arm64 | ✅ builds and runs | ✅ | the reference host |
| macOS arm64/amd64 | ✅ builds | ✅ | untested at runtime here |
| Android arm64 (Termux) | ✅ | ✅ | Phase 14 kit |
| Windows amd64 | ✅ builds | ✅ builds | prebuilt kit: `release/windows-amd64/`, see below |

## Windows 10, 4 GB RAM, no phone

Measured on this build (Linux host, same code paths): **11 MB RSS idle, 16 MB peak**
during a full turn with a model call, tool execution and SQLite writes. The model
runs on the provider, so the laptop only holds the process and your files. A 4 GB
machine is not the constraint; the browser is.

The prebuilt kit needs **no Go toolchain**: `release/windows-amd64/mosaid-windows-amd64.zip`
(9.5 MB) contains `mosaid.exe`, `mosaid-cli.exe`, `config.example.json`,
`start-mosaid.bat` and `RUN-WINDOWS.ar.md`. Rebuild it with
`scripts/dev/build-windows-kit.sh`; the archive ships a `SHA256SUMS.txt` and the
zip has a sidecar `.sha256`.

Best path for that machine:

1. Unzip to `C:\mosaid`, create `C:\mosaid\secrets\model.key` (one line, the
   Gemini key), copy `config.example.json` to `config.json`.
2. Run `start-mosaid.bat` (it sets `GOMEMLIMIT=64MiB` and keeps the window open
   on error). Use **`mosaid-cli.exe`, not the daemon**: no bot, no open port, no
   need to leave it running.
3. Skip the Telegram daemon unless you later want phone access; then add a
   `telegram` block and run `mosaid.exe`.

Windows-specific behaviour:

* `%LOCALAPPDATA%`-style per-user paths are the intended home for the key; when
  the kit is used it lives in a per-user folder, which is what protects it.
* Secret permissions are **not** checked on Windows (`internal/secrets/mode_windows.go`):
  NTFS ACLs decide access, and Go reports every readable file as `0666`.
  Rejecting that unconditionally would make the product unusable on Windows.
* `process.run` termination stops the child process only, not its grandchildren
  (`CREATE_NEW_PROCESS_GROUP` + `Process.Kill`), because no portable group kill
  exists without spawning a shell. That tool stays unregistered in `cmd/mosaid`.
* The singleton lock uses `OpenProcess` + `GetExitCodeProcess` instead of
  `syscall.Kill`, so a stale lock after a crash or reboot is still taken over.

Cross-compiled and compile-checked here for `windows/amd64` and `windows/arm64`;
Linux runtime behaviour is verified end to end. The Windows *runtime* run is the
one step that needs the owner's machine.

## Running it

```bash
# 1. No key, no bot, no network: scripted provider end to end.
scripts/dev/demo-agent-local.sh

# 2. Your real key, terminal session (needs config.json with model.api_key_file).
go run ./cmd/mosaid-cli --config /path/config.json
go run ./cmd/mosaid-cli --config /path/config.json --workspace ~/notes

# 3. Single message, useful in scripts.
go run ./cmd/mosaid-cli --config /path/config.json --message "/tools"
```

`mosaid-cli` acquires the same singleton lock as the daemon, so pass
`--data-dir` if the phone daemon is running and you want isolated state.

## Threat notes

* The model is untrusted. It can only name registered tools; unknown names
  become observations, not executions. Arguments must be a JSON object.
* Tool output is untrusted data. The system prompt tells the model to ignore
  instructions inside files, and the runtime never parses tool output as policy.
* Path traversal, symlink escapes, secret-like filenames and workspace escapes
  are rejected by `tools.Workspace.path` before any filesystem call; the loop
  surfaces those errors to the model as failures.
* `process.run` is **not** registered by `cmd/mosaid`: the gateway drains its
  inbox serially, so a five-minute child process would stall every other update.
  Wire it only behind a worker queue.
* Approval previews are bounded to 400 bytes, so a huge file body cannot flood
  the chat.

## CI state

* `gofmt`, `go vet`, unit tests, race tests and the secret scan pass in CI for
  this branch.
* `staticcheck` findings from the first push were fixed: a duration that was
  always zero, an S1016 struct conversion, and one unused test helper.
* `govulncheck` fails on symbols that predate this change (`research.Fetcher.Fetch`,
  `mcpclient.init`, `storage.RestoreBackup`). The same job fails on `main`, so it
  is a pre-existing stdlib advisory, not a regression here. Fixing it means
  changing the pinned Go version and therefore the phone-kit binary checksums —
  an owner decision.
* SBOM and license reproducibility could not run in the build sandbox because
  `proxy.golang.org` is blocked; `go.mod` and `go.sum` are untouched, so CI
  regenerates identical manifests.

## Known limitations

* One turn is processed at a time by the Telegram gateway; long tool calls delay
  other messages.
* Continuations are in-memory, not durable.
* No streaming: the owner waits for the full reply.
* `scheduler`, `skills`, `mcpclient`, `research`, `images`, `instagram`,
  `github` and `gitops` are still not wired into `cmd/mosaid` (≈7.3k lines,
  fully tested, unreachable at runtime).
