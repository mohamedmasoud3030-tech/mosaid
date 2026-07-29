# Mosaid — Zero-Cost Policy

**Date:** 2026-07-29  
**Status:** Accepted  
**Phase:** 14

---

## Policy Statement

**Mosaid MUST NEVER spend money on AI inference.** This is a hard constraint, not a goal. The system fails closed if any cost would be incurred.

---

## Enforcement Points

### 1. Configuration Level

```yaml
billing:
  mode: free_only
  max_spend_usd: 0
  allow_paid_fallback: false
  unknown_cost_policy: deny
```

If `max_spend_usd > 0` AND `mode == free_only`, the system refuses to start.

### 2. Provider Registry Level

Every provider is classified into a tier at registration:

| Tier | Allowed in `free_only` mode? |
|---|---|
| `verified_free` | ✅ Yes |
| `local_self_hosted` | ✅ Yes |
| `free_with_card` | ❌ No |
| `temporary_credits` | ❌ No |
| `paid_only` | ❌ No (blocked at registration) |
| `unknown` | ❌ No (fail-closed) |

### 3. Router Level

Before using any provider:

1. Check provider tier is allowed
2. Call `EstimateCost()` — must return 0
3. If cost > 0 → reject, try next provider
4. If all providers exhausted → persist state and stop (do not try paid)

### 4. Budget Level

```go
const MaxSpendUSD = 0.0

func (r *Router) routeWithPolicy(ctx, req) (Response, error) {
    for _, provider := range r.fallbackChain {
        cost, err := provider.EstimateCost(req)
        if err != nil || cost.Amount > 0 {
            continue // skip this provider
        }
        resp, err := provider.Generate(ctx, req)
        if err == nil {
            return resp, nil
        }
    }
    return ErrAllProvidersExhausted
}
```

---

## The Three-Tier Zero-Cost Stack

### Tier 1: Kaggle GPU Tunnel (Primary — Planning & 70B+ models)

- **What:** Free Kaggle GPU (T4 dual = 32GB VRAM) running vLLM with Qwen3-72B
- **How:** User runs a Kaggle Notebook, gets a Cloudflared tunnel URL
- **Cost:** $0 (30 hours/week free, no credit card)
- **Provider tier:** `local_self_hosted`
- **Limitation:** Session ends after ~9 hours; user must restart

### Tier 2: Free API Providers (Always-On — Groq, Google AI Studio, Cerebras)

- **What:** Free tiers of cloud AI services
- **How:** API keys with free quota (Groq: generous, Google: generous, Cerebras: generous)
- **Cost:** $0 (verified free tiers)
- **Provider tier:** `verified_free`
- **Limitation:** Rate limits, may change terms

### Tier 3: Local Inference (Emergency — G4F, MLC LLM)

- **G4F:** Reverse-engineers free website APIs into a local OpenAI-compatible server
  - **Cost:** $0 (local self-hosted)
  - **Provider tier:** `local_self_hosted`
  - **Warning:** May violate some sites' ToS. Use for personal/development only.
  
- **MLC LLM:** Runs models directly on Android GPU via WebGPU
  - **Cost:** $0 (fully offline)
  - **Provider tier:** `local_self_hosted`
  - **Limitation:** Smaller models (3-4B), slower on older phones

---

## Fail-Closed Behaviors

The system fails closed (stops and saves state) in ALL these cases:

1. `max_spend_usd > 0` when `mode == free_only`
2. Provider is classified `paid_only`
3. `EstimateCost()` returns amount > 0
4. Router tries to use a paid provider
5. Cost classification is `unknown`
6. All free + local providers exhausted

**The system NEVER auto-switches to a paid provider.** The user must explicitly change the billing mode.

---

## Monitoring

The `/providers` command shows:
- Each provider's tier
- Current health status
- Last successful request
- Kaggle session status (active/expired)
- G4F server status (running/stopped)
