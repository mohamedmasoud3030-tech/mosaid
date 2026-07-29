package agent

import (
	"context"
	"fmt"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/health"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
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
		out.Text = "Mosaid commands: /status /help /stop /approve <token> /deny <token>"
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
