package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTPError is a non-2xx provider response. It is typed so callers can tell a
// rate limit apart from a malformed request.
type HTTPError struct {
	StatusCode int
	RetryAfter time.Duration
	detail     string
}

func (e *HTTPError) Error() string {
	if e.detail != "" {
		return fmt.Sprintf("model HTTP %d: %s", e.StatusCode, e.detail)
	}
	return fmt.Sprintf("model HTTP %d", e.StatusCode)
}

// Retryable reports whether the status is worth retrying.
func (e *HTTPError) Retryable() bool {
	switch e.StatusCode {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

const (
	// defaultAttempts bounds retries so a free-tier 429 cannot pin the bot.
	defaultAttempts = 3
	// maxRetryAfter prevents a provider from parking a single Telegram turn.
	maxRetryAfter = 20 * time.Second
	// maxDiagnosticBody bounds the error body we read for logging.
	maxDiagnosticBody = 4 << 10
)

// Configure replaces the retry policy. It is used by main for configuration and
// by tests to keep retries instant. It returns the receiver for chaining.
func (o *OpenAI) Configure(attempts int, sleep func(time.Duration)) *OpenAI {
	if attempts > 0 {
		o.attempts = attempts
	}
	if sleep != nil {
		o.sleep = sleep
	}
	return o
}

// CompleteWithTools performs a native tool-calling completion against an
// OpenAI-compatible chat completions endpoint and retries transient failures.
func (o *OpenAI) CompleteWithTools(ctx context.Context, req ToolRequest) (Completion, error) {
	if o == nil || o.client == nil {
		return Completion{}, errors.New("model client unavailable")
	}
	if len(req.Messages) == 0 {
		return Completion{}, errors.New("messages required")
	}
	payload := map[string]any{
		"model":       o.name,
		"messages":    req.Messages,
		"temperature": req.Temperature,
	}
	limit := req.MaxTokens
	if limit <= 0 {
		limit = 1024
	}
	payload["max_tokens"] = limit
	if len(req.Tools) > 0 {
		wire := make([]wireTool, 0, len(req.Tools))
		for _, spec := range req.Tools {
			if err := validateToolSpec(spec); err != nil {
				return Completion{}, err
			}
			wire = append(wire, wireTool{Type: "function", Function: spec})
		}
		payload["tools"] = wire
		payload["tool_choice"] = "auto"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Completion{}, err
	}

	attempts := o.attempts
	if attempts < 1 {
		attempts = defaultAttempts
	}
	sleep := o.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = ctx.Err(); err != nil {
			return Completion{}, err
		}
		completion, retryable, retryAfter, callErr := o.once(ctx, body)
		completion.Attempts = attempt
		if callErr == nil {
			return completion, nil
		}
		lastErr = callErr
		if !retryable || attempt == attempts {
			break
		}
		wait := retryAfter
		if wait <= 0 {
			wait = time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond
		}
		if wait > maxRetryAfter {
			wait = maxRetryAfter
		}
		sleep(wait)
	}
	return Completion{}, lastErr
}

func (o *OpenAI) once(ctx context.Context, body []byte) (Completion, bool, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Completion{}, false, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+o.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		// Network errors are transient; a cancelled context is not.
		return Completion{}, !errors.Is(err, context.Canceled), 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, o.max+1))
	if err != nil {
		return Completion{}, true, 0, err
	}
	if int64(len(data)) > o.max {
		return Completion{}, false, 0, errors.New("model response too large")
	}
	if resp.StatusCode/100 != 2 {
		httpErr := &HTTPError{StatusCode: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
		if len(data) > 0 && len(data) <= maxDiagnosticBody {
			httpErr.detail = providerError(data)
		}
		return Completion{}, httpErr.Retryable(), httpErr.RetryAfter, httpErr
	}
	return parseCompletion(data)
}

func parseCompletion(data []byte) (Completion, bool, time.Duration, error) {
	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message      wireMessage `json:"message"`
			FinishReason string      `json:"finish_reason"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return Completion{}, false, 0, errors.New("model response is not valid JSON")
	}
	if out.Error != nil && out.Error.Message != "" {
		return Completion{}, false, 0, errors.New("model provider error: " + truncate(out.Error.Message, 200))
	}
	if len(out.Choices) == 0 {
		return Completion{}, false, 0, errors.New("model returned no choice")
	}
	choice := out.Choices[0]
	message, err := choice.Message.toChatMessage()
	if err != nil {
		return Completion{}, false, 0, err
	}
	if len(message.ToolCalls) == 0 && strings.TrimSpace(message.Content) == "" {
		return Completion{}, false, 0, errors.New("model returned an empty message")
	}
	return Completion{Message: message, Usage: out.Usage, FinishReason: choice.FinishReason, Model: out.Model}, false, 0, nil
}

type wireTool struct {
	Type     string   `json:"type"`
	Function ToolSpec `json:"function"`
}

type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls"`
	ToolCallID string         `json:"tool_call_id"`
}

func (w wireMessage) toChatMessage() (ChatMessage, error) {
	message := ChatMessage{Role: strings.TrimSpace(w.Role), Content: w.Content, ToolCallID: w.ToolCallID}
	if message.Role == "" {
		message.Role = "assistant"
	}
	if len(w.ToolCalls) > 32 {
		return ChatMessage{}, errors.New("model requested too many tool calls")
	}
	for _, call := range w.ToolCalls {
		arguments, err := normalizeArguments(call.Function.Arguments)
		if err != nil {
			return ChatMessage{}, err
		}
		kind := call.Type
		if kind == "" {
			kind = "function"
		}
		message.ToolCalls = append(message.ToolCalls, ToolCall{ID: call.ID, Type: kind, Function: FunctionCall{Name: call.Function.Name, Arguments: arguments}})
	}
	return message, nil
}

// normalizeArguments accepts both the OpenAI string encoding and the object
// encoding some compatible providers emit, and always returns JSON text.
func normalizeArguments(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "{}", nil
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return "", errors.New("tool arguments string is malformed")
		}
		if strings.TrimSpace(text) == "" {
			return "{}", nil
		}
		if !json.Valid([]byte(text)) {
			return "", errors.New("tool arguments are not valid JSON")
		}
		return strings.TrimSpace(text), nil
	}
	if !json.Valid(trimmed) {
		return "", errors.New("tool arguments are not valid JSON")
	}
	return string(trimmed), nil
}

func validateToolSpec(spec ToolSpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Description) == "" {
		return errors.New("tool spec requires name and description")
	}
	if len(spec.Parameters) == 0 || !json.Valid(spec.Parameters) {
		return fmt.Errorf("tool %q parameters are not valid JSON", spec.Name)
	}
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(spec.Parameters, &probe); err != nil || probe.Type != "object" {
		return fmt.Errorf("tool %q parameters must be a JSON Schema object", spec.Name)
	}
	return nil
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if d := time.Until(when); d > 0 {
			return d
		}
	}
	return 0
}

func providerError(data []byte) string {
	var out struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &out) == nil && out.Error.Message != "" {
		return truncate(out.Error.Message, 200)
	}
	return ""
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
