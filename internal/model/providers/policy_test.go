package providers_test

import (
	"testing"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers"
)

func TestDefaultFreeOnlyPolicy(t *testing.T) {
	p := providers.DefaultFreeOnlyPolicy()
	if p.Mode != providers.BillingModeFreeOnly {
		t.Errorf("mode = %q, want free_only", p.Mode)
	}
	if p.MaxSpendUSD != 0 {
		t.Errorf("max_spend_usd = %f, want 0", p.MaxSpendUSD)
	}
	if p.AllowPaidFallback {
		t.Error("allow_paid_fallback should be false")
	}
	if p.UnknownCostPolicy != "deny" {
		t.Errorf("unknown_cost_policy = %q, want deny", p.UnknownCostPolicy)
	}
}

func TestBillingPolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		policy  providers.BillingPolicy
		wantErr bool
	}{
		{
			"valid free only",
			providers.BillingPolicy{Mode: providers.BillingModeFreeOnly, MaxSpendUSD: 0, AllowPaidFallback: false, UnknownCostPolicy: "deny"},
			false,
		},
		{
			"free only with non-zero spend",
			providers.BillingPolicy{Mode: providers.BillingModeFreeOnly, MaxSpendUSD: 1, AllowPaidFallback: false, UnknownCostPolicy: "deny"},
			true,
		},
		{
			"free only with paid fallback",
			providers.BillingPolicy{Mode: providers.BillingModeFreeOnly, MaxSpendUSD: 0, AllowPaidFallback: true, UnknownCostPolicy: "deny"},
			true,
		},
		{
			"negative spend",
			providers.BillingPolicy{Mode: providers.BillingModePaidOK, MaxSpendUSD: -1, AllowPaidFallback: true, UnknownCostPolicy: "deny"},
			true,
		},
		{
			"invalid unknown policy",
			providers.BillingPolicy{Mode: providers.BillingModePaidOK, MaxSpendUSD: 10, AllowPaidFallback: true, UnknownCostPolicy: "maybe"},
			true,
		},
		{
			"valid paid mode",
			providers.BillingPolicy{Mode: providers.BillingModePaidOK, MaxSpendUSD: 10, AllowPaidFallback: true, UnknownCostPolicy: "allow"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckProviderAllowed(t *testing.T) {
	freePolicy := providers.DefaultFreeOnlyPolicy()

	tests := []struct {
		tier    providers.ProviderTier
		wantErr bool
	}{
		{providers.TierVerifiedFree, false},
		{providers.TierLocalSelfHosted, false},
		{providers.TierFreeWithCard, true},
		{providers.TierTemporaryCredits, true},
		{providers.TierPaidOnly, true},
		{providers.TierUnknown, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			err := freePolicy.CheckProviderAllowed(tt.tier)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckProviderAllowed(%q) error = %v, wantErr %v", tt.tier, err, tt.wantErr)
			}
		})
	}
}

func TestCheckCostAllowed(t *testing.T) {
	freePolicy := providers.DefaultFreeOnlyPolicy()

	tests := []struct {
		name    string
		cost    providers.CostEstimate
		wantErr bool
	}{
		{"zero cost", providers.CostEstimate{Amount: 0, Currency: "USD"}, false},
		{"nonzero cost", providers.CostEstimate{Amount: 0.01, Currency: "USD"}, true},
		{"large cost", providers.CostEstimate{Amount: 100, Currency: "USD"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := freePolicy.CheckCostAllowed(tt.cost)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckCostAllowed(%+v) error = %v, wantErr %v", tt.cost, err, tt.wantErr)
			}
		})
	}
}

func TestCheckUnknownCost(t *testing.T) {
	denyPolicy := providers.BillingPolicy{UnknownCostPolicy: "deny"}
	allowPolicy := providers.BillingPolicy{UnknownCostPolicy: "allow"}

	if err := denyPolicy.CheckUnknownCost(); err == nil {
		t.Error("deny policy should reject unknown cost")
	}
	if err := allowPolicy.CheckUnknownCost(); err != nil {
		t.Errorf("allow policy should accept unknown cost: %v", err)
	}
}
