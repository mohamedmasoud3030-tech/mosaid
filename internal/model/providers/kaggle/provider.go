// Package kaggle implements a provider for Kaggle GPU Tunnel.
//
// Kaggle provides 30 hours/week of free GPU (T4 dual = 32GB VRAM).
// The user runs a vLLM server in a Kaggle Notebook and exposes it
// via a Cloudflared tunnel. This provider connects to that tunnel.
//
// The user manually starts the Kaggle notebook and provides the
// tunnel URL in config. The provider does NOT open Kaggle automatically.
package kaggle

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

// Config holds the Kaggle tunnel provider configuration.
type Config struct {
	BaseURL      string `json:"base_url"`       // Cloudflared tunnel URL
	ModelID      string `json:"model_id"`        // Model name on vLLM
	Timeout      time.Duration `json:"timeout"`
	MaxResponseBytes int64 `json:"max_response_bytes"`
}

// Provider implements providers.Provider for Kaggle GPU Tunnel.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New creates a new Kaggle tunnel provider.
func New(cfg Config) *Provider {
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second // Longer timeout for 70B models
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = 8 * 1024 * 1024 // 8MB for large model outputs
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *Provider) ID() string {
	return "kaggle_tunnel"
}

func (p *Provider) Tier() providers.ProviderTier {
	return providers.TierLocalSelfHosted
}

func (p *Provider) Capabilities(ctx context.Context) (providers.Capabilities, error) {
	return providers.Capabilities{
		Text:              true,
		StructuredOutput:  true,
		NativeToolCalling: true,
		Coding:            true,
		LongContext:       true, // 32K tokens
		ArabicQuality:     providers.QualityHigh,
		MaxContextTokens:  32768,
		TunnelType:        "cloudflared",
		IsReverseEngineered: false,
		IsLocalOnly:       false, // Accessible via tunnel
		SessionBased:      true,  // Kaggle sessions expire
	}, nil
}

func (p *Provider) Health(ctx context.Context) providers.Health {
	start := time.Now()

	if p.cfg.BaseURL == "" {
		return providers.Health{
			Status:    providers.HealthDown,
			LastCheck: time.Now().UTC(),
			LastError: "no tunnel URL configured",
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(p.cfg.BaseURL, "/")+"/health", nil)
	if err != nil {
		return providers.Health{
			Status:    providers.HealthDown,
			LastCheck: time.Now().UTC(),
			LastError: err.Error(),
		}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		// Try /models endpoint as fallback health check
		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet,
			strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/models", nil)
		resp2, err2 := p.client.Do(req2)
		if err2 != nil {
			return providers.Health{
				Status:         providers.HealthDown,
				LastCheck:      time.Now().UTC(),
				LastError:      err.Error(),
				LatencyMs:      time.Since(start).Milliseconds(),
				SessionActive: false,
			}
		}
		defer resp2.Body.Close()
		io.Copy(io.Discard, resp2.Body)

		if resp2.StatusCode >= 500 {
			return providers.Health{
				Status:         providers.HealthDown,
				LastCheck:      time.Now().UTC(),
				LastError:      fmt.Sprintf("HTTP %d", resp2.StatusCode),
				LatencyMs:      time.Since(start).Milliseconds(),
				SessionActive: false,
			}
		}

		return providers.Health{
			Status:         providers.HealthUp,
			LastCheck:      time.Now().UTC(),
			LatencyMs:      time.Since(start).Milliseconds(),
			SessionActive: true,
		}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 500 {
		return providers.Health{
			Status:         providers.HealthDown,
			LastCheck:      time.Now().UTC(),
			LastError:      fmt.Sprintf("HTTP %d", resp.StatusCode),
			LatencyMs:      time.Since(start).Milliseconds(),
			SessionActive: false,
		}
	}

	return providers.Health{
		Status:         providers.HealthUp,
		LastCheck:      time.Now().UTC(),
		LatencyMs:      time.Since(start).Milliseconds(),
		SessionActive: true,
	}
}

func (p *Provider) Generate(ctx context.Context, req providers.Request) (providers.Response, error) {
	if p.cfg.BaseURL == "" {
		return providers.Response{}, fmt.Errorf("%w: no tunnel URL configured", providers.ErrProviderUnavailable)
	}

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

	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return providers.Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return providers.Response{}, fmt.Errorf("kaggle tunnel request failed: %w", err)
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
		return providers.Response{}, fmt.Errorf("kaggle tunnel server error: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 {
		return providers.Response{}, fmt.Errorf("kaggle tunnel HTTP %d: %s", resp.StatusCode, truncate(string(bodyBytes), 200))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		return providers.Response{}, fmt.Errorf("%w: %v", providers.ErrMalformedResponse, err)
	}

	if openAIResp.Error != nil {
		return providers.Response{}, fmt.Errorf("kaggle tunnel error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return providers.Response{}, fmt.Errorf("%w: no choices returned", providers.ErrMalformedResponse)
	}

	choice := openAIResp.Choices[0]
	result := providers.Response{
		Content:      strings.TrimSpace(choice.Message.Content),
		FinishReason: choice.FinishReason,
		Model:        openAIResp.Model,
		Provider:     "kaggle_tunnel",
		Usage: providers.Usage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		},
	}

	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]providers.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			result.ToolCalls[i] = providers.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
		}
	}

	return result, nil
}

func (p *Provider) EstimateCost(req providers.Request) (providers.CostEstimate, error) {
	return providers.CostEstimate{
		Amount:   0,
		Currency: "USD",
		Notes:    "kaggle free GPU (30h/week)",
	}, nil
}

// OpenAI-compatible types for Kaggle/vLLM

type openAIRequest struct {
	Model          string           `json:"model"`
	Messages       []openAIMessage  `json:"messages"`
	MaxTokens      int              `json:"max_tokens"`
	Temperature    float64          `json:"temperature"`
	Tools          []openAITool     `json:"tools,omitempty"`
	ResponseFormat *responseFormat  `json:"response_format,omitempty"`
	Stop           []string         `json:"stop,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

type responseFormat struct {
	Type string `json:"type"`
}

type openAIResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
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
