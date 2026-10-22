package model

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// ErrNoTools reports that a client cannot perform native tool calling.
var ErrNoTools = errors.New("model client does not support tool calling")

// ToolSpec is a tool exposed to the model. Parameters must be a JSON Schema
// object document; the agent core validates that before sending it.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// FunctionCall carries the tool name and the JSON-encoded arguments produced by
// the model. Arguments is always a JSON document as text, exactly as the model
// emitted it, so approval hashes stay stable between request and resume.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall is one requested tool invocation.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// Usage is the provider-reported token accounting for one completion.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (u Usage) Total() int {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.PromptTokens + u.CompletionTokens
}

// ChatMessage is a provider-neutral conversation turn that can carry tool calls
// and tool results.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolRequest is a single tool-aware completion request.
type ToolRequest struct {
	Messages    []ChatMessage
	Tools       []ToolSpec
	MaxTokens   int
	Temperature float64
}

// Completion is a single tool-aware completion response.
type Completion struct {
	Message      ChatMessage
	Usage        Usage
	FinishReason string
	Model        string
	Attempts     int
}

// ToolClient is implemented by model clients that support native tool calling.
type ToolClient interface {
	CompleteWithTools(context.Context, ToolRequest) (Completion, error)
}

// HasToolCalls reports whether the message asks for at least one tool.
func (m ChatMessage) HasToolCalls() bool { return len(m.ToolCalls) > 0 }

// Validate checks that a tool call is minimally well formed before it reaches
// the policy engine.
func (c ToolCall) Validate() error {
	if strings.TrimSpace(c.Function.Name) == "" {
		return errors.New("tool call has no function name")
	}
	if strings.TrimSpace(c.Function.Arguments) == "" {
		return errors.New("tool call has no arguments")
	}
	if _, err := c.ArgumentsObject(); err != nil {
		return err
	}
	return nil
}

// ArgumentsObject requires the arguments to be a JSON object, which is what
// every registered tool schema expects.
func (c ToolCall) ArgumentsObject() (json.RawMessage, error) {
	arguments := strings.TrimSpace(c.Function.Arguments)
	if arguments == "" {
		return nil, errors.New("tool call has no arguments")
	}
	if !json.Valid([]byte(arguments)) {
		return nil, errors.New("tool call arguments are not valid JSON")
	}
	if arguments[0] != '{' {
		return nil, errors.New("tool call arguments must be a JSON object")
	}
	return json.RawMessage(arguments), nil
}
