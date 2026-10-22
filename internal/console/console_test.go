package console

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
)

type echoHandler struct {
	inputs []string
	fail   bool
}

func (e *echoHandler) Handle(_ context.Context, in message.Inbound) (message.Outbound, error) {
	e.inputs = append(e.inputs, in.Text)
	if e.fail {
		return message.Outbound{}, errors.New("boom")
	}
	if strings.HasPrefix(in.Text, "/approve") {
		return message.Outbound{ChatID: in.ChatID, Text: "approved " + in.Text}, nil
	}
	return message.Outbound{ChatID: in.ChatID, Text: "reply to " + in.Text}, nil
}

func TestConsoleForwardsEveryLine(t *testing.T) {
	handler := &echoHandler{}
	var out bytes.Buffer
	c := &Console{Handler: handler, Out: &out, UserID: 42, ChatID: 7}
	if err := c.Run(context.Background(), strings.NewReader("hello\n\n/approve abc\n/exit\nafter\n")); err != nil {
		t.Fatal(err)
	}
	if len(handler.inputs) != 2 || handler.inputs[0] != "hello" || handler.inputs[1] != "/approve abc" {
		t.Fatalf("inputs = %v", handler.inputs)
	}
	if !strings.Contains(out.String(), "reply to hello") || !strings.Contains(out.String(), "approved /approve abc") {
		t.Fatalf("output = %q", out.String())
	}
	if strings.Contains(out.String(), "after") {
		t.Fatalf("lines after /exit processed: %q", out.String())
	}
}

func TestConsoleStopsAtEOFAndReportsErrors(t *testing.T) {
	handler := &echoHandler{fail: true}
	var out bytes.Buffer
	c := &Console{Handler: handler, Out: &out}
	if err := c.Run(context.Background(), strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "boom") {
		t.Fatalf("error not reported: %q", out.String())
	}
}

func TestConsoleHonoursCancellationAndLineLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	c := &Console{Handler: &echoHandler{}, Out: &out}
	if err := c.Run(ctx, strings.NewReader("hello\n")); err != nil {
		t.Fatal(err)
	}
	handler := &echoHandler{}
	c = &Console{Handler: handler, Out: &out, MaxLines: 2}
	if err := c.Run(context.Background(), strings.NewReader("a\nb\nc\n")); err != nil {
		t.Fatal(err)
	}
	if len(handler.inputs) != 2 {
		t.Fatalf("line limit ignored: %v", handler.inputs)
	}
	if !strings.Contains(out.String(), "line limit reached") {
		t.Fatalf("limit message missing: %q", out.String())
	}
}

func TestConsoleRequiresHandler(t *testing.T) {
	if err := (&Console{Out: &bytes.Buffer{}}).Run(context.Background(), strings.NewReader("")); !errors.Is(err, ErrHandlerMissing) {
		t.Fatalf("err = %v", err)
	}
}
