// Package console exposes the same owner agent through a line-oriented
// terminal, so the agent can be exercised with a model key alone, without a
// Telegram bot, a phone, or any inbound network service.
package console

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
)

// Handler is the agent surface the console drives. It is the same interface the
// Telegram gateway uses, so behaviour cannot diverge between channels.
type Handler interface {
	Handle(context.Context, message.Inbound) (message.Outbound, error)
}

// Console is a blocking terminal session.
type Console struct {
	Handler  Handler
	Out      io.Writer
	UserID   int64
	ChatID   int64
	Banner   string
	MaxLines int
}

const defaultMaxLines = 200

// ErrHandlerMissing reports a missing agent.
var ErrHandlerMissing = errors.New("console requires an agent handler")

// Run reads commands from in until EOF, /exit, or ctx cancellation. Every line
// is forwarded verbatim to the agent, which means Telegram commands such as
// /approve, /deny, /tools and /memory work identically in the terminal.
func (c *Console) Run(ctx context.Context, in io.Reader) error {
	if c.Handler == nil || c.Out == nil {
		return ErrHandlerMissing
	}
	maxLines := c.MaxLines
	if maxLines <= 0 {
		maxLines = defaultMaxLines
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 4096), 64<<10)
	if c.Banner != "" {
		fmt.Fprintln(c.Out, c.Banner)
	}
	for line := 0; scanner.Scan(); line++ {
		if line >= maxLines {
			fmt.Fprintln(c.Out, "line limit reached; ending session")
			break
		}
		if err := ctx.Err(); err != nil {
			return nil
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		switch text {
		case "/exit", "/quit":
			return nil
		}
		outbound, err := c.Handler.Handle(ctx, message.Inbound{
			UpdateID: int64(line + 1), MessageID: int64(line + 1),
			ChatID: c.ChatID, UserID: c.UserID, ChatType: "private", Text: text,
		})
		if err != nil {
			fmt.Fprintf(c.Out, "\n⚠️  %v\n\n", err)
			continue
		}
		if strings.TrimSpace(outbound.Text) == "" {
			continue
		}
		fmt.Fprintf(c.Out, "\n%s\n\n", strings.TrimSpace(outbound.Text))
	}
	return scanner.Err()
}
