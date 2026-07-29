package providers

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// FallbackEntry represents one entry in a fallback chain.
type FallbackEntry struct {
	ProviderID string       `json:"provider_id"`
	Tier       ProviderTier `json:"tier"`
	When       string       `json:"when"`   // "always", "session_active", "emergency_only", etc.
	Action     string       `json:"action"` // "persist_and_stop" for the final entry
}

// FallbackChain defines the order of providers to try for a task type.
type FallbackChain struct {
	TaskType TaskType        `json:"task_type"`
	Entries  []FallbackEntry `json:"entries"`
	Action   string          `json:"action"` // "persist_and_stop" at the end
}

// Router selects the best provider for a request based on capabilities and fallback chains.
type Router struct {
	mu         sync.RWMutex
	registry   *Registry
	chains     map[TaskType][]FallbackEntry
	lastHealth map[string]Health
}

// NewRouter creates a new capability router.
func NewRouter(registry *Registry) *Router {
	return &Router{
		registry:   registry,
		chains:     make(map[TaskType][]FallbackEntry),
		lastHealth: make(map[string]Health),
	}
}

// SetChain configures the fallback chain for a task type.
func (r *Router) SetChain(taskType TaskType, entries []FallbackEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chains[taskType] = entries
}

// RouteResult contains the result of a routing decision.
type RouteResult struct {
	Provider Provider
	Tried    []string // IDs of providers tried (and failed)
}

// Route selects the best provider for the given task type and required capabilities.
// It tries providers in fallback chain order, skipping unhealthy or incapable ones.
func (r *Router) Route(ctx context.Context, taskType TaskType, required Capabilities) (RouteResult, error) {
	r.mu.RLock()
	chain, exists := r.chains[taskType]
	r.mu.RUnlock()

	if !exists || len(chain) == 0 {
		return RouteResult{}, fmt.Errorf("no fallback chain configured for task type %q", taskType)
	}

	var tried []string

	for _, entry := range chain {
		if entry.Action == "persist_and_stop" {
			break
		}

		p, err := r.registry.Get(entry.ProviderID)
		if err != nil {
			tried = append(tried, entry.ProviderID)
			continue
		}

		// Check tier is allowed
		policy := r.registry.Policy()
		if err := policy.CheckProviderAllowed(p.Tier()); err != nil {
			tried = append(tried, entry.ProviderID)
			continue
		}

		// Check health
		health := p.Health(ctx)
		r.mu.Lock()
		r.lastHealth[entry.ProviderID] = health
		r.mu.Unlock()

		if health.Status == HealthDown {
			tried = append(tried, entry.ProviderID)
			continue
		}

		// Check capabilities
		caps, err := p.Capabilities(ctx)
		if err != nil {
			tried = append(tried, entry.ProviderID)
			continue
		}
		if !caps.Has(required) {
			tried = append(tried, entry.ProviderID)
			continue
		}

		return RouteResult{Provider: p, Tried: tried}, nil
	}

	return RouteResult{Tried: tried}, fmt.Errorf("%w: tried %v", ErrProviderExhausted, tried)
}

// Generate routes and generates a response, trying fallback providers on failure.
func (r *Router) Generate(ctx context.Context, taskType TaskType, req Request) (Response, error) {
	required := Capabilities{
		Text: true,
	}
	if req.JSONMode {
		required.StructuredOutput = true
	}
	if len(req.Tools) > 0 {
		required.NativeToolCalling = true
	}

	result, err := r.Route(ctx, taskType, required)
	if err != nil {
		return Response{}, err
	}

	// Check cost before generating
	cost, err := result.Provider.EstimateCost(req)
	if err != nil {
		return Response{}, fmt.Errorf("cost estimation failed for %q: %w", result.Provider.ID(), err)
	}

	policy := r.registry.Policy()
	if err := policy.CheckCostAllowed(cost); err != nil {
		return Response{}, fmt.Errorf("cost check failed for %q: %w", result.Provider.ID(), err)
	}

	// Generate
	resp, err := result.Provider.Generate(ctx, req)
	if err != nil {
		// Try fallback: re-route excluding the failed provider
		return r.generateWithFallback(ctx, taskType, req, append(result.Tried, result.Provider.ID()))
	}

	return resp, nil
}

// generateWithFallback tries the next provider in the chain.
func (r *Router) generateWithFallback(ctx context.Context, taskType TaskType, req Request, tried []string) (Response, error) {
	required := Capabilities{Text: true}
	if req.JSONMode {
		required.StructuredOutput = true
	}
	if len(req.Tools) > 0 {
		required.NativeToolCalling = true
	}

	r.mu.RLock()
	chain, exists := r.chains[taskType]
	r.mu.RUnlock()

	if !exists {
		return Response{}, ErrProviderExhausted
	}

	triedSet := make(map[string]bool, len(tried))
	for _, id := range tried {
		triedSet[id] = true
	}

	policy := r.registry.Policy()

	for _, entry := range chain {
		if entry.Action == "persist_and_stop" {
			break
		}

		if triedSet[entry.ProviderID] {
			continue
		}

		p, err := r.registry.Get(entry.ProviderID)
		if err != nil {
			continue
		}

		if err := policy.CheckProviderAllowed(p.Tier()); err != nil {
			continue
		}

		health := p.Health(ctx)
		if health.Status == HealthDown {
			continue
		}

		caps, err := p.Capabilities(ctx)
		if err != nil || !caps.Has(required) {
			continue
		}

		cost, err := p.EstimateCost(req)
		if err != nil || !cost.IsZero() {
			if err := policy.CheckCostAllowed(cost); err != nil {
				continue
			}
		}

		resp, err := p.Generate(ctx, req)
		if err != nil {
			triedSet[entry.ProviderID] = true
			continue
		}

		return resp, nil
	}

	return Response{}, fmt.Errorf("%w: tried %v", ErrProviderExhausted, tried)
}

// HealthSnapshot returns the last known health of all providers.
func (r *Router) HealthSnapshot() map[string]Health {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]Health, len(r.lastHealth))
	for k, v := range r.lastHealth {
		result[k] = v
	}
	return result
}

// ProviderStatus returns status info for all registered providers.
type ProviderStatus struct {
	ID       string       `json:"id"`
	Tier     ProviderTier `json:"tier"`
	Health   Health       `json:"health"`
	LastSeen time.Time    `json:"last_seen,omitempty"`
}

// AllStatus returns the status of all registered providers.
func (r *Router) AllStatus(ctx context.Context) []ProviderStatus {
	providers := r.registry.List()
	result := make([]ProviderStatus, 0, len(providers))

	for _, p := range providers {
		health := p.Health(ctx)
		r.mu.Lock()
		r.lastHealth[p.ID()] = health
		r.mu.Unlock()

		result = append(result, ProviderStatus{
			ID:     p.ID(),
			Tier:   p.Tier(),
			Health: health,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}
