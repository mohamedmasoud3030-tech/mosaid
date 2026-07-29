package security

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeLimitClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *fakeLimitClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}
func (c *fakeLimitClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.at = c.at.Add(duration)
	c.mu.Unlock()
}

func TestBudgetFailsClosedAtEveryLimit(t *testing.T) {
	budget, err := NewBudget(BudgetLimits{ModelSteps: 1, ToolCalls: 1, Tokens: 10, CostUSD: 0.5, Retries: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, use := range []func() error{budget.UseModelStep, budget.UseTool, func() error { return budget.UseTokens(10) }, func() error { return budget.UseCost(0.5) }, budget.UseRetry} {
		if err = use(); err != nil {
			t.Fatal(err)
		}
		if err = use(); !errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("second use err=%v", err)
		}
	}
	ctx := WithBudget(context.Background(), budget)
	if recovered, ok := BudgetFromContext(ctx); !ok || recovered != budget {
		t.Fatal("budget context binding failed")
	}
}

func TestConcurrentToolBudgetCannotOverspend(t *testing.T) {
	budget, _ := NewBudget(BudgetLimits{ModelSteps: 1, ToolCalls: 20, Tokens: 1, CostUSD: 1, Retries: 1})
	var accepted int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if budget.UseTool() == nil {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if accepted != 20 {
		t.Fatalf("accepted=%d", accepted)
	}
}

func TestMessageGuardFloodAndRefill(t *testing.T) {
	clock := &fakeLimitClock{at: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)}
	guard, err := NewMessageGuard(clock, 8, 60, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err = guard.Allow(1, "one"); err != nil {
		t.Fatal(err)
	}
	if err = guard.Allow(1, "two"); err != nil {
		t.Fatal(err)
	}
	if err = guard.Allow(1, "three"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err=%v", err)
	}
	clock.Advance(time.Second)
	if err = guard.Allow(1, "refill"); err != nil {
		t.Fatal(err)
	}
	if err = guard.Allow(1, "oversized-message"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("oversized err=%v", err)
	}
}
