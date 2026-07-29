// Package cognitive implements Mosaid's Cognitive Engine — the core execution
// loop that receives user goals, plans multi-step executions, authorizes
// sensitive actions, executes tools, verifies results, and delivers outcomes.
//
// Core loop: UNDERSTAND → PLAN → AUTHORIZE → ACT → OBSERVE → VERIFY → REPLAN|COMPLETE
package cognitive

import (
	"encoding/json"
	"time"
)

// GoalState represents the lifecycle state of a goal.
type GoalState string

const (
	GoalPending   GoalState = "pending"
	GoalActive    GoalState = "active"
	GoalCompleted GoalState = "completed"
	GoalFailed    GoalState = "failed"
	GoalCancelled GoalState = "cancelled"
)

// Goal represents what the user wants to accomplish.
type Goal struct {
	ID              string             `json:"id"`
	OwnerID         string             `json:"owner_id"`
	SourceMessage   string             `json:"source_message"`
	State           GoalState          `json:"state"`
	Constraints     GoalConstraints    `json:"constraints"`
	SuccessCriteria []SuccessCriterion `json:"success_criteria"`
	Risks           []Risk             `json:"risks"`
	PendingInputs   []PendingInput     `json:"pending_inputs,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// GoalConstraints defines limits on how a goal can be achieved.
type GoalConstraints struct {
	MaxBudgetUSD     float64  `json:"max_budget_usd"` // Always 0 in free_only
	MaxTimeMinutes   int      `json:"max_time_minutes"`
	AllowedTools     []string `json:"allowed_tools,omitempty"`
	ForbiddenTools   []string `json:"forbidden_tools,omitempty"`
	RequiresApproval bool     `json:"requires_approval"`
	Language         string   `json:"language"` // "ar", "en", "auto"
}

// SuccessCriterion defines how to verify a goal is complete.
type SuccessCriterion struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Verifiable  bool   `json:"verifiable"` // Can be automatically verified?
	Verified    bool   `json:"verified"`
	Evidence    string `json:"evidence,omitempty"`
}

// Risk represents a potential issue with achieving a goal.
type Risk struct {
	Level       RiskLevel `json:"level"`
	Description string    `json:"description"`
	Mitigation  string    `json:"mitigation,omitempty"`
}

// RiskLevel classifies the severity of a risk.
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// PendingInput represents information needed from the user before proceeding.
type PendingInput struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Required bool   `json:"required"`
	Received bool   `json:"received"`
	Value    string `json:"value,omitempty"`
}

// RunState represents the lifecycle state of a run.
type RunState string

const (
	RunPending         RunState = "pending"
	RunRunning         RunState = "running"
	RunWaitingApproval RunState = "waiting_approval"
	RunPaused          RunState = "paused"
	RunCompleted       RunState = "completed"
	RunFailed          RunState = "failed"
	RunCancelled       RunState = "cancelled"
)

// StepState represents the lifecycle state of a step.
type StepState string

const (
	StepPending   StepState = "pending"
	StepRunning   StepState = "running"
	StepCompleted StepState = "completed"
	StepFailed    StepState = "failed"
	StepSkipped   StepState = "skipped"
)

// Run represents a single execution attempt of a goal.
type Run struct {
	ID             string     `json:"id"`
	GoalID         string     `json:"goal_id"`
	State          RunState   `json:"state"`
	CurrentStepIdx int        `json:"current_step_idx"`
	Steps          []Step     `json:"steps"`
	Budgets        RunBudgets `json:"budgets"`
	BudgetUsed     RunBudgets `json:"budget_used"`
	Evidence       []Evidence `json:"evidence"`
	AuditRefs      []string   `json:"audit_refs"`
	ReplanCount    int        `json:"replan_count"`
	MaxReplans     int        `json:"max_replans"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ResumableAt    time.Time  `json:"resumable_at,omitempty"`
	Error          string     `json:"error,omitempty"`
}

// Step represents one step in an execution plan.
type Step struct {
	ID               string          `json:"id"`
	Index            int             `json:"index"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	ToolName         string          `json:"tool_name,omitempty"`
	ToolInput        json.RawMessage `json:"tool_input,omitempty"`
	ExpectedOutput   string          `json:"expected_output,omitempty"`
	State            StepState       `json:"state"`
	Risk             RiskLevel       `json:"risk"`
	RequiresApproval bool            `json:"requires_approval"`
	ApprovalToken    string          `json:"approval_token,omitempty"`
	ApprovalResolved bool            `json:"approval_resolved"`
	Result           json.RawMessage `json:"result,omitempty"`
	Error            string          `json:"error,omitempty"`
	StartedAt        time.Time       `json:"started_at,omitempty"`
	CompletedAt      time.Time       `json:"completed_at,omitempty"`
	IdempotencyKey   string          `json:"idempotency_key,omitempty"`
}

// RunBudgets defines resource limits for a run.
type RunBudgets struct {
	MaxSteps            int     `json:"max_steps"`
	MaxTokens           int     `json:"max_tokens"`
	MaxToolCalls        int     `json:"max_tool_calls"`
	MaxRetries          int     `json:"max_retries"`
	MaxProviderSwitches int     `json:"max_provider_switches"`
	TimeoutSeconds      int     `json:"timeout_seconds"`
	MaxSpendUSD         float64 `json:"max_spend_usd"` // Always 0.0 in free_only
}

// Evidence records a piece of evidence collected during execution.
type Evidence struct {
	ID          string          `json:"id"`
	StepID      string          `json:"step_id"`
	Kind        EvidenceKind    `json:"kind"`
	Content     json.RawMessage `json:"content"`
	Source      string          `json:"source"` // "tool_output", "user_input", "verification"
	CollectedAt time.Time       `json:"collected_at"`
}

// EvidenceKind classifies the type of evidence.
type EvidenceKind string

const (
	EvidenceToolOutput    EvidenceKind = "tool_output"
	EvidenceUserInput     EvidenceKind = "user_input"
	EvidenceVerification  EvidenceKind = "verification"
	EvidenceModelResponse EvidenceKind = "model_response"
)

// LoopPhase represents the current phase of the cognitive loop.
type LoopPhase string

const (
	PhaseUnderstand LoopPhase = "understand"
	PhasePlan       LoopPhase = "plan"
	PhaseAuthorize  LoopPhase = "authorize"
	PhaseAct        LoopPhase = "act"
	PhaseObserve    LoopPhase = "observe"
	PhaseVerify     LoopPhase = "verify"
	PhaseReplan     LoopPhase = "replan"
	PhaseComplete   LoopPhase = "complete"
)
