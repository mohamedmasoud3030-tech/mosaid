// Package providers defines the abstraction layer between Mosaid's cognitive
// engine and the underlying LLM inference providers.
//
// Every provider implements the Provider interface. The cognitive engine only
// ever interacts with Provider — never a concrete type. This ensures
// provider-agnostic design and easy testing.
package providers

import (
	"context"
	"errors"
	"time"
)

// Common errors returned by the provider platform.
var (
	ErrProviderNotFound      = errors.New("provider not found")
	ErrProviderUnavailable   = errors.New("provider unavailable")
	ErrProviderExhausted     = errors.New("all providers exhausted")
	ErrCostNotZero           = errors.New("provider estimated non-zero cost")
	ErrTierNotAllowed        = errors.New("provider tier not allowed in current billing mode")
	ErrCapabilityMissing     = errors.New("provider lacks required capability")
	ErrRequestTooLarge       = errors.New("request exceeds provider context limit")
	ErrInvalidRequest        = errors.New("invalid request")
	ErrSessionExpired        = errors.New("provider session expired")
	ErrRateLimited           = errors.New("provider rate limited")
	ErrMalformedResponse     = errors.New("malformed provider response")
	ErrContextOverflow       = errors.New("context window overflow")
)

// ProviderTier classifies the cost model of a provider.
type ProviderTier string

const (
	// TierVerifiedFree is confirmed free by ToS, no credit card required.
	TierVerifiedFree ProviderTier = "verified_free"
	// TierLocalSelfHosted runs locally (Kaggle tunnel, G4F, MLC LLM).
	TierLocalSelfHosted ProviderTier = "local_self_hosted"
	// TierFreeWithCard requires a credit card for signup. NOT allowed.
	TierFreeWithCard ProviderTier = "free_with_card"
	// TierTemporaryCredits is a free trial that expires. NOT allowed.
	TierTemporaryCredits ProviderTier = "temporary_credits"
	// TierPaidOnly always costs money. BLOCKED.
	TierPaidOnly ProviderTier = "paid_only"
	// TierUnknown has unknown cost classification. BLOCKED (fail-closed).
	TierUnknown ProviderTier = "unknown"
)

// AllowedInFreeMode returns true if this tier can be used in free_only billing mode.
func (t ProviderTier) AllowedInFreeMode() bool {
	return t == TierVerifiedFree || t == TierLocalSelfHosted
}

// QualityLevel describes the quality of a capability.
type QualityLevel string

const (
	QualityHigh   QualityLevel = "high"
	QualityMedium QualityLevel = "medium"
	QualityLow    QualityLevel = "low"
	QualityNone   QualityLevel = "none"
)

// Capabilities describes what a provider can do.
type Capabilities struct {
	Text              bool         `json:"text"`
	StructuredOutput  bool         `json:"structured_output"`
	NativeToolCalling bool         `json:"native_tool_calling"`
	Vision            bool         `json:"vision"`
	Coding            bool         `json:"coding"`
	LongContext       bool         `json:"long_context"` // > 32K tokens
	ArabicQuality     QualityLevel `json:"arabic_quality"`
	MaxContextTokens  int          `json:"max_context_tokens"`
	TunnelType        string       `json:"tunnel_type"`           // "cloudflared" | "ngrok" | "zrok" | "none"
	IsReverseEngineered bool      `json:"is_reverse_engineered"` // G4F etc.
	IsLocalOnly       bool         `json:"is_local_only"`         // MLC/llama.cpp
	SessionBased      bool         `json:"session_based"`         // Kaggle — session ends
}

// Has returns true if the capabilities include all the required capabilities.
func (c Capabilities) Has(required Capabilities) bool {
	if required.Text && !c.Text {
		return false
	}
	if required.StructuredOutput && !c.StructuredOutput {
		return false
	}
	if required.NativeToolCalling && !c.NativeToolCalling {
		return false
	}
	if required.Vision && !c.Vision {
		return false
	}
	if required.Coding && !c.Coding {
		return false
	}
	if required.LongContext && !c.LongContext {
		return false
	}
	if required.MaxContextTokens > 0 && c.MaxContextTokens < required.MaxContextTokens {
		return false
	}
	if required.ArabicQuality != "" && required.ArabicQuality != QualityNone {
		if qualityRank(c.ArabicQuality) < qualityRank(required.ArabicQuality) {
			return false
		}
	}
	return true
}

func qualityRank(q QualityLevel) int {
	switch q {
	case QualityHigh:
		return 3
	case QualityMedium:
		return 2
	case QualityLow:
		return 1
	default:
		return 0
	}
}

// HealthStatus represents the health state of a provider.
type HealthStatus string

const (
	HealthUp      HealthStatus = "up"
	HealthDegraded HealthStatus = "degraded"
	HealthDown    HealthStatus = "down"
	HealthUnknown HealthStatus = "unknown"
)

// Health describes the current health of a provider.
type Health struct {
	Status       HealthStatus `json:"status"`
	LastCheck    time.Time    `json:"last_check"`
	LastError    string       `json:"last_error,omitempty"`
	LatencyMs    int64        `json:"latency_ms,omitempty"`
	SessionActive bool        `json:"session_active,omitempty"` // For Kaggle
}

// CostEstimate represents the estimated cost of a request.
// For free providers, Amount must always be 0.
type CostEstimate struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Notes    string  `json:"notes,omitempty"`
}

// IsZero returns true if the cost is zero.
func (c CostEstimate) IsZero() bool {
	return c.Amount == 0
}

// Request represents a generation request to a provider.
type Request struct {
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
	JSONMode    bool      `json:"json_mode,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Tool represents a tool definition for tool-calling models.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  []byte `json:"parameters,omitempty"` // JSON Schema
}

// Response represents a provider's generation response.
type Response struct {
	Content      string     `json:"content"`
	FinishReason string     `json:"finish_reason"` // "stop", "length", "tool_calls", "error"
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	Usage        Usage      `json:"usage"`
	Model        string     `json:"model"`         // Actual model used
	Provider     string     `json:"provider"`      // Provider ID
}

// ToolCall represents a tool invocation from the model.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON
}

// Usage represents token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// TaskType represents the category of a task for routing purposes.
type TaskType string

const (
	TaskPlanning        TaskType = "planning_and_reasoning"
	TaskCoding          TaskType = "coding"
	TaskVision          TaskType = "vision"
	TaskFast            TaskType = "fast_tasks"
	TaskGeneral         TaskType = "general"
)

// Provider is the interface that all LLM providers must implement.
type Provider interface {
	// ID returns a unique identifier for this provider instance.
	ID() string

	// Tier returns the cost classification of this provider.
	Tier() ProviderTier

	// Capabilities returns what this provider can do.
	// May make a network call to check (cached after first call).
	Capabilities(ctx context.Context) (Capabilities, error)

	// Health returns the current health status of this provider.
	Health(ctx context.Context) Health

	// Generate sends a request and returns the response.
	Generate(ctx context.Context, req Request) (Response, error)

	// EstimateCost returns the estimated cost of a request.
	// For free providers, this MUST return CostEstimate{Amount: 0}.
	EstimateCost(req Request) (CostEstimate, error)
}
