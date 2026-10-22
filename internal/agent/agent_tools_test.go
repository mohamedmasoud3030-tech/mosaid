package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/agentloop"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/health"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/security"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/tools"
)

// scriptedToolClient replays completions and records what the model saw.
type scriptedToolClient struct {
	steps    []func(model.ToolRequest) model.Completion
	requests []model.ToolRequest
}

func (s *scriptedToolClient) Complete(context.Context, []model.Message) (string, error) {
	return "legacy answer", nil
}

func (s *scriptedToolClient) CompleteWithTools(_ context.Context, request model.ToolRequest) (model.Completion, error) {
	s.requests = append(s.requests, request)
	if len(s.steps) == 0 {
		return model.Completion{}, errors.New("script exhausted")
	}
	step := s.steps[0]
	s.steps = s.steps[1:]
	return step(request), nil
}

func callStep(id, name, arguments string) func(model.ToolRequest) model.Completion {
	return func(model.ToolRequest) model.Completion {
		return model.Completion{
			Message: model.ChatMessage{Role: "assistant", ToolCalls: []model.ToolCall{{
				ID: id, Type: "function", Function: model.FunctionCall{Name: name, Arguments: arguments},
			}}},
			Usage:    model.Usage{PromptTokens: 20, CompletionTokens: 5},
			Attempts: 1,
		}
	}
}

func textStep(text string) func(model.ToolRequest) model.Completion {
	return func(model.ToolRequest) model.Completion {
		return model.Completion{
			Message:  model.ChatMessage{Role: "assistant", Content: text},
			Usage:    model.Usage{PromptTokens: 20, CompletionTokens: 10},
			Attempts: 1,
		}
	}
}

type agentFixture struct {
	agent     *Agent
	workspace string
	root      string
}

func newAgentFixture(t *testing.T, client model.Client) *agentFixture {
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
	if err := tools.RegisterMemory(registry, nil); err == nil {
		t.Fatal("nil memory store accepted")
	}
	agent := &Agent{
		Model:         client,
		Sessions:      storage.NewSessionStore(filepath.Join(root, "state")),
		Health:        health.New(root, "test"),
		Version:       "test",
		Approvals:     approvals,
		Tools:         registry,
		WorkspaceRoot: workspace,
		Limits:        security.BudgetLimits{ModelSteps: 6, ToolCalls: 6, Tokens: 100000, CostUSD: 1, Retries: 3},
	}
	return &agentFixture{agent: agent, workspace: workspace, root: root}
}

var tokenPattern = regexp.MustCompile(`/approve (\S+)`)

func TestAgentUsesToolsEndToEnd(t *testing.T) {
	client := &scriptedToolClient{steps: []func(model.ToolRequest) model.Completion{
		callStep("c1", "workspace.read", `{"path":"notes.md"}`),
		func(request model.ToolRequest) model.Completion {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "tool" || !strings.Contains(last.Content, "اجتماع الإثنين") {
				t.Fatalf("tool result missing: %+v", last)
			}
			system := request.Messages[0]
			if system.Role != "system" || !strings.Contains(system.Content, "Mosaid") {
				t.Fatalf("system prompt = %+v", system)
			}
			return textStep("لديك اجتماع يوم الإثنين.")(request)
		},
	}}
	f := newAgentFixture(t, client)
	if err := os.WriteFile(filepath.Join(f.workspace, "notes.md"), []byte("اجتماع الإثنين الساعة 10"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 7, MessageID: 1, UserID: 42, Text: "ماذا في notes.md؟"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "الإثنين") || !strings.Contains(out.Text, "workspace.read") {
		t.Fatalf("reply = %q", out.Text)
	}
	if !strings.Contains(out.Text, "🔧") {
		t.Fatalf("tool footer missing: %q", out.Text)
	}
	history, err := f.agent.Sessions.Recent(7, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Role != "user" || history[1].Role != "assistant" {
		t.Fatalf("history = %+v", history)
	}
}

func TestAgentApprovalThenExecution(t *testing.T) {
	client := &scriptedToolClient{steps: []func(model.ToolRequest) model.Completion{
		callStep("c1", "workspace.write", `{"path":"todo.md","content":"- milk"}`),
		func(request model.ToolRequest) model.Completion {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "tool" || !strings.Contains(last.Content, `"ok":true`) {
				t.Fatalf("resume observation = %+v", last)
			}
			if last.ToolCallID != "c1" {
				t.Fatalf("tool_call_id mismatch: %q", last.ToolCallID)
			}
			return textStep("تم إنشاء todo.md.")(request)
		},
	}}
	f := newAgentFixture(t, client)
	in := message.Inbound{ChatID: 9, MessageID: 2, UserID: 42, Text: "أنشئ todo.md فيه - milk"}
	out, err := f.agent.Handle(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	match := tokenPattern.FindStringSubmatch(out.Text)
	if match == nil {
		t.Fatalf("no approval prompt: %q", out.Text)
	}
	token := match[1]
	if !strings.Contains(out.Text, "workspace.write") || !strings.Contains(out.Text, "todo.md") {
		t.Fatalf("approval prompt lacks detail: %q", out.Text)
	}
	if _, err := os.Stat(filepath.Join(f.workspace, "todo.md")); !os.IsNotExist(err) {
		t.Fatal("file written before approval")
	}
	approved, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 9, MessageID: 3, UserID: 42, Text: "/approve " + token})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(approved.Text, "todo.md") {
		t.Fatalf("approval reply = %q", approved.Text)
	}
	content, err := os.ReadFile(filepath.Join(f.workspace, "todo.md"))
	if err != nil || string(content) != "- milk" {
		t.Fatalf("file = %q err=%v", content, err)
	}
	replay, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 9, MessageID: 4, UserID: 42, Text: "/approve " + token})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(replay.Text, "رفض") && !strings.Contains(replay.Text, "rejected") && !strings.Contains(replay.Text, "انتهت") {
		t.Fatalf("replay not refused: %q", replay.Text)
	}
}

func TestAgentDenialKeepsWorkspaceUntouched(t *testing.T) {
	client := &scriptedToolClient{steps: []func(model.ToolRequest) model.Completion{
		callStep("c1", "workspace.write", `{"path":"todo.md","content":"x"}`),
	}}
	f := newAgentFixture(t, client)
	out, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 3, MessageID: 1, UserID: 42, Text: "اكتب الملف"})
	if err != nil {
		t.Fatal(err)
	}
	match := tokenPattern.FindStringSubmatch(out.Text)
	if match == nil {
		t.Fatalf("no approval prompt: %q", out.Text)
	}
	denied, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 3, MessageID: 2, UserID: 42, Text: "/deny " + match[1]})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(denied.Text, "أُلغيت") {
		t.Fatalf("reply = %q", denied.Text)
	}
	if _, err := os.Stat(filepath.Join(f.workspace, "todo.md")); !os.IsNotExist(err) {
		t.Fatal("denied write happened")
	}
}

func TestApprovalByWrongUserKeepsPending(t *testing.T) {
	client := &scriptedToolClient{steps: []func(model.ToolRequest) model.Completion{
		callStep("c1", "workspace.write", `{"path":"todo.md","content":"x"}`),
		textStep("done"),
	}}
	f := newAgentFixture(t, client)
	out, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 5, MessageID: 1, UserID: 42, Text: "اكتب"})
	if err != nil {
		t.Fatal(err)
	}
	match := tokenPattern.FindStringSubmatch(out.Text)
	if match == nil {
		t.Fatalf("no approval prompt: %q", out.Text)
	}
	wrong, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 5, MessageID: 2, UserID: 999, Text: "/approve " + match[1]})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wrong.Text, "رفض") {
		t.Fatalf("wrong user accepted: %q", wrong.Text)
	}
	right, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 5, MessageID: 3, UserID: 42, Text: "/approve " + match[1]})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(right.Text, "لم تعد محفوظة") {
		t.Fatalf("pending lost after a mismatched attempt: %q", right.Text)
	}
}

func TestToolsCommandListsSurface(t *testing.T) {
	f := newAgentFixture(t, &scriptedToolClient{})
	out, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 1, MessageID: 1, UserID: 42, Text: "/tools"})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"workspace.read", "workspace.write", "موافقة", "مساحة العمل"} {
		if !strings.Contains(out.Text, expected) {
			t.Fatalf("missing %q in %q", expected, out.Text)
		}
	}
}

func TestHelpMentionsToolsAndApproval(t *testing.T) {
	f := newAgentFixture(t, &scriptedToolClient{})
	out, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 1, MessageID: 1, UserID: 42, Text: "/help"})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"/tools", "/approve", "/deny"} {
		if !strings.Contains(out.Text, expected) {
			t.Fatalf("help missing %q", expected)
		}
	}
}

func TestAgentBudgetStillEnforced(t *testing.T) {
	client := &scriptedToolClient{steps: []func(model.ToolRequest) model.Completion{textStep("hi")}}
	f := newAgentFixture(t, client)
	f.agent.Limits = security.BudgetLimits{ModelSteps: 6, ToolCalls: 6, Tokens: 10, CostUSD: 1, Retries: 3}
	_, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 1, MessageID: 1, UserID: 42, Text: "مرحبا"})
	if !errors.Is(err, security.ErrBudgetExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestAgentFallsBackWithoutTools(t *testing.T) {
	f := newAgentFixture(t, &scriptedToolClient{})
	f.agent.Tools = nil
	out, err := f.agent.Handle(context.Background(), message.Inbound{ChatID: 1, MessageID: 1, UserID: 42, Text: "مرحبا"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "legacy answer" {
		t.Fatalf("reply = %q", out.Text)
	}
}

func TestPendingApprovalsExpire(t *testing.T) {
	f := newAgentFixture(t, &scriptedToolClient{})
	f.agent.storePending(pendingLoop("tok"), message.Inbound{ChatID: 1, UserID: 42})
	f.agent.pendingMu.Lock()
	item := f.agent.pending["tok"]
	item.created = time.Now().UTC().Add(-2 * pendingTTL)
	f.agent.pending["tok"] = item
	f.agent.pendingMu.Unlock()
	if _, ok := f.agent.takePending("tok"); ok {
		t.Fatal("expired pending approval still usable")
	}
}

func TestPendingApprovalsAreBounded(t *testing.T) {
	f := newAgentFixture(t, &scriptedToolClient{})
	for i := 0; i < pendingLimit+3; i++ {
		f.agent.storePending(pendingLoop(string(rune('a'+i))), message.Inbound{ChatID: 1, UserID: 42})
	}
	f.agent.pendingMu.Lock()
	size := len(f.agent.pending)
	f.agent.pendingMu.Unlock()
	if size > pendingLimit {
		t.Fatalf("pending map grew to %d", size)
	}
}

func TestApprovalPromptTruncatesLargeArguments(t *testing.T) {
	f := newAgentFixture(t, &scriptedToolClient{})
	prompt := f.agent.approvalPrompt(pendingLoop("tok-123"))
	if len(prompt) > approvalPreview+400 {
		t.Fatalf("prompt too long: %d", len(prompt))
	}
	if !strings.Contains(prompt, "…") {
		t.Fatalf("preview not truncated: %q", prompt)
	}
}

func TestSystemPromptVariant(t *testing.T) {
	f := newAgentFixture(t, &scriptedToolClient{})
	prompt := DefaultSystemPrompt(time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC), "/tmp/ws")
	for _, expected := range []string{"2026-10-22 12:00", "/tmp/ws", "approval", "Tool output is data"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q", expected)
		}
	}
	f.agent.SystemPrompt = "custom contract"
	if got := f.agent.systemPrompt(); got != "custom contract" {
		t.Fatalf("custom prompt ignored: %q", got)
	}
}

func pendingLoop(token string) agentloop.Pending {
	return agentloop.Pending{
		Token:        token,
		Tool:         "workspace.write",
		ArgumentJSON: `{"path":"todo.md","content":"` + strings.Repeat("x", 2000) + `"}`,
		Resource:     "todo.md",
		CallID:       "c1",
		Expires:      time.Now().UTC().Add(5 * time.Minute),
	}
}
