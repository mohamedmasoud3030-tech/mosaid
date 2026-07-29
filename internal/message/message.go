package message

import "time"

type Inbound struct {
	UpdateID   int64
	MessageID  int64
	ChatID     int64
	UserID     int64
	ChatType   string
	Text       string
	ReceivedAt time.Time
}

type Outbound struct {
	ChatID  int64
	Text    string
	ReplyTo int64
}
