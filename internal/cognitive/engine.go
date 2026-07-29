// Package cognitive implements Mosaid's Cognitive Engine — the core execution
// loop that receives user goals, plans multi-step executions, authorizes
// sensitive actions, executes tools, verifies results, and delivers outcomes.
//
// Core loop: UNDERSTAND → PLAN → AUTHORIZE → ACT → OBSERVE → VERIFY → REPLAN|COMPLETE
package cognitive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers"
)

var (
	ErrGoalNotFound    = errors.New("goal not found")
	ErrRunNotFound     = errors.New("run not found")
	ErrInvalidPhase    = errors.New("invalid phase transition")
	ErrApprovalPending = errors.New("approval pending, cannot proceed")
	ErrToolNotAllowed  = errors.New("tool not in allowed list")
	ErrToolForbidden   = errors.New("tool is in forbidden list")
	ErrStepFailed      = errors.New("step failed, run cannot continue")
)

// LLMCaller abstracts the LLM call for the cognitive engine.
type LLMCaller interface {
	CallLLM(ctx context.Context, taskType providers.TaskType, msgs []providers.Message, opts ...CallOption) (providers.Response, error)
}

// CallOption modifies LLM call behavior.
type CallOption func(*callConfig)

type callConfig struct {
	maxTokens   int
	temperature float64
	jsonMode    bool
}

func WithMaxTokens(n int) CallOption       { return func(c *callConfig) { c.maxTokens = n } }
func WithTemperature(t float64) CallOption { return func(c *callConfig) { c.temperature = t } }
func WithJSONMode() CallOption             { return func(c *callConfig) { c.jsonMode = true } }

// ToolExecutor executes tool calls on behalf of the cognitive engine.
type ToolExecutor interface {
	ExecuteTool(ctx context.Context, toolName string, input json.RawMessage) (json.RawMessage, error)
}

// GoalStore persists goals and runs.
type GoalStore interface {
	SaveGoal(ctx context.Context, goal Goal) error
	GetGoal(ctx context.Context, id string) (Goal, error)
	SaveRun(ctx context.Context, run Run) error
	GetRun(ctx context.Context, id string) (Run, error)
	ListRuns(ctx context.Context, goalID string) ([]Run, error)
}

// ApprovalManager manages approval tokens.
type ApprovalManager interface {
	// CreateApproval creates a new approval request and returns a token.
	CreateApproval(ctx context.Context, runID, stepID, description string) (token string, err error)
	// CheckApproval checks if a token has been approved.
	CheckApproval(ctx context.Context, token string) (approved bool, err error)
	// ResolveApproval resolves an approval token as approved or denied.
	ResolveApproval(ctx context.Context, token, decision string) error
}

// Engine is the Cognitive Engine that drives the execution loop.
type Engine struct {
	llm       LLMCaller
	tools     ToolExecutor // used in executeStep
	store     GoalStore
	approvals ApprovalManager
	mu        sync.Mutex
}

// NewEngine creates a new Cognitive Engine.
func NewEngine(llm LLMCaller, tools ToolExecutor, store GoalStore) *Engine {
	return &Engine{
		llm:   llm,
		tools: tools,
		store: store,
	}
}

// SetApprovalManager sets the approval manager for the engine.
func (e *Engine) SetApprovalManager(am ApprovalManager) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.approvals = am
}

// CreateGoal creates a new goal from a user message.
func (e *Engine) CreateGoal(ctx context.Context, ownerID, message string) (Goal, error) {
	goal := Goal{
		ID:            generateID("goal"),
		OwnerID:       ownerID,
		SourceMessage: message,
		State:         GoalPending,
		Constraints: GoalConstraints{
			MaxBudgetUSD: 0,
			Language:     "auto",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := e.store.SaveGoal(ctx, goal); err != nil {
		return Goal{}, fmt.Errorf("save goal: %w", err)
	}
	return goal, nil
}

// StartRun begins execution of a goal.
func (e *Engine) StartRun(ctx context.Context, goalID string) (Run, error) {
	goal, err := e.store.GetGoal(ctx, goalID)
	if err != nil {
		return Run{}, fmt.Errorf("get goal: %w", err)
	}

	run := Run{
		ID:     generateID("run"),
		GoalID: goalID,
		State:  RunPending,
		Budgets: RunBudgets{
			MaxSteps:            50,
			MaxTokens:           100000,
			MaxToolCalls:        100,
			MaxRetries:          10,
			MaxProviderSwitches: 5,
			TimeoutSeconds:      1800,
			MaxSpendUSD:         0,
		},
		MaxReplans:  3,
		ReplanCount: 0,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := e.store.SaveRun(ctx, run); err != nil {
		return Run{}, fmt.Errorf("save run: %w", err)
	}

	goal.State = GoalActive
	goal.UpdatedAt = time.Now().UTC()
	if err := e.store.SaveGoal(ctx, goal); err != nil {
		return Run{}, fmt.Errorf("update goal: %w", err)
	}

	return run, nil
}

// ExecuteLoop runs the cognitive loop for a run.
// If the run has already completed some steps (resume), it continues from the
// first non-completed step. It NEVER re-executes completed steps.
func (e *Engine) ExecuteLoop(ctx context.Context, runID string) (Run, error) {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return Run{}, fmt.Errorf("get run: %w", err)
	}

	goal, err := e.store.GetGoal(ctx, run.GoalID)
	if err != nil {
		return Run{}, fmt.Errorf("get goal: %w", err)
	}

	// Create a per-run loop guard (not shared between runs)
	guard := NewLoopGuardForRun(run.Budgets)

	run.State = RunRunning
	run.UpdatedAt = time.Now().UTC()

	// Phase 1: UNDERSTAND — only if we don't have steps yet
	if len(run.Steps) == 0 {
		plan, err := e.understand(ctx, goal, guard)
		if err != nil {
			return e.failRun(ctx, run, fmt.Errorf("understand: %w", err))
		}
		run.Steps = plan
		if err := e.store.SaveRun(ctx, run); err != nil {
			return Run{}, fmt.Errorf("save run after plan: %w", err)
		}
	}

	// Phase 3-6: Execute each step starting from first non-completed
	hasFailure := false
	for i := range run.Steps {
		step := &run.Steps[i]

		// Skip already completed steps (crash resume / idempotency)
		if step.State == StepCompleted {
			continue
		}
		// Skip failed steps (they stay failed)
		if step.State == StepFailed {
			hasFailure = true
			continue
		}

		run.CurrentStepIdx = i

		// Check loop guard
		if err := guard.CheckLimits(); err != nil {
			return e.failRun(ctx, run, fmt.Errorf("loop guard: %w", err))
		}

		// Check context cancellation
		if ctx.Err() != nil {
			return e.failRun(ctx, run, ctx.Err())
		}

		// Phase 3: AUTHORIZE
		if step.RequiresApproval && !step.ApprovalResolved {
			token, err := e.createApproval(ctx, run, step)
			if err != nil {
				return e.failRun(ctx, run, fmt.Errorf("create approval: %w", err))
			}
			step.ApprovalToken = token
			run.State = RunWaitingApproval
			run.UpdatedAt = time.Now().UTC()
			if err := e.store.SaveRun(ctx, run); err != nil {
				return Run{}, err
			}
			return run, ErrApprovalPending
		}

		// Tool authorization: check allowed/forbidden lists
		if step.ToolName != "" {
			if err := e.authorizeTool(goal, step.ToolName); err != nil {
				step.State = StepFailed
				step.Error = err.Error()
				hasFailure = true
				run.UpdatedAt = time.Now().UTC()
				e.store.SaveRun(ctx, run)
				return e.failRun(ctx, run, err)
			}
		}

		// Phase 4: ACT
		step.State = StepRunning
		step.StartedAt = time.Now().UTC()

		// Only record tool call for actual tool steps, not model-only
		if step.ToolName != "" {
			if err := guard.RecordToolCall(); err != nil {
				return e.failRun(ctx, run, err)
			}
		}

		result, err := e.executeStep(ctx, step, guard)
		if err != nil {
			// CRITICAL: Failed step = failed run. No continue.
			step.State = StepFailed
			step.Error = err.Error()
			step.CompletedAt = time.Now().UTC()
			run.UpdatedAt = time.Now().UTC()
			e.store.SaveRun(ctx, run)
			return e.failRun(ctx, run, fmt.Errorf("step %q failed: %w", step.Name, err))
		}

		// Phase 5: OBSERVE
		step.State = StepCompleted
		step.CompletedAt = time.Now().UTC()
		step.Result = result

		run.Evidence = append(run.Evidence, Evidence{
			ID:          generateID("ev"),
			StepID:      step.ID,
			Kind:        EvidenceToolOutput,
			Content:     result,
			Source:      "tool_output",
			CollectedAt: time.Now().UTC(),
		})

		if err := guard.RecordStep(*step, result); err != nil {
			return e.failRun(ctx, run, err)
		}

		if step.ToolName != "" {
			run.BudgetUsed.MaxToolCalls++
		}
		run.UpdatedAt = time.Now().UTC()

		if err := e.store.SaveRun(ctx, run); err != nil {
			return Run{}, err
		}
	}

	// Cannot complete if any step failed
	if hasFailure {
		return e.failRun(ctx, run, errors.New("run contains failed steps"))
	}

	// Phase 6: VERIFY
	if err := e.verify(ctx, goal, run, guard); err != nil {
		if run.ReplanCount < run.MaxReplans {
			run.ReplanCount++
			run.UpdatedAt = time.Now().UTC()
			e.store.SaveRun(ctx, run)
			return e.failRun(ctx, run, fmt.Errorf("verification failed, replan %d/%d: %w",
				run.ReplanCount, run.MaxReplans, err))
		}
		return e.failRun(ctx, run, fmt.Errorf("verification failed after %d replans: %w",
			run.MaxReplans, err))
	}

	// Phase 7: COMPLETE
	run.State = RunCompleted
	run.UpdatedAt = time.Now().UTC()
	if err := e.store.SaveRun(ctx, run); err != nil {
		return Run{}, err
	}

	goal.State = GoalCompleted
	goal.UpdatedAt = time.Now().UTC()
	if err := e.store.SaveGoal(ctx, goal); err != nil {
		return Run{}, err
	}

	return run, nil
}

// authorizeTool checks if a tool is allowed for this goal.
func (e *Engine) authorizeTool(goal Goal, toolName string) error {
	// Check forbidden list first
	for _, forbidden := range goal.Constraints.ForbiddenTools {
		if toolName == forbidden {
			return fmt.Errorf("%w: %q", ErrToolForbidden, toolName)
		}
	}

	// If allowed list is specified, tool must be in it
	if len(goal.Constraints.AllowedTools) > 0 {
		allowed := false
		for _, t := range goal.Constraints.AllowedTools {
			if t == toolName {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("%w: %q", ErrToolNotAllowed, toolName)
		}
	}

	return nil
}

// createApproval creates an approval request for a step.
func (e *Engine) createApproval(ctx context.Context, run Run, step *Step) (string, error) {
	e.mu.Lock()
	am := e.approvals
	e.mu.Unlock()

	if am == nil {
		return "", errors.New("approval manager not configured")
	}

	return am.CreateApproval(ctx, run.ID, step.ID, fmt.Sprintf("Approve: %s", step.Description))
}

// understand analyzes the user's goal and extracts structured information.
func (e *Engine) understand(ctx context.Context, goal Goal, guard *LoopGuard) ([]Step, error) {
	msgs := []providers.Message{
		{Role: "system", Content: understandPrompt},
		{Role: "user", Content: goal.SourceMessage},
	}

	resp, err := e.llm.CallLLM(ctx, providers.TaskPlanning, msgs,
		WithJSONMode(),
		WithMaxTokens(2048),
	)
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	guard.RecordTokens(resp.Usage.TotalTokens)

	var plan struct {
		Steps []Step `json:"steps"`
		Risks []Risk `json:"risks"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &plan); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}

	for i := range plan.Steps {
		plan.Steps[i].ID = generateID("step")
		plan.Steps[i].Index = i
		if plan.Steps[i].State == "" {
			plan.Steps[i].State = StepPending
		}
	}

	if len(plan.Steps) == 0 {
		return nil, errors.New("LLM returned empty plan")
	}

	return plan.Steps, nil
}

// executeStep executes a single step of the plan.
func (e *Engine) executeStep(ctx context.Context, step *Step, guard *LoopGuard) (json.RawMessage, error) {
	if step.ToolName == "" {
		// Model-only step
		msgs := []providers.Message{
			{Role: "system", Content: executionPrompt},
			{Role: "user", Content: step.Description},
		}

		resp, err := e.llm.CallLLM(ctx, providers.TaskGeneral, msgs,
			WithMaxTokens(1024),
		)
		if err != nil {
			return nil, err
		}

		guard.RecordTokens(resp.Usage.TotalTokens)
		return json.Marshal(resp.Content)
	}

	// Tool-based step
	if e.tools == nil {
		return nil, errors.New("tool executor not available")
	}

	return e.tools.ExecuteTool(ctx, step.ToolName, step.ToolInput)
}

// verify checks if the goal's success criteria are met.
func (e *Engine) verify(ctx context.Context, goal Goal, run Run, guard *LoopGuard) error {
	evidenceSummary := ""
	for _, ev := range run.Evidence {
		evidenceSummary += fmt.Sprintf("- %s: %s\n", ev.Kind, string(ev.Content))
	}

	criteriaSummary := ""
	for _, c := range goal.SuccessCriteria {
		criteriaSummary += fmt.Sprintf("- %s (verifiable: %v)\n", c.Description, c.Verifiable)
	}

	msgs := []providers.Message{
		{Role: "system", Content: verificationPrompt},
		{Role: "user", Content: fmt.Sprintf("Goal: %s\n\nSuccess Criteria:\n%s\n\nEvidence:\n%s",
			goal.SourceMessage, criteriaSummary, evidenceSummary)},
	}

	resp, err := e.llm.CallLLM(ctx, providers.TaskPlanning, msgs,
		WithJSONMode(),
		WithMaxTokens(512),
	)
	if err != nil {
		return fmt.Errorf("verification LLM call: %w", err)
	}

	guard.RecordTokens(resp.Usage.TotalTokens)

	var result struct {
		Passed bool   `json:"passed"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		return fmt.Errorf("parse verification: %w", err)
	}

	if !result.Passed {
		return fmt.Errorf("verification failed: %s", result.Reason)
	}

	return nil
}

// failRun marks a run as failed.
func (e *Engine) failRun(ctx context.Context, run Run, err error) (Run, error) {
	run.State = RunFailed
	run.Error = err.Error()
	run.UpdatedAt = time.Now().UTC()
	if saveErr := e.store.SaveRun(ctx, run); saveErr != nil {
		return Run{}, fmt.Errorf("save failed run: %w (original error: %v)", saveErr, err)
	}
	return run, err
}

// ResumeRun resumes a run that was waiting for approval.
// It continues from the step that was waiting, NOT from the beginning.
func (e *Engine) ResumeRun(ctx context.Context, runID string) (Run, error) {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return Run{}, err
	}

	if run.State != RunWaitingApproval {
		return Run{}, fmt.Errorf("run is in state %q, cannot resume", run.State)
	}

	// Find the step that was waiting for approval and check its token
	for i := range run.Steps {
		step := &run.Steps[i]
		if step.RequiresApproval && !step.ApprovalResolved && step.ApprovalToken != "" {
			e.mu.Lock()
			am := e.approvals
			e.mu.Unlock()

			if am != nil {
				approved, err := am.CheckApproval(ctx, step.ApprovalToken)
				if err != nil {
					return Run{}, fmt.Errorf("check approval: %w", err)
				}
				if !approved {
					return Run{}, fmt.Errorf("approval token %q not yet approved", step.ApprovalToken)
				}
				step.ApprovalResolved = true
			}
		}
	}

	run.State = RunRunning
	run.UpdatedAt = time.Now().UTC()
	if err := e.store.SaveRun(ctx, run); err != nil {
		return Run{}, err
	}

	// Continue the loop (it will skip completed steps)
	return e.ExecuteLoop(ctx, runID)
}

// CancelRun cancels a running or pending run.
func (e *Engine) CancelRun(ctx context.Context, runID string) error {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}

	if run.State == RunCompleted || run.State == RunFailed || run.State == RunCancelled {
		return fmt.Errorf("run is already in terminal state %q", run.State)
	}

	run.State = RunCancelled
	run.UpdatedAt = time.Now().UTC()
	return e.store.SaveRun(ctx, run)
}

// GetRun returns the current state of a run.
func (e *Engine) GetRun(ctx context.Context, runID string) (Run, error) {
	return e.store.GetRun(ctx, runID)
}

// ListRuns returns all runs for a goal.
func (e *Engine) ListRuns(ctx context.Context, goalID string) ([]Run, error) {
	return e.store.ListRuns(ctx, goalID)
}
