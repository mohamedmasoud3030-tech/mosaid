package providers_test

import (
	"testing"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers"
)

func TestProviderTierAllowedInFreeMode(t *testing.T) {
	tests := []struct {
		tier    providers.ProviderTier
		allowed bool
	}{
		{providers.TierVerifiedFree, true},
		{providers.TierLocalSelfHosted, true},
		{providers.TierFreeWithCard, false},
		{providers.TierTemporaryCredits, false},
		{providers.TierPaidOnly, false},
		{providers.TierUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			if got := tt.tier.AllowedInFreeMode(); got != tt.allowed {
				t.Errorf("ProviderTier(%q).AllowedInFreeMode() = %v, want %v", tt.tier, got, tt.allowed)
			}
		})
	}
}

func TestCapabilitiesHas(t *testing.T) {
	full := providers.Capabilities{
		Text:              true,
		StructuredOutput:  true,
		NativeToolCalling: true,
		Vision:            true,
		Coding:            true,
		LongContext:       true,
		ArabicQuality:     providers.QualityHigh,
		MaxContextTokens:  128000,
	}

	tests := []struct {
		name     string
		required providers.Capabilities
		want     bool
	}{
		{"text only", providers.Capabilities{Text: true}, true},
		{"text+coding", providers.Capabilities{Text: true, Coding: true}, true},
		{"vision", providers.Capabilities{Text: true, Vision: true}, true},
		{"structured output", providers.Capabilities{Text: true, StructuredOutput: true}, true},
		{"tool calling", providers.Capabilities{Text: true, NativeToolCalling: true}, true},
		{"long context", providers.Capabilities{Text: true, LongContext: true}, true},
		{"arabic high", providers.Capabilities{Text: true, ArabicQuality: providers.QualityHigh}, true},
		{"arabic medium", providers.Capabilities{Text: true, ArabicQuality: providers.QualityMedium}, true},
		{"arabic low", providers.Capabilities{Text: true, ArabicQuality: providers.QualityLow}, true},
		{"context tokens exact", providers.Capabilities{Text: true, MaxContextTokens: 128000}, true},
		{"context tokens less", providers.Capabilities{Text: true, MaxContextTokens: 32000}, true},
		{"context tokens more", providers.Capabilities{Text: true, MaxContextTokens: 256000}, false},
		{"missing text", providers.Capabilities{Text: false}, true},
		{"all caps", providers.Capabilities{Text: true, StructuredOutput: true, NativeToolCalling: true, Vision: true, Coding: true, LongContext: true, ArabicQuality: providers.QualityHigh, MaxContextTokens: 64000}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := full.Has(tt.required); got != tt.want {
				t.Errorf("Capabilities.Has(%+v) = %v, want %v", tt.required, got, tt.want)
			}
		})
	}
}

func TestCapabilitiesHasLimited(t *testing.T) {
	limited := providers.Capabilities{
		Text:             true,
		Coding:           true,
		ArabicQuality:    providers.QualityMedium,
		MaxContextTokens: 8192,
	}

	tests := []struct {
		name     string
		required providers.Capabilities
		want     bool
	}{
		{"text ok", providers.Capabilities{Text: true}, true},
		{"vision fail", providers.Capabilities{Text: true, Vision: true}, false},
		{"structured fail", providers.Capabilities{Text: true, StructuredOutput: true}, false},
		{"arabic high fail", providers.Capabilities{Text: true, ArabicQuality: providers.QualityHigh}, false},
		{"arabic medium ok", providers.Capabilities{Text: true, ArabicQuality: providers.QualityMedium}, true},
		{"context too large", providers.Capabilities{Text: true, MaxContextTokens: 32000}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := limited.Has(tt.required); got != tt.want {
				t.Errorf("limited.Has(%+v) = %v, want %v", tt.required, got, tt.want)
			}
		})
	}
}

func TestCostEstimateIsZero(t *testing.T) {
	tests := []struct {
		amount float64
		want   bool
	}{
		{0, true},
		{0.001, false},
		{1.0, false},
	}

	for _, tt := range tests {
		cost := providers.CostEstimate{Amount: tt.amount, Currency: "USD"}
		if got := cost.IsZero(); got != tt.want {
			t.Errorf("CostEstimate{Amount: %f}.IsZero() = %v, want %v", tt.amount, got, tt.want)
		}
	}
}
