package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client interface {
	Updates(context.Context, int64, int) ([]message.Inbound, error)
	Send(context.Context, message.Outbound) error
}
type HTTPClient struct {
	token string
	http  *http.Client
	max   int64
}

func New(token string) *HTTPClient {
	return &HTTPClient{token: token, http: &http.Client{Timeout: 45 * time.Second}, max: 1 << 20}
}
func (c *HTTPClient) endpoint(method string) string {
	return "https://api.telegram.org/bot" + c.token + "/" + method
}
func (c *HTTPClient) Updates(ctx context.Context, offset int64, timeout int) ([]message.Inbound, error) {
	u, _ := url.Parse(c.endpoint("getUpdates"))
	q := u.Query()
	q.Set("offset", strconv.FormatInt(offset, 10))
	q.Set("timeout", strconv.Itoa(timeout))
	q.Set("allowed_updates", `["message"]`)
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.max))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("telegram HTTP %d", resp.StatusCode)
	}
	var raw struct {
		OK     bool `json:"ok"`
		Result []struct {
			UpdateID int64 `json:"update_id"`
			Message  *struct {
				MessageID int64  `json:"message_id"`
				Text      string `json:"text"`
				Date      int64  `json:"date"`
				Chat      struct {
					ID   int64  `json:"id"`
					Type string `json:"type"`
				} `json:"chat"`
				From *struct {
					ID int64 `json:"id"`
				} `json:"from"`
			} `json:"message"`
		} `json:"result"`
	}
	if json.Unmarshal(data, &raw) != nil || !raw.OK {
		return nil, errors.New("invalid telegram response")
	}
	out := make([]message.Inbound, 0, len(raw.Result))
	for _, r := range raw.Result {
		if r.Message == nil || r.Message.From == nil {
			continue
		}
		out = append(out, message.Inbound{UpdateID: r.UpdateID, MessageID: r.Message.MessageID, ChatID: r.Message.Chat.ID, UserID: r.Message.From.ID, ChatType: r.Message.Chat.Type, Text: r.Message.Text, ReceivedAt: time.Unix(r.Message.Date, 0).UTC()})
	}
	return out, nil
}
func (c *HTTPClient) Send(ctx context.Context, m message.Outbound) error {
	body := map[string]any{"chat_id": m.ChatID, "text": m.Text}
	if m.ReplyTo > 0 {
		body["reply_parameters"] = map[string]any{"message_id": m.ReplyTo}
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("sendMessage"), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("telegram send HTTP %d", resp.StatusCode)
	}
	return nil
}

var _ = strings.Builder{}
