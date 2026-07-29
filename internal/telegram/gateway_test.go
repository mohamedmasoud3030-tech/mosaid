package telegram

import (
	"context"
	"errors"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/health"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"log/slog"
	"testing"
)

type fake struct {
	updates []message.Inbound
	sent    []message.Outbound
	n       int
}

func (f *fake) Updates(ctx context.Context, o int64, t int) ([]message.Inbound, error) {
	if f.n > 0 {
		return nil, context.Canceled
	}
	f.n++
	return f.updates, nil
}
func (f *fake) Send(_ context.Context, m message.Outbound) error {
	f.sent = append(f.sent, m)
	return nil
}

type hnd struct{ n int }

func (h *hnd) Handle(_ context.Context, m message.Inbound) (message.Outbound, error) {
	h.n++
	return message.Outbound{ChatID: m.ChatID, Text: "ok"}, nil
}
func TestOwnerPrivateOnly(t *testing.T) {
	f := &fake{updates: []message.Inbound{{UpdateID: 1, ChatID: 1, UserID: 2, ChatType: "private"}, {UpdateID: 2, ChatID: -1, UserID: 1, ChatType: "group"}, {UpdateID: 3, ChatID: 1, UserID: 1, ChatType: "private"}}}
	h := &hnd{}
	g := Gateway{Client: f, Handler: h, Owner: 1, PollTimeout: 1, Log: slog.Default(), Health: health.New(t.TempDir(), "v")}
	if e := g.Run(context.Background()); e != nil && !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if h.n != 1 {
		t.Fatalf("handled=%d", h.n)
	}
	if len(f.sent) != 2 {
		t.Fatalf("sent=%d", len(f.sent))
	}
}
