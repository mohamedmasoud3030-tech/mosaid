package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/agentloop"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/security"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/tools"
)

// pendingApproval is the in-memory continuation for one approval token. It is
// intentionally not persisted: an approval that outlives the process is refused
// rather than replayed, which keeps approval binding fail-closed.
type pendingApproval struct {
	loop     agentloop.Pending
	chatID   int64
	userID   int64
	created  time.Time
	messages []model.ChatMessage
}

const (
	pendingLimit      = 8
	pendingTTL        = 15 * time.Minute
	approvalPreview   = 400
	toolFooterLines   = 6
	defaultAgentSteps = agentloop.DefaultMaxSteps
)

// agenticEnabled reports whether the configured model and tools can run the
// tool-calling loop. Anything missing falls back to plain chat.
func (a *Agent) agenticEnabled() (model.ToolClient, bool) {
	if a.Tools == nil || a.Model == nil {
		return nil, false
	}
	client, ok := a.Model.(model.ToolClient)
	return client, ok
}

func (a *Agent) loopConfig(client model.ToolClient, messages []model.ChatMessage, userID int64) agentloop.Config {
	steps := a.MaxSteps
	if steps <= 0 {
		steps = defaultAgentSteps
	}
	mode := a.Mode
	if mode == "" {
		mode = policy.Admin
	}
	return agentloop.Config{
		Model:               client,
		Tools:               a.Tools,
		System:              a.systemPrompt(),
		History:             messages,
		UserID:              userID,
		Mode:                mode,
		MaxSteps:            steps,
		MaxCallsPerStep:     a.MaxCallsPerStep,
		MaxToolResultBytes:  a.MaxToolResultBytes,
		MaxTokens:           a.MaxTokens,
		Temperature:         0.2,
		Limits:              a.Limits,
		Prices:              a.Prices,
		SuppressToolsOnLast: true,
	}
}

func (a *Agent) systemPrompt() string {
	if custom := strings.TrimSpace(a.SystemPrompt); custom != "" {
		return custom
	}
	return DefaultSystemPrompt(time.Now().UTC(), a.WorkspaceRoot)
}

// handleAgenticChat runs one user turn through the tool-calling loop.
func (a *Agent) handleAgenticChat(ctx context.Context, in message.Inbound, history []model.Message) (message.Outbound, error) {
	out := message.Outbound{ChatID: in.ChatID, ReplyTo: in.MessageID}
	client, ok := a.agenticEnabled()
	if !ok {
		return out, errors.New("agent core unavailable")
	}
	messages := make([]model.ChatMessage, 0, len(history))
	for _, item := range history {
		role := strings.TrimSpace(item.Role)
		if role != "user" && role != "assistant" {
			role = "user"
		}
		messages = append(messages, model.ChatMessage{Role: role, Content: item.Content})
	}
	outcome, err := agentloop.Run(ctx, a.loopConfig(client, messages, in.UserID))
	if err != nil {
		return out, err
	}
	return a.finishTurn(ctx, in, out, outcome)
}

// finishTurn converts a loop outcome into an owner-facing reply, registering any
// pending approval and persisting the assistant message for continuity.
func (a *Agent) finishTurn(ctx context.Context, in message.Inbound, out message.Outbound, outcome agentloop.Outcome) (message.Outbound, error) {
	footer := a.toolFooter(outcome.ToolEvents)
	if outcome.Pending != nil {
		a.storePending(*outcome.Pending, in)
		out.Text = a.approvalPrompt(*outcome.Pending) + footer
		return out, nil
	}
	text := strings.TrimSpace(outcome.Text)
	if text == "" {
		if outcome.Truncated {
			text = "توقفتُ قبل إكمال المهمة لأن حد الخطوات أو الأدوات انتهى. أعد صياغة الطلب على شكل خطوة واحدة محددة."
		} else {
			text = "لم يصل رد من النموذج. أعد المحاولة."
		}
	}
	out.Text = text + footer
	if err := a.Sessions.Append(in.ChatID, "assistant", out.Text); err != nil {
		return out, err
	}
	return out, nil
}

func (a *Agent) toolFooter(events []agentloop.ToolEvent) string {
	if len(events) == 0 {
		return ""
	}
	var lines []string
	for _, event := range events {
		if len(lines) >= toolFooterLines {
			lines = append(lines, "…")
			break
		}
		mark := "✓"
		detail := ""
		if event.Approval {
			mark = "⏳"
			detail = " موافقة مطلوبة"
		} else if event.Failed {
			mark = "✗"
			if event.Summary != "" {
				detail = " — " + event.Summary
			}
		} else if event.Resource != "" {
			detail = " — " + event.Resource
		}
		lines = append(lines, fmt.Sprintf("%s %s%s (%.2fs)", mark, event.Name, detail, event.Duration.Seconds()))
	}
	return "\n\n🔧 " + strings.Join(lines, "\n🔧 ")
}

func (a *Agent) approvalPrompt(pending agentloop.Pending) string {
	var detail strings.Builder
	fmt.Fprintf(&detail, "⚠️ موافقة مطلوبة قبل التنفيذ: %s\n", pending.Tool)
	if pending.Resource != "" {
		fmt.Fprintf(&detail, "الهدف: %s\n", pending.Resource)
	}
	fmt.Fprintf(&detail, "المدخلات: %s\n", previewArguments(pending.ArgumentJSON))
	expires := pending.Expires
	if expires.IsZero() {
		expires = time.Now().UTC().Add(5 * time.Minute)
	}
	fmt.Fprintf(&detail, "\nللموافقة: /approve %s\nللرفض: /deny %s\nتنتهي الصلاحية: %s UTC",
		pending.Token, pending.Token, expires.Format("15:04"))
	return detail.String()
}

func previewArguments(raw string) string {
	if len(raw) <= approvalPreview {
		return raw
	}
	return raw[:approvalPreview] + "…"
}

func (a *Agent) storePending(pending agentloop.Pending, in message.Inbound) {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	if a.pending == nil {
		a.pending = map[string]pendingApproval{}
	}
	now := time.Now().UTC()
	for token, item := range a.pending {
		if now.Sub(item.created) > pendingTTL {
			delete(a.pending, token)
		}
	}
	// A bounded map keeps a chatty model from pinning memory on a phone.
	if len(a.pending) >= pendingLimit {
		var oldestToken string
		var oldest time.Time
		for token, item := range a.pending {
			if oldestToken == "" || item.created.Before(oldest) {
				oldestToken, oldest = token, item.created
			}
		}
		delete(a.pending, oldestToken)
	}
	a.pending[pending.Token] = pendingApproval{
		loop:     pending,
		chatID:   in.ChatID,
		userID:   in.UserID,
		created:  now,
		messages: append([]model.ChatMessage(nil), pending.Messages...),
	}
}

func (a *Agent) takePending(token string) (pendingApproval, bool) {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	item, ok := a.pending[token]
	if !ok {
		return pendingApproval{}, false
	}
	delete(a.pending, token)
	if time.Since(item.created) > pendingTTL {
		return pendingApproval{}, false
	}
	return item, true
}

// handleApproval resolves an approval token and, when approved, executes exactly
// the tool call that was frozen at request time and resumes the loop.
func (a *Agent) handleApproval(ctx context.Context, in message.Inbound, fields []string) (message.Outbound, error) {
	out := message.Outbound{ChatID: in.ChatID, ReplyTo: in.MessageID}
	if a.Approvals == nil || a.Tools == nil || len(fields) != 2 {
		out.Text = "الاستخدام: /approve <token> أو /deny <token>"
		return out, nil
	}
	token := strings.TrimSpace(fields[1])
	decision := "approved"
	if fields[0] == "/deny" {
		decision = "denied"
	}
	item, known := a.takePending(token)
	if err := a.Approvals.ResolveToken(ctx, token, in.UserID, decision); err != nil {
		if known {
			// Resolution failed: keep the continuation available for a retry
			// with the correct token or user.
			a.restorePending(item)
		}
		out.Text = "تم رفض الموافقة: " + err.Error()
		return out, nil
	}
	if decision == "denied" {
		out.Text = "أُلغيت العملية. لم يُنفَّذ أي تغيير." + a.toolFooter([]agentloop.ToolEvent{{Name: item.loop.Tool, Resource: item.loop.Resource, Failed: true, Summary: "owner denied"}})
		_ = a.Sessions.Append(in.ChatID, "assistant", out.Text)
		return out, nil
	}
	if !known {
		out.Text = "تم تسجيل الموافقة، لكن تفاصيل الاستدعاء لم تعد محفوظة (انتهت الجلسة). أعد إرسال الطلب."
		return out, nil
	}
	client, ok := a.agenticEnabled()
	if !ok {
		out.Text = "تم تسجيل الموافقة، لكن الوكل غير متاح الآن. لم يُنفَّذ شيء."
		return out, nil
	}
	result, err := a.Tools.Execute(ctx, tools.Request{
		Name:          item.loop.Tool,
		Arguments:     json.RawMessage(item.loop.ArgumentJSON),
		Mode:          a.mode(),
		UserID:        in.UserID,
		Resource:      item.loop.Resource,
		ApprovalToken: token,
	})
	if err != nil {
		out.Text = "تمت الموافقة، لكن التنفيذ فشل: " + err.Error()
		_ = a.Sessions.Append(in.ChatID, "assistant", out.Text)
		return out, nil
	}
	observation, marshalErr := json.Marshal(map[string]any{"ok": true, "tool": item.loop.Tool, "result": result.Value})
	if marshalErr != nil {
		out.Text = "تمت الموافقة والتنفيذ، لكن تعذّر تنسيق النتيجة."
		return out, nil
	}
	cfg := a.loopConfig(client, nil, in.UserID)
	outcome, err := agentloop.Resume(ctx, cfg, item.loop, string(observation))
	if err != nil {
		if errors.Is(err, security.ErrBudgetExceeded) {
			return out, err
		}
		out.Text = "تم تنفيذ العملية بنجاح، لكن تعذّر إكمال الرد: " + err.Error()
		_ = a.Sessions.Append(in.ChatID, "assistant", out.Text)
		return out, nil
	}
	outcome.ToolEvents = append([]agentloop.ToolEvent{{Name: item.loop.Tool, Resource: item.loop.Resource}}, outcome.ToolEvents...)
	return a.finishTurn(ctx, in, out, outcome)
}

func (a *Agent) restorePending(item pendingApproval) {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	if a.pending == nil {
		a.pending = map[string]pendingApproval{}
	}
	a.pending[item.loop.Token] = item
}

func (a *Agent) mode() policy.Mode {
	if a.Mode != "" {
		return a.Mode
	}
	return policy.Admin
}

// handleTools lists the model-facing tool surface for the owner.
func (a *Agent) handleTools(in message.Inbound) (message.Outbound, error) {
	out := message.Outbound{ChatID: in.ChatID, ReplyTo: in.MessageID}
	if a.Tools == nil {
		out.Text = "لا توجد أدوات مفعّلة."
		return out, nil
	}
	catalog, err := a.Tools.Catalog()
	if err != nil {
		return out, err
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "الأدوات المتاحة (%d):\n", len(catalog))
	for _, entry := range catalog {
		badge := "قراءة"
		if entry.Approval {
			badge = "كتابة + موافقة"
		}
		fmt.Fprintf(&builder, "• %s — %s [%s]\n", entry.Name, truncateText(entry.Description, 90), badge)
	}
	if a.WorkspaceRoot != "" {
		fmt.Fprintf(&builder, "\nمساحة العمل: %s", a.WorkspaceRoot)
	}
	out.Text = strings.TrimSpace(builder.String())
	return out, nil
}

func truncateText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
