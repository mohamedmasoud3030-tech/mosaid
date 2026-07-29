# Adding a New Provider to Mosaid

**Date:** 2026-07-29  
**Status:** Accepted

---

## Checklist

### Step 1: Classify the Provider

- [ ] Determine the provider tier:
  - `verified_free` — Confirmed free by ToS, no credit card
  - `local_self_hosted` — Runs on user's machine or tunnel
  - `free_with_card` — Requires credit card ❌ (not allowed)
  - `temporary_credits` — Free trial ❌ (not allowed)
  - `paid_only` — Always costs money ❌ (blocked)
  - `unknown` — Cost unknown ❌ (blocked)

- [ ] If tier is NOT `verified_free` or `local_self_hosted`, STOP. It cannot be added.

### Step 2: Implement the Provider

Create `internal/model/providers/<name>/provider.go`:

```go
package <name>

import (
    "context"
    "net/http"
    "github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers"
)

type Provider struct {
    id         string
    baseURL    string
    modelID    string
    httpClient *http.Client
}

func New(cfg Config) *Provider { ... }

func (p *Provider) ID() string { return p.id }

func (p *Provider) Tier() providers.ProviderTier {
    return providers.TierVerifiedFree // or TierLocalSelfHosted
}

func (p *Provider) Capabilities(ctx context.Context) (providers.Capabilities, error) {
    return providers.Capabilities{
        Text:             true,
        MaxContextTokens: 32768,
        ArabicQuality:    providers.QualityHigh,
        // ... fill in actual capabilities
    }, nil
}

func (p *Provider) Health(ctx context.Context) providers.Health {
    // Check if the provider is reachable
    return providers.Health{Status: providers.HealthUp}
}

func (p *Provider) Generate(ctx context.Context, req providers.Request) (providers.Response, error) {
    // Make the actual API call
    // Return structured response
}

func (p *Provider) EstimateCost(req providers.Request) (providers.CostEstimate, error) {
    return providers.CostEstimate{Amount: 0, Currency: "USD"}, nil
}
```

### Step 3: Write Tests

Create `internal/model/providers/<name>/provider_test.go`:

- [ ] Test `Tier()` returns correct tier
- [ ] Test `Capabilities()` returns valid capabilities
- [ ] Test `EstimateCost()` returns 0
- [ ] Test `Health()` returns valid health
- [ ] Test `Generate()` with mock HTTP server
- [ ] Test error handling (timeout, bad response, etc.)
- [ ] Test with race detector: `go test -race`

### Step 4: Register in Router

Update `internal/model/providers/router.go`:

```go
// In NewRouter or similar initialization:
router.Register(<name>.New(cfg))
```

### Step 5: Add to Fallback Chain

Update `config/providers.yaml` (or equivalent):

```yaml
fallback_chain:
  planning_and_reasoning:
    - provider: <name>
      tier: verified_free  # or local_self_hosted
      when: "always"
```

### Step 6: Add CI Tests

- [ ] Add mock scenario in `internal/model/providers/mock/`
- [ ] Add tier classification test
- [ ] Add to CI workflow

### Step 7: Document

- [ ] Create `docs/providers/<NAME>.md`
- [ ] Include: setup instructions, configuration, limitations, warnings
- [ ] Update `docs/roadmap/IMPLEMENTATION-STATUS.md`

---

## Anti-Patterns (NEVER do these)

- ❌ Hard-code model names in the cognitive engine
- ❌ Add a paid provider to the fallback chain
- ❌ Allow the provider to auto-charge a credit card
- ❌ Skip the cost estimation check
- ❌ Add a provider without tests
- ❌ Store API keys in Git (use secrets files)
