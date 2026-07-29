# Mosaid — Phase 16 Acceptance Criteria

**Date:** 2026-07-29  
**Status:** Accepted

---

## Phase 14: Model Provider Platform

### Must Have
- [x] `Provider` interface with `ID()`, `Tier()`, `Capabilities()`, `Health()`, `Generate()`, `EstimateCost()`
- [x] `ProviderTier` enum with all 6 tiers
- [x] `Capabilities` struct (text, vision, coding, long context, Arabic quality, etc.)
- [x] `CostEstimate` type (must be 0 for all free providers)
- [x] OpenAI-compatible adapter (works for Groq, Cerebras, Google AI Studio, Kaggle)
- [x] Kaggle Tunnel provider
- [x] G4F local provider
- [x] MLC LLM local provider
- [x] Mock provider for CI (all scenarios)
- [x] Provider Registry with tier validation
- [x] Free-only Policy Engine (fail-closed)
- [x] Capability Router with fallback chain
- [x] Provider tier classification tests
- [x] Zero-cost policy tests

### Must NOT Have
- ❌ Hard-coded model names in cognitive engine
- ❌ Paid provider in fallback chain
- ❌ Real API calls in CI

---

## Phase 15: Cognitive Engine

### Must Have
- [x] Goal, Run, Evidence, RunState types
- [x] Execution loop: UNDERSTAND → PLAN → ACT → VERIFY → COMPLETE
- [x] Loop guard (max steps, max repeats, max no-progress)
- [x] Crash resume via SQLite persistence
- [x] Idempotency key support
- [x] Budget enforcement (steps, tokens, tools, retries)
- [x] Approval integration for sensitive steps
- [x] Memory context builder

### Must NOT Have
- ❌ Hard-coded model or provider references
- ❌ Auto-execution without approval for high-risk actions
- ❌ Infinite loops or unbounded retries

---

## Phase 16: Skills Platform

### Must Have
- [x] `agent_compatible` field on Manifest (optional, backward-compatible)
- [x] `workpack_ref` field on Manifest (optional, backward-compatible)
- [x] All existing skills continue to work
- [x] Prompt Registry with metadata (id, version, purpose, integrity, capabilities)
- [x] System prompts cannot be overridden by external content

---

## Phase 17: Digital Work Intelligence

### Must Have (6 Work Packs)
- [x] Virtual Assistant (intake + workflow + tools + quality + delivery + tests)
- [x] Research & Information
- [x] Content Writing
- [x] Coding Assistance
- [x] Social Media Management
- [x] Spreadsheets & Reporting

### Each Work Pack Must Have
```
intake + workflow + tools + quality_checks +
delivery + evidence + tests + beginner_path
```

---

## Phase 18: Benchmark Harness

### Must Have
- [x] Mock-based benchmark suite (18 scenarios)
- [x] Arabic understanding test
- [x] Provider fallback simulation (Kaggle expired, G4F rotated, MLC slow)
- [x] Zero-cost policy enforcement test
- [x] Benchmark results schema

---

## CI Gates

All of these must pass:
```
gofmt, go mod verify, go test ./...,
go test -race ./..., go vet ./...,
staticcheck, govulncheck,
linux_build, android_arm64_build,
secret_scan, sbom, license_report,
clean_tree, zero_cost_policy_test,
provider_tier_classification_test
```
