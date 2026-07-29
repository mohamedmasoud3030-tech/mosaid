package observability

import (
	"bytes"
	"testing"
)

func TestRedaction(t *testing.T) {
	var b bytes.Buffer
	l := New(&b, NewRedactor("secret-value"))
	l.Info("secret-value", "token", "secret-value")
	if bytes.Contains(b.Bytes(), []byte("secret-value")) {
		t.Fatal("leak")
	}
}
