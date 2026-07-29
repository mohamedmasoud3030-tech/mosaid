// Package mlcllm implements a provider for MLC LLM local server.
//
// MLC LLM runs LLM models directly on the device's GPU via WebGPU.
// This is the last-resort provider — used when all online providers fail
// or when the device has no internet.
//
// The user must manually start the MLC server before using this provider.
package mlcllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers"
)

// Config holds the MLC LLM provider configuration.
type Config struct {
	BaseURL          string        `json:"base_url"`           // Default: http://127.0.0.1:8081
	ModelID          string        `json:"model_id"`           // Model name
	Timeout          time.Duration `json:"timeout"`
	MaxResponseBytes int64         `json:"max_response_bytes"`
}

// Provider implements providers.Provider for MLC LLM local server.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New creates a new MLC LLM provider.
func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://127.0.0.1:8081"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second // Local should be fast
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = 2 * 1024 * 1024 // 2MB
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *Provider) ID() string {
	return "mlcllm_local"
}

func (p *Provider) Tier() providers.ProviderTier {
	return providers.TierLocalSelfHosted
}

func (p *Provider) Capabilities(ctx context.Context) (providers.Capabilities, error) {
	return providers.Capabilities{
		Text:              true,
		StructuredOutput:  false, // Small models may not support structured output well
		NativeToolCalling: false, // Small models may not support tool calling
		Coding:            true,
		ArabicQuality:     providers.QualityMedium,
		MaxContextTokens:  8192, // Small models have limited context
		TunnelType:        "none",
		IsReverseEngineered: false,
		IsLocalOnly:       true,
		SessionBased:      false,
	}, nil
}

func (p *Provider) Health(ctx context.Context) providers.Health {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.cfg.BaseURL+"/v1/models", nil)
	if err != nil {
		return providers.Health{
			Status:    providers.HealthDown,
			LastCheck: time.Now().UTC(),
			LastError: err.Error(),
		}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return providers.Health{
			Status:    providers.HealthDown,
			LastCheck: time.Now().UTC(),
			LastError: err.Error(),
			LatencyMs: time.Since(start).Milliseconds(),
		}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 500 {
		return providers.Health{
			Status:    providers.HealthDown,
			LastCheck: time.Now().UTC(),
			LastError: fmt.Sprintf("HTTP %d", resp.StatusCode),
			LatencyMs: time.Since(start).Milliseconds(),
		}
	}

	status := providers.HealthUp
	if resp.StatusCode >= 400 {
		status = providers.HealthDegraded
	}

	return providers.Health{
		Status:    status,
		LastCheck: time.Now().UTC(),
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

func (p *Provider) Generate(ctx context.Context, req providers.Request) (providers.Response, error) {
	// Build OpenAI-compatible request
	body := openAIRequest{
		Model:       p.cfg.ModelID,
		Messages:    make([]openAIMessage, len(req.Messages)),
		Temperature: 0.2,
	}
	if req.Temperature > 0 {
		body.Temperature = req.Temperature
	}
	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	} else {
		body.MaxTokens = 512 // Smaller default for local models
	}
	if len(req.Stop) > 0 {
		body.Stop = req.Stop
	}

	for i, m := range req.Messages {
		body.Messages[i] = openAIMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	data, err := json.Marshal(body)
	if err != nil {
		return providers.Response{}, fmt.Errorf("marshal request: %w", err)
	}

	url := p.cfg.BaseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return providers.Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return providers.Response{}, fmt.Errorf("mlc request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, p.cfg.MaxResponseBytes+1))
	if err != nil {
		return providers.Response{}, fmt.Errorf("read response: %w", err)
	}
	if int64(len(bodyBytes)) > p.cfg.MaxResponseBytes {
		return providers.Response{}, providers.ErrRequestTooLarge
	}

	if resp.StatusCode == 429 {
		return providers.Response{}, fmt.Errorf("%w: HTTP 429", providers.ErrRateLimited)
	}
	if resp.StatusCode >= 500 {
		return providers.Response{}, fmt.Errorf("mlc server error: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 {
		return providers.Response{}, fmt.Errorf("mlc HTTP %d: %s", resp.StatusCode, truncate(string(bodyBytes), 200))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		return providers.Response{}, fmt.Errorf("%w: %v", providers.ErrMalformedResponse, err)
	}

	if openAIResp.Error != nil {
		return providers.Response{}, fmt.Errorf("mlc error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return providers.Response{}, fmt.Errorf("%w: no choices returned", providers.ErrMalformedResponse)
	}

	choice := openAIResp.Choices[0]
	return providers.Response{
		Content:      strings.TrimSpace(choice.Message.Content),
		FinishReason: choice.FinishReason,
		Model:        openAIResp.Model,
		Provider:     "mlcllm_local",
		Usage: providers.Usage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		},
	}, nil
}

func (p *Provider) EstimateCost(req providers.Request) (providers.CostEstimate, error) {
	return providers.CostEstimate{
		Amount:   0,
		Currency: "USD",
		Notes:    "local on-device inference (MLC LLM)",
	}, nil
}

// OpenAI-compatible types for MLC LLM

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
	Stop        []string        `json:"stop,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
