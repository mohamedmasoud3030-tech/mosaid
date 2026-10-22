package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/memory"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
)

// MemoryStore is the durable, owner-approved memory used by the memory tools.
type MemoryStore interface {
	Search(context.Context, string, int) ([]memory.Item, error)
	Remember(context.Context, string) (int64, error)
}

type memoryTools struct{ store MemoryStore }

// RegisterMemory exposes the owner's long-term memory to the model. Reading is
// safe; writing is low risk because recall is reversible through /forget and
// secret-like content is rejected by the store itself.
func RegisterMemory(r *Registry, store MemoryStore) error {
	if store == nil {
		return errors.New("memory store required")
	}
	handlers := memoryTools{store: store}
	specs := []Registered{
		{
			Spec: policy.Tool{
				Name: "memory.search", Version: "1", Risk: policy.Safe,
				Modes:       []policy.Mode{policy.Read, policy.Write, policy.Admin},
				Description: "Search the owner's long-term memory notes with full-text search and return the best matches. Use it before asking the owner to repeat stable facts, preferences, or decisions.",
				InputSchema: schema(map[string]map[string]any{
					"query": stringProperty("Full-text search terms. Use plain words, not SQL."),
					"limit": integerProperty("Maximum number of notes to return.", 1, 20),
				}, "query"),
				Timeout: 5 * time.Second, OutputLimit: 64 << 10, Idempotency: policy.Idempotent,
			},
			Run: handlers.search,
		},
		{
			Spec: policy.Tool{
				Name: "memory.remember", Version: "1", Risk: policy.Low,
				Modes:       []policy.Mode{policy.Write, policy.Admin},
				Description: "Store one durable fact, preference, or decision in the owner's long-term memory. Only store information the owner would want recalled later; never store secrets, credentials, or guesses.",
				InputSchema: schema(map[string]map[string]any{
					"content": stringProperty("One self-contained sentence worth recalling later."),
				}, "content"),
				Timeout: 5 * time.Second, OutputLimit: 4096, Idempotency: policy.AtLeastOnce,
			},
			Run: handlers.remember,
		},
	}
	for _, x := range specs {
		if err := r.Register(x); err != nil {
			return err
		}
	}
	return nil
}

func (m memoryTools) search(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, errors.New("bad args")
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return nil, errors.New("query required")
	}
	items, err := m.store.Search(ctx, args.Query, args.Limit)
	if err != nil {
		return nil, err
	}
	type hit struct {
		ID      int64  `json:"id"`
		Content string `json:"content"`
		Source  string `json:"source"`
		Created string `json:"created"`
	}
	hits := make([]hit, 0, len(items))
	for _, item := range items {
		hits = append(hits, hit{ID: item.ID, Content: item.Content, Source: item.Source, Created: item.Created})
	}
	return map[string]any{"count": len(hits), "items": hits}, nil
}

func (m memoryTools) remember(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, errors.New("bad args")
	}
	args.Content = strings.TrimSpace(args.Content)
	if args.Content == "" {
		return nil, errors.New("content required")
	}
	if len(args.Content) > 2000 {
		return nil, errors.New("content too large")
	}
	id, err := m.store.Remember(ctx, args.Content)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "stored": true}, nil
}
