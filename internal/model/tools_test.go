package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func jsonServer(t *testing.T, handler func(int, *http.Request) (int, http.Header, string)) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		status, header, body := handler(calls, r)
		for key, values := range header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestCompleteWithToolsSendsSchemaAndParsesCalls(t *testing.T) {
	var received map[string]any
	server, _ := jsonServer(t, func(_ int, r *http.Request) (int, http.Header, string) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &received); err != nil {
			t.Fatalf("request not JSON: %v", err)
		}
		return 200, nil, `{"model":"m1","choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"workspace.read","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`
	})
	client := NewOpenAI(server.URL, "k", "m", 5*time.Second, 1<<20)
	completion, err := client.CompleteWithTools(context.Background(), ToolRequest{
		Messages: []ChatMessage{{Role: "user", Content: "read a.txt"}},
		Tools: []ToolSpec{{
			Name:        "workspace.read",
			Description: "Read a UTF-8 text file from the workspace.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(completion.Message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", completion.Message.ToolCalls)
	}
	call := completion.Message.ToolCalls[0]
	if call.ID != "call_1" || call.Function.Name != "workspace.read" || call.Function.Arguments != `{"path":"a.txt"}` {
		t.Fatalf("call = %+v", call)
	}
	if _, err := call.ArgumentsObject(); err != nil {
		t.Fatalf("arguments object: %v", err)
	}
	if completion.Usage.Total() != 18 || completion.Attempts != 1 || completion.FinishReason != "tool_calls" {
		t.Fatalf("completion = %+v", completion)
	}
	tools, ok := received["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools not sent: %+v", received["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["type"] != "function" || received["tool_choice"] != "auto" {
		t.Fatalf("tool envelope = %+v", tool)
	}
	function := tool["function"].(map[string]any)
	if function["name"] != "workspace.read" {
		t.Fatalf("function = %+v", function)
	}
	if _, ok := function["parameters"].(map[string]any); !ok {
		t.Fatalf("parameters not an object: %+v", function["parameters"])
	}
}

func TestCompleteWithToolsAcceptsObjectArguments(t *testing.T) {
	server, _ := jsonServer(t, func(int, *http.Request) (int, http.Header, string) {
		return 200, nil, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c","function":{"name":"workspace.list","arguments":{"path":"."}}}]},"finish_reason":"tool_calls"}]}`
	})
	client := NewOpenAI(server.URL, "k", "m", 5*time.Second, 1<<20)
	completion, err := client.CompleteWithTools(context.Background(), ToolRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := completion.Message.ToolCalls[0].Function.Arguments; got != `{"path":"."}` {
		t.Fatalf("arguments = %q", got)
	}
}

func TestEmptyArgumentsNormalizeToObject(t *testing.T) {
	server, _ := jsonServer(t, func(int, *http.Request) (int, http.Header, string) {
		return 200, nil, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c","function":{"name":"workspace.list","arguments":""}}]}}]}`
	})
	client := NewOpenAI(server.URL, "k", "m", 5*time.Second, 1<<20)
	completion, err := client.CompleteWithTools(context.Background(), ToolRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := completion.Message.ToolCalls[0].Function.Arguments; got != "{}" {
		t.Fatalf("arguments = %q", got)
	}
}

func TestRetryOnRateLimitHonorsRetryAfter(t *testing.T) {
	server, calls := jsonServer(t, func(n int, _ *http.Request) (int, http.Header, string) {
		if n == 1 {
			return 429, http.Header{"Retry-After": []string{"2"}}, `{"error":{"message":"rate limited"}}`
		}
		return 200, nil, `{"choices":[{"message":{"role":"assistant","content":"done"}}]}`
	})
	var slept []time.Duration
	client := NewOpenAI(server.URL, "k", "m", 5*time.Second, 1<<20).
		Configure(3, func(d time.Duration) { slept = append(slept, d) })
	completion, err := client.CompleteWithTools(context.Background(), ToolRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if completion.Message.Content != "done" || completion.Attempts != 2 || *calls != 2 {
		t.Fatalf("completion=%+v calls=%d", completion, *calls)
	}
	if len(slept) != 1 || slept[0] != 2*time.Second {
		t.Fatalf("backoff = %v", slept)
	}
}

func TestRetryBackoffIsBounded(t *testing.T) {
	server, calls := jsonServer(t, func(int, *http.Request) (int, http.Header, string) {
		return 429, http.Header{"Retry-After": []string{"3600"}}, `{}`
	})
	var slept []time.Duration
	client := NewOpenAI(server.URL, "k", "m", 5*time.Second, 1<<20).
		Configure(3, func(d time.Duration) { slept = append(slept, d) })
	_, err := client.CompleteWithTools(context.Background(), ToolRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	var httpErr *HTTPError
	if err == nil || !asHTTPError(err, &httpErr) || httpErr.StatusCode != 429 {
		t.Fatalf("err = %v", err)
	}
	if *calls != 3 || len(slept) != 2 {
		t.Fatalf("calls=%d slept=%v", *calls, slept)
	}
	for _, d := range slept {
		if d > maxRetryAfter {
			t.Fatalf("backoff not capped: %v", d)
		}
	}
}

func TestClientErrorsAreNotRetried(t *testing.T) {
	server, calls := jsonServer(t, func(int, *http.Request) (int, http.Header, string) {
		return 400, nil, `{"error":{"message":"bad tool schema","type":"invalid_request_error"}}`
	})
	client := NewOpenAI(server.URL, "k", "m", 5*time.Second, 1<<20).Configure(3, func(time.Duration) { t.Fatal("must not sleep") })
	_, err := client.CompleteWithTools(context.Background(), ToolRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	if err == nil || !strings.Contains(err.Error(), "bad tool schema") {
		t.Fatalf("err = %v", err)
	}
	if *calls != 1 {
		t.Fatalf("calls = %d", *calls)
	}
}

func TestInvalidToolSpecRejectedBeforeNetwork(t *testing.T) {
	server, calls := jsonServer(t, func(int, *http.Request) (int, http.Header, string) {
		return 200, nil, `{"choices":[{"message":{"role":"assistant","content":"x"}}]}`
	})
	client := NewOpenAI(server.URL, "k", "m", 5*time.Second, 1<<20)
	for _, spec := range []ToolSpec{
		{Name: "x", Description: "", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "x", Description: "d", Parameters: json.RawMessage(`[]`)},
		{Name: "x", Description: "d", Parameters: json.RawMessage(`not json`)},
	} {
		if _, err := client.CompleteWithTools(context.Background(), ToolRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}, Tools: []ToolSpec{spec}}); err == nil {
			t.Fatalf("spec accepted: %+v", spec)
		}
	}
	if *calls != 0 {
		t.Fatalf("network used %d times", *calls)
	}
}

func TestMalformedResponsesRejected(t *testing.T) {
	for _, body := range []string{
		`not json`,
		`{"choices":[]}`,
		`{"choices":[{"message":{"role":"assistant","content":""}}]}`,
		`{"choices":[{"message":{"role":"assistant","tool_calls":[{"function":{"name":"x","arguments":"{bad"}}]}}]}`,
	} {
		server, _ := jsonServer(t, func(int, *http.Request) (int, http.Header, string) { return 200, nil, body })
		client := NewOpenAI(server.URL, "k", "m", 5*time.Second, 1<<20)
		if _, err := client.CompleteWithTools(context.Background(), ToolRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}}); err == nil {
			t.Fatalf("body accepted: %s", body)
		}
	}
}

func TestResponseSizeLimitEnforced(t *testing.T) {
	server, _ := jsonServer(t, func(int, *http.Request) (int, http.Header, string) {
		return 200, nil, `{"choices":[{"message":{"role":"assistant","content":"` + strings.Repeat("a", 2048) + `"}}]}`
	})
	client := NewOpenAI(server.URL, "k", "m", 5*time.Second, 128)
	if _, err := client.CompleteWithTools(context.Background(), ToolRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}}); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err = %v", err)
	}
}

func TestToolCallValidation(t *testing.T) {
	valid := ToolCall{ID: "1", Function: FunctionCall{Name: "workspace.read", Arguments: `{"path":"a"}`}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := valid.ArgumentsObject(); err != nil {
		t.Fatal(err)
	}
	for _, call := range []ToolCall{
		{Function: FunctionCall{Name: "", Arguments: "{}"}},
		{Function: FunctionCall{Name: "x", Arguments: ""}},
		{Function: FunctionCall{Name: "x", Arguments: "{"}},
		{Function: FunctionCall{Name: "x", Arguments: `["a"]`}},
	} {
		if err := call.Validate(); err == nil {
			t.Fatalf("accepted %+v", call)
		}
	}
	if _, err := (ToolCall{Function: FunctionCall{Name: "x", Arguments: `"text"`}}).ArgumentsObject(); err == nil {
		t.Fatal("string arguments accepted as object")
	}
}

func TestRetryAfterParsing(t *testing.T) {
	if d := parseRetryAfter("3"); d != 3*time.Second {
		t.Fatalf("seconds = %v", d)
	}
	if d := parseRetryAfter(""); d != 0 {
		t.Fatalf("empty = %v", d)
	}
	if d := parseRetryAfter("not-a-date"); d != 0 {
		t.Fatalf("garbage = %v", d)
	}
	if d := parseRetryAfter(time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat)); d <= 0 || d > 6*time.Second {
		t.Fatalf("http date = %v", d)
	}
}

func asHTTPError(err error, target **HTTPError) bool {
	for err != nil {
		if typed, ok := err.(*HTTPError); ok {
			*target = typed
			return true
		}
		type unwrapper interface{ Unwrap() error }
		next, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = next.Unwrap()
	}
	return false
}
