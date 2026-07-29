package images

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type TokenSource interface {
	BearerToken(context.Context) (string, error)
}

type OpenAIConfig struct {
	Endpoint         string
	Model            string
	Tokens           TokenSource
	Client           *http.Client
	MaxResponseBytes int64
}

type OpenAIProvider struct{ config OpenAIConfig }

func NewOpenAIProvider(config OpenAIConfig) (*OpenAIProvider, error) {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" {
		return nil, errors.New("image provider endpoint must be HTTPS")
	}
	if config.Model == "" || len(config.Model) > 128 || config.Tokens == nil {
		return nil, errors.New("image provider model and token source required")
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = 16 * 1024 * 1024
	}
	if config.MaxResponseBytes < 1024 || config.MaxResponseBytes > 32*1024*1024 {
		return nil, errors.New("image provider response limit invalid")
	}
	if config.Client == nil {
		config.Client = &http.Client{}
	}
	return &OpenAIProvider{config: config}, nil
}

func (p *OpenAIProvider) Generate(ctx context.Context, request ProviderRequest) (ProviderResult, error) {
	references := make([]map[string]string, 0, len(request.References))
	for _, reference := range request.References {
		references = append(references, map[string]string{
			"mime_type": reference.MIME,
			"sha256":    reference.SHA256,
			"data":      base64.StdEncoding.EncodeToString(reference.Data),
		})
	}
	payload := struct {
		Model          string              `json:"model"`
		Prompt         string              `json:"prompt"`
		Size           string              `json:"size"`
		AspectRatio    string              `json:"aspect_ratio"`
		ResponseFormat string              `json:"response_format"`
		Count          int                 `json:"n"`
		References     []map[string]string `json:"references,omitempty"`
	}{p.config.Model, request.Prompt, fmt.Sprintf("%dx%d", request.Width, request.Height), request.AspectRatio, "b64_json", 1, references}
	body, err := json.Marshal(payload)
	if err != nil {
		return ProviderResult{}, err
	}
	token, err := p.config.Tokens.BearerToken(ctx)
	if err != nil || token == "" || strings.ContainsAny(token, "\r\n") {
		return ProviderResult{}, errors.New("image provider credential unavailable")
	}
	endpoint := strings.TrimRight(p.config.Endpoint, "/") + "/images/generations"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ProviderResult{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	client := *p.config.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("image provider redirects disabled") }
	response, err := client.Do(httpRequest)
	if err != nil {
		return ProviderResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return ProviderResult{}, fmt.Errorf("image provider HTTP status %d", response.StatusCode)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, p.config.MaxResponseBytes+1))
	if err != nil {
		return ProviderResult{}, err
	}
	if int64(len(encoded)) > p.config.MaxResponseBytes {
		return ProviderResult{}, errors.New("image provider response too large")
	}
	var decoded struct {
		Created int64 `json:"created,omitempty"`
		Data    []struct {
			B64JSON       string `json:"b64_json"`
			MIME          string `json:"mime_type,omitempty"`
			RevisedPrompt string `json:"revised_prompt,omitempty"`
		} `json:"data"`
		Usage struct {
			CostUSD float64 `json:"cost_usd"`
			Images  int     `json:"images"`
		} `json:"usage"`
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&decoded); err != nil || len(decoded.Data) != 1 || decoded.Data[0].B64JSON == "" {
		return ProviderResult{}, errors.New("invalid image provider response")
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ProviderResult{}, errors.New("invalid image provider response")
	}
	data, err := base64.StdEncoding.DecodeString(decoded.Data[0].B64JSON)
	if err != nil || len(data) > maxArtifactBytes {
		return ProviderResult{}, errors.New("invalid image provider payload")
	}
	mimeType := decoded.Data[0].MIME
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	units := decoded.Usage.Images
	if units == 0 {
		units = 1
	}
	return ProviderResult{Data: data, MIME: mimeType, Provider: "openai-compatible", Model: p.config.Model, Cost: Cost{Currency: "USD", Amount: decoded.Usage.CostUSD, Units: units}}, nil
}
