package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mymmrac/telego"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
)

func TestPhase0QualificationResponse(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		privateChat bool
		allowed     bool
		content     string
		wantHandled bool
		wantPrefix  string
	}{
		{"disabled", false, true, true, "/status", false, ""},
		{"group denied", true, false, true, "/status", true, "This qualification bot accepts private chats only."},
		{"unknown denied", true, true, false, "/status", true, "Access denied."},
		{"status", true, true, true, "/status", true, "phase0_status=ok uptime_seconds=12"},
		{"echo", true, true, true, "/echo hello", true, "echo: hello"},
		{"echo usage", true, true, true, "/echo", true, "Usage:"},
		{"plain owner message reaches agent", true, true, true, "hello model", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handled, response := phase0QualificationResponse(
				tt.enabled, tt.privateChat, tt.allowed, tt.content, 12*time.Second,
			)
			if handled != tt.wantHandled {
				t.Fatalf("handled=%v want=%v", handled, tt.wantHandled)
			}
			if !strings.HasPrefix(response, tt.wantPrefix) {
				t.Fatalf("response=%q want prefix=%q", response, tt.wantPrefix)
			}
		})
	}
}

func TestPhase0EchoIsBounded(t *testing.T) {
	_, response := phase0QualificationResponse(true, true, true, "/echo "+strings.Repeat("x", 700), 0)
	if got := len([]rune(strings.TrimPrefix(response, "echo: "))); got != 512 {
		t.Fatalf("echo rune length=%d want=512", got)
	}
}

func TestPhase0IngressRejectsUnauthorizedAndGroupsBeforeBus(t *testing.T) {
	messageBus := bus.NewMessageBus()
	ch := &TelegramChannel{
		BaseChannel:        channels.NewBaseChannel("telegram", nil, messageBus, []string{"42"}),
		chatIDs:            make(map[string]int64),
		ctx:                context.Background(),
		qualificationMode:  true,
		qualificationStart: time.Now(),
	}

	cases := []*telego.Message{
		{
			Text: "/status", MessageID: 1,
			Chat: telego.Chat{ID: 999, Type: "private"},
			From: &telego.User{ID: 99, FirstName: "Unknown"},
		},
		{
			Text: "run a tool", MessageID: 2,
			Chat: telego.Chat{ID: -100, Type: "group"},
			From: &telego.User{ID: 42, FirstName: "Owner"},
		},
	}
	for _, msg := range cases {
		if err := ch.handleMessage(context.Background(), msg); err != nil {
			t.Fatalf("handleMessage error: %v", err)
		}
		select {
		case inbound := <-messageBus.InboundChan():
			t.Fatalf("rejected message reached agent bus: %#v", inbound)
		default:
		}
	}
}

func TestPhase0PrivateOwnerPlainMessageReachesBus(t *testing.T) {
	messageBus := bus.NewMessageBus()
	ch := &TelegramChannel{
		BaseChannel:        channels.NewBaseChannel("telegram", nil, messageBus, []string{"42"}),
		chatIDs:            make(map[string]int64),
		ctx:                context.Background(),
		qualificationMode:  true,
		qualificationStart: time.Now(),
	}
	msg := &telego.Message{
		Text: "hello model", MessageID: 3,
		Chat: telego.Chat{ID: 42, Type: "private"},
		From: &telego.User{ID: 42, FirstName: "Owner"},
	}
	if err := ch.handleMessage(context.Background(), msg); err != nil {
		t.Fatalf("handleMessage error: %v", err)
	}
	select {
	case inbound := <-messageBus.InboundChan():
		if inbound.Content != "hello model" {
			t.Fatalf("content=%q", inbound.Content)
		}
	default:
		t.Fatal("owner private message did not reach agent bus")
	}
}
