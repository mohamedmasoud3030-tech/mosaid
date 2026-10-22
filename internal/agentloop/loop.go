// Package agentloop runs a bounded tool-calling loop: the model may request
// tools, the loop executes them through the policy/approval engine, feeds the
// results back, and repeats until the model answers or a budget is exhausted.
//
// Every step is bounded: model steps, tool calls, tokens, cost, retries, tool
// result size, and per-step parallelism. Anything that cannot be bounded is not
// executed. A tool that requires approval is never executed speculatively: the
// loop stops, returns a Pending record, and the owner decides.
package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/security"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/tools"
)

const (
	// DefaultMaxSteps bounds model turns per user request (including the final
	// answer turn).
	DefaultMaxSteps = 4
	// DefaultMaxCallsPerStep bounds parallel tool calls in a single model reply.
	DefaultMaxCallsPerStep = 4
	// DefaultMaxToolResultBytes bounds one tool result before it is fed back.
	DefaultMaxToolResultBytes = 8 << 10
	// DefaultMaxTokens bounds one completion.
	DefaultMaxTokens = 1024
	// TruncationMarker is appended to oversized tool observations.
	TruncationMarker = "…[truncated]"
)

var (
	// ErrNoHistory means the loop was called without a conversation.
	ErrNoHistory = errors.New("agent loop requires conversation history")
	// ErrClientMissing means no tool-capable model client was configured.
	ErrClientMissing = errors.New("agent loop requires a tool-capable model client")
	// ErrToolsMissing means no tool registry was configured.
	ErrToolsMissing = errors.New("agent loop requires a tool registry")
	// ErrBadPending means a pending approval could not be resumed.
	ErrBadPending = errors.New("pending approval is incomplete")
)

// Config is one loop run. Config is validated, not trusted.
type Config struct {
	Model               model.ToolClient
	Tools               *tools.Registry
	System              string
	History             []model.ChatMessage
	UserID              int64
	Mode                policy.Mode
	MaxSteps            int
	MaxCallsPerStep     int
	MaxToolResultBytes  int
	MaxTokens           int
	Temperature         float64
	Limits              security.BudgetLimits
	Prices              security.PriceLimits
	FinalAnswerNudge    string
	SuppressToolsOnLast bool
	Clock               func() time.Time
}

// ToolEvent is the audit-friendly record of one attempted tool call.
type ToolEvent struct {
	Name     string
	Resource string
	Approval bool
	Failed   bool
	Duration time.Duration
	Summary  string
}

// Outcome is the result of one loop run.
type Outcome struct {
	Text       string
	Steps      int
	ToolEvents []ToolEvent
	Usage      model.Usage
	CostUSD    float64
	Pending    *Pending
	Truncated  bool
}

// Pending is a tool call that is waiting for the owner's approval. It carries
// the exact conversation prefix and argument JSON so the approved call can be
// executed with the same approval binding that was hashed.
type Pending struct {
	Token        string
	Tool         string
	ArgumentJSON string
	Resource     string
	CallID       string
	Expires      time.Time
	Messages     []model.ChatMessage
}

// Run starts a loop run from scratch.
func Run(ctx context.Context, cfg Config) (Outcome, error) {
	if cfg.Model == nil {
		return Outcome{}, ErrClientMissing
	}
	if cfg.Tools == nil {
		return Outcome{}, ErrToolsMissing
	}
	if len(cfg.History) == 0 {
		return Outcome{}, ErrNoHistory
	}
	messages := make([]model.ChatMessage, 0, len(cfg.History)+1)
	if system := strings.TrimSpace(cfg.System); system != "" {
		messages = append(messages, model.ChatMessage{Role: "system", Content: system})
	}
	messages = append(messages, cfg.History...)
	return run(ctx, cfg, messages)
}

// Resume continues a loop after the owner approved a pending tool call. The
// observation is the serialized tool result.
func Resume(ctx context.Context, cfg Config, pending Pending, observation string) (Outcome, error) {
	if cfg.Model == nil {
		return Outcome{}, ErrClientMissing
	}
	if cfg.Tools == nil {
		return Outcome{}, ErrToolsMissing
	}
	if pending.Tool == "" || pending.CallID == "" || len(pending.Messages) == 0 {
		return Outcome{}, ErrBadPending
	}
	messages := make([]model.ChatMessage, len(pending.Messages), len(pending.Messages)+1)
	copy(messages, pending.Messages)
	messages = append(messages, model.ChatMessage{Role: "tool", ToolCallID: pending.CallID, Content: observation})
	return run(ctx, cfg, messages)
}

type runner struct {
	cfg      Config
	budget   *security.Budget
	specs    []model.ToolSpec
	messages []model.ChatMessage
	outcome  Outcome
}

func run(ctx context.Context, cfg Config, messages []model.ChatMessage) (Outcome, error) {
	cfg = withDefaults(cfg)
	budget, err := security.NewBudget(cfg.Limits)
	if err != nil {
		return Outcome{}, err
	}
	catalog, err := cfg.Tools.Catalog()
	if err != nil {
		return Outcome{}, err
	}
	specs := make([]model.ToolSpec, 0, len(catalog))
	for _, entry := range catalog {
		specs = append(specs, model.ToolSpec{Name: entry.Name, Description: entry.Description, Parameters: entry.Parameters})
	}
	r := &runner{cfg: cfg, budget: budget, specs: specs, messages: messages}
	return r.loop(ctx)
}

func withDefaults(cfg Config) Config {
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = DefaultMaxSteps
	}
	if cfg.MaxSteps > 32 {
		cfg.MaxSteps = 32
	}
	if cfg.MaxCallsPerStep <= 0 {
		cfg.MaxCallsPerStep = DefaultMaxCallsPerStep
	}
	if cfg.MaxToolResultBytes <= 0 {
		cfg.MaxToolResultBytes = DefaultMaxToolResultBytes
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	if cfg.Mode == "" {
		cfg.Mode = policy.Read
	}
	if cfg.Clock == nil {
		cfg.Clock = func() time.Time { return time.Now().UTC() }
	}
	if strings.TrimSpace(cfg.FinalAnswerNudge) == "" {
		cfg.FinalAnswerNudge = "The tool budget for this turn is exhausted. Answer the owner now with the information you already have, and state clearly what is still unknown. Do not request more tools."
	}
	return cfg
}

func (r *runner) loop(ctx context.Context) (Outcome, error) {
	maxSteps := r.cfg.MaxSteps
	for step := 0; step < maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return r.outcome, err
		}
		if err := r.budget.UseModelStep(); err != nil {
			return r.outcome, err
		}
		lastStep := step == maxSteps-1
		request := model.ToolRequest{
			Messages:    r.messages,
			Tools:       r.specs,
			MaxTokens:   r.cfg.MaxTokens,
			Temperature: r.cfg.Temperature,
		}
		if lastStep && r.cfg.SuppressToolsOnLast {
			request.Tools = nil
			request.Messages = append(append([]model.ChatMessage{}, r.messages...), model.ChatMessage{Role: "system", Content: r.cfg.FinalAnswerNudge})
		}
		completion, err := r.cfg.Model.CompleteWithTools(ctx, request)
		if err != nil {
			return r.outcome, err
		}
		if err = r.charge(completion); err != nil {
			return r.outcome, err
		}
		r.outcome.Steps++
		r.messages = append(r.messages, completion.Message)
		if !completion.Message.HasToolCalls() {
			r.outcome.Text = strings.TrimSpace(completion.Message.Content)
			return r.outcome, nil
		}
		if lastStep {
			// The model ignored the final-answer nudge; return whatever text it
			// produced rather than starting a tool round we cannot finish.
			r.outcome.Text = strings.TrimSpace(completion.Message.Content)
			r.outcome.Truncated = true
			return r.outcome, nil
		}
		calls := completion.Message.ToolCalls
		if len(calls) > r.cfg.MaxCallsPerStep {
			calls = calls[:r.cfg.MaxCallsPerStep]
			r.outcome.Truncated = true
		}
		for _, call := range calls {
			if err := ctx.Err(); err != nil {
				return r.outcome, err
			}
			if err := r.budget.UseTool(); err != nil {
				return r.outcome, err
			}
			observation, pending, event := r.execute(ctx, call)
			r.outcome.ToolEvents = append(r.outcome.ToolEvents, event)
			if pending != nil {
				r.outcome.Pending = pending
				return r.outcome, nil
			}
			r.messages = append(r.messages, model.ChatMessage{Role: "tool", ToolCallID: r.callID(call), Content: observation})
		}
	}
	r.outcome.Truncated = true
	return r.outcome, nil
}

func (r *runner) callID(call model.ToolCall) string {
	if strings.TrimSpace(call.ID) != "" {
		return call.ID
	}
	// Some compatible providers omit the id; a stable synthetic id keeps the
	// assistant/tool pairing valid for the next request.
	return fmt.Sprintf("call_%d_%s", len(r.outcome.ToolEvents), call.Function.Name)
}

func (r *runner) execute(ctx context.Context, call model.ToolCall) (string, *Pending, ToolEvent) {
	started := r.cfg.Clock()
	event := ToolEvent{Name: call.Function.Name}
	defer func() { event.Duration = r.cfg.Clock().Sub(started) }()

	if err := call.Validate(); err != nil {
		event.Failed = true
		event.Summary = err.Error()
		return observationError(err), nil, event
	}
	arguments, err := call.ArgumentsObject()
	if err != nil {
		event.Failed = true
		event.Summary = err.Error()
		return observationError(err), nil, event
	}
	resource := resourceOf(arguments)
	event.Resource = resource
	result, err := r.cfg.Tools.Execute(ctx, tools.Request{
		Name:      call.Function.Name,
		Arguments: arguments,
		Mode:      r.cfg.Mode,
		UserID:    r.cfg.UserID,
		Resource:  resource,
	})
	if err != nil {
		event.Failed = true
		event.Summary = err.Error()
		return observationError(err), nil, event
	}
	if result.Approval != nil {
		event.Approval = true
		event.Summary = "approval required"
		pending := &Pending{
			Token:        result.Approval.Token,
			Tool:         result.Approval.Tool,
			ArgumentJSON: string(arguments),
			Resource:     result.Approval.Resource,
			CallID:       r.callID(call),
			Expires:      result.Approval.Expires,
			Messages:     append([]model.ChatMessage{}, r.messages...),
		}
		return "", pending, event
	}
	encoded, err := json.Marshal(map[string]any{"ok": true, "tool": call.Function.Name, "result": result.Value})
	if err != nil {
		event.Failed = true
		event.Summary = "result not serializable"
		return observationError(errors.New("tool result could not be serialized")), nil, event
	}
	observation := truncate(string(encoded), r.cfg.MaxToolResultBytes)
	event.Summary = summarize(observation)
	return observation, nil, event
}

func (r *runner) charge(completion model.Completion) error {
	for attempt := 1; attempt < completion.Attempts; attempt++ {
		if err := r.budget.UseRetry(); err != nil {
			return err
		}
	}
	usage := completion.Usage
	if usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 {
		// Providers that omit usage still get accounted for by estimate, so a
		// free-tier surprise cannot silently exceed the cost tripwire.
		usage = model.Usage{
			PromptTokens:     estimateMessages(r.messages),
			CompletionTokens: estimateTokens(len(completion.Message.Content)),
		}
	}
	if err := r.budget.UseTokens(usage.Total()); err != nil {
		return err
	}
	cost := security.TokenCostUSD(usage.PromptTokens, r.cfg.Prices.InputPer1M) +
		security.TokenCostUSD(usage.CompletionTokens, r.cfg.Prices.OutputPer1M)
	if err := r.budget.UseCost(cost); err != nil {
		return err
	}
	r.outcome.Usage.PromptTokens += usage.PromptTokens
	r.outcome.Usage.CompletionTokens += usage.CompletionTokens
	r.outcome.Usage.TotalTokens += usage.Total()
	r.outcome.CostUSD += cost
	return nil
}

func estimateTokens(characters int) int {
	if characters <= 0 {
		return 0
	}
	return (characters + 3) / 4
}

func estimateMessages(messages []model.ChatMessage) int {
	total := 0
	for _, message := range messages {
		total += len(message.Content) + len(message.ToolCallID)
		for _, call := range message.ToolCalls {
			total += len(call.Function.Name) + len(call.Function.Arguments)
		}
	}
	return estimateTokens(total)
}

func observationError(err error) string {
	encoded, marshalErr := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
	if marshalErr != nil {
		return `{"ok":false,"error":"tool failed"}`
	}
	return string(encoded)
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + TruncationMarker
}

func summarize(observation string) string {
	summary := observation
	if len(summary) > 160 {
		summary = summary[:160] + "…"
	}
	return summary
}

// resourceOf extracts the resource an approval should be bound to. Path-like
// arguments come first so an approval can never apply to a different file.
func resourceOf(arguments json.RawMessage) string {
	var probe map[string]any
	if err := json.Unmarshal(arguments, &probe); err != nil {
		return ""
	}
	for _, key := range []string{"path", "cwd", "query", "prompt"} {
		if value, ok := probe[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// ToolNames returns the catalog names in sorted order. It is used by the
// /tools command and by tests that assert the exposed surface.
func ToolNames(registry *tools.Registry) ([]string, error) {
	catalog, err := registry.Catalog()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(catalog))
	for _, entry := range catalog {
		names = append(names, entry.Name)
	}
	sort.Strings(names)
	return names, nil
}
