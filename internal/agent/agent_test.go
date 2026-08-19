package agent

import (
	"context"
	"errors"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/health"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/security"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"strings"
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

func TestCostBudgetTrips(t *testing.T) {
	a := &Agent{
		Model:    fm{},
		Sessions: storage.NewSessionStore(t.TempDir()),
		Health:   health.New(t.TempDir(), "v"),
		Version:  "v",
		Limits:   security.BudgetLimits{ModelSteps: 1, ToolCalls: 1, Tokens: 100000, CostUSD: 0.01, Retries: 1},
		Prices:   security.PriceLimits{InputPer1M: 10, OutputPer1M: 10},
	}
	// ~1250 estimated tokens at $10/1M = $0.0125 > the $0.01 cap.
	_, e := a.Handle(context.Background(), message.Inbound{ChatID: 1, MessageID: 1, Text: strings.Repeat("word ", 1000)})
	if !errors.Is(e, security.ErrBudgetExceeded) {
		t.Fatalf("err=%v", e)
	}
}

func TestZeroPricesIncurNoCost(t *testing.T) {
	a := &Agent{
		Model:    fm{},
		Sessions: storage.NewSessionStore(t.TempDir()),
		Health:   health.New(t.TempDir(), "v"),
		Version:  "v",
		Limits:   security.BudgetLimits{ModelSteps: 4, ToolCalls: 4, Tokens: 100000, CostUSD: 0.01, Retries: 1},
	}
	o, e := a.Handle(context.Background(), message.Inbound{ChatID: 1, MessageID: 1, Text: "hello"})
	if e != nil || o.Text == "" {
		t.Fatalf("err=%v out=%+v", e, o)
	}
}
