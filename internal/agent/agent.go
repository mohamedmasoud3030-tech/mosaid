package agent

import (
	"context"
	"fmt"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/health"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/memory"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Agent struct {
	Model     model.Client
	Sessions  *storage.SessionStore
	Health    *health.Writer
	Version   string
	Approvals *approval.Manager
	Memory    *memory.Store
	mu        sync.Mutex
	cancel    context.CancelFunc
}

func (a *Agent) Handle(ctx context.Context, in message.Inbound) (message.Outbound, error) {
	text := strings.TrimSpace(in.Text)
	out := message.Outbound{ChatID: in.ChatID, ReplyTo: in.MessageID}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return out, nil
	}
	switch fields[0] {
	case "/help":
		out.Text = "Mosaid commands: /status /help /stop /approve /deny /memory <query> /remember <text> /forget <id> /export"
		return out, nil
	case "/approve", "/deny":
		if a.Approvals == nil || len(fields) != 2 {
			out.Text = "Approval command requires one token."
			return out, nil
		}
		decision := "approved"
		if fields[0] == "/deny" {
			decision = "denied"
		}
		if err := a.Approvals.ResolveToken(ctx, fields[1], in.UserID, decision); err != nil {
			out.Text = "Approval rejected: " + err.Error()
			return out, nil
		}
		out.Text = "Approval " + decision + "."
		return out, nil
	case "/memory":
		if a.Memory == nil || len(fields) < 2 {
			out.Text = "Usage: /memory <query>"
			return out, nil
		}
		items, err := a.Memory.Search(ctx, strings.Join(fields[1:], " "), 10)
		if err != nil {
			return out, err
		}
		if len(items) == 0 {
			out.Text = "No memory found."
		} else {
			var b strings.Builder
			for _, m := range items {
				fmt.Fprintf(&b, "#%d %s\n", m.ID, m.Content)
			}
			out.Text = strings.TrimSpace(b.String())
		}
		return out, nil
	case "/remember":
		if a.Memory == nil || len(fields) < 2 {
			out.Text = "Usage: /remember <text>"
			return out, nil
		}
		id, err := a.Memory.Remember(ctx, strings.Join(fields[1:], " "))
		if err != nil {
			return out, err
		}
		out.Text = fmt.Sprintf("Remembered #%d.", id)
		return out, nil
	case "/forget":
		if a.Memory == nil || len(fields) != 2 {
			out.Text = "Usage: /forget <id>"
			return out, nil
		}
		id, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return out, err
		}
		if err = a.Memory.Forget(ctx, id); err != nil {
			return out, err
		}
		out.Text = "Memory forgotten."
		return out, nil
	case "/export":
		if a.Memory == nil {
			return out, nil
		}
		b, err := a.Memory.Export(ctx)
		if err != nil {
			return out, err
		}
		out.Text = string(b)
		return out, nil
	case "/status":
		s := a.Health.Snapshot()
		out.Text = fmt.Sprintf("Mosaid %s status=%s uptime=%s messages=%d", a.Version, s.Status, time.Since(s.StartedAt).Round(time.Second), s.Messages)
		return out, nil
	case "/stop":
		a.mu.Lock()
		if a.cancel != nil {
			a.cancel()
			a.cancel = nil
		}
		a.mu.Unlock()
		out.Text = "Active request stopped."
		return out, nil
	}
	_ = a.Sessions.Append(in.ChatID, "user", text)
	hist, err := a.Sessions.Recent(in.ChatID, 20)
	if err != nil {
		return out, err
	}
	run, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.cancel = cancel
	a.mu.Unlock()
	answer, err := a.Model.Complete(run, hist)
	cancel()
	a.mu.Lock()
	a.cancel = nil
	a.mu.Unlock()
	if err != nil {
		return out, err
	}
	_ = a.Sessions.Append(in.ChatID, "assistant", answer)
	out.Text = answer
	return out, nil
}
