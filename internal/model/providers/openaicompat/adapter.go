// Package openaicompat implements a provider adapter for any OpenAI-compatible
// API endpoint. This works with Groq, Cerebras, Google AI Studio, Kaggle (vLLM),
// and any other service that exposes the /v1/chat/completions endpoint.
package openaicompat

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

// Config holds the configuration for an OpenAI-compatible provider.
type Config struct {
	ID               string                 `json:"id"`
	BaseURL          string                 `json:"base_url"`
	APIKey           string                 `json:"api_key"` // Empty for local/tunnel providers
	ModelID          string                 `json:"model_id"`
	Tier             providers.ProviderTier `json:"tier"`
	Timeout          time.Duration          `json:"timeout"`
	MaxResponseBytes int64                  `json:"max_response_bytes"`
	Caps             providers.Capabilities `json:"caps"`
}

// Provider implements providers.Provider for OpenAI-compatible endpoints.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New creates a new OpenAI-compatible provider.
func New(cfg Config) *Provider {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = 4 * 1024 * 1024 // 4MB
	}
	if cfg.Caps.MaxContextTokens == 0 {
		cfg.Caps.MaxContextTokens = 32768
	}
	if cfg.Caps.ArabicQuality == "" {
		cfg.Caps.ArabicQuality = providers.QualityMedium
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
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
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(p.cfg.BaseURL, "/")+"/models", nil)
	if err != nil {
		return providers.Health{
			Status:    providers.HealthDown,
			LastCheck: time.Now().UTC(),
			LastError: err.Error(),
		}
	}
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
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
	// Build OpenAI-compatible request body
	body := openAIRequest{
		Model:       p.cfg.ModelID,
		Messages:    make([]openAIMessage, len(req.Messages)),
		Temperature: req.Temperature,
	}
	if body.Temperature == 0 {
		body.Temperature = 0.2
	}
	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	} else {
		body.MaxTokens = 1024
	}
	if req.JSONMode {
		body.ResponseFormat = &responseFormat{Type: "json_object"}
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

	if len(req.Tools) > 0 {
		body.Tools = make([]openAITool, len(req.Tools))
		for i, t := range req.Tools {
			var params json.RawMessage
			if len(t.Parameters) > 0 {
				params = t.Parameters
			} else {
				params = json.RawMessage(`{}`)
			}
			body.Tools[i] = openAITool{
				Type: "function",
				Function: openAIFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  params,
				},
			}
		}
	}

	data, err := json.Marshal(body)
	if err != nil {
		return providers.Response{}, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return providers.Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return providers.Response{}, fmt.Errorf("provider %q request failed: %w", p.cfg.ID, err)
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
		return providers.Response{}, fmt.Errorf("provider %q server error: HTTP %d", p.cfg.ID, resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 {
		return providers.Response{}, fmt.Errorf("provider %q HTTP %d: %s", p.cfg.ID, resp.StatusCode, truncate(string(bodyBytes), 200))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		return providers.Response{}, fmt.Errorf("%w: %v", providers.ErrMalformedResponse, err)
	}

	if openAIResp.Error != nil {
		return providers.Response{}, fmt.Errorf("provider %q error: %s", p.cfg.ID, openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return providers.Response{}, fmt.Errorf("%w: no choices returned", providers.ErrMalformedResponse)
	}

	choice := openAIResp.Choices[0]
	resp2 := providers.Response{
		Content:      strings.TrimSpace(choice.Message.Content),
		FinishReason: choice.FinishReason,
		Model:        openAIResp.Model,
		Provider:     p.cfg.ID,
		Usage: providers.Usage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		},
	}

	if len(choice.Message.ToolCalls) > 0 {
		resp2.ToolCalls = make([]providers.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			resp2.ToolCalls[i] = providers.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
		}
	}

	return resp2, nil
}

func (p *Provider) EstimateCost(req providers.Request) (providers.CostEstimate, error) {
	return providers.CostEstimate{
		Amount:   0,
		Currency: "USD",
		Notes:    "free tier",
	}, nil
}

// OpenAI API types

type openAIRequest struct {
	Model          string          `json:"model"`
	Messages       []openAIMessage `json:"messages"`
	MaxTokens      int             `json:"max_tokens"`
	Temperature    float64         `json:"temperature"`
	Tools          []openAITool    `json:"tools,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Stop           []string        `json:"stop,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
