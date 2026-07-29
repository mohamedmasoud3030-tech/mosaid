package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"sync"
	"time"
)

type Handler func(context.Context, json.RawMessage) (any, error)
type Registered struct {
	Spec policy.Tool
	Run  Handler
}
type Registry struct {
	mu        sync.RWMutex
	tools     map[string]Registered
	Approvals *approval.Manager
}
type Request struct {
	Name          string
	Arguments     json.RawMessage
	Mode          policy.Mode
	UserID        int64
	Resource      string
	ApprovalToken string
}
type Result struct {
	Value    any
	Approval *approval.Request
}

type ExecutionMetadata struct {
	Name     string
	Mode     policy.Mode
	UserID   int64
	Resource string
	ArgsHash string
	Approval *approval.Receipt
}

type executionMetadataKey struct{}

func Metadata(ctx context.Context) (ExecutionMetadata, bool) {
	value, ok := ctx.Value(executionMetadataKey{}).(ExecutionMetadata)
	return value, ok
}

func NewRegistry(a *approval.Manager) *Registry {
	return &Registry{tools: map[string]Registered{}, Approvals: a}
}
func (r *Registry) Register(x Registered) error {
	if err := policy.Validate(x.Spec); err != nil {
		return err
	}
	if x.Run == nil {
		return errors.New("handler required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[x.Spec.Name]; ok {
		return errors.New("duplicate tool")
	}
	r.tools[x.Spec.Name] = x
	return nil
}
func (r *Registry) Execute(ctx context.Context, q Request) (Result, error) {
	r.mu.RLock()
	x, ok := r.tools[q.Name]
	r.mu.RUnlock()
	if !ok {
		return Result{}, errors.New("tool not registered")
	}
	d := policy.Evaluate(x.Spec, q.Mode)
	if !d.Allowed && !d.NeedsApproval {
		return Result{}, errors.New("policy denied: " + d.Reason)
	}
	h := sha256.Sum256(q.Arguments)
	argsHash := hex.EncodeToString(h[:])
	metadata := ExecutionMetadata{Name: q.Name, Mode: q.Mode, UserID: q.UserID, Resource: q.Resource, ArgsHash: argsHash}
	if d.NeedsApproval {
		if r.Approvals == nil {
			return Result{}, errors.New("approval service unavailable")
		}
		if q.ApprovalToken == "" {
			a, e := r.Approvals.Create(ctx, q.UserID, q.Name, argsHash, q.Resource, 5*time.Minute)
			return Result{Approval: &a}, e
		}
		receipt, e := r.Approvals.AuthorizeReceipt(ctx, q.ApprovalToken, q.UserID, q.Name, argsHash, q.Resource)
		if e != nil {
			return Result{}, e
		}
		metadata.Approval = &receipt
	}
	ctx = context.WithValue(ctx, executionMetadataKey{}, metadata)
	cctx, cancel := context.WithTimeout(ctx, x.Spec.Timeout)
	defer cancel()
	v, e := x.Run(cctx, q.Arguments)
	return Result{Value: v}, e
}
