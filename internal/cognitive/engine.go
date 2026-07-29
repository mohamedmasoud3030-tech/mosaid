package cognitive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers"
)

var (
	ErrGoalNotFound    = errors.New("goal not found")
	ErrRunNotFound     = errors.New("run not found")
	ErrInvalidPhase    = errors.New("invalid phase transition")
	ErrApprovalPending = errors.New("approval pending, cannot proceed")
)

// LLMCaller abstracts the LLM call for the cognitive engine.
// The engine never knows which provider or model is being used.
type LLMCaller interface {
	// CallLLM sends messages to the LLM and returns the response.
	CallLLM(ctx context.Context, taskType providers.TaskType, msgs []providers.Message, opts ...CallOption) (providers.Response, error)
}

// CallOption modifies LLM call behavior.
type CallOption func(*callConfig)

type callConfig struct {
	maxTokens   int
	temperature float64
	jsonMode    bool
	tools       []providers.Tool
}

// WithMaxTokens sets the max tokens for the LLM call.
func WithMaxTokens(n int) CallOption {
	return func(c *callConfig) { c.maxTokens = n }
}

// WithTemperature sets the temperature for the LLM call.
func WithTemperature(t float64) CallOption {
	return func(c *callConfig) { c.temperature = t }
}

// WithJSONMode enables JSON mode for the LLM call.
func WithJSONMode() CallOption {
	return func(c *callConfig) { c.jsonMode = true }
}

// ToolExecutor executes tool calls on behalf of the cognitive engine.
type ToolExecutor interface {
	// ExecuteTool runs a tool and returns the result.
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

// Engine is the Cognitive Engine that drives the execution loop.
type Engine struct {
	llm    LLMCaller
	tools  ToolExecutor
	store  GoalStore
	guard  *LoopGuard
}

// NewEngine creates a new Cognitive Engine.
func NewEngine(llm LLMCaller, tools ToolExecutor, store GoalStore) *Engine {
	return &Engine{
		llm:   llm,
		tools: tools,
		store: store,
		guard: DefaultLoopGuard(),
	}
}

// CreateGoal creates a new goal from a user message.
func (e *Engine) CreateGoal(ctx context.Context, ownerID, message string) (Goal, error) {
	goal := Goal{
		ID:            generateID("goal"),
		OwnerID:       ownerID,
		SourceMessage: message,
		State:         GoalPending,
		Constraints: GoalConstraints{
			MaxBudgetUSD: 0, // Always zero in free_only
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
		ID:         generateID("run"),
		GoalID:     goalID,
		State:      RunPending,
		Budgets: RunBudgets{
			MaxSteps:            50,
			MaxTokens:           100000,
			MaxToolCalls:        100,
			MaxRetries:          10,
			MaxProviderSwitches: 5,
			TimeoutSeconds:      1800, // 30 minutes
			MaxSpendUSD:         0,    // Always zero
		},
		MaxReplans: 3,
		ReplanCount: 0,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	if err := e.store.SaveRun(ctx, run); err != nil {
		return Run{}, fmt.Errorf("save run: %w", err)
	}

	// Update goal state
	goal.State = GoalActive
	goal.UpdatedAt = time.Now().UTC()
	if err := e.store.SaveGoal(ctx, goal); err != nil {
		return Run{}, fmt.Errorf("update goal: %w", err)
	}

	return run, nil
}

// ExecuteLoop runs the cognitive loop: UNDERSTAND → PLAN → AUTHORIZE → ACT → OBSERVE → VERIFY → COMPLETE
func (e *Engine) ExecuteLoop(ctx context.Context, runID string) (Run, error) {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return Run{}, fmt.Errorf("get run: %w", err)
	}

	goal, err := e.store.GetGoal(ctx, run.GoalID)
	if err != nil {
		return Run{}, fmt.Errorf("get goal: %w", err)
	}

	run.State = RunRunning
	run.UpdatedAt = time.Now().UTC()

	// Phase 1: UNDERSTAND
	plan, err := e.understand(ctx, goal)
	if err != nil {
		return e.failRun(ctx, run, fmt.Errorf("understand: %w", err))
	}

	// Phase 2: PLAN
	run.Steps = plan
	if err := e.store.SaveRun(ctx, run); err != nil {
		return Run{}, fmt.Errorf("save run after plan: %w", err)
	}

	// Phase 3-6: Execute each step
	for i, step := range run.Steps {
		run.CurrentStepIdx = i

		// Check loop guard
		if err := e.guard.CheckLimits(); err != nil {
			return e.failRun(ctx, run, fmt.Errorf("loop guard: %w", err))
		}

		// Check context cancellation
		if ctx.Err() != nil {
			return e.failRun(ctx, run, ctx.Err())
		}

		// Phase 3: AUTHORIZE (if needed)
		if step.RequiresApproval && !step.ApprovalResolved {
			run.State = RunWaitingApproval
			run.UpdatedAt = time.Now().UTC()
			if err := e.store.SaveRun(ctx, run); err != nil {
				return Run{}, err
			}
			return run, ErrApprovalPending
		}

		// Phase 4: ACT
		run.Steps[i].State = StepRunning
		run.Steps[i].StartedAt = time.Now().UTC()

		if err := e.guard.RecordToolCall(); err != nil {
			return e.failRun(ctx, run, err)
		}

		result, err := e.executeStep(ctx, step)
		if err != nil {
			run.Steps[i].State = StepFailed
			run.Steps[i].Error = err.Error()
			run.UpdatedAt = time.Now().UTC()

			// Record retry
			if retryErr := e.guard.RecordRetry(); retryErr != nil {
				return e.failRun(ctx, run, retryErr)
			}

			if err := e.store.SaveRun(ctx, run); err != nil {
				return Run{}, err
			}
			continue
		}

		// Phase 5: OBSERVE
		run.Steps[i].State = StepCompleted
		run.Steps[i].CompletedAt = time.Now().UTC()
		run.Steps[i].Result = result

		// Record evidence
		run.Evidence = append(run.Evidence, Evidence{
			ID:          generateID("ev"),
			StepID:      step.ID,
			Kind:        EvidenceToolOutput,
			Content:     result,
			Source:      "tool_output",
			CollectedAt: time.Now().UTC(),
		})

		// Update loop guard
		if err := e.guard.RecordStep(run.Steps[i], result); err != nil {
			return e.failRun(ctx, run, err)
		}

		run.BudgetUsed.MaxToolCalls++
		run.UpdatedAt = time.Now().UTC()

		if err := e.store.SaveRun(ctx, run); err != nil {
			return Run{}, err
		}
	}

	// Phase 6: VERIFY
	if err := e.verify(ctx, goal, run); err != nil {
		// REPLAN if allowed
		if run.ReplanCount < run.MaxReplans {
			run.ReplanCount++
			run.UpdatedAt = time.Now().UTC()
			if err := e.store.SaveRun(ctx, run); err != nil {
				return Run{}, err
			}
			// In a full implementation, this would re-enter the loop
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

	// Update goal
	goal.State = GoalCompleted
	goal.UpdatedAt = time.Now().UTC()
	if err := e.store.SaveGoal(ctx, goal); err != nil {
		return Run{}, err
	}

	return run, nil
}

// understand analyzes the user's goal and extracts structured information.
func (e *Engine) understand(ctx context.Context, goal Goal) ([]Step, error) {
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

	e.guard.RecordTokens(resp.Usage.TotalTokens)

	var plan struct {
		Steps []Step `json:"steps"`
		Risks []Risk `json:"risks"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &plan); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}

	// Assign IDs and indices
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
func (e *Engine) executeStep(ctx context.Context, step Step) (json.RawMessage, error) {
	if step.ToolName == "" {
		// Model-only step (no tool call)
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

		e.guard.RecordTokens(resp.Usage.TotalTokens)
		return json.Marshal(resp.Content)
	}

	// Tool-based step
	if e.tools == nil {
		return nil, errors.New("tool executor not available")
	}

	return e.tools.ExecuteTool(ctx, step.ToolName, step.ToolInput)
}

// verify checks if the goal's success criteria are met.
func (e *Engine) verify(ctx context.Context, goal Goal, run Run) error {
	// Collect evidence summary
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

	e.guard.RecordTokens(resp.Usage.TotalTokens)

	var result struct {
		Passed  bool   `json:"passed"`
		Reason  string `json:"reason"`
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
func (e *Engine) ResumeRun(ctx context.Context, runID string) (Run, error) {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return Run{}, err
	}

	if run.State != RunWaitingApproval {
		return Run{}, fmt.Errorf("run is in state %q, cannot resume", run.State)
	}

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
