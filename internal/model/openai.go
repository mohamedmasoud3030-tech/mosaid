package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAI struct {
	base, key, name string
	client          *http.Client
	max             int64
}

func NewOpenAI(base, key, name string, timeout time.Duration, max int64) *OpenAI {
	return &OpenAI{strings.TrimRight(base, "/"), key, name, &http.Client{Timeout: timeout}, max}
}
func (o *OpenAI) Complete(ctx context.Context, msgs []Message) (string, error) {
	body := struct {
		Model       string    `json:"model"`
		Messages    []Message `json:"messages"`
		MaxTokens   int       `json:"max_tokens"`
		Temperature float64   `json:"temperature"`
	}{o.name, msgs, 1024, 0.2}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.base+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+o.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	r := io.LimitReader(resp.Body, o.max+1)
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if int64(len(data)) > o.max {
		return "", errors.New("model response too large")
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("model HTTP %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err = json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("model returned no choice")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
