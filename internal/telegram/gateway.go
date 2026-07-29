package telegram

import (
	"context"
	"errors"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/health"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"log/slog"
	"math"
	"time"
)

type Handler interface {
	Handle(context.Context, message.Inbound) (message.Outbound, error)
}
type Gateway struct {
	Client      Client
	Handler     Handler
	Owner       int64
	PollTimeout int
	Log         *slog.Logger
	Health      *health.Writer
}

func (g *Gateway) Run(ctx context.Context) error {
	var offset int64
	failures := 0
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
			out, e := g.Handler.Handle(ctx, in)
			if e != nil {
				g.Log.Error("message failed", "update_id", in.UpdateID, "error", e.Error())
				out = message.Outbound{ChatID: in.ChatID, ReplyTo: in.MessageID, Text: "Request failed safely."}
			}
			if out.Text != "" {
				if e = g.Client.Send(ctx, out); e != nil {
					g.Log.Error("send failed", "update_id", in.UpdateID, "error", e.Error())
				}
			}
			_ = g.Health.Update(func(s *health.State) { s.Messages++ })
		}
	}
}
