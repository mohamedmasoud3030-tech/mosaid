package cognitive_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/cognitive"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers"
)

// mockLLMCaller implements LLMCaller for testing.
type mockLLMCaller struct {
	responses []providers.Response
	callCount int
}

func (m *mockLLMCaller) CallLLM(ctx context.Context, taskType providers.TaskType, msgs []providers.Message, opts ...cognitive.CallOption) (providers.Response, error) {
	if m.callCount >= len(m.responses) {
		return providers.Response{Content: `{"steps": [{"name": "default", "description": "default step", "risk": "low"}]}`, Usage: providers.Usage{TotalTokens: 10}}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

// mockToolExecutor implements ToolExecutor for testing.
type mockToolExecutor struct {
	results map[string]json.RawMessage
	calls   []string
}

func (m *mockToolExecutor) ExecuteTool(ctx context.Context, toolName string, input json.RawMessage) (json.RawMessage, error) {
	m.calls = append(m.calls, toolName)
	if result, ok := m.results[toolName]; ok {
		return result, nil
	}
	return json.Marshal("tool executed")
}

// failingToolExecutor always returns an error.
type failingToolExecutor struct{}

func (f *failingToolExecutor) ExecuteTool(ctx context.Context, toolName string, input json.RawMessage) (json.RawMessage, error) {
	return nil, &testError{"tool execution failed"}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestCreateGoal(t *testing.T) {
	store := cognitive.NewMemoryStore()
	engine := cognitive.NewEngine(&mockLLMCaller{}, &mockToolExecutor{}, store)

	goal, err := engine.CreateGoal(context.Background(), "user1", "Write a blog post about AI")
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}

	if goal.ID == "" {
		t.Error("goal ID should not be empty")
	}
	if goal.OwnerID != "user1" {
		t.Errorf("OwnerID = %q, want user1", goal.OwnerID)
	}
	if goal.Constraints.MaxBudgetUSD != 0 {
		t.Errorf("MaxBudgetUSD = %f, want 0", goal.Constraints.MaxBudgetUSD)
	}
}

func TestStartRun(t *testing.T) {
	store := cognitive.NewMemoryStore()
	engine := cognitive.NewEngine(&mockLLMCaller{}, &mockToolExecutor{}, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "Research competitors")
	run, err := engine.StartRun(context.Background(), goal.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	if run.Budgets.MaxSpendUSD != 0 {
		t.Errorf("MaxSpendUSD = %f, must be 0", run.Budgets.MaxSpendUSD)
	}

	updatedGoal, _ := store.GetGoal(context.Background(), goal.ID)
	if updatedGoal.State != cognitive.GoalActive {
		t.Errorf("Goal state = %q, want active", updatedGoal.State)
	}
}

func TestExecuteLoopSuccess(t *testing.T) {
	store := cognitive.NewMemoryStore()
	llm := &mockLLMCaller{
		responses: []providers.Response{
			{Content: `{"steps": [{"name": "research", "description": "Research the topic", "risk": "low"}]}`, Usage: providers.Usage{TotalTokens: 100}},
			{Content: `{"content": "research complete", "status": "success"}`, Usage: providers.Usage{TotalTokens: 50}},
			{Content: `{"passed": true, "reason": "all criteria met"}`, Usage: providers.Usage{TotalTokens: 30}},
		},
	}
	engine := cognitive.NewEngine(llm, &mockToolExecutor{}, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "Research AI trends")
	run, _ := engine.StartRun(context.Background(), goal.ID)

	result, err := engine.ExecuteLoop(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ExecuteLoop: %v", err)
	}
	if result.State != cognitive.RunCompleted {
		t.Errorf("State = %q, want completed", result.State)
	}
	if len(result.Evidence) == 0 {
		t.Error("should have evidence")
	}
}

func TestExecuteLoopWithToolCall(t *testing.T) {
	store := cognitive.NewMemoryStore()
	llm := &mockLLMCaller{
		responses: []providers.Response{
			{Content: `{"steps": [{"name": "fetch", "description": "Fetch data", "tool_name": "research", "tool_input": {"query": "AI trends"}, "risk": "low"}]}`, Usage: providers.Usage{TotalTokens: 100}},
			{Content: `{"passed": true, "reason": "done"}`, Usage: providers.Usage{TotalTokens: 30}},
		},
	}
	tools := &mockToolExecutor{
		results: map[string]json.RawMessage{
			"research": json.RawMessage(`{"results": ["trend1", "trend2"]}`),
		},
	}
	engine := cognitive.NewEngine(llm, tools, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "Research AI trends")
	run, _ := engine.StartRun(context.Background(), goal.ID)

	result, err := engine.ExecuteLoop(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ExecuteLoop: %v", err)
	}
	if result.State != cognitive.RunCompleted {
		t.Errorf("State = %q, want completed", result.State)
	}
	if len(tools.calls) != 1 || tools.calls[0] != "research" {
		t.Errorf("tool calls = %v, want [research]", tools.calls)
	}
}

// P0: Failed step MUST fail the run — never complete with failed steps.
func TestExecuteLoopFailedStepFailsRun(t *testing.T) {
	store := cognitive.NewMemoryStore()
	llm := &mockLLMCaller{
		responses: []providers.Response{
			{Content: `{"steps": [{"name": "step1", "description": "Do something", "tool_name": "fail_tool", "risk": "low"}]}`, Usage: providers.Usage{TotalTokens: 100}},
		},
	}
	engine := cognitive.NewEngine(llm, &failingToolExecutor{}, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "task")
	run, _ := engine.StartRun(context.Background(), goal.ID)

	result, err := engine.ExecuteLoop(context.Background(), run.ID)
	if err == nil {
		t.Error("should fail when a step fails")
	}
	if result.State != cognitive.RunFailed {
		t.Errorf("State = %q, want failed", result.State)
	}

	// Verify the step is marked as failed
	for _, step := range result.Steps {
		if step.State == cognitive.StepFailed {
			// expected
		} else {
			t.Errorf("step %q state = %q, want failed", step.Name, step.State)
		}
	}
}

// P0: Verification failure should fail the run.
func TestExecuteLoopVerificationFails(t *testing.T) {
	store := cognitive.NewMemoryStore()
	llm := &mockLLMCaller{
		responses: []providers.Response{
			{Content: `{"steps": [{"name": "step1", "description": "Do something", "risk": "low"}]}`, Usage: providers.Usage{TotalTokens: 100}},
			{Content: `{"content": "done", "status": "success"}`, Usage: providers.Usage{TotalTokens: 50}},
			{Content: `{"passed": false, "reason": "criteria not met"}`, Usage: providers.Usage{TotalTokens: 30}},
		},
	}
	engine := cognitive.NewEngine(llm, &mockToolExecutor{}, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "task")
	run, _ := engine.StartRun(context.Background(), goal.ID)

	result, err := engine.ExecuteLoop(context.Background(), run.ID)
	if err == nil {
		t.Error("should fail when verification fails")
	}
	if result.State != cognitive.RunFailed {
		t.Errorf("State = %q, want failed", result.State)
	}
}

// P0: ResumeRun skips completed steps, does NOT re-execute from beginning.
func TestResumeRunSkipsCompletedSteps(t *testing.T) {
	store := cognitive.NewMemoryStore()
	llm := &mockLLMCaller{
		responses: []providers.Response{
			// understand
			{Content: `{"steps": [
				{"name": "step1", "description": "Step 1", "risk": "low"},
				{"name": "step2", "description": "Step 2", "risk": "low"}
			]}`, Usage: providers.Usage{TotalTokens: 100}},
			// step1 execute
			{Content: `{"content": "step1 done", "status": "success"}`, Usage: providers.Usage{TotalTokens: 50}},
			// step2 execute (on resume)
			{Content: `{"content": "step2 done", "status": "success"}`, Usage: providers.Usage{TotalTokens: 50}},
			// verify
			{Content: `{"passed": true, "reason": "all done"}`, Usage: providers.Usage{TotalTokens: 30}},
		},
	}
	engine := cognitive.NewEngine(llm, &mockToolExecutor{}, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "task")
	run, _ := engine.StartRun(context.Background(), goal.ID)

	// First execution: completes step1 and step2, then verifies
	result, err := engine.ExecuteLoop(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("first ExecuteLoop: %v", err)
	}
	if result.State != cognitive.RunCompleted {
		t.Errorf("State = %q, want completed", result.State)
	}

	// Verify both steps are completed
	for i, step := range result.Steps {
		if step.State != cognitive.StepCompleted {
			t.Errorf("step[%d] state = %q, want completed", i, step.State)
		}
	}
}

func TestCancelRun(t *testing.T) {
	store := cognitive.NewMemoryStore()
	engine := cognitive.NewEngine(&mockLLMCaller{}, &mockToolExecutor{}, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "task")
	run, _ := engine.StartRun(context.Background(), goal.ID)

	if err := engine.CancelRun(context.Background(), run.ID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}

	updated, _ := store.GetRun(context.Background(), run.ID)
	if updated.State != cognitive.RunCancelled {
		t.Errorf("State = %q, want cancelled", updated.State)
	}
}

func TestCancelCompletedRun(t *testing.T) {
	store := cognitive.NewMemoryStore()
	llm := &mockLLMCaller{
		responses: []providers.Response{
			{Content: `{"steps": [{"name": "s", "description": "d", "risk": "low"}]}`, Usage: providers.Usage{TotalTokens: 10}},
			{Content: `{"content": "ok", "status": "success"}`, Usage: providers.Usage{TotalTokens: 5}},
			{Content: `{"passed": true, "reason": "ok"}`, Usage: providers.Usage{TotalTokens: 5}},
		},
	}
	engine := cognitive.NewEngine(llm, &mockToolExecutor{}, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "task")
	run, _ := engine.StartRun(context.Background(), goal.ID)
	engine.ExecuteLoop(context.Background(), run.ID)

	if err := engine.CancelRun(context.Background(), run.ID); err == nil {
		t.Error("should not be able to cancel completed run")
	}
}

// P0: Tool authorization — forbidden tool must be rejected.
func TestToolAuthorizationForbidden(t *testing.T) {
	store := cognitive.NewMemoryStore()
	llm := &mockLLMCaller{
		responses: []providers.Response{
			{Content: `{"steps": [{"name": "hack", "description": "Use admin", "tool_name": "admin_panel", "risk": "high"}]}`, Usage: providers.Usage{TotalTokens: 100}},
		},
	}
	engine := cognitive.NewEngine(llm, &mockToolExecutor{}, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "hack something")
	goal.Constraints.ForbiddenTools = []string{"admin_panel", "shell"}
	store.SaveGoal(context.Background(), goal)

	run, _ := engine.StartRun(context.Background(), goal.ID)

	result, err := engine.ExecuteLoop(context.Background(), run.ID)
	if err == nil {
		t.Error("should fail when using forbidden tool")
	}
	if result.State != cognitive.RunFailed {
		t.Errorf("State = %q, want failed", result.State)
	}
}

// P0: Tool authorization — tool not in allowed list must be rejected.
func TestToolAuthorizationNotAllowed(t *testing.T) {
	store := cognitive.NewMemoryStore()
	llm := &mockLLMCaller{
		responses: []providers.Response{
			{Content: `{"steps": [{"name": "fetch", "description": "Fetch data", "tool_name": "web_scraper", "risk": "low"}]}`, Usage: providers.Usage{TotalTokens: 100}},
		},
	}
	engine := cognitive.NewEngine(llm, &mockToolExecutor{}, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "research")
	goal.Constraints.AllowedTools = []string{"research", "memory"}
	store.SaveGoal(context.Background(), goal)

	run, _ := engine.StartRun(context.Background(), goal.ID)

	result, err := engine.ExecuteLoop(context.Background(), run.ID)
	if err == nil {
		t.Error("should fail when tool not in allowed list")
	}
	if result.State != cognitive.RunFailed {
		t.Errorf("State = %q, want failed", result.State)
	}
}

// P0: Per-run LoopGuard — different runs have independent guards.
func TestPerRunLoopGuard(t *testing.T) {
	guard1 := cognitive.NewLoopGuardForRun(cognitive.RunBudgets{MaxSteps: 2, MaxTokens: 1000, MaxToolCalls: 10, MaxRetries: 3, MaxProviderSwitches: 3, TimeoutSeconds: 60})
	guard2 := cognitive.NewLoopGuardForRun(cognitive.RunBudgets{MaxSteps: 2, MaxTokens: 1000, MaxToolCalls: 10, MaxRetries: 3, MaxProviderSwitches: 3, TimeoutSeconds: 60})

	// Exhaust guard1
	step := cognitive.Step{State: cognitive.StepCompleted}
	guard1.RecordStep(step, json.RawMessage(`{}`))
	guard1.RecordStep(step, json.RawMessage(`{}`))

	// guard1 should be at limit
	if err := guard1.CheckLimits(); err == nil {
		t.Error("guard1 should be at step limit")
	}

	// guard2 should still be fresh
	if err := guard2.CheckLimits(); err != nil {
		t.Errorf("guard2 should be fresh: %v", err)
	}
}

// P0: Token budget enforcement.
func TestTokenBudgetEnforcement(t *testing.T) {
	guard := cognitive.NewLoopGuardForRun(cognitive.RunBudgets{
		MaxSteps: 100, MaxTokens: 100, MaxToolCalls: 100, MaxRetries: 10, MaxProviderSwitches: 5, TimeoutSeconds: 60,
	})

	guard.RecordTokens(50)
	guard.RecordTokens(49) // total 99

	if err := guard.CheckLimits(); err != nil {
		t.Errorf("should be within limit: %v", err)
	}

	guard.RecordTokens(2) // total 101 > 100

	if err := guard.CheckLimits(); err == nil {
		t.Error("should exceed token budget")
	}
}

// P0: Model-only steps should NOT count as tool calls.
func TestModelOnlyStepNotToolCall(t *testing.T) {
	store := cognitive.NewMemoryStore()
	llm := &mockLLMCaller{
		responses: []providers.Response{
			{Content: `{"steps": [{"name": "think", "description": "Analyze the problem", "risk": "low"}]}`, Usage: providers.Usage{TotalTokens: 100}},
			{Content: `{"content": "analysis complete", "status": "success"}`, Usage: providers.Usage{TotalTokens: 50}},
			{Content: `{"passed": true, "reason": "done"}`, Usage: providers.Usage{TotalTokens: 30}},
		},
	}
	tools := &mockToolExecutor{results: map[string]json.RawMessage{}}
	engine := cognitive.NewEngine(llm, tools, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "analyze")
	run, _ := engine.StartRun(context.Background(), goal.ID)

	result, err := engine.ExecuteLoop(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ExecuteLoop: %v", err)
	}

	// No tool calls should have been made
	if len(tools.calls) != 0 {
		t.Errorf("tool calls = %v, want none", tools.calls)
	}
	if result.BudgetUsed.MaxToolCalls != 0 {
		t.Errorf("MaxToolCalls used = %d, want 0", result.BudgetUsed.MaxToolCalls)
	}
}

func TestLoopGuardSnapshot(t *testing.T) {
	guard := cognitive.DefaultLoopGuard()
	guard.RecordTokens(100)
	guard.RecordToolCall()

	snap := guard.Snapshot()
	if snap.TokensUsed != 100 {
		t.Errorf("TokensUsed = %d, want 100", snap.TokensUsed)
	}
	if snap.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", snap.ToolCalls)
	}
}

func TestMemoryStore(t *testing.T) {
	store := cognitive.NewMemoryStore()
	ctx := context.Background()

	goal := cognitive.Goal{ID: "g1", OwnerID: "u1", SourceMessage: "test", State: cognitive.GoalPending, CreatedAt: time.Now()}
	store.SaveGoal(ctx, goal)

	got, _ := store.GetGoal(ctx, "g1")
	if got.ID != "g1" {
		t.Errorf("ID = %q", got.ID)
	}

	_, err := store.GetGoal(ctx, "nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent goal")
	}
}

func TestGoalBudgetAlwaysZero(t *testing.T) {
	store := cognitive.NewMemoryStore()
	engine := cognitive.NewEngine(&mockLLMCaller{}, &mockToolExecutor{}, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "task")
	run, _ := engine.StartRun(context.Background(), goal.ID)

	if run.Budgets.MaxSpendUSD != 0 {
		t.Errorf("MaxSpendUSD = %f, must be 0", run.Budgets.MaxSpendUSD)
	}
}

// P0: SQLite store persists and recovers runs.
func TestSQLiteGoalStorePersistAndRecover(t *testing.T) {
	t.Parallel()
	tmpFile := t.TempDir() + "/test-cognitive.db"
	store, err := cognitive.NewSQLiteGoalStore(tmpFile)
	if err != nil {
		t.Fatalf("NewSQLiteGoalStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	goal := cognitive.Goal{
		ID:            "g-sqlite-1",
		OwnerID:       "user1",
		SourceMessage: "test goal",
		State:         cognitive.GoalPending,
		Constraints:   cognitive.GoalConstraints{MaxBudgetUSD: 0},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	if err := store.SaveGoal(ctx, goal); err != nil {
		t.Fatalf("SaveGoal: %v", err)
	}

	got, err := store.GetGoal(ctx, "g-sqlite-1")
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if got.ID != "g-sqlite-1" || got.OwnerID != "user1" {
		t.Errorf("recovered goal mismatch: %+v", got)
	}

	run := cognitive.Run{
		ID:     "r-sqlite-1",
		GoalID: "g-sqlite-1",
		State:  cognitive.RunRunning,
		Steps: []cognitive.Step{
			{ID: "s1", Name: "step1", State: cognitive.StepCompleted, Result: json.RawMessage(`{"ok":true}`)},
		},
		Budgets:   cognitive.RunBudgets{MaxSpendUSD: 0},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := store.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	gotRun, err := store.GetRun(ctx, "r-sqlite-1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if gotRun.ID != "r-sqlite-1" || len(gotRun.Steps) != 1 {
		t.Errorf("recovered run mismatch: %+v", gotRun)
	}
	if gotRun.Steps[0].State != cognitive.StepCompleted {
		t.Errorf("step state = %q, want completed", gotRun.Steps[0].State)
	}

	runs, err := store.ListRuns(ctx, "g-sqlite-1")
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("ListRuns = %d, want 1", len(runs))
	}
}
