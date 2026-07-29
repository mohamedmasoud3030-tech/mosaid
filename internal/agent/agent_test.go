package agent

import (
	"context"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/health"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/security"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"testing"
)

type fm struct{}

func (fm) Complete(context.Context, []model.Message) (string, error) { return "answer", nil }
func TestCommandsAndChat(t *testing.T) {
	a := &Agent{Model: fm{}, Sessions: storage.NewSessionStore(t.TempDir()), Health: health.New(t.TempDir(), "v"), Version: "v", Limits: security.BudgetLimits{ModelSteps: 1, ToolCalls: 1, Tokens: 100, CostUSD: 1, Retries: 1}}
	for _, x := range []string{"/help", "/status", "/stop", "hello"} {
		o, e := a.Handle(context.Background(), message.Inbound{ChatID: 1, MessageID: 1, Text: x})
		if e != nil || o.Text == "" {
			t.Fatal(x, o, e)
		}
	}
}
