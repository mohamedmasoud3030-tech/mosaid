package providers

import (
	"errors"
	"fmt"
)

// BillingMode represents the billing policy.
type BillingMode string

const (
	BillingModeFreeOnly BillingMode = "free_only"
	BillingModePaidOK   BillingMode = "paid_ok"
)

// BillingPolicy enforces the zero-cost constraint.
type BillingPolicy struct {
	Mode              BillingMode `json:"mode"`
	MaxSpendUSD       float64     `json:"max_spend_usd"`
	AllowPaidFallback bool        `json:"allow_paid_fallback"`
	UnknownCostPolicy string      `json:"unknown_cost_policy"` // "deny" or "allow"
}

// Validate checks the policy is internally consistent.
func (p BillingPolicy) Validate() error {
	if p.Mode == BillingModeFreeOnly {
		if p.MaxSpendUSD != 0 {
			return fmt.Errorf("max_spend_usd must be 0 in free_only mode, got %f", p.MaxSpendUSD)
		}
		if p.AllowPaidFallback {
			return errors.New("allow_paid_fallback must be false in free_only mode")
		}
	}
	if p.MaxSpendUSD < 0 {
		return errors.New("max_spend_usd must be non-negative")
	}
	if p.UnknownCostPolicy != "deny" && p.UnknownCostPolicy != "allow" {
		return fmt.Errorf("unknown_cost_policy must be 'deny' or 'allow', got %q", p.UnknownCostPolicy)
	}
	return nil
}

// DefaultFreeOnlyPolicy returns the strict zero-cost policy.
func DefaultFreeOnlyPolicy() BillingPolicy {
	return BillingPolicy{
		Mode:              BillingModeFreeOnly,
		MaxSpendUSD:       0,
		AllowPaidFallback: false,
		UnknownCostPolicy: "deny",
	}
}

// CheckProviderAllowed returns nil if the provider is allowed under this policy.
func (p BillingPolicy) CheckProviderAllowed(tier ProviderTier) error {
	if p.Mode == BillingModeFreeOnly {
		if !tier.AllowedInFreeMode() {
			return fmt.Errorf("%w: tier %q is not allowed in free_only mode", ErrTierNotAllowed, tier)
		}
	}
	return nil
}

// CheckCostAllowed returns nil if the estimated cost is allowed under this policy.
func (p BillingPolicy) CheckCostAllowed(estimate CostEstimate) error {
	if p.Mode == BillingModeFreeOnly {
		if estimate.Amount > 0 {
			return fmt.Errorf("%w: estimated cost %f %s", ErrCostNotZero, estimate.Amount, estimate.Currency)
		}
	} else {
		if estimate.Amount > p.MaxSpendUSD {
			return fmt.Errorf("estimated cost %f exceeds max %f", estimate.Amount, p.MaxSpendUSD)
		}
	}
	return nil
}

// CheckUnknownCost returns nil if an unknown cost is allowed.
func (p BillingPolicy) CheckUnknownCost() error {
	if p.UnknownCostPolicy == "deny" {
		return errors.New("unknown cost classification: denied by policy")
	}
	return nil
}
