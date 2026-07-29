package cognitive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrMaxStepsExceeded        = errors.New("maximum steps exceeded")
	ErrMaxRepeatedActions      = errors.New("maximum repeated actions exceeded")
	ErrMaxNoProgressSteps      = errors.New("maximum no-progress steps exceeded")
	ErrRetryBudgetExhausted    = errors.New("retry budget exhausted")
	ErrProviderSwitchLimit     = errors.New("provider switch limit exceeded")
	ErrTokenBudgetExhausted    = errors.New("token budget exhausted")
	ErrToolCallBudgetExhausted = errors.New("tool call budget exhausted")
	ErrTimeBudgetExhausted     = errors.New("time budget exhausted")
)

// LoopGuard prevents infinite loops and unbounded execution.
// Each Run gets its own LoopGuard instance — never shared between runs.
type LoopGuard struct {
	mu                   sync.Mutex
	MaxSteps             int
	MaxRepeatedActions   int
	MaxNoProgressSteps   int
	StateFingerprints    map[string]int
	RetryBudget          int
	ProviderSwitchBudget int
	TokenBudget          int
	ToolCallBudget       int
	TimeBudget           time.Duration

	// Counters (all protected by mu)
	steps            int
	repeatedActions  int
	noProgressSteps  int
	retries          int
	providerSwitches int
	tokensUsed       int
	toolCalls        int
	startTime        time.Time
	lastProgressStep int
}

// DefaultLoopGuard returns a loop guard with sensible defaults.
func DefaultLoopGuard() *LoopGuard {
	return &LoopGuard{
		MaxSteps:             50,
		MaxRepeatedActions:   3,
		MaxNoProgressSteps:   10,
		StateFingerprints:    make(map[string]int),
		RetryBudget:          10,
		ProviderSwitchBudget: 5,
		TokenBudget:          100000,
		ToolCallBudget:       100,
		TimeBudget:           30 * time.Minute,
		startTime:            time.Now().UTC(),
		lastProgressStep:     -1,
	}
}

// NewLoopGuardForRun creates a per-run loop guard from run budgets.
func NewLoopGuardForRun(budgets RunBudgets) *LoopGuard {
	return &LoopGuard{
		MaxSteps:             budgets.MaxSteps,
		MaxRepeatedActions:   3,
		MaxNoProgressSteps:   10,
		StateFingerprints:    make(map[string]int),
		RetryBudget:          budgets.MaxRetries,
		ProviderSwitchBudget: budgets.MaxProviderSwitches,
		TokenBudget:          budgets.MaxTokens,
		ToolCallBudget:       budgets.MaxToolCalls,
		TimeBudget:           time.Duration(budgets.TimeoutSeconds) * time.Second,
		startTime:            time.Now().UTC(),
		lastProgressStep:     -1,
	}
}

// CheckLimits verifies that all execution limits are still within bounds.
func (g *LoopGuard) CheckLimits() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.steps >= g.MaxSteps {
		return fmt.Errorf("%w: %d/%d", ErrMaxStepsExceeded, g.steps, g.MaxSteps)
	}
	if g.noProgressSteps >= g.MaxNoProgressSteps {
		return fmt.Errorf("%w: %d/%d", ErrMaxNoProgressSteps, g.noProgressSteps, g.MaxNoProgressSteps)
	}
	if g.retries >= g.RetryBudget {
		return fmt.Errorf("%w: %d/%d", ErrRetryBudgetExhausted, g.retries, g.RetryBudget)
	}
	if g.providerSwitches >= g.ProviderSwitchBudget {
		return fmt.Errorf("%w: %d/%d", ErrProviderSwitchLimit, g.providerSwitches, g.ProviderSwitchBudget)
	}
	if g.tokensUsed >= g.TokenBudget {
		return fmt.Errorf("%w: %d/%d", ErrTokenBudgetExhausted, g.tokensUsed, g.TokenBudget)
	}
	if g.toolCalls >= g.ToolCallBudget {
		return fmt.Errorf("%w: %d/%d", ErrToolCallBudgetExhausted, g.toolCalls, g.ToolCallBudget)
	}
	if time.Since(g.startTime) > g.TimeBudget {
		return fmt.Errorf("%w: elapsed %v", ErrTimeBudgetExhausted, time.Since(g.startTime).Round(time.Second))
	}
	return nil
}

// RecordStep records a completed step and checks for loops.
func (g *LoopGuard) RecordStep(step Step, result json.RawMessage) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.steps++

	fingerprint := g.fingerprint(step, result)
	g.StateFingerprints[fingerprint]++
	if g.StateFingerprints[fingerprint] > g.MaxRepeatedActions {
		return fmt.Errorf("%w: action fingerprint seen %d times",
			ErrMaxRepeatedActions, g.StateFingerprints[fingerprint])
	}

	if step.State == StepCompleted {
		g.noProgressSteps = 0
		g.lastProgressStep = g.steps
	} else {
		g.noProgressSteps++
	}

	return nil
}

// RecordRetry increments the retry counter.
func (g *LoopGuard) RecordRetry() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.retries++
	if g.retries >= g.RetryBudget {
		return ErrRetryBudgetExhausted
	}
	return nil
}

// RecordProviderSwitch increments the provider switch counter.
func (g *LoopGuard) RecordProviderSwitch() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.providerSwitches++
	if g.providerSwitches >= g.ProviderSwitchBudget {
		return ErrProviderSwitchLimit
	}
	return nil
}

// RecordTokens adds tokens to the usage counter.
func (g *LoopGuard) RecordTokens(tokens int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tokensUsed += tokens
}

// RecordToolCall increments the tool call counter.
func (g *LoopGuard) RecordToolCall() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.toolCalls++
	if g.toolCalls >= g.ToolCallBudget {
		return ErrToolCallBudgetExhausted
	}
	return nil
}

// Snapshot returns the current state of the loop guard for persistence.
func (g *LoopGuard) Snapshot() LoopGuardSnapshot {
	g.mu.Lock()
	defer g.mu.Unlock()
	return LoopGuardSnapshot{
		Steps:            g.steps,
		RepeatedActions:  g.repeatedActions,
		NoProgressSteps:  g.noProgressSteps,
		Retries:          g.retries,
		ProviderSwitches: g.providerSwitches,
		TokensUsed:       g.tokensUsed,
		ToolCalls:        g.toolCalls,
		Elapsed:          time.Since(g.startTime),
	}
}

// LoopGuardSnapshot is a serializable snapshot of the loop guard state.
type LoopGuardSnapshot struct {
	Steps            int           `json:"steps"`
	RepeatedActions  int           `json:"repeated_actions"`
	NoProgressSteps  int           `json:"no_progress_steps"`
	Retries          int           `json:"retries"`
	ProviderSwitches int           `json:"provider_switches"`
	TokensUsed       int           `json:"tokens_used"`
	ToolCalls        int           `json:"tool_calls"`
	Elapsed          time.Duration `json:"elapsed"`
}

func (g *LoopGuard) fingerprint(step Step, result json.RawMessage) string {
	h := sha256.New()
	h.Write([]byte(step.ToolName))
	h.Write(step.ToolInput)
	h.Write(result)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
