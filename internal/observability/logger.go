package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"sort"
	"strings"
)

type Redactor struct {
	values   []string
	patterns []*regexp.Regexp
}

func NewRedactor(values ...string) *Redactor {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return len(filtered[i]) > len(filtered[j]) })
	patterns := []string{
		`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{8,}`,
		`\bgithub_pat_[A-Za-z0-9_]{20,}\b`,
		`\bgh[pousr]_[A-Za-z0-9]{20,}\b`,
		`\b[0-9]{6,12}:[A-Za-z0-9_-]{20,}\b`,
		`(?i)(access_token|api_key|apikey|token|secret|password)=([^&\s]+)`,
		`(?i)https://[^/@\s:]+:[^/@\s]+@`,
		`(?i)("(?:access_token|api_key|apikey|token|secret|password)"\s*:\s*")[^"]+(")`,
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		compiled = append(compiled, regexp.MustCompile(pattern))
	}
	return &Redactor{values: filtered, patterns: compiled}
}

func (r *Redactor) Redact(input string) string {
	if r == nil {
		return "[REDACTED]"
	}
	output := input
	for _, value := range r.values {
		output = strings.ReplaceAll(output, value, "[REDACTED]")
	}
	for index, pattern := range r.patterns {
		if index == 4 {
			output = pattern.ReplaceAllString(output, "$1=[REDACTED]")
		} else if index == 5 {
			output = pattern.ReplaceAllString(output, "https://[REDACTED]@")
		} else if index == 6 {
			output = pattern.ReplaceAllString(output, "$1[REDACTED]$2")
		} else {
			output = pattern.ReplaceAllString(output, "[REDACTED]")
		}
	}
	return output
}

func New(writer io.Writer, redactor *Redactor) *slog.Logger {
	return slog.New(&redactingHandler{next: slog.NewJSONHandler(writer, nil), redactor: redactor})
}

type redactingHandler struct {
	next     slog.Handler
	redactor *Redactor
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	output := slog.NewRecord(record.Time, record.Level, h.redactor.Redact(record.Message), record.PC)
	record.Attrs(func(attribute slog.Attr) bool {
		output.AddAttrs(h.redactAttr(attribute))
		return true
	})
	return h.next.Handle(ctx, output)
}

func (h *redactingHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attributes))
	for index, attribute := range attributes {
		redacted[index] = h.redactAttr(attribute)
	}
	return &redactingHandler{next: h.next.WithAttrs(redacted), redactor: h.redactor}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(name), redactor: h.redactor}
}

func (h *redactingHandler) redactAttr(attribute slog.Attr) slog.Attr {
	attribute.Value = attribute.Value.Resolve()
	switch attribute.Value.Kind() {
	case slog.KindString:
		attribute.Value = slog.StringValue(h.redactor.Redact(attribute.Value.String()))
	case slog.KindGroup:
		group := attribute.Value.Group()
		for index := range group {
			group[index] = h.redactAttr(group[index])
		}
		attribute.Value = slog.GroupValue(group...)
	case slog.KindAny:
		attribute.Value = slog.AnyValue(h.redactAny(attribute.Value.Any()))
	}
	return attribute
}

func (h *redactingHandler) redactAny(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return h.redactor.Redact(typed)
	case []byte:
		return h.redactor.Redact(string(typed))
	case error:
		return h.redactor.Redact(typed.Error())
	case fmt.Stringer:
		return h.redactor.Redact(typed.String())
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return "[UNSERIALIZABLE_REDACTED]"
		}
		redacted := h.redactor.Redact(string(encoded))
		if json.Valid([]byte(redacted)) {
			return json.RawMessage(redacted)
		}
		return redacted
	}
}
