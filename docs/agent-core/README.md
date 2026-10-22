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
