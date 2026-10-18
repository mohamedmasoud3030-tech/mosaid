# Technical Decisions

Record of technical decisions taken for Mosaid, with alternatives considered
and rationale. Decisions are made by the Arena orchestrator under
owner-authorized direct execution; no specialist-model claims are made.

## D-01 — Free-only model provider: Google Gemini free tier (2026-08-19)
- Context: the owner requires a free-only model policy.
- Alternatives: OpenAI/Anthropic (one-time credits, require billing),
  Groq/Mistral/OpenRouter free tiers.
- Decision: Gemini free tier through the official OpenAI-compatible endpoint
  `https://generativelanguage.googleapis.com/v1beta/openai`, default model
  `gemini-2.5-flash`; no credit card, ongoing free tier, official endpoint
  documentation. Flood limits mirrored to the free tier (10 msg/min).
- Reversibility: full — base URL/model/key are installer prompts.

## D-02 — Cost control design (2026-08-19)
- Context: `security.Budget.UseCost` existed but was never called, so
  `max_cost_usd` could not stop spending (owner "money" risk).
- Alternatives: (a) cumulative ledger with persistence; (b) per-request
  estimated cost against the cap; (c) remove the field.
- Decision: (b) now — `PriceLimits{InputPer1M, OutputPer1M}` config fields
  (0 = free tier, no cost accrues), token estimate `chars/4`,
  `TokenCostUSD(tokens, pricePer1M)`, consumed per request before model call
  and after the reply; trip is fail-closed. Budget-exceeded requests get a
  visible Telegram reply instead of silent retries. (a) remains planned as
  M4 (cumulative ledger) — smallest correct step first, keeping the phone's
  `max_cost_usd=0.01` tripwire meaningful.
- Reversibility: full — prices default to 0; removing the fields restores
  prior behavior.

## D-03 — Android DNS: proot bind (2026-08-19)
- Context: the stock-Go binary read `/etc/resolv.conf` (→ `/system/etc`,
  unwritable) and fell back to `localhost:53`; `[::1]:53` refused on device.
- Alternatives: (a) local DNS server on port 53 (needs root); (b) patching
  the binary to read `$PREFIX/etc/resolv.conf` (fork of stdlib behavior,
  maintenance burden); (c) termux-golang rebuild of the toolchain (changes
  the build identity, not available here); (d) proot bind.
- Decision: (d) — supervisor and foreground runner exec
  `proot -b $PREFIX/etc/resolv.conf:/etc/resolv.conf` on Termux, plus a
  Termux-only DNS guard normalizing `$PREFIX/etc/resolv.conf` to
  `1.1.1.1`/`8.8.8.8` when no IPv4 nameserver is configured. Documented fix
  for stock-Go-on-Android, no root, no binary change.
- Reversibility: full — remove proot and the launch falls back to direct
  exec.

## D-04 — Supervisor TERM contract (2026-08-19)
- Context: on-device logs showed `sv restart` orphaning the agent child; the
  child held the singleton lock while a new supervisor cycled on exit 73.
  Root cause: the TERM trap cleaned up but did not exit, so runit
  force-killed the supervisor after its timeout.
- Decision: TERM/INT/HUP → graceful child shutdown + cleanup + `exit 0`;
  EXIT trap performs cleanup only (preserves natural exit codes). A
  dedicated regression test (`selftest-supervisor.sh`) runs a fake agent and
  asserts exit-0, child termination with TERM delivered, and state cleanup.
- Reversibility: full (script-level change).

## D-05 — Kit checksum format (2026-08-19)
- Context: `sha256sum -c` failed on the phone because the committed
  `.sha256` contained the build machine's absolute path.
- Decision: checksum files always use the bare relative filename; the kit
  builder asserts this; the self-test verifies from a clean directory.

## D-06 — Toolchain and CI pin (2026-08-19)
- Context: the Go vulnerability DB gained stdlib entries affecting 1.25.12
  (fixed in 1.25.13); CI's setup-go pins `GOTOOLCHAIN=local`, so the
  `toolchain` directive cannot switch CI.
- Decision: keep `go 1.25.12` + `toolchain go1.25.13` in `go.mod` (local
  builds use 1.25.13) and stage the one-line CI fix (`go-version: '1.25.13'`)
  plus a `phone-kit` job in `docs/handoff/activation/`, pending an
  owner-approved credential with Workflows write (GitHub correctly rejects
  the App credential for workflow edits). Phase 0 preservation CI gets the
  same bump plus a pinned `expected-govulncheck.txt` measured by a temporary
  re-pin workflow.
- Reversibility: full — revert the staged files.

## D-07 — Verified local module staging (sandbox policy) (2026-08-19)
- Context: the sandbox blocks all non-GitHub egress, so the module proxy is
  unreachable; but verification must not weaken checksum discipline.
- Decision: stage module trees locally only where the computed module zip
  hash equals the `go.sum` entry exactly; use compile-time-only stand-ins
  for tool modules never imported by the build graph; run everything against
  a scratch `-modfile` so the repo's `go.mod`/`go.sum` stay untouched.
  `GOPROXY=direct` downloads still verify against the committed go.sum.
- Reversibility: sandbox-local only; nothing committed.

## D-08 — Phone-kit template stability (2026-08-19)
- Context: optional cost-pricing fields were added to the config schema.
- Decision: the phone-kit template does not carry the new fields (zero is
  the free-tier default); only `config/mosaid.example.json` documents them.
  Avoids kit churn on the owner's in-flight device and keeps installed
  configs valid (missing fields are zero values).

## D-09 — Budget-exceeded visibility (2026-08-19)
- Context: budget trips were silent (handler error → retry → dead letter).
- Decision: the gateway completes budget-tripped inbox items with a visible
  "Request rejected: execution budget exceeded." reply (no retry loop);
  other handler errors keep the retry/dead-letter path.
