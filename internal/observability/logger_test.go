package observability

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactionCoversNestedAndPreboundValues(t *testing.T) {
	var buffer bytes.Buffer
	secret := "unit-secret-value"
	logger := New(&buffer, NewRedactor(secret)).With(
		"prebound", secret,
		"nested_map", map[string]any{"password": "unknown-password", "value": secret},
	)
	telegramLike := "123456789:" + strings.Repeat("A", 28)
	logger.Info(
		"Bearer ABCDEFGHIJKLMNOP secret="+secret,
		slog.Group("group", slog.String("token", secret)),
		"error", errors.New("failed with "+secret),
		"bytes", []byte(secret),
		"credential_url", "https://user:pass@example.invalid/path",
		"telegram", telegramLike,
	)
	output := buffer.String()
	for _, forbidden := range []string{secret, "unknown-password", "ABCDEFGHIJKLMNOP", "user:pass", strings.Repeat("A", 28)} {
		if bytes.Contains(buffer.Bytes(), []byte(forbidden)) {
			t.Fatalf("log leaked %q: %s", forbidden, output)
		}
	}
	if !bytes.Contains(buffer.Bytes(), []byte("[REDACTED]")) {
		t.Fatalf("redaction marker missing: %s", output)
	}
}
