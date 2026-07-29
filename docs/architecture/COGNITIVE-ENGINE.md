# Mosaid — Cognitive Engine

**Date:** 2026-07-29  
**Status:** Accepted  
**Phase:** 15

---

## Purpose

The Cognitive Engine is the brain of Mosaid. It receives a user goal, plans a multi-step execution, authorizes sensitive actions, executes tools, verifies results, and delivers an outcome — all while teaching the beginner user along the way.

---

## Core Loop

```
UNDERSTAND → PLAN → AUTHORIZE → ACT → OBSERVE → VERIFY → REPLAN|COMPLETE
```

### 1. UNDERSTAND
- Parse the user's natural language (Arabic/English/dialect)
- Extract: goal, constraints, success criteria, risks, missing inputs
- If ambiguous → ask the user (do not guess)

### 2. PLAN
- Decompose goal into ordered steps
- Each step: tool + input + expected output + verification method
- Assign risk levels and approval requirements
- Estimate token/tool/time budgets

### 3. AUTHORIZE
- For sensitive steps (publish, write, high-risk), generate an approval token
- Present to user: "I need your approval to [action]. Approve? /approve <token> or /deny <token>"
- Wait for response (with timeout)

### 4. ACT
- Execute each step via the tool registry
- Record evidence (inputs, outputs, timestamps, provider used)
- Respect budget limits (steps, tokens, tools, retries)

### 5. OBSERVE
- Check if the tool returned expected output
- If unexpected → record anomaly

### 6. VERIFY
- Validate output against success criteria
- If quality insufficient → REPLAN (with max 3 replans)
- If good → COMPLETE

### 7. COMPLETE
- Deliver final output to user
- Record to audit trail
- Update memory with learnings

---

## Key Types

```go
type Goal struct {
    ID              string
    OwnerID         string
    SourceMessage   string
    Constraints     GoalConstraints
    SuccessCriteria []SuccessCriterion
    Risks           []Risk
    PendingInputs   []PendingInput
    CreatedAt       time.Time
}

type Run struct {
    ID             string
    GoalID         string
    State          RunState
    CurrentStepIdx int
    Budgets        RunBudgets
    Evidence       []Evidence
    AuditRefs      []string
    ResumableAt    time.Time
}

type RunState string
const (
    RunPending    RunState = "pending"
    RunRunning    RunState = "running"
    RunWaitingApproval RunState = "waiting_approval"
    RunPaused     RunState = "paused"
    RunCompleted  RunState = "completed"
    RunFailed     RunState = "failed"
    RunCancelled  RunState = "cancelled"
)
```

---

## Loop Prevention

The `LoopGuard` prevents infinite loops:

```go
type LoopGuard struct {
    MaxSteps              int
    MaxRepeatedActions    int
    MaxNoProgressSteps    int
    StateFingerprints     map[string]int
    RetryBudget           int
    ProviderSwitchBudget  int
    TokenBudget           int
    ToolCallBudget        int
    TimeBudget            time.Duration
}
```

If any limit is exceeded, the engine stops and reports the issue.

---

## Crash Resume

Every Run is persisted to SQLite after each step:

- On resume: load the last completed step
- Do NOT re-execute side effects that already completed
- Re-verify evidence is still valid
- Preserve approval bindings
- Respect idempotency keys

---

## Integration with Existing Agent

The current `agent.go` handles simple request/response. The Cognitive Engine will be the new execution path for `/goal` commands, while existing commands (`/status`, `/memory`, etc.) continue to work unchanged.
