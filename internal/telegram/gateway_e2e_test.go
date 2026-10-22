package telegram

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/agent"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/health"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/memory"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/observability"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/security"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/tools"
)

// fakeTelegram is an in-memory Telegram transport: it hands updates to the
// gateway and records what the gateway would have sent to the owner.
type fakeTelegram struct {
	mu      sync.Mutex
	updates []message.Inbound
	sent    []message.Outbound
}

func (f *fakeTelegram) Updates(context.Context, int64, int) ([]message.Inbound, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	updates := f.updates
	f.updates = nil
	return updates, nil
}

func (f *fakeTelegram) Send(_ context.Context, out message.Outbound) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, out)
	return nil
}

func (f *fakeTelegram) replies() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	replies := make([]string, 0, len(f.sent))
	for _, out := range f.sent {
		replies = append(replies, out.Text)
	}
	return replies
}

// e2eModel asks for one write, then answers from the tool observation.
type e2eModel struct {
	wrote bool
}

func (m *e2eModel) Complete(context.Context, []model.Message) (string, error) {
	return "", errors.New("legacy path must not be used when tools are enabled")
}

func (m *e2eModel) CompleteWithTools(_ context.Context, request model.ToolRequest) (model.Completion, error) {
	last := request.Messages[len(request.Messages)-1]
	switch {
	case last.Role == "tool":
		m.wrote = strings.Contains(last.Content, `"ok":true`)
		return model.Completion{
			Message:  model.ChatMessage{Role: "assistant", Content: "تم إنشاء الملف ✅"},
			Usage:    model.Usage{PromptTokens: 30, CompletionTokens: 8},
			Attempts: 1,
		}, nil
	case last.Role == "user":
		return model.Completion{
			Message: model.ChatMessage{Role: "assistant", ToolCalls: []model.ToolCall{{
				ID: "call_1", Type: "function",
				Function: model.FunctionCall{Name: "workspace.write", Arguments: `{"path":"todo.md","content":"- milk"}`},
			}}},
			Usage:    model.Usage{PromptTokens: 40, CompletionTokens: 12},
			Attempts: 1,
		}, nil
	}
	return model.Completion{}, errors.New("unexpected conversation")
}

// TestGatewayApprovalRoundTrip drives the real gateway, the real durable inbox
// and outbox, the real policy and approval engine, and the real workspace tools
// without any network access. It is the credential-free end-to-end proof that
// an owner can ask for a file, approve it, and get the file written.
func TestGatewayApprovalRoundTrip(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(filepath.Join(root, "mosaid.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	approvals := &approval.Manager{DB: db.SQL(), Audit: audit.Logger{DB: db.SQL()}}
	memoryStore := &memory.Store{DB: db.SQL()}
	registry := tools.NewRegistry(approvals)
	if err := tools.RegisterWorkspace(registry, tools.NewWorkspace(workspace), nil); err != nil {
		t.Fatal(err)
	}
	if err := tools.RegisterMemory(registry, memoryStore); err != nil {
		t.Fatal(err)
	}
	modelClient := &e2eModel{}
	owner := &agent.Agent{
		Model: modelClient, Sessions: storage.NewSessionStore(filepath.Join(root, "state")),
		Health: health.New(root, "e2e"), Version: "e2e", Approvals: approvals, Memory: memoryStore,
		Tools: registry, WorkspaceRoot: workspace, Mode: policy.Admin,
		Limits: security.BudgetLimits{ModelSteps: 6, ToolCalls: 6, Tokens: 100000, CostUSD: 1, Retries: 3},
	}
	guard, err := security.NewMessageGuard(security.SystemClock{}, 16384, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	transport := &fakeTelegram{}
	log := observability.New(io.Discard, observability.NewRedactor("token"))
	gateway := &Gateway{
		Client: transport, Handler: owner, Owner: 42, PollTimeout: 1,
		Log: log, Health: health.New(root, "e2e"), Store: db, Guard: guard, MaxAttempts: 3,
	}

	// Turn 1: the owner asks for a file. Nothing may be written yet.
	first := message.Inbound{UpdateID: 100, MessageID: 1, ChatID: 42, UserID: 42, ChatType: "private", Text: "أنشئ todo.md فيه - milk"}
	if inserted, err := db.Ingest(ctx, first, "hash-1"); err != nil || !inserted {
		t.Fatalf("ingest: %v %v", inserted, err)
	}
	gateway.drainInbox(ctx)
	gateway.drainOutbox(ctx)
	replies := transport.replies()
	if len(replies) != 1 {
		t.Fatalf("replies = %v", replies)
	}
	match := regexp.MustCompile(`/approve (\S+)`).FindStringSubmatch(replies[0])
	if match == nil {
		t.Fatalf("no approval prompt: %q", replies[0])
	}
	if _, err := os.Stat(filepath.Join(workspace, "todo.md")); !os.IsNotExist(err) {
		t.Fatal("file written before owner approval")
	}
	// The prompt must be durable: a restart between prompt and approval keeps
	// the outbox honest, and the approval row lives in SQLite.
	var pendingRows int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM approval_requests WHERE state='pending'`).Scan(&pendingRows); err != nil || pendingRows != 1 {
		t.Fatalf("pending approvals = %d err=%v", pendingRows, err)
	}

	// Turn 2: the owner approves by replying with the token.
	second := message.Inbound{UpdateID: 101, MessageID: 2, ChatID: 42, UserID: 42, ChatType: "private", Text: "/approve " + match[1]}
	if inserted, err := db.Ingest(ctx, second, "hash-2"); err != nil || !inserted {
		t.Fatalf("ingest: %v %v", inserted, err)
	}
	gateway.drainInbox(ctx)
	gateway.drainOutbox(ctx)
	replies = transport.replies()
	if len(replies) != 2 {
		t.Fatalf("replies = %v", replies)
	}
	if !strings.Contains(replies[1], "✅") {
		t.Fatalf("approval reply = %q", replies[1])
	}
	content, err := os.ReadFile(filepath.Join(workspace, "todo.md"))
	if err != nil || string(content) != "- milk" {
		t.Fatalf("file = %q err=%v", content, err)
	}
	if !modelClient.wrote {
		t.Fatal("model never observed the successful tool result")
	}

	// Replaying the Telegram update must not create a second write or reply.
	if inserted, err := db.Ingest(ctx, second, "hash-2"); err != nil || inserted {
		t.Fatalf("duplicate accepted: %v %v", inserted, err)
	}
	gateway.drainInbox(ctx)
	gateway.drainOutbox(ctx)
	if got := len(transport.replies()); got != 2 {
		t.Fatalf("duplicate produced a reply: %d", got)
	}
}
