// Package mock implements a mock provider for CI testing.
//
// It covers all the scenarios required for testing the provider platform
// without making any real network calls.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers"
)

// Scenario defines the behavior of a mock provider.
type Scenario string

const (
	ScenarioSuccess               Scenario = "success"
	ScenarioStructuredJSON        Scenario = "structured_json"
	ScenarioToolCalls             Scenario = "tool_calls"
	ScenarioMalformedJSON         Scenario = "malformed_json"
	ScenarioTimeout               Scenario = "timeout"
	ScenarioRateLimit             Scenario = "rate_limit"
	ScenarioTemporaryFailure      Scenario = "temporary_failure"
	ScenarioPermanentFailure      Scenario = "permanent_failure"
	ScenarioContextOverflow       Scenario = "context_overflow"
	ScenarioNonzeroCostRejection  Scenario = "nonzero_cost_rejection"
	ScenarioKaggleSessionExpired  Scenario = "kaggle_session_expired"
	ScenarioG4FEndpointRotated    Scenario = "g4f_endpoint_rotated"
	ScenarioMLCSlowResponse       Scenario = "mlc_slow_response"
)

// Config holds mock provider configuration.
type Config struct {
	ID       string         `json:"id"`
	Tier     providers.ProviderTier `json:"tier"`
	Scenario Scenario       `json:"scenario"`
	Caps     providers.Capabilities `json:"caps"`
}

// Provider implements providers.Provider for testing.
type Provider struct {
	cfg      Config
	callCount int
}

// New creates a new mock provider.
func New(cfg Config) *Provider {
	if cfg.Tier == "" {
		cfg.Tier = providers.TierVerifiedFree
	}
	if cfg.Caps.MaxContextTokens == 0 {
		cfg.Caps.MaxContextTokens = 32768
	}
	if cfg.Caps.ArabicQuality == "" {
		cfg.Caps.ArabicQuality = providers.QualityHigh
	}
	cfg.Caps.Text = true
	return &Provider{cfg: cfg}
}

func (p *Provider) ID() string {
	return p.cfg.ID
}

func (p *Provider) Tier() providers.ProviderTier {
	return p.cfg.Tier
}

func (p *Provider) Capabilities(ctx context.Context) (providers.Capabilities, error) {
	return p.cfg.Caps, nil
}

func (p *Provider) Health(ctx context.Context) providers.Health {
	switch p.cfg.Scenario {
	case ScenarioPermanentFailure:
		return providers.Health{
			Status:    providers.HealthDown,
			LastCheck: time.Now().UTC(),
			LastError: "mock permanent failure",
		}
	case ScenarioKaggleSessionExpired:
		return providers.Health{
			Status:         providers.HealthDown,
			LastCheck:      time.Now().UTC(),
			LastError:      "kaggle session expired",
			SessionActive: false,
		}
	default:
		return providers.Health{
			Status:    providers.HealthUp,
			LastCheck: time.Now().UTC(),
		}
	}
}

func (p *Provider) Generate(ctx context.Context, req providers.Request) (providers.Response, error) {
	p.callCount++

	switch p.cfg.Scenario {
	case ScenarioSuccess:
		return providers.Response{
			Content:      "mock response",
			FinishReason: "stop",
			Model:        "mock-model",
			Provider:     p.cfg.ID,
			Usage:        providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}, nil

	case ScenarioStructuredJSON:
		return providers.Response{
			Content:      `{"result": "mock structured output"}`,
			FinishReason: "stop",
			Model:        "mock-model",
			Provider:     p.cfg.ID,
			Usage:        providers.Usage{PromptTokens: 10, CompletionTokens: 8, TotalTokens: 18},
		}, nil

	case ScenarioToolCalls:
		return providers.Response{
			Content:      "",
			FinishReason: "tool_calls",
			Model:        "mock-model",
			Provider:     p.cfg.ID,
			ToolCalls: []providers.ToolCall{
				{
					ID:        "call_mock_1",
					Name:      "mock_tool",
					Arguments: `{"param": "value"}`,
				},
			},
			Usage: providers.Usage{PromptTokens: 10, CompletionTokens: 12, TotalTokens: 22},
		}, nil

	case ScenarioMalformedJSON:
		return providers.Response{}, fmt.Errorf("%w: invalid JSON in response", providers.ErrMalformedResponse)

	case ScenarioTimeout:
		select {
		case <-ctx.Done():
			return providers.Response{}, ctx.Err()
		case <-time.After(10 * time.Second):
			return providers.Response{}, fmt.Errorf("mock timeout exceeded")
		}

	case ScenarioRateLimit:
		return providers.Response{}, fmt.Errorf("%w: rate limited", providers.ErrRateLimited)

	case ScenarioTemporaryFailure:
		if p.callCount <= 2 {
			return providers.Response{}, fmt.Errorf("temporary failure (attempt %d)", p.callCount)
		}
		return providers.Response{
			Content:      "mock response after retry",
			FinishReason: "stop",
			Model:        "mock-model",
			Provider:     p.cfg.ID,
			Usage:        providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}, nil

	case ScenarioPermanentFailure:
		return providers.Response{}, fmt.Errorf("permanent failure")

	case ScenarioContextOverflow:
		return providers.Response{}, fmt.Errorf("%w: request exceeds context limit", providers.ErrContextOverflow)

	case ScenarioNonzeroCostRejection:
		return providers.Response{}, fmt.Errorf("%w: cost > 0", providers.ErrCostNotZero)

	case ScenarioKaggleSessionExpired:
		return providers.Response{}, fmt.Errorf("%w: kaggle session expired", providers.ErrSessionExpired)

	case ScenarioG4FEndpointRotated:
		return providers.Response{}, fmt.Errorf("g4f endpoint rotated, need new URL")

	case ScenarioMLCSlowResponse:
		time.Sleep(5 * time.Second)
		return providers.Response{
			Content:      "slow mock response",
			FinishReason: "stop",
			Model:        "mock-model-local",
			Provider:     p.cfg.ID,
			Usage:        providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}, nil

	default:
		return providers.Response{
			Content:      "mock response",
			FinishReason: "stop",
			Model:        "mock-model",
			Provider:     p.cfg.ID,
			Usage:        providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}, nil
	}
}

func (p *Provider) EstimateCost(req providers.Request) (providers.CostEstimate, error) {
	switch p.cfg.Scenario {
	case ScenarioNonzeroCostRejection:
		return providers.CostEstimate{
			Amount:   0.01,
			Currency: "USD",
			Notes:    "mock nonzero cost",
		}, nil
	default:
		return providers.CostEstimate{
			Amount:   0,
			Currency: "USD",
			Notes:    "mock free",
		}, nil
	}
}

// CallCount returns the number of times Generate was called.
func (p *Provider) CallCount() int {
	return p.callCount
}

// MarshalJSON implements json.Marshaler for inspection in tests.
func (p *Provider) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID        string         `json:"id"`
		Tier      providers.ProviderTier `json:"tier"`
		Scenario  Scenario       `json:"scenario"`
		CallCount int            `json:"call_count"`
	}{
		ID:        p.cfg.ID,
		Tier:      p.cfg.Tier,
		Scenario:  p.cfg.Scenario,
		CallCount: p.callCount,
	})
}
