package telegram

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/health"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/security"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
)

type Handler interface {
	Handle(context.Context, message.Inbound) (message.Outbound, error)
}
type MessageGuard interface {
	Allow(int64, string) error
}
type Gateway struct {
	Client      Client
	Handler     Handler
	Owner       int64
	PollTimeout int
	Log         *slog.Logger
	Health      *health.Writer
	Store       *storage.DB
	Guard       MessageGuard
	MaxAttempts int
}

func (g *Gateway) Run(ctx context.Context) error {
	if g.Client == nil || g.Handler == nil || g.Log == nil || g.Health == nil || g.Guard == nil || g.Owner <= 0 {
		return errors.New("telegram gateway dependencies unavailable")
	}
	var offset int64
	failures := 0
	if g.MaxAttempts <= 0 {
		g.MaxAttempts = 5
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		updates, err := g.Client.Updates(ctx, offset, g.PollTimeout)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			failures++
			d := time.Duration(math.Min(60, math.Pow(2, float64(failures)))) * time.Second
			g.Log.Warn("telegram poll failed", "backoff", d, "error", err.Error())
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(d):
				continue
			}
		}
		failures = 0
		for _, in := range updates {
			if in.UpdateID >= offset {
				offset = in.UpdateID + 1
			}
			_ = g.Health.Update(func(s *health.State) { s.LastUpdateID = in.UpdateID })
			if in.ChatType != "private" {
				g.Log.Warn("group rejected", "update_id", in.UpdateID)
				continue
			}
			if in.UserID != g.Owner {
				_ = g.Client.Send(ctx, message.Outbound{ChatID: in.ChatID, Text: "Access denied."})
				g.Log.Warn("unauthorized user rejected", "update_id", in.UpdateID)
				continue
			}
			if err := g.Guard.Allow(in.UserID, in.Text); err != nil {
				_ = g.Client.Send(ctx, message.Outbound{ChatID: in.ChatID, Text: "Request rate limited."})
				g.Log.Warn("telegram message limited", "update_id", in.UpdateID)
				continue
			}
			if g.Store == nil {
				out, e := g.Handler.Handle(ctx, in)
				if e == nil && out.Text != "" {
					_ = g.Client.Send(ctx, out)
				}
				continue
			}
			h := sha256.Sum256([]byte(strconv.FormatInt(in.UpdateID, 10) + ":" + strconv.FormatInt(in.MessageID, 10) + ":" + in.Text))
			inserted, e := g.Store.Ingest(ctx, in, hex.EncodeToString(h[:]))
			if e != nil {
				g.Log.Error("inbox ingest failed", "error", e.Error())
				continue
			}
			if !inserted {
				g.Log.Info("duplicate update ignored", "update_id", in.UpdateID)
			}
		}
		if g.Store != nil {
			g.drainInbox(ctx)
			g.drainOutbox(ctx)
		}
	}
}
func (g *Gateway) drainInbox(ctx context.Context) {
	for {
		in, e := g.Store.ClaimInbox(ctx, g.MaxAttempts)
		if e != nil {
			g.Log.Error("claim inbox", "error", e.Error())
			return
		}
		if in == nil {
			return
		}
		out, e := g.Handler.Handle(ctx, in.Message)
		if e != nil {
			if errors.Is(e, security.ErrBudgetExceeded) {
				key := "telegram-reply:" + strconv.FormatInt(in.Message.UpdateID, 10)
				reply := message.Outbound{ChatID: in.Message.ChatID, ReplyTo: in.Message.MessageID, Text: "Request rejected: execution budget exceeded."}
				if cerr := g.Store.CompleteWithOutbox(ctx, in.ID, reply, key); cerr != nil {
					_ = g.Store.FailInbox(ctx, in.ID, cerr, g.MaxAttempts)
				}
				g.Log.Warn("request budget exceeded", "update_id", in.Message.UpdateID)
				continue
			}
			_ = g.Store.FailInbox(ctx, in.ID, e, g.MaxAttempts)
			continue
		}
		key := "telegram-reply:" + strconv.FormatInt(in.Message.UpdateID, 10)
		if e = g.Store.CompleteWithOutbox(ctx, in.ID, out, key); e != nil {
			_ = g.Store.FailInbox(ctx, in.ID, e, g.MaxAttempts)
		}
	}
}
func (g *Gateway) drainOutbox(ctx context.Context) {
	for {
		o, e := g.Store.ClaimOutbox(ctx)
		if e != nil {
			g.Log.Error("claim outbox", "error", e.Error())
			return
		}
		if o == nil {
			return
		}
		if e = g.Client.Send(ctx, o.Message); e != nil {
			_ = g.Store.OutboxFailed(ctx, o.ID, e, g.MaxAttempts)
			return
		}
		_ = g.Store.OutboxSent(ctx, o.ID)
		_ = g.Health.Update(func(s *health.State) { s.Messages++ })
	}
}
