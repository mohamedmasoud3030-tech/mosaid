# Mosaid — Model Provider Platform

**Date:** 2026-07-29  
**Status:** Accepted  
**Phase:** 14

---

## Overview

The Model Provider Platform is the abstraction layer between Mosaid's cognitive engine and the underlying LLM inference providers. It ensures:

1. **No provider-specific code** in the cognitive engine or planner or executor.
2. **Capability-based routing** — the engine requests capabilities (e.g., "Arabic", "long context", "coding"), and the router selects the best available provider.
3. **Automatic fallback** — if one provider fails, the next in the chain is tried.
4. **Zero-cost enforcement** — the router refuses to use any provider that would incur cost.

---

## Architecture Diagram

```
┌─────────────────────────────┐
│      Cognitive Engine       │
│  (Planner / Executor /      │
│   Verifier / Memory)        │
└──────────────┬──────────────┘
               │ Generate(ctx, Request)
               ▼
┌─────────────────────────────┐
│      Capability Router      │
│  - Reads task type           │
│  - Reads required caps       │
│  - Consults fallback chain   │
│  - Enforces free-only policy │
└──────────────┬──────────────┘
               │
    ┌──────────┼──────────┐──────────┐──────────┐
    ▼          ▼          ▼          ▼          ▼
┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐
│Kaggle │ │ Groq  │ │Google │ │Cerebr.│ │ G4F   │
│Tunnel │ │ Free  │ │AIS    │ │ Free  │ │ Local │
│(70B+) │ │(32B)  │ │(Flash)│ │(Scout)│ │(GPT4) │
└───────┘ └───────┘ └───────┘ └───────┘ └───────┘
```

---

## Provider Interface

Every provider implements this interface:

```go
type Provider interface {
    ID() string
    Tier() ProviderTier
    Capabilities(context.Context) (Capabilities, error)
    Health(context.Context) Health
    Generate(context.Context, Request) (Response, error)
    EstimateCost(Request) (CostEstimate, error)
}
```

The cognitive engine only ever sees `Provider` — never a concrete type.

---

## Provider Tiers

| Tier | Meaning | In Fallback Chain? |
|---|---|---|
| `verified_free` | Confirmed free by ToS (Groq, Google AI Studio, Cerebras) | ✅ Yes |
| `local_self_hosted` | Runs locally (Kaggle tunnel, G4F, MLC LLM) | ✅ Yes |
| `free_with_card` | Requires credit card for signup | ❌ No |
| `temporary_credits` | Free trial that expires | ❌ No |
| `paid_only` | Always costs money | ❌ Blocked |
| `unknown` | Cost classification pending | ❌ Blocked |

---

## Fallback Chain

The fallback chain is configured per task type in `providers.yaml`:

- **planning_and_reasoning:** Kaggle → Groq → Google AI Studio → Cerebras → G4F → MLC → stop
- **coding:** Groq → Google AI Studio → Kaggle → G4F → MLC → stop
- **vision:** Google AI Studio → G4F → stop
- **fast_tasks:** Cerebras → Groq → Cloudflare Workers AI → MLC → stop

The last entry is always `persist_and_stop` — never a paid provider.

---

## Adding a New Provider

See [docs/providers/ADDING-A-PROVIDER.md](../providers/ADDING-A-PROVIDER.md) for the full checklist.
