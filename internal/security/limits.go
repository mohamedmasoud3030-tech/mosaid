package security

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
	"unicode/utf8"
)

var ErrBudgetExceeded = errors.New("execution budget exceeded")
var ErrRateLimited = errors.New("message rate limit exceeded")

type BudgetLimits struct {
	ModelSteps int
	ToolCalls  int
	Tokens     int
	CostUSD    float64
	Retries    int
}

// PriceLimits holds optional model pricing in USD per 1M tokens.
// Zero prices mean "free tier": no cost accrues and the budget never trips.
type PriceLimits struct {
	InputPer1M  float64
	OutputPer1M float64
}

// TokenCostUSD estimates the USD cost of tokens at pricePer1M.
func TokenCostUSD(tokens int, pricePer1M float64) float64 {
	if tokens <= 0 || pricePer1M <= 0 {
		return 0
	}
	return float64(tokens) * pricePer1M / 1_000_000
}

type Budget struct {
	mu     sync.Mutex
	limits BudgetLimits
	used   BudgetLimits
}

func NewBudget(limits BudgetLimits) (*Budget, error) {
	if limits.ModelSteps < 1 || limits.ToolCalls < 1 || limits.Tokens < 1 || limits.CostUSD <= 0 || math.IsNaN(limits.CostUSD) || math.IsInf(limits.CostUSD, 0) || limits.Retries < 1 {
		return nil, errors.New("positive execution budgets required")
	}
	return &Budget{limits: limits}, nil
}

func (b *Budget) UseModelStep() error { return b.consume(BudgetLimits{ModelSteps: 1}) }
func (b *Budget) UseTool() error      { return b.consume(BudgetLimits{ToolCalls: 1}) }
func (b *Budget) UseTokens(tokens int) error {
	if tokens < 0 {
		return ErrBudgetExceeded
	}
	return b.consume(BudgetLimits{Tokens: tokens})
}
func (b *Budget) UseCost(amount float64) error {
	if amount < 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return ErrBudgetExceeded
	}
	return b.consume(BudgetLimits{CostUSD: amount})
}
func (b *Budget) UseRetry() error { return b.consume(BudgetLimits{Retries: 1}) }

func (b *Budget) consume(delta BudgetLimits) error {
	if b == nil {
		return ErrBudgetExceeded
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	next := BudgetLimits{
		ModelSteps: b.used.ModelSteps + delta.ModelSteps,
		ToolCalls:  b.used.ToolCalls + delta.ToolCalls,
		Tokens:     b.used.Tokens + delta.Tokens,
		CostUSD:    b.used.CostUSD + delta.CostUSD,
		Retries:    b.used.Retries + delta.Retries,
	}
	if next.ModelSteps > b.limits.ModelSteps || next.ToolCalls > b.limits.ToolCalls || next.Tokens > b.limits.Tokens || next.CostUSD > b.limits.CostUSD || next.Retries > b.limits.Retries {
		return ErrBudgetExceeded
	}
	b.used = next
	return nil
}

func (b *Budget) Snapshot() (used, limits BudgetLimits) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used, b.limits
}

type budgetContextKey struct{}

func WithBudget(ctx context.Context, budget *Budget) context.Context {
	return context.WithValue(ctx, budgetContextKey{}, budget)
}

func BudgetFromContext(ctx context.Context) (*Budget, bool) {
	budget, ok := ctx.Value(budgetContextKey{}).(*Budget)
	return budget, ok
}

type LimitClock interface{ Now() time.Time }

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type rateState struct {
	tokens float64
	last   time.Time
}

type MessageGuard struct {
	mu                sync.Mutex
	clock             LimitClock
	maxBytes          int
	messagesPerMinute float64
	burst             float64
	users             map[int64]rateState
}

func NewMessageGuard(clock LimitClock, maxBytes, messagesPerMinute, burst int) (*MessageGuard, error) {
	if clock == nil || maxBytes < 1 || messagesPerMinute < 1 || burst < 1 || burst > messagesPerMinute {
		return nil, errors.New("valid message flood limits required")
	}
	return &MessageGuard{clock: clock, maxBytes: maxBytes, messagesPerMinute: float64(messagesPerMinute), burst: float64(burst), users: map[int64]rateState{}}, nil
}

func (g *MessageGuard) Allow(userID int64, text string) error {
	if userID <= 0 || len(text) > g.maxBytes || !utf8.ValidString(text) {
		return fmt.Errorf("%w: invalid or oversized message", ErrRateLimited)
	}
	now := g.clock.Now().UTC()
	g.mu.Lock()
	defer g.mu.Unlock()
	state, exists := g.users[userID]
	if !exists {
		state = rateState{tokens: g.burst, last: now}
	}
	elapsed := now.Sub(state.last)
	if elapsed < 0 {
		elapsed = 0
	}
	state.tokens = math.Min(g.burst, state.tokens+elapsed.Minutes()*g.messagesPerMinute)
	state.last = now
	if state.tokens < 1 {
		g.users[userID] = state
		return ErrRateLimited
	}
	state.tokens--
	g.users[userID] = state
	return nil
}
