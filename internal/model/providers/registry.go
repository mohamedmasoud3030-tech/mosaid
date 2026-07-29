package providers

import (
	"fmt"
	"sync"
)

// Registry manages the collection of registered providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	policy    BillingPolicy
}

// NewRegistry creates a new provider registry with the given billing policy.
func NewRegistry(policy BillingPolicy) (*Registry, error) {
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("invalid billing policy: %w", err)
	}
	return &Registry{
		providers: make(map[string]Provider),
		policy:    policy,
	}, nil
}

// Register adds a provider to the registry.
// It validates that the provider's tier is allowed under the current billing policy.
func (r *Registry) Register(p Provider) error {
	if p == nil {
		return fmt.Errorf("provider cannot be nil")
	}
	id := p.ID()
	if id == "" {
		return fmt.Errorf("provider ID cannot be empty")
	}

	tier := p.Tier()
	if err := r.policy.CheckProviderAllowed(tier); err != nil {
		return fmt.Errorf("cannot register provider %q: %w", id, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("provider %q already registered", id)
	}

	r.providers[id] = p
	return nil
}

// Get returns a provider by ID.
func (r *Registry) Get(id string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.providers[id]
	if !exists {
		return nil, fmt.Errorf("%w: %q", ErrProviderNotFound, id)
	}
	return p, nil
}

// List returns all registered providers.
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

// ListByTier returns providers matching the given tier.
func (r *Registry) ListByTier(tier ProviderTier) []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Provider
	for _, p := range r.providers {
		if p.Tier() == tier {
			result = append(result, p)
		}
	}
	return result
}

// Policy returns the current billing policy.
func (r *Registry) Policy() BillingPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.policy
}

// Count returns the number of registered providers.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}
