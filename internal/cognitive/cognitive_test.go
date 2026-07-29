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
	if goal.SourceMessage != "Write a blog post about AI" {
		t.Errorf("SourceMessage = %q", goal.SourceMessage)
	}
	if goal.State != cognitive.GoalPending {
		t.Errorf("State = %q, want pending", goal.State)
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

	if run.ID == "" {
		t.Error("run ID should not be empty")
	}
	if run.GoalID != goal.ID {
		t.Errorf("GoalID = %q, want %q", run.GoalID, goal.ID)
	}
	if run.State != cognitive.RunPending {
		t.Errorf("State = %q, want pending", run.State)
	}
	if run.Budgets.MaxSpendUSD != 0 {
		t.Errorf("MaxSpendUSD = %f, want 0", run.Budgets.MaxSpendUSD)
	}

	// Verify goal was updated to active
	updatedGoal, _ := store.GetGoal(context.Background(), goal.ID)
	if updatedGoal.State != cognitive.GoalActive {
		t.Errorf("Goal state = %q, want active", updatedGoal.State)
	}
}

func TestExecuteLoopSuccess(t *testing.T) {
	store := cognitive.NewMemoryStore()

	llm := &mockLLMCaller{
		responses: []providers.Response{
			// understand response
			{Content: `{"steps": [{"name": "research", "description": "Research the topic", "risk": "low"}]}`, Usage: providers.Usage{TotalTokens: 100}},
			// execute step response
			{Content: `{"content": "research complete", "status": "success"}`, Usage: providers.Usage{TotalTokens: 50}},
			// verify response
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
			// understand
			{Content: `{"steps": [{"name": "fetch", "description": "Fetch data", "tool_name": "research", "tool_input": {"query": "AI trends"}, "risk": "low"}]}`, Usage: providers.Usage{TotalTokens: 100}},
			// verify
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

func TestExecuteLoopVerificationFails(t *testing.T) {
	store := cognitive.NewMemoryStore()

	llm := &mockLLMCaller{
		responses: []providers.Response{
			// understand
			{Content: `{"steps": [{"name": "step1", "description": "Do something", "risk": "low"}]}`, Usage: providers.Usage{TotalTokens: 100}},
			// execute
			{Content: `{"content": "done", "status": "success"}`, Usage: providers.Usage{TotalTokens: 50}},
			// verify fails
			{Content: `{"passed": false, "reason": "criteria not met"}`, Usage: providers.Usage{TotalTokens: 30}},
		},
	}

	engine := cognitive.NewEngine(llm, &mockToolExecutor{}, store)

	goal, _ := engine.CreateGoal(context.Background(), "user1", "Do something complex")
	run, _ := engine.StartRun(context.Background(), goal.ID)

	result, err := engine.ExecuteLoop(context.Background(), run.ID)
	if err == nil {
		t.Error("should fail when verification fails")
	}
	if result.State != cognitive.RunFailed {
		t.Errorf("State = %q, want failed", result.State)
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

func TestLoopGuardMaxSteps(t *testing.T) {
	guard := cognitive.DefaultLoopGuard()
	guard.MaxSteps = 2

	step := cognitive.Step{State: cognitive.StepCompleted}

	// First two steps should be fine
	if err := guard.RecordStep(step, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("step 1: %v", err)
	}
	if err := guard.RecordStep(step, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("step 2: %v", err)
	}

	// Third step should exceed limit
	if err := guard.CheckLimits(); err == nil {
		t.Error("should exceed max steps")
	}
}

func TestLoopGuardRepeatedActions(t *testing.T) {
	guard := cognitive.DefaultLoopGuard()
	guard.MaxRepeatedActions = 2

	step := cognitive.Step{ToolName: "same_tool", ToolInput: json.RawMessage(`{"q":"test"}`)}
	result := json.RawMessage(`{"r":"same"}`)

	// Same fingerprint twice is ok
	if err := guard.RecordStep(step, result); err != nil {
		t.Fatalf("record 1: %v", err)
	}
	if err := guard.RecordStep(step, result); err != nil {
		t.Fatalf("record 2: %v", err)
	}

	// Third time should be rejected
	if err := guard.RecordStep(step, result); err == nil {
		t.Error("should reject repeated action")
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
	if snap.Elapsed <= 0 {
		t.Error("Elapsed should be positive")
	}
}

func TestMemoryStore(t *testing.T) {
	store := cognitive.NewMemoryStore()
	ctx := context.Background()

	// Save and get goal
	goal := cognitive.Goal{ID: "g1", OwnerID: "u1", SourceMessage: "test", State: cognitive.GoalPending, CreatedAt: time.Now()}
	if err := store.SaveGoal(ctx, goal); err != nil {
		t.Fatalf("SaveGoal: %v", err)
	}

	got, err := store.GetGoal(ctx, "g1")
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if got.ID != "g1" {
		t.Errorf("ID = %q, want g1", got.ID)
	}

	// Get nonexistent
	_, err = store.GetGoal(ctx, "nonexistent")
	if err == nil {
		t.Error("should fail for nonexistent goal")
	}

	// Save and list runs
	run1 := cognitive.Run{ID: "r1", GoalID: "g1", State: cognitive.RunRunning, CreatedAt: time.Now()}
	run2 := cognitive.Run{ID: "r2", GoalID: "g1", State: cognitive.RunCompleted, CreatedAt: time.Now()}
	store.SaveRun(ctx, run1)
	store.SaveRun(ctx, run2)

	runs, err := store.ListRuns(ctx, "g1")
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("ListRuns = %d, want 2", len(runs))
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
