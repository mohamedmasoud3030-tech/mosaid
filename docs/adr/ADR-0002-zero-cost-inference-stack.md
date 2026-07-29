# ADR-0002: Zero-Cost Inference Stack

**Date:** 2026-07-29  
**Status:** Accepted  
**Decision Makers:** Mosaid Architecture Team

---

## Context

Mosaid needs LLM inference for its cognitive engine. The target users are beginner Arabic-speaking freelancers who cannot afford paid API subscriptions. The system must work with $0 inference cost.

---

## Decision

We adopt a three-tier zero-cost inference stack:

### Tier 1: Kaggle GPU Tunnel (Primary)
- Free GPU access (30h/week) via Kaggle notebooks
- vLLM server exposed through Cloudflared tunnel
- Supports 70B+ parameter models (Qwen3-72B)
- Provider tier: `local_self_hosted`

### Tier 2: Free API Providers (Always-On)
- Groq free tier (Qwen3-32B, Llama 70B)
- Google AI Studio free tier (Gemini 2.5 Flash)
- Cerebras free tier (Llama 4 Scout)
- Provider tier: `verified_free`

### Tier 3: Local Inference (Emergency)
- G4F (GPT4Free) — reverse-engineered website APIs
- MLC LLM — on-device inference via WebGPU
- Provider tier: `local_self_hosted`

---

## Consequences

### Positive
- Zero cost for all users
- No credit card required anywhere
- Multiple fallback options
- Works offline (MLC)

### Negative
- Kaggle sessions expire after ~9 hours
- Free API tiers may change terms
- G4F may violate some ToS (documented and warned)
- Local models are smaller (3-4B vs 70B+)

### Mitigations
- Automatic fallback chain handles provider failures
- Users can use multiple Kaggle accounts
- G4F is classified as "emergency only" with warnings
- MLC is last resort with documented limitations

---

## Alternatives Considered

1. **Paid APIs with budget cap:** Rejected — violates zero-cost principle
2. **Self-hosted GPU server:** Rejected — requires hardware investment
3. **Only free API tiers:** Rejected — too fragile if one provider changes terms
4. **Only Kaggle:** Rejected — session limits make it unreliable as sole provider

---

## Related

- [ZERO-COST-POLICY.md](../architecture/ZERO-COST-POLICY.md)
- [MODEL-PROVIDER-PLATFORM.md](../architecture/MODEL-PROVIDER-PLATFORM.md)
