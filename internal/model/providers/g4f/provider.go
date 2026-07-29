// Package g4f implements a provider for G4F (GPT4Free) local server.
//
// G4F is an open-source project that converts free AI website interfaces
// into a local OpenAI-compatible API server. It provides access to
// GPT-4.1, Claude 4 Sonnet, Gemini 2.5 Pro, and others.
//
// ⚠️ WARNING: G4F uses reverse engineering and may violate some ToS.
// Use for personal/development purposes only.
//
// The user must manually start the G4F server before using this provider.
package g4f

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

// Config holds the G4F provider configuration.
type Config struct {
	BaseURL          string        `json:"base_url"`           // Default: http://localhost:8080
	ModelID          string        `json:"model_id"`           // Model to use
	Timeout          time.Duration `json:"timeout"`
	MaxResponseBytes int64         `json:"max_response_bytes"`
}

// Provider implements providers.Provider for G4F local server.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New creates a new G4F provider.
func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8080"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = 4 * 1024 * 1024 // 4MB
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *Provider) ID() string {
	return "g4f_local"
}

func (p *Provider) Tier() providers.ProviderTier {
	return providers.TierLocalSelfHosted
}

func (p *Provider) Capabilities(ctx context.Context) (providers.Capabilities, error) {
	return providers.Capabilities{
		Text:              true,
		StructuredOutput:  true,
		Vision:            true, // G4F supports vision via GPT-4o
		Coding:            true,
		ArabicQuality:     providers.QualityHigh,
		MaxContextTokens:  128000, // GPT-4 level context
		TunnelType:        "none",
		IsReverseEngineered: true, // G4F uses reverse engineering
		IsLocalOnly:       true,   // Runs on localhost
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

	url := p.cfg.BaseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return providers.Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return providers.Response{}, fmt.Errorf("g4f request failed: %w", err)
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
		return providers.Response{}, fmt.Errorf("g4f server error: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 {
		return providers.Response{}, fmt.Errorf("g4f HTTP %d: %s", resp.StatusCode, truncate(string(bodyBytes), 200))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		return providers.Response{}, fmt.Errorf("%w: %v", providers.ErrMalformedResponse, err)
	}

	if openAIResp.Error != nil {
		return providers.Response{}, fmt.Errorf("g4f error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return providers.Response{}, fmt.Errorf("%w: no choices returned", providers.ErrMalformedResponse)
	}

	choice := openAIResp.Choices[0]
	result := providers.Response{
		Content:      strings.TrimSpace(choice.Message.Content),
		FinishReason: choice.FinishReason,
		Model:        openAIResp.Model,
		Provider:     "g4f_local",
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
		Notes:    "g4f local (reverse-engineered free APIs)",
	}, nil
}

// OpenAI-compatible types for G4F

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
