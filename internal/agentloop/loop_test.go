package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/security"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/tools"
)

// scriptedModel replays a fixed sequence of completions and records requests.
type scriptedModel struct {
	steps    []func(model.ToolRequest) model.Completion
	requests []model.ToolRequest
}

func (s *scriptedModel) CompleteWithTools(_ context.Context, request model.ToolRequest) (model.Completion, error) {
	s.requests = append(s.requests, request)
	if len(s.steps) == 0 {
		return model.Completion{}, errors.New("script exhausted")
	}
	step := s.steps[0]
	s.steps = s.steps[1:]
	return step(request), nil
}

func toolCompletion(id, name, arguments string) model.Completion {
	return model.Completion{
		Message: model.ChatMessage{Role: "assistant", ToolCalls: []model.ToolCall{{
			ID: id, Type: "function", Function: model.FunctionCall{Name: name, Arguments: arguments},
		}}},
		Usage:        model.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110},
		FinishReason: "tool_calls",
		Attempts:     1,
	}
}

func answerCompletion(text string) model.Completion {
	return model.Completion{
		Message:      model.ChatMessage{Role: "assistant", Content: text},
		Usage:        model.Usage{PromptTokens: 50, CompletionTokens: 20, TotalTokens: 70},
		FinishReason: "stop",
		Attempts:     1,
	}
}

type fixture struct {
	workspace string
	registry  *tools.Registry
	approvals *approval.Manager
	limits    security.BudgetLimits
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(filepath.Join(root, "mosaid.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	approvals := &approval.Manager{DB: db.SQL(), Audit: audit.Logger{DB: db.SQL()}}
	registry := tools.NewRegistry(approvals)
	if err := tools.RegisterWorkspace(registry, tools.NewWorkspace(workspace), nil); err != nil {
		t.Fatal(err)
	}
	return &fixture{
		workspace: workspace,
		registry:  registry,
		approvals: approvals,
		limits:    security.BudgetLimits{ModelSteps: 8, ToolCalls: 8, Tokens: 100000, CostUSD: 1, Retries: 4},
	}
}

func (f *fixture) config(m model.ToolClient) Config {
	return Config{
		Model:   m,
		Tools:   f.registry,
		System:  "test system prompt",
		History: []model.ChatMessage{{Role: "user", Content: "do the task"}},
		UserID:  42,
		Mode:    policy.Admin,
		Limits:  f.limits,
	}
}

func (f *fixture) writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.workspace, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunExecutesReadToolThenAnswers(t *testing.T) {
	f := newFixture(t)
	f.writeFile(t, "notes.md", "buy milk")
	scripted := &scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "workspace.read", `{"path":"notes.md"}`)
		},
		func(request model.ToolRequest) model.Completion {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "tool" || last.ToolCallID != "c1" || !strings.Contains(last.Content, "buy milk") {
				t.Fatalf("tool observation = %+v", last)
			}
			return answerCompletion("You need milk.")
		},
	}}
	outcome, err := Run(context.Background(), f.config(scripted))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Text != "You need milk." || outcome.Steps != 2 || outcome.Truncated {
		t.Fatalf("outcome = %+v", outcome)
	}
	if len(outcome.ToolEvents) != 1 || outcome.ToolEvents[0].Name != "workspace.read" || outcome.ToolEvents[0].Failed {
		t.Fatalf("events = %+v", outcome.ToolEvents)
	}
	if outcome.ToolEvents[0].Resource != "notes.md" {
		t.Fatalf("resource = %q", outcome.ToolEvents[0].Resource)
	}
	if outcome.Usage.Total() != 180 || outcome.CostUSD != 0 {
		t.Fatalf("accounting = %+v cost=%v", outcome.Usage, outcome.CostUSD)
	}
	if len(scripted.requests[0].Tools) < 5 || scripted.requests[0].Tools[0].Name == "" {
		t.Fatalf("tools not advertised: %+v", scripted.requests[0].Tools)
	}
	first := scripted.requests[0].Messages[0]
	if first.Role != "system" || first.Content != "test system prompt" {
		t.Fatalf("system message = %+v", first)
	}
}

func TestUnknownToolBecomesObservationAndModelRecovers(t *testing.T) {
	f := newFixture(t)
	scripted := &scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "shell.exec", `{"cmd":"rm -rf /"}`)
		},
		func(request model.ToolRequest) model.Completion {
			last := request.Messages[len(request.Messages)-1]
			if !strings.Contains(last.Content, `"ok":false`) || !strings.Contains(last.Content, "not registered") {
				t.Fatalf("observation = %s", last.Content)
			}
			return answerCompletion("That tool does not exist; ask me differently.")
		},
	}}
	outcome, err := Run(context.Background(), f.config(scripted))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Text == "" || !outcome.ToolEvents[0].Failed {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestMalformedArgumentsNeverReachTools(t *testing.T) {
	f := newFixture(t)
	scripted := &scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion { return toolCompletion("c1", "workspace.read", `{"path":`) },
		func(request model.ToolRequest) model.Completion {
			last := request.Messages[len(request.Messages)-1]
			if !strings.Contains(last.Content, "not valid JSON") {
				t.Fatalf("observation = %s", last.Content)
			}
			return answerCompletion("Recovered.")
		},
	}}
	outcome, err := Run(context.Background(), f.config(scripted))
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.ToolEvents[0].Failed || outcome.Text != "Recovered." {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestApprovalStopsExecutionBeforeWriting(t *testing.T) {
	f := newFixture(t)
	scripted := &scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "workspace.write", `{"path":"todo.md","content":"- milk"}`)
		},
	}}
	outcome, err := Run(context.Background(), f.config(scripted))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Pending == nil {
		t.Fatalf("expected pending approval, got %+v", outcome)
	}
	if outcome.Text != "" || len(scripted.requests) != 1 {
		t.Fatalf("loop continued after approval request: %+v", outcome)
	}
	if _, err := os.Stat(filepath.Join(f.workspace, "todo.md")); !os.IsNotExist(err) {
		t.Fatal("file was written before approval")
	}
	pending := outcome.Pending
	if pending.Tool != "workspace.write" || pending.Resource != "todo.md" || pending.CallID != "c1" {
		t.Fatalf("pending = %+v", pending)
	}
	if pending.ArgumentJSON != `{"path":"todo.md","content":"- milk"}` {
		t.Fatalf("arguments = %s", pending.ArgumentJSON)
	}
	if len(pending.Messages) != 3 {
		t.Fatalf("snapshot = %+v", pending.Messages)
	}
	if !outcome.ToolEvents[0].Approval {
		t.Fatalf("event = %+v", outcome.ToolEvents[0])
	}
}

func TestApproveThenResumeWritesFile(t *testing.T) {
	f := newFixture(t)
	scripted := &scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "workspace.write", `{"path":"todo.md","content":"- milk"}`)
		},
		func(request model.ToolRequest) model.Completion {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "tool" || last.ToolCallID != "c1" || !strings.Contains(last.Content, `"ok":true`) {
				t.Fatalf("observation = %+v", last)
			}
			return answerCompletion("Written.")
		},
	}}
	outcome, err := Run(context.Background(), f.config(scripted))
	if err != nil || outcome.Pending == nil {
		t.Fatalf("run: %v %+v", err, outcome)
	}
	pending := *outcome.Pending
	if err = f.approvals.ResolveToken(context.Background(), pending.Token, 42, "approved"); err != nil {
		t.Fatal(err)
	}
	result, err := f.registry.Execute(context.Background(), tools.Request{
		Name: pending.Tool, Arguments: json.RawMessage(pending.ArgumentJSON), Mode: policy.Admin,
		UserID: 42, Resource: pending.Resource, ApprovalToken: pending.Token,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Approval != nil {
		t.Fatal("approved call asked for approval again")
	}
	content, err := os.ReadFile(filepath.Join(f.workspace, "todo.md"))
	if err != nil || string(content) != "- milk" {
		t.Fatalf("content=%q err=%v", content, err)
	}
	observation, _ := json.Marshal(map[string]any{"ok": true, "tool": pending.Tool, "result": result.Value})
	resumed, err := Resume(context.Background(), f.config(scripted), pending, string(observation))
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Text != "Written." || len(resumed.ToolEvents) != 0 {
		t.Fatalf("resume = %+v", resumed)
	}
	// Replay of the same approval must fail: approvals are single use.
	if _, err := f.registry.Execute(context.Background(), tools.Request{
		Name: pending.Tool, Arguments: json.RawMessage(pending.ArgumentJSON), Mode: policy.Admin,
		UserID: 42, Resource: pending.Resource, ApprovalToken: pending.Token,
	}); err == nil {
		t.Fatal("replayed approval accepted")
	}
}

func TestDeniedApprovalNeverWrites(t *testing.T) {
	f := newFixture(t)
	scripted := &scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "workspace.write", `{"path":"todo.md","content":"- milk"}`)
		},
	}}
	outcome, err := Run(context.Background(), f.config(scripted))
	if err != nil || outcome.Pending == nil {
		t.Fatalf("run: %v %+v", err, outcome)
	}
	pending := *outcome.Pending
	if err = f.approvals.ResolveToken(context.Background(), pending.Token, 42, "denied"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.registry.Execute(context.Background(), tools.Request{
		Name: pending.Tool, Arguments: json.RawMessage(pending.ArgumentJSON), Mode: policy.Admin,
		UserID: 42, Resource: pending.Resource, ApprovalToken: pending.Token,
	}); err == nil {
		t.Fatal("denied approval executed")
	}
	if _, err = os.Stat(filepath.Join(f.workspace, "todo.md")); !os.IsNotExist(err) {
		t.Fatal("file written despite denial")
	}
}

func TestApprovalTokenIsBoundToUser(t *testing.T) {
	f := newFixture(t)
	scripted := &scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "workspace.write", `{"path":"todo.md","content":"- milk"}`)
		},
	}}
	outcome, err := Run(context.Background(), f.config(scripted))
	if err != nil || outcome.Pending == nil {
		t.Fatalf("run: %v %+v", err, outcome)
	}
	pending := *outcome.Pending
	if err = f.approvals.ResolveToken(context.Background(), pending.Token, 43, "approved"); err == nil {
		t.Fatal("approval resolved by a different user")
	}
	if err = f.approvals.ResolveToken(context.Background(), pending.Token, 42, "approved"); err != nil {
		t.Fatal(err)
	}
	// A different path must not reuse the same approval.
	if _, err = f.registry.Execute(context.Background(), tools.Request{
		Name: pending.Tool, Arguments: json.RawMessage(`{"path":"other.md","content":"x"}`), Mode: policy.Admin,
		UserID: 42, Resource: "other.md", ApprovalToken: pending.Token,
	}); err == nil {
		t.Fatal("approval applied to a different resource")
	}
}

func TestReadModeCannotWrite(t *testing.T) {
	f := newFixture(t)
	cfg := f.config(&scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "workspace.write", `{"path":"todo.md","content":"x"}`)
		},
		func(request model.ToolRequest) model.Completion {
			last := request.Messages[len(request.Messages)-1]
			if !strings.Contains(last.Content, "policy denied") {
				t.Fatalf("observation = %s", last.Content)
			}
			return answerCompletion("I cannot write in read-only mode.")
		},
	}})
	cfg.Mode = policy.Read
	outcome, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Pending != nil || outcome.ToolEvents[0].Approval || !outcome.ToolEvents[0].Failed {
		t.Fatalf("outcome = %+v", outcome)
	}
	if _, err := os.Stat(filepath.Join(f.workspace, "todo.md")); !os.IsNotExist(err) {
		t.Fatal("read mode wrote a file")
	}
}

func TestPathTraversalIsDeniedInsideLoop(t *testing.T) {
	f := newFixture(t)
	scripted := &scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "workspace.read", `{"path":"../../etc/passwd"}`)
		},
		func(request model.ToolRequest) model.Completion {
			last := request.Messages[len(request.Messages)-1]
			if !strings.Contains(last.Content, `"ok":false`) {
				t.Fatalf("observation = %s", last.Content)
			}
			return answerCompletion("Denied.")
		},
	}}
	outcome, err := Run(context.Background(), f.config(scripted))
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.ToolEvents[0].Failed || strings.Contains(outcome.Text, "root:") {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestStepLimitForcesFinalAnswer(t *testing.T) {
	f := newFixture(t)
	steps := make([]func(model.ToolRequest) model.Completion, 0, 4)
	for i := 0; i < 3; i++ {
		steps = append(steps, func(model.ToolRequest) model.Completion {
			return toolCompletion("c", "workspace.list", `{"path":"."}`)
		})
	}
	var lastRequest model.ToolRequest
	steps = append(steps, func(request model.ToolRequest) model.Completion {
		lastRequest = request
		return answerCompletion("Final answer without tools.")
	})
	cfg := f.config(&scriptedModel{steps: steps})
	cfg.MaxSteps = 4
	cfg.SuppressToolsOnLast = true
	outcome, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Text != "Final answer without tools." || outcome.Steps != 4 {
		t.Fatalf("outcome = %+v", outcome)
	}
	if len(lastRequest.Tools) != 0 {
		t.Fatalf("tools still advertised on final step: %d", len(lastRequest.Tools))
	}
	last := lastRequest.Messages[len(lastRequest.Messages)-1]
	if last.Role != "system" || !strings.Contains(last.Content, "budget") {
		t.Fatalf("missing final-answer nudge: %+v", last)
	}
}

func TestModelIgnoringFinalNudgeIsTruncated(t *testing.T) {
	f := newFixture(t)
	cfg := f.config(&scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "workspace.list", `{"path":"."}`)
		},
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c2", "workspace.list", `{"path":"."}`)
		},
	}})
	cfg.MaxSteps = 2
	cfg.SuppressToolsOnLast = true
	outcome, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Truncated || outcome.Pending != nil {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestTokenBudgetStopsRunawayLoop(t *testing.T) {
	f := newFixture(t)
	f.limits = security.BudgetLimits{ModelSteps: 8, ToolCalls: 8, Tokens: 150, CostUSD: 1, Retries: 4}
	cfg := f.config(&scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "workspace.list", `{"path":"."}`)
		},
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c2", "workspace.list", `{"path":"."}`)
		},
	}})
	_, err := Run(context.Background(), cfg)
	if !errors.Is(err, security.ErrBudgetExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestCostTripwireCountsProviderUsage(t *testing.T) {
	f := newFixture(t)
	f.limits = security.BudgetLimits{ModelSteps: 8, ToolCalls: 8, Tokens: 100000, CostUSD: 0.001, Retries: 4}
	cfg := f.config(&scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			completion := answerCompletion("expensive")
			completion.Usage = model.Usage{PromptTokens: 10000, CompletionTokens: 10000}
			return completion
		},
	}})
	cfg.Prices = security.PriceLimits{InputPer1M: 10, OutputPer1M: 10}
	_, err := Run(context.Background(), cfg)
	if !errors.Is(err, security.ErrBudgetExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestRetriesAreChargedToBudget(t *testing.T) {
	f := newFixture(t)
	f.limits = security.BudgetLimits{ModelSteps: 4, ToolCalls: 4, Tokens: 10000, CostUSD: 1, Retries: 1}
	cfg := f.config(&scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			completion := answerCompletion("slow")
			completion.Attempts = 3
			return completion
		},
	}})
	_, err := Run(context.Background(), cfg)
	if !errors.Is(err, security.ErrBudgetExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestToolResultTruncation(t *testing.T) {
	f := newFixture(t)
	f.writeFile(t, "big.txt", strings.Repeat("a", 4000))
	scripted := &scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "workspace.read", `{"path":"big.txt"}`)
		},
		func(request model.ToolRequest) model.Completion {
			last := request.Messages[len(request.Messages)-1]
			if !strings.HasSuffix(last.Content, TruncationMarker) {
				t.Fatalf("not truncated: %d bytes", len(last.Content))
			}
			if len(last.Content) > 512+len(TruncationMarker) {
				t.Fatalf("truncation limit ignored: %d", len(last.Content))
			}
			return answerCompletion("Read a large file.")
		},
	}}
	cfg := f.config(scripted)
	cfg.MaxToolResultBytes = 512
	if _, err := Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
}

func TestParallelCallsAreBounded(t *testing.T) {
	f := newFixture(t)
	f.writeFile(t, "a.md", "a")
	multi := model.Completion{
		Message: model.ChatMessage{Role: "assistant", ToolCalls: []model.ToolCall{
			{ID: "1", Function: model.FunctionCall{Name: "workspace.read", Arguments: `{"path":"a.md"}`}},
			{ID: "2", Function: model.FunctionCall{Name: "workspace.read", Arguments: `{"path":"a.md"}`}},
			{ID: "3", Function: model.FunctionCall{Name: "workspace.read", Arguments: `{"path":"a.md"}`}},
			{ID: "4", Function: model.FunctionCall{Name: "workspace.read", Arguments: `{"path":"a.md"}`}},
			{ID: "5", Function: model.FunctionCall{Name: "workspace.read", Arguments: `{"path":"a.md"}`}},
		}},
		Usage:    model.Usage{PromptTokens: 10, CompletionTokens: 10},
		Attempts: 1,
	}
	cfg := f.config(&scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion { return multi },
		func(model.ToolRequest) model.Completion { return answerCompletion("done") },
	}})
	cfg.MaxCallsPerStep = 2
	outcome, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.ToolEvents) != 2 || !outcome.Truncated {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestConfigValidation(t *testing.T) {
	f := newFixture(t)
	if _, err := Run(context.Background(), Config{}); !errors.Is(err, ErrClientMissing) {
		t.Fatalf("err = %v", err)
	}
	if _, err := Run(context.Background(), Config{Model: &scriptedModel{}}); !errors.Is(err, ErrToolsMissing) {
		t.Fatalf("err = %v", err)
	}
	cfg := f.config(&scriptedModel{})
	cfg.History = nil
	if _, err := Run(context.Background(), cfg); !errors.Is(err, ErrNoHistory) {
		t.Fatalf("err = %v", err)
	}
	cfg = f.config(&scriptedModel{})
	cfg.Limits.CostUSD = 0
	if _, err := Run(context.Background(), cfg); err == nil {
		t.Fatal("invalid budget accepted")
	}
	if _, err := Resume(context.Background(), f.config(&scriptedModel{}), Pending{}, "x"); !errors.Is(err, ErrBadPending) {
		t.Fatalf("err = %v", err)
	}
}

func TestCancelledContextStops(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, f.config(&scriptedModel{})); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestResourceExtraction(t *testing.T) {
	cases := map[string]string{
		`{"path":"a/b.md","content":"x"}`: "a/b.md",
		`{"cwd":"sub","profile":"go"}`:    "sub",
		`{"query":"roadmap"}`:             "roadmap",
		`{"prompt":"a cat"}`:              "a cat",
		`{"limit":3}`:                     "",
		`not-json`:                        "",
	}
	for arguments, expected := range cases {
		if got := resourceOf(json.RawMessage(arguments)); got != expected {
			t.Fatalf("%s => %q, want %q", arguments, got, expected)
		}
	}
}

func TestToolNamesSorted(t *testing.T) {
	f := newFixture(t)
	names, err := ToolNames(f.registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 5 {
		t.Fatalf("names = %v", names)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("not sorted: %v", names)
		}
	}
	if names[0] != "workspace.list" && names[0] != "workspace.mkdir" && names[0] != "workspace.move_to_trash" {
		t.Fatalf("unexpected first tool: %s", names[0])
	}
}

func TestProcessToolAbsentWithoutRunner(t *testing.T) {
	f := newFixture(t)
	names, err := ToolNames(f.registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name == "process.run" {
			t.Fatal("process.run registered without a runner")
		}
	}
}

func TestToolDurationIsMeasured(t *testing.T) {
	f := newFixture(t)
	f.writeFile(t, "a.md", "a")
	cfg := f.config(&scriptedModel{steps: []func(model.ToolRequest) model.Completion{
		func(model.ToolRequest) model.Completion {
			return toolCompletion("c1", "workspace.list", `{"path":"."}`)
		},
		func(model.ToolRequest) model.Completion { return answerCompletion("done") },
	}})
	ticks := 0
	cfg.Clock = func() time.Time {
		ticks++
		return time.Unix(0, 0).Add(time.Duration(ticks) * 10 * time.Millisecond)
	}
	outcome, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.ToolEvents) != 1 || outcome.ToolEvents[0].Duration != 10*time.Millisecond {
		t.Fatalf("duration = %v", outcome.ToolEvents)
	}
}
