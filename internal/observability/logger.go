package observability

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

type Redactor struct{ values []string }

func NewRedactor(values ...string) *Redactor { return &Redactor{values: values} }
func (r *Redactor) Redact(s string) string {
	for _, v := range r.values {
		if v != "" {
			s = strings.ReplaceAll(s, v, "[REDACTED]")
		}
	}
	return s
}
func New(w io.Writer, r *Redactor) *slog.Logger {
	return slog.New(&redactingHandler{next: slog.NewJSONHandler(w, nil), r: r})
}

type redactingHandler struct {
	next slog.Handler
	r    *Redactor
}

func (h *redactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}
func (h *redactingHandler) Handle(ctx context.Context, rec slog.Record) error {
	rec.Message = h.r.Redact(rec.Message)
	out := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		if a.Value.Kind() == slog.KindString {
			a.Value = slog.StringValue(h.r.Redact(a.Value.String()))
		}
		out.AddAttrs(a)
		return true
	})
	return h.next.Handle(ctx, out)
}
func (h *redactingHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return &redactingHandler{next: h.next.WithAttrs(a), r: h.r}
}
func (h *redactingHandler) WithGroup(n string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(n), r: h.r}
}
