package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/security"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceBoundaryAndAtomicWrite(t *testing.T) {
	root := t.TempDir()
	w := NewWorkspace(root)
	if _, e := w.Write(context.Background(), json.RawMessage(`{"Path":"../x","Content":"bad"}`)); e == nil {
		t.Fatal("traversal")
	}
	if _, e := w.Write(context.Background(), json.RawMessage(`{"Path":".env","Content":"bad"}`)); e == nil {
		t.Fatal("secret")
	}
	if _, e := w.Write(context.Background(), json.RawMessage(`{"Path":"a.txt","Content":"hello"}`)); e != nil {
		t.Fatal(e)
	}
	v, e := w.Read(context.Background(), json.RawMessage(`{"path":"a.txt"}`))
	if e != nil || v != "hello" {
		t.Fatal(v, e)
	}
	os.Symlink("/tmp", filepath.Join(root, "link"))
	if _, e = w.Write(context.Background(), json.RawMessage(`{"Path":"link/x","Content":"bad"}`)); e == nil {
		t.Fatal("symlink escape")
	}
	if _, e = w.Trash(context.Background(), json.RawMessage(`{"Path":"a.txt"}`)); e != nil {
		t.Fatal(e)
	}
}
func TestProcessProfiles(t *testing.T) {
	p := &ProcessRunner{Workspace: t.TempDir(), Profiles: DefaultProfiles(), MaxOutput: 1024}
	if _, e := p.Run(context.Background(), json.RawMessage(`{"profile":"go","argv":["version"]}`)); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Run(context.Background(), json.RawMessage(`{"profile":"go","argv":["env"]}`)); e == nil {
		t.Fatal("subcommand")
	}
	if _, e := p.Run(context.Background(), json.RawMessage(`{"profile":"sh","argv":["-c","id"]}`)); e == nil {
		t.Fatal("shell")
	}
}
func TestRegistryEnforcesContextToolBudget(t *testing.T) {
	registry := NewRegistry(nil)
	if err := registry.Register(Registered{Spec: policy.Tool{Name: "read", Version: "1", Risk: policy.Safe, Modes: []policy.Mode{policy.Read}, Timeout: time.Second, OutputLimit: 10}, Run: func(context.Context, json.RawMessage) (any, error) { return "ok", nil }}); err != nil {
		t.Fatal(err)
	}
	budget, _ := security.NewBudget(security.BudgetLimits{ModelSteps: 1, ToolCalls: 1, Tokens: 1, CostUSD: 1, Retries: 1})
	ctx := security.WithBudget(context.Background(), budget)
	request := Request{Name: "read", Arguments: json.RawMessage(`{}`), Mode: policy.Read}
	if _, err := registry.Execute(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(ctx, request); !errors.Is(err, security.ErrBudgetExceeded) {
		t.Fatalf("err=%v", err)
	}
}

func TestRegistryApprovalBinding(t *testing.T) {
	d, e := storage.Open(filepath.Join(t.TempDir(), "d.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	am := &approval.Manager{DB: d.SQL(), Audit: audit.Logger{DB: d.SQL()}}
	r := NewRegistry(am)
	if e = r.Register(Registered{Spec: policy.Tool{Name: "write", Version: "1", Risk: policy.High, Modes: []policy.Mode{policy.Write}, Timeout: time.Second, OutputLimit: 10}, Run: func(context.Context, json.RawMessage) (any, error) { return "ok", nil }}); e != nil {
		t.Fatal(e)
	}
	args := json.RawMessage(`{"x":1}`)
	res, e := r.Execute(context.Background(), Request{Name: "write", Arguments: args, Mode: policy.Write, UserID: 1, Resource: "r"})
	if e != nil || res.Approval == nil {
		t.Fatal(e)
	}
	token := res.Approval.Token
	if e = am.ResolveToken(context.Background(), token, 1, "approved"); e != nil {
		t.Fatal(e)
	}
	res, e = r.Execute(context.Background(), Request{Name: "write", Arguments: args, Mode: policy.Write, UserID: 1, Resource: "r", ApprovalToken: token})
	if e != nil || res.Value != "ok" {
		t.Fatal(res, e)
	}
	if _, e = r.Execute(context.Background(), Request{Name: "write", Arguments: args, Mode: policy.Write, UserID: 1, Resource: "r", ApprovalToken: token}); e == nil {
		t.Fatal("reuse")
	}
}
